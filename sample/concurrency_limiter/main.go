package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ConcurrencyLimiter は同時実行数を制限
type ConcurrencyLimiter struct {
	limit     int32
	current   int32
	waiting   int32
	queue     chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewConcurrencyLimiter は新しい並行数制限器を作成
func NewConcurrencyLimiter(limit int) *ConcurrencyLimiter {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConcurrencyLimiter{
		limit:  int32(limit),
		queue:  make(chan struct{}, limit*2), // バッファ付きキュー
		ctx:    ctx,
		cancel: cancel,
	}
}

// Acquire は実行権を取得
func (cl *ConcurrencyLimiter) Acquire(ctx context.Context) error {
	// 待機中カウントを増加
	atomic.AddInt32(&cl.waiting, 1)
	defer atomic.AddInt32(&cl.waiting, -1)
	
	// 現在の実行数をチェック
	for {
		current := atomic.LoadInt32(&cl.current)
		if current < cl.limit {
			if atomic.CompareAndSwapInt32(&cl.current, current, current+1) {
				return nil
			}
			continue
		}
		
		// リミットに達している場合は待機
		select {
		case cl.queue <- struct{}{}:
			// キューに入れた
		case <-ctx.Done():
			return ctx.Err()
		case <-cl.ctx.Done():
			return fmt.Errorf("limiter closed")
		}
		
		// キューから出るのを待つ
		select {
		case <-cl.queue:
			// もう一度試す
		case <-ctx.Done():
			return ctx.Err()
		case <-cl.ctx.Done():
			return fmt.Errorf("limiter closed")
		}
	}
}

// Release は実行権を解放
func (cl *ConcurrencyLimiter) Release() {
	atomic.AddInt32(&cl.current, -1)
	
	// 待機中のゴルーチンに通知
	select {
	case <-cl.queue:
		// キューから1つ取り出す（待機者を起こす）
	default:
		// キューが空なら何もしない
	}
}

// GetStats は統計情報を取得
func (cl *ConcurrencyLimiter) GetStats() (current, waiting, limit int32) {
	return atomic.LoadInt32(&cl.current),
		atomic.LoadInt32(&cl.waiting),
		cl.limit
}

// Close はリミッターを閉じる
func (cl *ConcurrencyLimiter) Close() {
	cl.cancel()
}

// SemaphoreLimiter はセマフォベースの実装
type SemaphoreLimiter struct {
	sem       chan struct{}
	timeout   time.Duration
	stats     *Stats
}

// Stats は統計情報
type Stats struct {
	acquired  int64
	timedOut  int64
	released  int64
	maxWait   int64 // ナノ秒
	totalWait int64 // ナノ秒
	mu        sync.Mutex
}

// NewSemaphoreLimiter は新しいセマフォリミッターを作成
func NewSemaphoreLimiter(limit int, timeout time.Duration) *SemaphoreLimiter {
	return &SemaphoreLimiter{
		sem:     make(chan struct{}, limit),
		timeout: timeout,
		stats:   &Stats{},
	}
}

// Acquire はタイムアウト付きで実行権を取得
func (sl *SemaphoreLimiter) Acquire() bool {
	start := time.Now()
	
	select {
	case sl.sem <- struct{}{}:
		wait := time.Since(start).Nanoseconds()
		sl.stats.recordAcquire(wait)
		return true
	case <-time.After(sl.timeout):
		atomic.AddInt64(&sl.stats.timedOut, 1)
		return false
	}
}

// Release は実行権を解放
func (sl *SemaphoreLimiter) Release() {
	select {
	case <-sl.sem:
		atomic.AddInt64(&sl.stats.released, 1)
	default:
		// すでに空の場合（エラーケース）
	}
}

func (s *Stats) recordAcquire(wait int64) {
	atomic.AddInt64(&s.acquired, 1)
	atomic.AddInt64(&s.totalWait, wait)
	
	// 最大待機時間を更新
	for {
		current := atomic.LoadInt64(&s.maxWait)
		if wait <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&s.maxWait, current, wait) {
			break
		}
	}
}

