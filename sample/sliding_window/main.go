package main

import (
	"container/ring"
	"fmt"
	"sync"
	"time"
)

// SlidingWindowLimiter はスライディングウィンドウ方式のレートリミッターを実装します
type SlidingWindowLimiter struct {
	limit      int           // ウィンドウあたりの最大リクエスト数
	window     time.Duration // ウィンドウの長さ
	timestamps *ring.Ring    // タイムスタンプを保存するリングバッファ
	mu         sync.Mutex
}

// NewSlidingWindowLimiter は新しいスライディングウィンドウレートリミッターを作成します
func NewSlidingWindowLimiter(limit int, window time.Duration) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		limit:      limit,
		window:     window,
		timestamps: ring.New(limit),
	}
}

// Allow はリクエストを許可するかどうかを判定します
func (sw *SlidingWindowLimiter) Allow() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-sw.window)

	// 古いタイムスタンプを削除し、有効なリクエスト数をカウント
	validRequests := 0
	sw.timestamps.Do(func(x interface{}) {
		if x != nil {
			timestamp := x.(time.Time)
			if timestamp.After(windowStart) {
				validRequests++
			}
		}
	})

	// リミット内であれば新しいリクエストを記録
	if validRequests < sw.limit {
		// 空きスロットを探して記録
		recorded := false
		sw.timestamps.Do(func(x interface{}) {
			if !recorded {
				if x == nil || x.(time.Time).Before(windowStart) {
					sw.timestamps.Value = now
					recorded = true
				}
				sw.timestamps = sw.timestamps.Next()
			}
		})
		return true
	}

	return false
}

// GetCurrentCount は現在のウィンドウ内の有効なリクエスト数を返します
func (sw *SlidingWindowLimiter) GetCurrentCount() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-sw.window)
	count := 0

	sw.timestamps.Do(func(x interface{}) {
		if x != nil {
			timestamp := x.(time.Time)
			if timestamp.After(windowStart) {
				count++
			}
		}
	})

	return count
}

// GetOldestTimestamp は最も古い有効なタイムスタンプを返します
func (sw *SlidingWindowLimiter) GetOldestTimestamp() (time.Time, bool) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-sw.window)
	var oldest time.Time
	found := false

	sw.timestamps.Do(func(x interface{}) {
		if x != nil {
			timestamp := x.(time.Time)
			if timestamp.After(windowStart) {
				if !found || timestamp.Before(oldest) {
					oldest = timestamp
					found = true
				}
			}
		}
	})

	return oldest, found
}

// デモンストレーション
func main() {
	fmt.Println("スライディングウィンドウ方式のレートリミッターデモ")
	fmt.Println("=================================================")
	
	// 1秒間に5リクエストまで許可するリミッターを作成
	limiter := NewSlidingWindowLimiter(5, 1*time.Second)
	
	fmt.Printf("初期状態 - 有効リクエスト数: %d/5\n\n", limiter.GetCurrentCount())
	
	// 連続リクエストのテスト
	fmt.Println("6個のリクエストを200msごとに送信:")
	for i := 0; i < 6; i++ {
		if limiter.Allow() {
			fmt.Printf("時刻 %s - リクエスト %d: 許可 (有効数: %d/5)\n",
				time.Now().Format("15:04:05.000"), i+1, limiter.GetCurrentCount())
		} else {
			fmt.Printf("時刻 %s - リクエスト %d: 拒否 (有効数: %d/5)\n",
				time.Now().Format("15:04:05.000"), i+1, limiter.GetCurrentCount())
		}
		time.Sleep(200 * time.Millisecond)
	}
	
	// 古いリクエストが期限切れになるのを待つ
	fmt.Println("\n600ms待機中（古いリクエストが期限切れに）...")
	time.Sleep(600 * time.Millisecond)
	
	fmt.Printf("有効リクエスト数: %d/5\n", limiter.GetCurrentCount())
	
	// 追加リクエスト
	fmt.Println("\n追加リクエスト:")
	for i := 0; i < 3; i++ {
		if limiter.Allow() {
			fmt.Printf("時刻 %s - リクエスト %d: 許可 (有効数: %d/5)\n",
				time.Now().Format("15:04:05.000"), i+1, limiter.GetCurrentCount())
		} else {
			fmt.Printf("時刻 %s - リクエスト %d: 拒否 (有効数: %d/5)\n",
				time.Now().Format("15:04:05.000"), i+1, limiter.GetCurrentCount())
		}
	}
	
	// スライディングウィンドウの特性デモ
	fmt.Println("\n\nスライディングウィンドウの特性デモ:")
	limiter2 := NewSlidingWindowLimiter(10, 2*time.Second)
	
	fmt.Println("10リクエストを100msごとに送信:")
	successTimes := []time.Time{}
	
	for i := 0; i < 10; i++ {
		if limiter2.Allow() {
			now := time.Now()
			successTimes = append(successTimes, now)
			fmt.Printf("リクエスト %d: 許可 (%s)\n", i+1, now.Format("15:04:05.000"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	// 追加リクエスト（拒否されるはず）
	fmt.Println("\n追加リクエスト（即座に）:")
	if !limiter2.Allow() {
		fmt.Println("リクエスト 11: 拒否（予想通り）")
	}
	
	// 最初のリクエストが期限切れになるのを待つ
	if len(successTimes) > 0 {
		waitTime := successTimes[0].Add(2 * time.Second).Sub(time.Now())
		if waitTime > 0 {
			fmt.Printf("\n%v 待機中（最初のリクエストが期限切れに）...\n", waitTime.Round(time.Millisecond))
			time.Sleep(waitTime + 10*time.Millisecond)
		}
	}
	
	fmt.Println("新しいリクエスト:")
	if limiter2.Allow() {
		fmt.Printf("リクエスト: 許可 (有効数: %d/10)\n", limiter2.GetCurrentCount())
	}
	
	// 並行アクセスのテスト
	fmt.Println("\n\n並行アクセステスト:")
	limiter3 := NewSlidingWindowLimiter(5, 1*time.Second)
	
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[int]bool)
	
	fmt.Println("10個のゴルーチンから同時にアクセス:")
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			result := limiter3.Allow()
			mu.Lock()
			results[id] = result
			mu.Unlock()
			
			if result {
				fmt.Printf("ゴルーチン %d: 許可\n", id)
			} else {
				fmt.Printf("ゴルーチン %d: 拒否\n", id)
			}
		}(i)
	}
	
	wg.Wait()
	
	allowed := 0
	for _, v := range results {
		if v {
			allowed++
		}
	}
	fmt.Printf("\n結果 - 許可: %d, 拒否: %d\n", allowed, 10-allowed)
	
	// 固定ウィンドウとの比較
	fmt.Println("\n\n固定ウィンドウとの比較:")
	fmt.Println("スライディングウィンドウは任意の1秒間で正確に5リクエストまでを許可")
	fmt.Println("固定ウィンドウのような境界問題は発生しません")
}