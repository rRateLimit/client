package main

import (
	"fmt"
	"sync"
	"time"
)

// TokenBucket はトークンバケット方式のレートリミッターを実装します
type TokenBucket struct {
	capacity     int           // バケットの最大容量
	tokens       int           // 現在のトークン数
	refillRate   int           // 1秒あたりの補充トークン数
	refillPeriod time.Duration // トークン補充間隔
	mu           sync.Mutex
	lastRefill   time.Time
}

// NewTokenBucket は新しいトークンバケットを作成します
func NewTokenBucket(capacity, refillRate int) *TokenBucket {
	return &TokenBucket{
		capacity:     capacity,
		tokens:       capacity, // 初期状態では満タン
		refillRate:   refillRate,
		refillPeriod: time.Second / time.Duration(refillRate),
		lastRefill:   time.Now(),
	}
}

// Allow はリクエストを許可するかどうかを判定します
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// トークンを補充
	tb.refill()

	// トークンがあれば消費して許可
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

// AllowN は指定された数のトークンを消費できるかチェックします
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= n {
		tb.tokens -= n
		return true
	}

	return false
}

// refill はトークンを補充します（内部メソッド）
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	
	// 経過時間に基づいてトークンを補充
	tokensToAdd := int(elapsed / tb.refillPeriod)
	if tokensToAdd > 0 {
		tb.tokens = min(tb.tokens+tokensToAdd, tb.capacity)
		tb.lastRefill = now
	}
}

// GetTokenCount は現在のトークン数を返します
func (tb *TokenBucket) GetTokenCount() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// デモンストレーション
func main() {
	fmt.Println("トークンバケット方式のレートリミッターデモ")
	fmt.Println("==========================================")
	
	// 容量10、毎秒5トークン補充のバケットを作成
	bucket := NewTokenBucket(10, 5)
	
	fmt.Printf("初期トークン数: %d\n\n", bucket.GetTokenCount())
	
	// 連続リクエストのテスト
	fmt.Println("10個の連続リクエストを送信:")
	for i := 0; i < 10; i++ {
		if bucket.Allow() {
			fmt.Printf("リクエスト %d: 許可 (残りトークン: %d)\n", i+1, bucket.GetTokenCount())
		} else {
			fmt.Printf("リクエスト %d: 拒否 (残りトークン: %d)\n", i+1, bucket.GetTokenCount())
		}
	}
	
	// 追加リクエスト（トークンなし）
	fmt.Println("\n追加リクエスト（トークン枯渇時）:")
	for i := 0; i < 3; i++ {
		if bucket.Allow() {
			fmt.Printf("リクエスト %d: 許可\n", i+1)
		} else {
			fmt.Printf("リクエスト %d: 拒否\n", i+1)
		}
	}
	
	// 1秒待機してトークン回復
	fmt.Println("\n1秒待機中...")
	time.Sleep(1 * time.Second)
	fmt.Printf("トークン数（1秒後）: %d\n", bucket.GetTokenCount())
	
	// 回復後のリクエスト
	fmt.Println("\nトークン回復後のリクエスト:")
	for i := 0; i < 5; i++ {
		if bucket.Allow() {
			fmt.Printf("リクエスト %d: 許可 (残りトークン: %d)\n", i+1, bucket.GetTokenCount())
		} else {
			fmt.Printf("リクエスト %d: 拒否 (残りトークン: %d)\n", i+1, bucket.GetTokenCount())
		}
	}
	
	// バースト処理のデモ
	fmt.Println("\n\nバースト処理のテスト:")
	fmt.Println("2秒待機してトークンを満タンに...")
	time.Sleep(2 * time.Second)
	fmt.Printf("トークン数: %d\n", bucket.GetTokenCount())
	
	// 一度に複数トークンを消費
	fmt.Println("\n一度に3トークンを要求:")
	if bucket.AllowN(3) {
		fmt.Printf("3トークンの消費: 許可 (残りトークン: %d)\n", bucket.GetTokenCount())
	} else {
		fmt.Printf("3トークンの消費: 拒否 (残りトークン: %d)\n", bucket.GetTokenCount())
	}
	
	// 並行アクセスのテスト
	fmt.Println("\n\n並行アクセステスト:")
	fmt.Println("5つのゴルーチンから同時にアクセス...")
	
	var wg sync.WaitGroup
	successCount := 0
	var countMu sync.Mutex
	
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if bucket.Allow() {
				countMu.Lock()
				successCount++
				countMu.Unlock()
				fmt.Printf("ゴルーチン %d: 許可\n", id)
			} else {
				fmt.Printf("ゴルーチン %d: 拒否\n", id)
			}
		}(i)
	}
	
	wg.Wait()
	fmt.Printf("\n成功したリクエスト数: %d/5\n", successCount)
	fmt.Printf("残りトークン数: %d\n", bucket.GetTokenCount())
}