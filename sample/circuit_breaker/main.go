package main

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreakerRateLimiter はサーキットブレーカーとレートリミッターを統合
type CircuitBreakerRateLimiter struct {
	// レートリミッター部分
	limiter RateLimiter
	
	// サーキットブレーカー部分
	state           State
	failures        int64
	successes       int64
	consecutiveFails int64
	lastFailTime    time.Time
	lastTransition  time.Time
	
	// 設定
	failureThreshold   int64
	successThreshold   int64
	timeout            time.Duration
	halfOpenRequests   int64
	maxHalfOpenRequests int64
	
	// メトリクス
	totalRequests    int64
	rejectedRequests int64
	
	mu sync.RWMutex
}

// State はサーキットブレーカーの状態
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

// RateLimiter インターフェース
type RateLimiter interface {
	Allow() bool
}

// SimpleRateLimiter は単純なトークンバケット実装
type SimpleRateLimiter struct {
	tokens   int64
	capacity int64
	rate     int64
}

func NewSimpleRateLimiter(capacity, rate int64) *SimpleRateLimiter {
	rl := &SimpleRateLimiter{
		tokens:   capacity,
		capacity: capacity,
		rate:     rate,
	}
	
	// トークン補充
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(rate))
		defer ticker.Stop()
		
		for range ticker.C {
			current := atomic.LoadInt64(&rl.tokens)
			if current < rl.capacity {
				atomic.CompareAndSwapInt64(&rl.tokens, current, current+1)
			}
		}
	}()
	
	return rl
}

func (rl *SimpleRateLimiter) Allow() bool {
	current := atomic.LoadInt64(&rl.tokens)
	if current > 0 {
		return atomic.CompareAndSwapInt64(&rl.tokens, current, current-1)
	}
	return false
}

// NewCircuitBreakerRateLimiter は新しいサーキットブレーカー付きレートリミッターを作成
func NewCircuitBreakerRateLimiter(limiter RateLimiter) *CircuitBreakerRateLimiter {
	return &CircuitBreakerRateLimiter{
		limiter:             limiter,
		state:               StateClosed,
		failureThreshold:    5,
		successThreshold:    3,
		timeout:             10 * time.Second,
		maxHalfOpenRequests: 3,
		lastTransition:      time.Now(),
	}
}

// Allow はリクエストを許可するかチェック
func (cb *CircuitBreakerRateLimiter) Allow() bool {
	atomic.AddInt64(&cb.totalRequests, 1)
	
	// まずレートリミッターをチェック
	if !cb.limiter.Allow() {
		atomic.AddInt64(&cb.rejectedRequests, 1)
		return false
	}
	
	cb.mu.RLock()
	state := cb.state
	cb.mu.RUnlock()
	
	switch state {
	case StateClosed:
		return true
		
	case StateOpen:
		// タイムアウトをチェック
		cb.mu.Lock()
		if time.Since(cb.lastTransition) > cb.timeout {
			cb.transitionTo(StateHalfOpen)
			cb.mu.Unlock()
			return cb.allowHalfOpen()
		}
		cb.mu.Unlock()
		atomic.AddInt64(&cb.rejectedRequests, 1)
		return false
		
	case StateHalfOpen:
		return cb.allowHalfOpen()
		
	default:
		return false
	}
}

// allowHalfOpen はHalf-Open状態でのリクエスト処理
func (cb *CircuitBreakerRateLimiter) allowHalfOpen() bool {
	current := atomic.LoadInt64(&cb.halfOpenRequests)
	if current >= cb.maxHalfOpenRequests {
		atomic.AddInt64(&cb.rejectedRequests, 1)
		return false
	}
	
	atomic.AddInt64(&cb.halfOpenRequests, 1)
	return true
}

// RecordSuccess は成功を記録
func (cb *CircuitBreakerRateLimiter) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	atomic.StoreInt64(&cb.consecutiveFails, 0)
	atomic.AddInt64(&cb.successes, 1)
	
	switch cb.state {
	case StateHalfOpen:
		successes := atomic.LoadInt64(&cb.successes)
		if successes >= cb.successThreshold {
			cb.transitionTo(StateClosed)
		}
	}
}

// RecordFailure は失敗を記録
func (cb *CircuitBreakerRateLimiter) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	atomic.AddInt64(&cb.failures, 1)
	atomic.AddInt64(&cb.consecutiveFails, 1)
	cb.lastFailTime = time.Now()
	
	switch cb.state {
	case StateClosed:
		if atomic.LoadInt64(&cb.consecutiveFails) >= cb.failureThreshold {
			cb.transitionTo(StateOpen)
		}
		
	case StateHalfOpen:
		cb.transitionTo(StateOpen)
	}
}

