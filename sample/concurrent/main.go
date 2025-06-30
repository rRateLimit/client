package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// RateLimiter インターフェース
type RateLimiter interface {
	Allow() bool
	Name() string
}

// ConcurrentTokenBucket は並行アクセスに対応したトークンバケット実装
type ConcurrentTokenBucket struct {
	capacity   int64
	tokens     int64
	refillRate int64
	mu         sync.Mutex
	done       chan struct{}
}

func NewConcurrentTokenBucket(capacity, refillRate int64) *ConcurrentTokenBucket {
	tb := &ConcurrentTokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		done:       make(chan struct{}),
	}
	
	// バックグラウンドでトークンを補充
	go tb.refillLoop()
	
	return tb
}

func (tb *ConcurrentTokenBucket) Allow() bool {
	return atomic.AddInt64(&tb.tokens, -1) >= 0
}

func (tb *ConcurrentTokenBucket) Name() string {
	return "ConcurrentTokenBucket"
}

func (tb *ConcurrentTokenBucket) refillLoop() {
	ticker := time.NewTicker(time.Second / time.Duration(tb.refillRate))
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			current := atomic.LoadInt64(&tb.tokens)
			if current < tb.capacity {
				atomic.CompareAndSwapInt64(&tb.tokens, current, min(current+1, tb.capacity))
			}
		case <-tb.done:
			return
		}
	}
}

func (tb *ConcurrentTokenBucket) Stop() {
	close(tb.done)
}

// DistributedRateLimiter は分散環境対応のレートリミッター（シミュレーション）
type DistributedRateLimiter struct {
	localLimit  int64
	globalLimit int64
	localCount  int64
	syncPeriod  time.Duration
	mu          sync.RWMutex
	lastSync    time.Time
}

func NewDistributedRateLimiter(globalLimit int64, nodes int, syncPeriod time.Duration) *DistributedRateLimiter {
	localLimit := globalLimit / int64(nodes)
	return &DistributedRateLimiter{
		localLimit:  localLimit,
		globalLimit: globalLimit,
		syncPeriod:  syncPeriod,
		lastSync:    time.Now(),
	}
}

func (dr *DistributedRateLimiter) Allow() bool {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	
	// 同期タイミングのチェック
	if time.Since(dr.lastSync) >= dr.syncPeriod {
		dr.sync()
	}
	
	if dr.localCount < dr.localLimit {
		dr.localCount++
		return true
	}
	
	return false
}

func (dr *DistributedRateLimiter) sync() {
	// 実際の分散環境では、ここで他のノードと通信して
	// グローバルカウントを同期し、ローカルリミットを調整する
	dr.localCount = 0
	dr.lastSync = time.Now()
}

func (dr *DistributedRateLimiter) Name() string {
	return "DistributedRateLimiter"
}

// HTTPハンドラーでのレートリミッター使用例
type RateLimitedHandler struct {
	limiter RateLimiter
	handler http.Handler
}