// AdaptiveConcurrencyLimiter は動的に並行数を調整
type AdaptiveConcurrencyLimiter struct {
	*ConcurrencyLimiter
	minLimit      int32
	maxLimit      int32
	targetLatency time.Duration
	window        *LatencyWindow
}

// LatencyWindow はレイテンシを記録
type LatencyWindow struct {
	samples []time.Duration
	index   int
	size    int
	mu      sync.Mutex
}

// NewAdaptiveConcurrencyLimiter は適応的並行数制限器を作成
func NewAdaptiveConcurrencyLimiter(initial, min, max int) *AdaptiveConcurrencyLimiter {
	return &AdaptiveConcurrencyLimiter{
		ConcurrencyLimiter: NewConcurrencyLimiter(initial),
		minLimit:          int32(min),
		maxLimit:          int32(max),
		targetLatency:     100 * time.Millisecond,
		window: &LatencyWindow{
			samples: make([]time.Duration, 100),
			size:    100,
		},
	}
}

// RecordLatency はレイテンシを記録して制限を調整
func (acl *AdaptiveConcurrencyLimiter) RecordLatency(latency time.Duration) {
	acl.window.mu.Lock()
	acl.window.samples[acl.window.index] = latency
	acl.window.index = (acl.window.index + 1) % acl.window.size
	acl.window.mu.Unlock()
	
	// 平均レイテンシを計算
	avg := acl.window.average()
	
	// リトルの法則に基づいて調整
	// L = λ * W (並行数 = スループット * レイテンシ)
	current := atomic.LoadInt32(&acl.limit)
	
	if avg > acl.targetLatency*2 {
		// レイテンシが高すぎる場合は減少
		newLimit := current - 1
		if newLimit >= acl.minLimit {
			atomic.StoreInt32(&acl.limit, newLimit)
			fmt.Printf("並行数を削減: %d → %d (レイテンシ: %v)\n", current, newLimit, avg)
		}
	} else if avg < acl.targetLatency/2 {
		// レイテンシが低い場合は増加
		newLimit := current + 1
		if newLimit <= acl.maxLimit {
			atomic.StoreInt32(&acl.limit, newLimit)
			fmt.Printf("並行数を増加: %d → %d (レイテンシ: %v)\n", current, newLimit, avg)
		}
	}
}

func (lw *LatencyWindow) average() time.Duration {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	
	var sum int64
	count := 0
	
	for _, sample := range lw.samples {
		if sample > 0 {
			sum += int64(sample)
			count++
		}
	}
	
	if count == 0 {
		return 0
	}
	
	return time.Duration(sum / int64(count))
}

// BulkheadLimiter はバルクヘッドパターンの実装
type BulkheadLimiter struct {
	compartments map[string]*ConcurrencyLimiter
	mu           sync.RWMutex
}

// NewBulkheadLimiter は新しいバルクヘッドリミッターを作成
func NewBulkheadLimiter() *BulkheadLimiter {
	return &BulkheadLimiter{
		compartments: make(map[string]*ConcurrencyLimiter),
	}
}

// AddCompartment は新しいコンパートメントを追加
func (bl *BulkheadLimiter) AddCompartment(name string, limit int) {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	
	bl.compartments[name] = NewConcurrencyLimiter(limit)
}

// Acquire は指定コンパートメントの実行権を取得
func (bl *BulkheadLimiter) Acquire(compartment string, ctx context.Context) error {
	bl.mu.RLock()
	limiter, exists := bl.compartments[compartment]
	bl.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("compartment %s not found", compartment)
	}
	
	return limiter.Acquire(ctx)
}

// Release は指定コンパートメントの実行権を解放
func (bl *BulkheadLimiter) Release(compartment string) {
	bl.mu.RLock()
	limiter, exists := bl.compartments[compartment]
	bl.mu.RUnlock()
	
	if exists {
		limiter.Release()
	}
}