// transitionTo は状態を遷移
func (cb *CircuitBreakerRateLimiter) transitionTo(newState State) {
	if cb.state == newState {
		return
	}
	
	fmt.Printf("サーキットブレーカー状態遷移: %s → %s\n", cb.state, newState)
	
	cb.state = newState
	cb.lastTransition = time.Now()
	
	// 状態リセット
	switch newState {
	case StateClosed:
		atomic.StoreInt64(&cb.failures, 0)
		atomic.StoreInt64(&cb.successes, 0)
		atomic.StoreInt64(&cb.consecutiveFails, 0)
		
	case StateHalfOpen:
		atomic.StoreInt64(&cb.halfOpenRequests, 0)
		atomic.StoreInt64(&cb.successes, 0)
	}
}

// GetState は現在の状態を取得
func (cb *CircuitBreakerRateLimiter) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats は統計情報を取得
func (cb *CircuitBreakerRateLimiter) GetStats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	return map[string]interface{}{
		"state":            cb.state.String(),
		"totalRequests":    atomic.LoadInt64(&cb.totalRequests),
		"rejectedRequests": atomic.LoadInt64(&cb.rejectedRequests),
		"failures":         atomic.LoadInt64(&cb.failures),
		"successes":        atomic.LoadInt64(&cb.successes),
		"consecutiveFails": atomic.LoadInt64(&cb.consecutiveFails),
		"lastFailTime":     cb.lastFailTime,
		"lastTransition":   cb.lastTransition,
	}
}

// AdaptiveCircuitBreaker は適応的なサーキットブレーカー
type AdaptiveCircuitBreaker struct {
	*CircuitBreakerRateLimiter
	
	// 適応的パラメータ
	errorRate        float64
	responseTime     time.Duration
	windowSize       time.Duration
	metrics          *MetricsWindow
}

// MetricsWindow は時間窓でのメトリクス
type MetricsWindow struct {
	requests []RequestMetric
	mu       sync.Mutex
}

// RequestMetric は個々のリクエストのメトリクス
type RequestMetric struct {
	timestamp    time.Time
	success      bool
	responseTime time.Duration
}

// NewAdaptiveCircuitBreaker は適応的サーキットブレーカーを作成
func NewAdaptiveCircuitBreaker(limiter RateLimiter) *AdaptiveCircuitBreaker {
	return &AdaptiveCircuitBreaker{
		CircuitBreakerRateLimiter: NewCircuitBreakerRateLimiter(limiter),
		windowSize:                30 * time.Second,
		metrics: &MetricsWindow{
			requests: make([]RequestMetric, 0),
		},
	}
}

// RecordRequest はリクエストメトリクスを記録
func (acb *AdaptiveCircuitBreaker) RecordRequest(success bool, responseTime time.Duration) {
	acb.metrics.mu.Lock()
	defer acb.metrics.mu.Unlock()
	
	// メトリクスを追加
	acb.metrics.requests = append(acb.metrics.requests, RequestMetric{
		timestamp:    time.Now(),
		success:      success,
		responseTime: responseTime,
	})
	
	// 古いメトリクスを削除
	cutoff := time.Now().Add(-acb.windowSize)
	newRequests := make([]RequestMetric, 0)
	for _, req := range acb.metrics.requests {
		if req.timestamp.After(cutoff) {
			newRequests = append(newRequests, req)
		}
	}
	acb.metrics.requests = newRequests
	
	// エラー率と応答時間を計算
	acb.calculateMetrics()
	
	// 適応的な閾値調整
	acb.adjustThresholds()
}

// calculateMetrics はメトリクスを計算
func (acb *AdaptiveCircuitBreaker) calculateMetrics() {
	if len(acb.metrics.requests) == 0 {
		return
	}
	
	var successCount, totalTime int64
	for _, req := range acb.metrics.requests {
		if req.success {
			successCount++
		}
		totalTime += int64(req.responseTime)
	}
	
	total := int64(len(acb.metrics.requests))
	acb.errorRate = float64(total-successCount) / float64(total)
	acb.responseTime = time.Duration(totalTime / total)
}

// adjustThresholds は閾値を動的に調整
func (acb *AdaptiveCircuitBreaker) adjustThresholds() {
	// エラー率に基づいて失敗閾値を調整
	if acb.errorRate > 0.5 {
		acb.failureThreshold = 3 // より厳しく
	} else if acb.errorRate > 0.2 {
		acb.failureThreshold = 5
	} else {
		acb.failureThreshold = 10 // より寛容に
	}
	
	// 応答時間に基づいてタイムアウトを調整
	if acb.responseTime > 5*time.Second {
		acb.timeout = 30 * time.Second // 長めのタイムアウト
	} else if acb.responseTime > 1*time.Second {
		acb.timeout = 15 * time.Second
	} else {
		acb.timeout = 10 * time.Second
	}
}

