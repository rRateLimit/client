package main

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// AdaptiveRateLimiter は動的にレート制限を調整します
type AdaptiveRateLimiter struct {
	// 基本パラメータ
	baseRate      float64 // 基本レート（req/sec）
	currentRate   atomic.Value // 現在のレート
	minRate       float64 // 最小レート
	maxRate       float64 // 最大レート
	
	// メトリクス
	successCount  int64
	failureCount  int64
	latencySum    int64 // ナノ秒単位の合計レイテンシ
	requestCount  int64
	
	// 制御パラメータ
	targetSuccessRate float64       // 目標成功率
	targetLatency     time.Duration // 目標レイテンシ
	adjustInterval    time.Duration // 調整間隔
	
	// 内部状態
	window        *SlidingWindow
	lastAdjust    time.Time
	mu            sync.RWMutex
	done          chan struct{}
}

// SlidingWindow は時間ベースのスライディングウィンドウ
type SlidingWindow struct {
	windowSize time.Duration
	buckets    map[int64]*Bucket
	mu         sync.RWMutex
}

// Bucket は時間バケット内のメトリクス
type Bucket struct {
	requests  int64
	successes int64
	failures  int64
	latency   int64
}

// NewAdaptiveRateLimiter は新しい適応的レートリミッターを作成
func NewAdaptiveRateLimiter(baseRate float64) *AdaptiveRateLimiter {
	arl := &AdaptiveRateLimiter{
		baseRate:          baseRate,
		minRate:           baseRate * 0.1,
		maxRate:           baseRate * 10,
		targetSuccessRate: 0.95,
		targetLatency:     100 * time.Millisecond,
		adjustInterval:    5 * time.Second,
		window: &SlidingWindow{
			windowSize: 30 * time.Second,
			buckets:    make(map[int64]*Bucket),
		},
		lastAdjust: time.Now(),
		done:       make(chan struct{}),
	}
	
	arl.currentRate.Store(baseRate)
	
	// バックグラウンドで調整を実行
	go arl.adjustLoop()
	
	return arl
}

// Allow はリクエストを許可するかチェック
func (arl *AdaptiveRateLimiter) Allow() bool {
	rate := arl.currentRate.Load().(float64)
	
	// 簡易的なトークンバケット実装
	// 実際にはより精密な実装が必要
	threshold := rate / 1000.0 // ミリ秒あたりのレート
	randomValue := float64(time.Now().UnixNano()%1000) / 1000.0
	
	allowed := randomValue < threshold
	
	// メトリクスを記録
	arl.recordRequest(allowed, 0)
	
	return allowed
}

// RecordLatency はレイテンシを記録
func (arl *AdaptiveRateLimiter) RecordLatency(latency time.Duration) {
	atomic.AddInt64(&arl.latencySum, int64(latency))
	atomic.AddInt64(&arl.requestCount, 1)
	
	// スライディングウィンドウに記録
	arl.window.record(latency)
}

// recordRequest はリクエストの結果を記録
func (arl *AdaptiveRateLimiter) recordRequest(success bool, latency time.Duration) {
	if success {
		atomic.AddInt64(&arl.successCount, 1)
	} else {
		atomic.AddInt64(&arl.failureCount, 1)
	}
	
	if latency > 0 {
		arl.RecordLatency(latency)
	}
}

// adjustLoop は定期的にレートを調整
func (arl *AdaptiveRateLimiter) adjustLoop() {
	ticker := time.NewTicker(arl.adjustInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-arl.done:
			return
		case <-ticker.C:
			arl.adjust()
		}
	}
}