// デモンストレーション
func main() {
	fmt.Println("並行数制限アルゴリズムデモ")
	fmt.Println("=========================")
	
	// 1. 基本的な並行数制限
	fmt.Println("\n1. 基本的な並行数制限 (最大3並行)")
	cl := NewConcurrencyLimiter(3)
	defer cl.Close()
	
	var wg sync.WaitGroup
	
	// 10個のタスクを実行
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			ctx := context.Background()
			fmt.Printf("タスク %d: 実行権を要求\n", id)
			
			if err := cl.Acquire(ctx); err != nil {
				fmt.Printf("タスク %d: エラー %v\n", id, err)
				return
			}
			defer cl.Release()
			
			current, waiting, limit := cl.GetStats()
			fmt.Printf("タスク %d: 実行開始 (実行中: %d/%d, 待機: %d)\n",
				id, current, limit, waiting)
			
			// 処理をシミュレート
			time.Sleep(200 * time.Millisecond)
			
			fmt.Printf("タスク %d: 完了\n", id)
		}(i + 1)
		
		time.Sleep(50 * time.Millisecond)
	}
	
	wg.Wait()
	
	// 2. セマフォベースの実装
	fmt.Println("\n\n2. セマフォベース実装 (タイムアウト付き)")
	sl := NewSemaphoreLimiter(2, 500*time.Millisecond)
	
	// 高負荷をシミュレート
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			if sl.Acquire() {
				fmt.Printf("タスク %d: 実行権取得\n", id)
				time.Sleep(300 * time.Millisecond)
				sl.Release()
				fmt.Printf("タスク %d: 完了\n", id)
			} else {
				fmt.Printf("タスク %d: タイムアウト\n", id)
			}
		}(i + 1)
	}
	
	wg.Wait()
	
	fmt.Printf("\n統計: 取得=%d, タイムアウト=%d, 解放=%d\n",
		sl.stats.acquired, sl.stats.timedOut, sl.stats.released)
	
	// 3. 適応的並行数制限
	fmt.Println("\n\n3. 適応的並行数制限")
	acl := NewAdaptiveConcurrencyLimiter(5, 2, 10)
	defer acl.Close()
	
	// レイテンシが変化するワークロード
	for phase := 0; phase < 3; phase++ {
		fmt.Printf("\nフェーズ %d:\n", phase+1)
		
		// 各フェーズで異なるレイテンシ
		baseLatency := time.Duration(50+phase*100) * time.Millisecond
		
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				ctx := context.Background()
				if err := acl.Acquire(ctx); err != nil {
					return
				}
				defer acl.Release()
				
				// 処理とレイテンシ記録
				start := time.Now()
				time.Sleep(baseLatency + time.Duration(id%3)*10*time.Millisecond)
				latency := time.Since(start)
				
				acl.RecordLatency(latency)
			}(i)
			
			time.Sleep(20 * time.Millisecond)
		}
		
		wg.Wait()
		current, _, _ := acl.GetStats()
		fmt.Printf("現在の並行数制限: %d\n", current)
	}
	
	// 4. バルクヘッドパターン
	fmt.Println("\n\n4. バルクヘッドパターン")
	bl := NewBulkheadLimiter()
	
	// 異なるサービス用のコンパートメント
	bl.AddCompartment("database", 3)
	bl.AddCompartment("api", 5)
	bl.AddCompartment("cache", 10)
	
	services := []string{"database", "api", "cache"}
	
	for _, service := range services {
		fmt.Printf("\n%s サービスへのアクセス:\n", service)
		
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func(svc string, id int) {
				defer wg.Done()
				
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				
				if err := bl.Acquire(svc, ctx); err != nil {
					fmt.Printf("%s[%d]: 取得失敗 - %v\n", svc, id, err)
					return
				}
				defer bl.Release(svc)
				
				fmt.Printf("%s[%d]: 処理中\n", svc, id)
				time.Sleep(100 * time.Millisecond)
			}(service, i+1)
		}
	}
	
	wg.Wait()
	
	fmt.Println("\n\n並行数制限の特徴:")
	fmt.Println("- リソースの過負荷を防止")
	fmt.Println("- レスポンスタイムの改善")
	fmt.Println("- システムの安定性向上")
	fmt.Println("- 障害の局所化（バルクヘッド）")
}