func (rl *RateLimitedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !rl.limiter.Allow() {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	rl.handler.ServeHTTP(w, r)
}

// ミドルウェアとしてのレートリミッター
func RateLimitMiddleware(limiter RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// パフォーマンステスト関数
func performanceTest(limiter RateLimiter, goroutines int, duration time.Duration) {
	fmt.Printf("\n%s パフォーマンステスト\n", limiter.Name())
	fmt.Printf("ゴルーチン数: %d, テスト時間: %v\n", goroutines, duration)
	
	var allowed int64
	var denied int64
	
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	
	var wg sync.WaitGroup
	start := time.Now()
	
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					if limiter.Allow() {
						atomic.AddInt64(&allowed, 1)
					} else {
						atomic.AddInt64(&denied, 1)
					}
				}
			}
		}()
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	total := allowed + denied
	fmt.Printf("結果:\n")
	fmt.Printf("  総リクエスト数: %d\n", total)
	fmt.Printf("  許可: %d (%.2f%%)\n", allowed, float64(allowed)/float64(total)*100)
	fmt.Printf("  拒否: %d (%.2f%%)\n", denied, float64(denied)/float64(total)*100)
	fmt.Printf("  スループット: %.2f req/sec\n", float64(total)/elapsed.Seconds())
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// デモンストレーション
func main() {
	fmt.Println("並行アクセス対応レートリミッターデモ")
	fmt.Println("=====================================")
	
	// 基本的な並行テスト
	fmt.Println("\n1. 基本的な並行アクセステスト")
	limiter := NewConcurrentTokenBucket(100, 50)
	defer limiter.Stop()
	
	var wg sync.WaitGroup
	successCount := int64(0)
	
	// 200個の並行リクエスト
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if limiter.Allow() {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}
	
	wg.Wait()
	fmt.Printf("200リクエスト中 %d が成功\n", successCount)
	
	// HTTPサーバーでの使用例
	fmt.Println("\n2. HTTPサーバーでの使用例（シミュレーション）")
	
	// APIエンドポイントのハンドラー
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "API Response: %s", time.Now().Format(time.RFC3339))
	})
	
	// レートリミッターを適用
	httpLimiter := NewConcurrentTokenBucket(10, 10)
	defer httpLimiter.Stop()
	
	limitedHandler := RateLimitMiddleware(httpLimiter)(apiHandler)
	
	// シミュレーション: 15個のリクエストを送信
	fmt.Println("15個のHTTPリクエストをシミュレート:")
	for i := 0; i < 15; i++ {
		w := &mockResponseWriter{}
		r, _ := http.NewRequest("GET", "/api/data", nil)
		limitedHandler.ServeHTTP(w, r)
		
		if w.statusCode == http.StatusTooManyRequests {
			fmt.Printf("リクエスト %d: 拒否 (429)\n", i+1)
		} else {
			fmt.Printf("リクエスト %d: 成功 (200)\n", i+1)
		}
	}
	
	// パフォーマンステスト
	fmt.Println("\n3. パフォーマンステスト")
	
	// 高性能トークンバケット
	perfLimiter := NewConcurrentTokenBucket(10000, 5000)
	defer perfLimiter.Stop()
	performanceTest(perfLimiter, 100, 2*time.Second)
	
	// 分散レートリミッター
	distLimiter := NewDistributedRateLimiter(10000, 4, 100*time.Millisecond)
	performanceTest(distLimiter, 100, 2*time.Second)
	
	// ユーザー別レートリミッターの例
	fmt.Println("\n4. ユーザー別レートリミッター")
	userLimiters := make(map[string]RateLimiter)
	var userMu sync.RWMutex
	
	getUserLimiter := func(userID string) RateLimiter {
		userMu.RLock()
		limiter, exists := userLimiters[userID]
		userMu.RUnlock()
		
		if !exists {
			userMu.Lock()
			limiter = NewConcurrentTokenBucket(10, 5)
			userLimiters[userID] = limiter
			userMu.Unlock()
		}
		
		return limiter
	}
	
	// 3人のユーザーからのリクエストをシミュレート
	users := []string{"user1", "user2", "user3"}
	for _, userID := range users {
		fmt.Printf("\n%s のリクエスト:\n", userID)
		limiter := getUserLimiter(userID)
		
		for i := 0; i < 12; i++ {
			if limiter.Allow() {
				fmt.Printf("  リクエスト %d: 許可\n", i+1)
			} else {
				fmt.Printf("  リクエスト %d: 拒否\n", i+1)
			}
		}
	}
	
	// クリーンアップ
	for _, limiter := range userLimiters {
		if tb, ok := limiter.(*ConcurrentTokenBucket); ok {
			tb.Stop()
		}
	}
	
	fmt.Println("\n\nまとめ:")
	fmt.Println("- ConcurrentTokenBucket: 高性能な並行アクセス対応")
	fmt.Println("- DistributedRateLimiter: 分散環境での使用を想定")
	fmt.Println("- HTTPミドルウェア: 実際のWebアプリケーションでの使用例")
	fmt.Println("- ユーザー別制限: きめ細かなレート制御")
}

// モックResponseWriter（テスト用）
type mockResponseWriter struct {
	statusCode int
	header     http.Header
}

func (m *mockResponseWriter) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockResponseWriter) Write([]byte) (int, error) {
	if m.statusCode == 0 {
		m.statusCode = http.StatusOK
	}
	return 0, nil
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}