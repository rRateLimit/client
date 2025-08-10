package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rRateLimit/client/ratelimit"
)

func main() {
	fmt.Println("=== Rate Limiter 基本使用例 ===\n")

	// 1. Token Bucket の例
	tokenBucketExample()
	fmt.Println()

	// 2. Fixed Window の例
	fixedWindowExample()
	fmt.Println()

	// 3. Sliding Window の例
	slidingWindowExample()
	fmt.Println()

	// 4. Wait機能の例
	waitExample()
}

func tokenBucketExample() {
	fmt.Println("--- Token Bucket Example ---")
	
	// 毎秒10リクエスト、バースト20まで許可
	limiter := ratelimit.NewTokenBucket(
		ratelimit.WithRate(10),
		ratelimit.WithPeriod(time.Second),
		ratelimit.WithBurst(20),
	)

	fmt.Printf("初期利用可能トークン: %d\n", limiter.Available())

	// バーストテスト: 最初の20リクエストは成功
	success := 0
	for i := 0; i < 25; i++ {
		if limiter.Allow() {
			success++
		}
	}
	fmt.Printf("25リクエスト中 %d 成功 (バースト利用)\n", success)
	fmt.Printf("残りトークン: %d\n", limiter.Available())

	// 1秒待ってトークンが回復
	fmt.Println("1秒待機中...")
	time.Sleep(time.Second)
	fmt.Printf("1秒後の利用可能トークン: %d\n", limiter.Available())

	// 複数トークンを一度に使用
	if limiter.AllowN(5) {
		fmt.Println("5トークンの一括使用: 成功")
	}
	fmt.Printf("残りトークン: %d\n", limiter.Available())
}

func fixedWindowExample() {
	fmt.Println("--- Fixed Window Example ---")
	
	// 1秒間に5リクエストまで
	limiter := ratelimit.NewFixedWindow(
		ratelimit.WithRate(5),
		ratelimit.WithPeriod(time.Second),
	)

	fmt.Printf("初期利用可能リクエスト: %d\n", limiter.Available())

	// 現在のウィンドウで5リクエスト送信
	for i := 0; i < 7; i++ {
		if limiter.Allow() {
			fmt.Printf("リクエスト %d: 許可 (残り: %d)\n", i+1, limiter.Available())
		} else {
			fmt.Printf("リクエスト %d: 拒否 (残り: %d)\n", i+1, limiter.Available())
		}
	}

	// 新しいウィンドウまで待機
	fmt.Println("新しいウィンドウまで1秒待機中...")
	time.Sleep(time.Second)

	// 新しいウィンドウでは再び5リクエスト可能
	fmt.Printf("新ウィンドウの利用可能リクエスト: %d\n", limiter.Available())
	if limiter.Allow() {
		fmt.Println("新ウィンドウでのリクエスト: 許可")
	}
}

func slidingWindowExample() {
	fmt.Println("--- Sliding Window Example ---")
	
	// 2秒間に10リクエストまで（より正確な制限）
	limiter := ratelimit.NewSlidingWindow(
		ratelimit.WithRate(10),
		ratelimit.WithPeriod(2 * time.Second),
	)

	fmt.Printf("初期利用可能リクエスト: %d\n", limiter.Available())

	// 最初の5リクエスト
	for i := 0; i < 5; i++ {
		if limiter.Allow() {
			fmt.Printf("リクエスト %d: 許可\n", i+1)
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("0.5秒後の利用可能リクエスト: %d\n", limiter.Available())

	// さらに5リクエスト
	for i := 5; i < 10; i++ {
		if limiter.Allow() {
			fmt.Printf("リクエスト %d: 許可\n", i+1)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// レート制限に達した
	fmt.Printf("1秒後の利用可能リクエスト: %d\n", limiter.Available())
	
	// 追加リクエストは拒否される
	if !limiter.Allow() {
		fmt.Println("追加リクエスト: 拒否 (レート制限)")
	}

	// 古いリクエストがウィンドウから外れるまで待つ
	fmt.Println("1.5秒待機中...")
	time.Sleep(1500 * time.Millisecond)
	fmt.Printf("2.5秒後の利用可能リクエスト: %d\n", limiter.Available())
}

func waitExample() {
	fmt.Println("--- Wait機能の例 ---")
	
	// 毎秒2リクエストの制限
	limiter := ratelimit.NewTokenBucket(
		ratelimit.WithRate(2),
		ratelimit.WithPeriod(time.Second),
		ratelimit.WithBurst(2),
	)

	fmt.Println("Wait機能を使用して5つのリクエストを順次処理:")
	
	for i := 0; i < 5; i++ {
		startTime := time.Now()
		
		// タイムアウト付きコンテキスト
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		
		// レート制限を待つ
		if err := limiter.Wait(ctx); err != nil {
			log.Printf("リクエスト %d: エラー - %v", i+1, err)
			cancel()
			continue
		}
		
		elapsed := time.Since(startTime)
		fmt.Printf("リクエスト %d: 処理開始 (待機時間: %v)\n", i+1, elapsed)
		
		// リクエスト処理をシミュレート
		processRequest(i + 1)
		
		cancel()
	}
}

func processRequest(id int) {
	// 実際のリクエスト処理をシミュレート
	time.Sleep(50 * time.Millisecond)
	fmt.Printf("  -> リクエスト %d: 処理完了\n", id)
}