package main

import (
	"fmt"
	"sync"
	"time"
)

// FixedWindowLimiter は固定ウィンドウ方式のレートリミッターを実装します
type FixedWindowLimiter struct {
	limit        int           // ウィンドウあたりの最大リクエスト数
	window       time.Duration // ウィンドウの長さ
	counter      int           // 現在のウィンドウでのリクエスト数
	windowStart  time.Time     // 現在のウィンドウの開始時刻
	mu           sync.Mutex
}

// NewFixedWindowLimiter は新しい固定ウィンドウレートリミッターを作成します
func NewFixedWindowLimiter(limit int, window time.Duration) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		limit:       limit,
		window:      window,
		counter:     0,
		windowStart: time.Now(),
	}
}

// Allow はリクエストを許可するかどうかを判定します
func (fw *FixedWindowLimiter) Allow() bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := time.Now()
	
	// 新しいウィンドウに入った場合、カウンターをリセット
	if now.Sub(fw.windowStart) >= fw.window {
		fw.counter = 0
		fw.windowStart = now
	}

	// リミット内であれば許可
	if fw.counter < fw.limit {
		fw.counter++
		return true
	}

	return false
}

// GetStatus は現在のステータスを返します
func (fw *FixedWindowLimiter) GetStatus() (currentCount int, windowRemaining time.Duration) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := time.Now()
	
	// ウィンドウが終了していたらリセット
	if now.Sub(fw.windowStart) >= fw.window {
		fw.counter = 0
		fw.windowStart = now
	}

	windowEnd := fw.windowStart.Add(fw.window)
	remaining := windowEnd.Sub(now)
	
	return fw.counter, remaining
}

// Reset は手動でカウンターをリセットします
func (fw *FixedWindowLimiter) Reset() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	
	fw.counter = 0
	fw.windowStart = time.Now()
}

// デモンストレーション
func main() {
	fmt.Println("固定ウィンドウ方式のレートリミッターデモ")
	fmt.Println("=========================================")
	
	// 1秒間に5リクエストまで許可するリミッターを作成
	limiter := NewFixedWindowLimiter(5, 1*time.Second)
	
	// 現在のステータスを表示
	count, remaining := limiter.GetStatus()
	fmt.Printf("初期状態 - リクエスト数: %d/5, ウィンドウ残り時間: %v\n\n", count, remaining)
	
	// 連続リクエストのテスト
	fmt.Println("7個の連続リクエストを送信:")
	for i := 0; i < 7; i++ {
		if limiter.Allow() {
			count, remaining = limiter.GetStatus()
			fmt.Printf("リクエスト %d: 許可 (カウント: %d/5, 残り時間: %v)\n", 
				i+1, count, remaining.Round(time.Millisecond))
		} else {
			count, remaining = limiter.GetStatus()
			fmt.Printf("リクエスト %d: 拒否 (カウント: %d/5, 残り時間: %v)\n", 
				i+1, count, remaining.Round(time.Millisecond))
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	// ウィンドウが切り替わるまで待機
	count, remaining = limiter.GetStatus()
	fmt.Printf("\n次のウィンドウまで %v 待機中...\n", remaining.Round(time.Millisecond))
	time.Sleep(remaining + 10*time.Millisecond)
	
	// 新しいウィンドウでのリクエスト
	fmt.Println("\n新しいウィンドウでのリクエスト:")
	for i := 0; i < 3; i++ {
		if limiter.Allow() {
			count, remaining = limiter.GetStatus()
			fmt.Printf("リクエスト %d: 許可 (カウント: %d/5)\n", i+1, count)
		}
	}
	
	// バースト処理のテスト
	fmt.Println("\n\nバースト処理のテスト:")
	fmt.Println("リミッターをリセット...")
	limiter.Reset()
	
	fmt.Println("5つのリクエストを即座に送信:")
	start := time.Now()
	successCount := 0
	for i := 0; i < 5; i++ {
		if limiter.Allow() {
			successCount++
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("成功: %d/5, 処理時間: %v\n", successCount, elapsed)
	
	// 並行アクセスのテスト
	fmt.Println("\n\n並行アクセステスト:")
	limiter.Reset()
	fmt.Println("10個のゴルーチンから同時にアクセス...")
	
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	denied := 0
	
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			// ランダムな遅延を追加して現実的なシナリオをシミュレート
			time.Sleep(time.Duration(id*10) * time.Millisecond)
			
			if limiter.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
				fmt.Printf("ゴルーチン %d: 許可\n", id)
			} else {
				mu.Lock()
				denied++
				mu.Unlock()
				fmt.Printf("ゴルーチン %d: 拒否\n", id)
			}
		}(i)
	}
	
	wg.Wait()
	fmt.Printf("\n結果 - 許可: %d, 拒否: %d\n", allowed, denied)
	
	// 固定ウィンドウの問題点のデモ
	fmt.Println("\n\n固定ウィンドウの境界問題のデモ:")
	limiter2 := NewFixedWindowLimiter(10, 1*time.Second)
	
	fmt.Println("ウィンドウの終わり近くで5リクエスト:")
	for i := 0; i < 5; i++ {
		limiter2.Allow()
	}
	count, remaining = limiter2.GetStatus()
	fmt.Printf("現在のカウント: %d/10, 残り時間: %v\n", count, remaining)
	
	fmt.Printf("\n%v 待機してウィンドウを切り替え...\n", remaining)
	time.Sleep(remaining + 10*time.Millisecond)
	
	fmt.Println("新しいウィンドウの開始直後に10リクエスト:")
	successInNewWindow := 0
	for i := 0; i < 10; i++ {
		if limiter2.Allow() {
			successInNewWindow++
		}
	}
	fmt.Printf("成功: %d/10\n", successInNewWindow)
	fmt.Println("\n注: 2秒間で15リクエストが成功（理論上は10リクエスト/秒のはず）")
	fmt.Println("これが固定ウィンドウ方式の既知の問題です。")
}