// adjust はメトリクスに基づいてレートを調整
func (arl *AdaptiveRateLimiter) adjust() {
	arl.mu.Lock()
	defer arl.mu.Unlock()
	
	// メトリクスを取得
	success := atomic.LoadInt64(&arl.successCount)
	failure := atomic.LoadInt64(&arl.failureCount)
	total := success + failure
	
	if total == 0 {
		return
	}
	
	successRate := float64(success) / float64(total)
	
	// 平均レイテンシを計算
	latencySum := atomic.LoadInt64(&arl.latencySum)
	requestCount := atomic.LoadInt64(&arl.requestCount)
	avgLatency := time.Duration(0)
	if requestCount > 0 {
		avgLatency = time.Duration(latencySum / requestCount)
	}
	
	// 現在のレートを取得
	currentRate := arl.currentRate.Load().(float64)
	newRate := currentRate
	
	// AIMD (Additive Increase Multiplicative Decrease) アルゴリズム
	if successRate < arl.targetSuccessRate || avgLatency > arl.targetLatency {
		// 性能が悪い場合は積極的に減少
		newRate = currentRate * 0.8
	} else if successRate > arl.targetSuccessRate*1.05 && avgLatency < arl.targetLatency*0.8 {
		// 性能が良い場合は慎重に増加
		newRate = currentRate + (arl.baseRate * 0.1)
	}
	
	// 制限を適用
	newRate = math.Max(arl.minRate, math.Min(newRate, arl.maxRate))
	
	// レートを更新
	arl.currentRate.Store(newRate)
	
	// カウンタをリセット
	atomic.StoreInt64(&arl.successCount, 0)
	atomic.StoreInt64(&arl.failureCount, 0)
	atomic.StoreInt64(&arl.latencySum, 0)
	atomic.StoreInt64(&arl.requestCount, 0)
	
	fmt.Printf("レート調整: %.2f → %.2f (成功率: %.2f%%, 平均レイテンシ: %v)\n",
		currentRate, newRate, successRate*100, avgLatency)
}

// GetMetrics は現在のメトリクスを返す
func (arl *AdaptiveRateLimiter) GetMetrics() (rate float64, successRate float64, avgLatency time.Duration) {
	rate = arl.currentRate.Load().(float64)
	
	success := atomic.LoadInt64(&arl.successCount)
	failure := atomic.LoadInt64(&arl.failureCount)
	total := success + failure
	
	if total > 0 {
		successRate = float64(success) / float64(total)
	}
	
	latencySum := atomic.LoadInt64(&arl.latencySum)
	requestCount := atomic.LoadInt64(&arl.requestCount)
	if requestCount > 0 {
		avgLatency = time.Duration(latencySum / requestCount)
	}
	
	return
}

// Stop はレートリミッターを停止
func (arl *AdaptiveRateLimiter) Stop() {
	close(arl.done)
}

// SlidingWindow のメソッド
func (sw *SlidingWindow) record(latency time.Duration) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	
	now := time.Now()
	bucketKey := now.Unix() / 10 // 10秒バケット
	
	bucket, exists := sw.buckets[bucketKey]
	if !exists {
		bucket = &Bucket{}
		sw.buckets[bucketKey] = bucket
	}
	
	bucket.requests++
	bucket.latency += int64(latency)
	
	// 古いバケットを削除
	cutoff := now.Add(-sw.windowSize).Unix() / 10
	for key := range sw.buckets {
		if key < cutoff {
			delete(sw.buckets, key)
		}
	}
}

// PIDController はPID制御を使用した適応的レートリミッター
type PIDController struct {
	setpoint float64 // 目標値
	kp       float64 // 比例ゲイン
	ki       float64 // 積分ゲイン
	kd       float64 // 微分ゲイン
	
	integral   float64
	lastError  float64
	lastUpdate time.Time
	
	mu sync.Mutex
}

// NewPIDController は新しいPIDコントローラーを作成
func NewPIDController(setpoint, kp, ki, kd float64) *PIDController {
	return &PIDController{
		setpoint:   setpoint,
		kp:         kp,
		ki:         ki,
		kd:         kd,
		lastUpdate: time.Now(),
	}
}

// Update はPID制御を実行
func (pid *PIDController) Update(measured float64) float64 {
	pid.mu.Lock()
	defer pid.mu.Unlock()
	
	now := time.Now()
	dt := now.Sub(pid.lastUpdate).Seconds()
	
	// エラーを計算
	error := pid.setpoint - measured
	
	// 積分項を更新
	pid.integral += error * dt
	
	// 微分項を計算
	derivative := 0.0
	if dt > 0 {
		derivative = (error - pid.lastError) / dt
	}
	
	// PID出力を計算
	output := pid.kp*error + pid.ki*pid.integral + pid.kd*derivative
	
	// 状態を更新
	pid.lastError = error
	pid.lastUpdate = now
	
	return output
}