// デモンストレーション
func main() {
	fmt.Println("サーキットブレーカー統合レートリミッターデモ")
	fmt.Println("==========================================")
	
	// 基本的なサーキットブレーカー
	fmt.Println("\n1. 基本的なサーキットブレーカー動作")
	
	limiter := NewSimpleRateLimiter(100, 10)
	cb := NewCircuitBreakerRateLimiter(limiter)
	
	// 正常なリクエスト
	fmt.Println("\n正常なリクエスト:")
	for i := 0; i < 5; i++ {
		if cb.Allow() {
			fmt.Printf("リクエスト %d: 許可\n", i+1)
			cb.RecordSuccess()
		}
	}
	
	fmt.Printf("状態: %s\n", cb.GetState())
	
	// 連続失敗でOPEN状態へ
	fmt.Println("\n\n連続失敗シミュレーション:")
	for i := 0; i < 6; i++ {
		if cb.Allow() {
			fmt.Printf("リクエスト %d: 許可 → 失敗を記録\n", i+1)
			cb.RecordFailure()
		} else {
			fmt.Printf("リクエスト %d: 拒否\n", i+1)
		}
	}
	
	fmt.Printf("状態: %s\n", cb.GetState())
	
	// OPEN状態でのリクエスト
	fmt.Println("\n\nOPEN状態でのリクエスト:")
	for i := 0; i < 3; i++ {
		if cb.Allow() {
			fmt.Printf("リクエスト %d: 許可（想定外）\n", i+1)
		} else {
			fmt.Printf("リクエスト %d: 拒否（サーキット開放）\n", i+1)
		}
	}
	
	// タイムアウト待機
	fmt.Println("\n\nタイムアウト待機中...")
	time.Sleep(11 * time.Second)
	
	// HALF-OPEN状態でのテスト
	fmt.Println("\nHALF-OPEN状態でのテスト:")
	for i := 0; i < 5; i++ {
		if cb.Allow() {
			fmt.Printf("リクエスト %d: 許可（テスト中）\n", i+1)
			cb.RecordSuccess()
		} else {
			fmt.Printf("リクエスト %d: 拒否（制限到達）\n", i+1)
		}
	}
	
	fmt.Printf("状態: %s\n", cb.GetState())
	
	// 適応的サーキットブレーカー
	fmt.Println("\n\n2. 適応的サーキットブレーカー")
	
	limiter2 := NewSimpleRateLimiter(50, 10)
	acb := NewAdaptiveCircuitBreaker(limiter2)
	
	// 変動する成功率でのシミュレーション
	phases := []struct {
		name        string
		successRate float64
		latency     time.Duration
		requests    int
	}{
		{"正常期", 0.95, 100 * time.Millisecond, 20},
		{"劣化期", 0.7, 500 * time.Millisecond, 20},
		{"障害期", 0.3, 2 * time.Second, 20},
		{"回復期", 0.85, 200 * time.Millisecond, 20},
	}
	
	for _, phase := range phases {
		fmt.Printf("\n\n%s (成功率: %.0f%%, レイテンシ: %v):\n", 
			phase.name, phase.successRate*100, phase.latency)
		
		for i := 0; i < phase.requests; i++ {
			if acb.Allow() {
				// シミュレート: 指定された成功率で成功/失敗
				success := rand.Float64() < phase.successRate
				
				if success {
					acb.RecordSuccess()
					acb.RecordRequest(true, phase.latency)
				} else {
					acb.RecordFailure()
					acb.RecordRequest(false, phase.latency)
				}
			}
			
			time.Sleep(50 * time.Millisecond)
		}
		
		stats := acb.GetStats()
		fmt.Printf("状態: %s, エラー率: %.2f%%, 失敗閾値: %d\n",
			stats["state"], acb.errorRate*100, acb.failureThreshold)
	}
	
	// 統計情報
	fmt.Println("\n\n3. 最終統計:")
	finalStats := acb.GetStats()
	for key, value := range finalStats {
		fmt.Printf("%s: %v\n", key, value)
	}
	
	// エクスポネンシャルバックオフ付きサーキットブレーカー
	fmt.Println("\n\n4. エクスポネンシャルバックオフ")
	
	backoffMultiplier := 1
	for i := 0; i < 5; i++ {
		timeout := time.Duration(math.Pow(2, float64(backoffMultiplier))) * time.Second
		fmt.Printf("試行 %d: タイムアウト %v\n", i+1, timeout)
		backoffMultiplier++
	}
	
	fmt.Println("\n\nサーキットブレーカー統合の利点:")
	fmt.Println("- カスケード障害の防止")
	fmt.Println("- 自動的な障害検知と回復")
	fmt.Println("- レート制限との相乗効果")
	fmt.Println("- 適応的な閾値調整")
}