// デモンストレーション
func main() {
	fmt.Println("適応的レートリミッターデモ")
	fmt.Println("===========================")
	
	// 基本的な適応的レートリミッター
	fmt.Println("\n1. AIMD方式の適応的レートリミッター")
	limiter := NewAdaptiveRateLimiter(100) // 基本レート: 100 req/sec
	defer limiter.Stop()
	
	// シミュレーション: 負荷パターンを変化させる
	simulate := func(duration time.Duration, requestRate int, latency time.Duration, name string) {
		fmt.Printf("\n%s: %d req/sec, レイテンシ %v\n", name, requestRate, latency)
		
		start := time.Now()
		allowed := 0
		total := 0
		
		ticker := time.NewTicker(time.Second / time.Duration(requestRate))
		defer ticker.Stop()
		
		for time.Since(start) < duration {
			select {
			case <-ticker.C:
				total++
				if limiter.Allow() {
					allowed++
					// 擬似的なレイテンシを記録
					limiter.RecordLatency(latency)
				}
			}
		}
		
		rate, successRate, avgLatency := limiter.GetMetrics()
		fmt.Printf("結果: %d/%d 許可, レート: %.2f, 成功率: %.2f%%, 平均レイテンシ: %v\n",
			allowed, total, rate, successRate*100, avgLatency)
	}
	
	// 低負荷
	simulate(3*time.Second, 50, 50*time.Millisecond, "低負荷フェーズ")
	
	// 高負荷
	simulate(3*time.Second, 200, 150*time.Millisecond, "高負荷フェーズ")
	
	// 調整待ち
	fmt.Println("\n調整期間（5秒）...")
	time.Sleep(5 * time.Second)
	
	// 通常負荷
	simulate(3*time.Second, 100, 80*time.Millisecond, "通常負荷フェーズ")
	
	// PID制御デモ
	fmt.Println("\n\n2. PID制御による適応的レート制御")
	
	pid := NewPIDController(
		0.95,  // 目標成功率: 95%
		50.0,  // Kp: 比例ゲイン
		10.0,  // Ki: 積分ゲイン
		5.0,   // Kd: 微分ゲイン
	)
	
	currentRate := 100.0
	
	fmt.Println("\nPID制御シミュレーション:")
	for i := 0; i < 10; i++ {
		// 擬似的な成功率（負荷により変動）
		successRate := 0.9 + 0.1*math.Sin(float64(i)*0.5)
		if currentRate > 150 {
			successRate *= 0.9 // 高レートでは成功率が下がる
		}
		
		// PID制御で調整値を計算
		adjustment := pid.Update(successRate)
		
		// レートを調整
		currentRate = math.Max(10, math.Min(200, currentRate+adjustment))
		
		fmt.Printf("イテレーション %d: 成功率=%.2f%%, 調整=%.2f, 新レート=%.2f\n",
			i+1, successRate*100, adjustment, currentRate)
		
		time.Sleep(500 * time.Millisecond)
	}
	
	// 機械学習ベースの予測（簡易版）
	fmt.Println("\n\n3. パターン認識による適応制御")
	
	// 時間帯別のパターンを学習（簡易的なデモ）
	patterns := map[string]float64{
		"morning":   1.2, // 朝は120%の負荷
		"afternoon": 0.8, // 午後は80%の負荷
		"evening":   1.5, // 夕方は150%の負荷
		"night":     0.5, // 夜は50%の負荷
	}
	
	baseRate := 100.0
	hour := time.Now().Hour()
	
	timeOfDay := "morning"
	switch {
	case hour >= 6 && hour < 12:
		timeOfDay = "morning"
	case hour >= 12 && hour < 17:
		timeOfDay = "afternoon"
	case hour >= 17 && hour < 22:
		timeOfDay = "evening"
	default:
		timeOfDay = "night"
	}
	
	predictedLoad := patterns[timeOfDay]
	adjustedRate := baseRate / predictedLoad
	
	fmt.Printf("現在の時間帯: %s\n", timeOfDay)
	fmt.Printf("予測負荷係数: %.2f\n", predictedLoad)
	fmt.Printf("調整後レート: %.2f req/sec\n", adjustedRate)
	
	fmt.Println("\n\n適応的レートリミッターの特徴:")
	fmt.Println("- 動的な負荷に応じて自動調整")
	fmt.Println("- 成功率とレイテンシを考慮")
	fmt.Println("- PID制御による安定した調整")
	fmt.Println("- パターン認識による予測的制御")
}