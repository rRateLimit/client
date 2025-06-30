package main

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

// LeakyBucket はリーキーバケットアルゴリズムを実装します
// トークンバケットとは異なり、リクエストをキューに保存し、一定レートで処理します
type LeakyBucket struct {
	capacity   int              // バケットの容量（キューの最大サイズ）
	rate       time.Duration    // リーク（処理）レート
	queue      *list.List       // リクエストキュー
	mu         sync.Mutex
	processing chan struct{}    // 処理ゴルーチンの制御
	done       chan struct{}    // 終了シグナル
}

// Request はキューに保存されるリクエストを表します
type Request struct {
	ID        int
	Timestamp time.Time
	Done      chan bool // リクエストの処理結果を通知
}

// NewLeakyBucket は新しいリーキーバケットを作成します
func NewLeakyBucket(capacity int, rate time.Duration) *LeakyBucket {
	lb := &LeakyBucket{
		capacity:   capacity,
		rate:       rate,
		queue:      list.New(),
		processing: make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
	
	// バックグラウンドでリクエストを処理
	go lb.leak()
	
	return lb
}

// Submit はリクエストをキューに追加します
func (lb *LeakyBucket) Submit(id int) (chan bool, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	// キューが満杯の場合は拒否
	if lb.queue.Len() >= lb.capacity {
		return nil, fmt.Errorf("bucket is full")
	}
	
	// リクエストをキューに追加
	req := &Request{
		ID:        id,
		Timestamp: time.Now(),
		Done:      make(chan bool, 1),
	}
	lb.queue.PushBack(req)
	
	// 処理を開始
	select {
	case lb.processing <- struct{}{}:
	default:
	}
	
	return req.Done, nil
}

// leak はキューからリクエストを一定レートで処理します
func (lb *LeakyBucket) leak() {
	ticker := time.NewTicker(lb.rate)
	defer ticker.Stop()
	
	for {
		select {
		case <-lb.done:
			return
		case <-lb.processing:
			// 処理ループ
			for {
				select {
				case <-lb.done:
					return
				case <-ticker.C:
					lb.mu.Lock()
					if lb.queue.Len() == 0 {
						lb.mu.Unlock()
						goto wait
					}
					
					// キューの先頭からリクエストを取り出して処理
					front := lb.queue.Front()
					req := front.Value.(*Request)
					lb.queue.Remove(front)
					lb.mu.Unlock()
					
					// リクエストを処理（実際のアプリケーションではここで処理を実行）
					req.Done <- true
					close(req.Done)
					
					fmt.Printf("処理: リクエスト %d (待機時間: %v)\n", 
						req.ID, time.Since(req.Timestamp))
				}
			}
		wait:
		}
	}
}

// GetQueueSize は現在のキューサイズを返します
func (lb *LeakyBucket) GetQueueSize() int {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.queue.Len()
}

// Stop はリーキーバケットを停止します
func (lb *LeakyBucket) Stop() {
	close(lb.done)
}

// AdaptiveLeakyBucket は負荷に応じて処理レートを調整するリーキーバケット
type AdaptiveLeakyBucket struct {
	*LeakyBucket
	minRate      time.Duration
	maxRate      time.Duration
	targetLatency time.Duration
	adjustPeriod  time.Duration
}

// NewAdaptiveLeakyBucket は適応的なリーキーバケットを作成します
func NewAdaptiveLeakyBucket(capacity int, initialRate, minRate, maxRate time.Duration) *AdaptiveLeakyBucket {
	alb := &AdaptiveLeakyBucket{
		LeakyBucket:   NewLeakyBucket(capacity, initialRate),
		minRate:       minRate,
		maxRate:       maxRate,
		targetLatency: time.Second, // 目標レイテンシ
		adjustPeriod:  5 * time.Second,
	}
	
	// レート調整ループを開始
	go alb.adjustRate()
	
	return alb
}

// adjustRate は定期的に処理レートを調整します
func (alb *AdaptiveLeakyBucket) adjustRate() {
	ticker := time.NewTicker(alb.adjustPeriod)
	defer ticker.Stop()
	
	for {
		select {
		case <-alb.done:
			return
		case <-ticker.C:
			alb.mu.Lock()
			queueSize := alb.queue.Len()
			currentRate := alb.rate
			
			// キューサイズに基づいてレートを調整
			if queueSize > alb.capacity/2 {
				// キューが半分以上埋まっている場合は処理を高速化
				newRate := currentRate * 9 / 10
				if newRate < alb.minRate {
					newRate = alb.minRate
				}
				alb.rate = newRate
				fmt.Printf("レート調整: %v → %v (キューサイズ: %d)\n", 
					currentRate, newRate, queueSize)
			} else if queueSize < alb.capacity/4 {
				// キューが1/4未満の場合は処理を低速化
				newRate := currentRate * 11 / 10
				if newRate > alb.maxRate {
					newRate = alb.maxRate
				}
				alb.rate = newRate
				fmt.Printf("レート調整: %v → %v (キューサイズ: %d)\n", 
					currentRate, newRate, queueSize)
			}
			
			alb.mu.Unlock()
		}
	}
}

// デモンストレーション
func main() {
	fmt.Println("リーキーバケットアルゴリズムデモ")
	fmt.Println("=================================")
	
	// 基本的なリーキーバケット
	fmt.Println("\n1. 基本的なリーキーバケット")
	bucket := NewLeakyBucket(5, 200*time.Millisecond)
	defer bucket.Stop()
	
	fmt.Println("10個のリクエストを送信（容量: 5）")
	
	var wg sync.WaitGroup
	successCount := 0
	failCount := 0
	
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			done, err := bucket.Submit(id)
			if err != nil {
				fmt.Printf("リクエスト %d: 拒否（バケット満杯）\n", id)
				failCount++
				return
			}
			
			fmt.Printf("リクエスト %d: キューに追加 (キューサイズ: %d)\n", 
				id, bucket.GetQueueSize())
			
			// 処理完了を待つ
			<-done
			successCount++
		}(i + 1)
		
		time.Sleep(50 * time.Millisecond)
	}
	
	wg.Wait()
	fmt.Printf("\n結果: 成功 %d, 失敗 %d\n", successCount, failCount)
	
	// バースト処理のテスト
	fmt.Println("\n\n2. バースト処理のテスト")
	bucket2 := NewLeakyBucket(10, 100*time.Millisecond)
	defer bucket2.Stop()
	
	fmt.Println("20個のリクエストを一度に送信")
	
	start := time.Now()
	processed := make(chan int, 20)
	
	for i := 0; i < 20; i++ {
		go func(id int) {
			done, err := bucket2.Submit(id)
			if err != nil {
				processed <- -1
				return
			}
			<-done
			processed <- id
		}(i + 1)
	}
	
	// 処理結果を収集
	successIds := []int{}
	for i := 0; i < 20; i++ {
		id := <-processed
		if id > 0 {
			successIds = append(successIds, id)
		}
	}
	
	elapsed := time.Since(start)
	fmt.Printf("処理完了: %d個のリクエスト, 所要時間: %v\n", 
		len(successIds), elapsed)
	fmt.Printf("実効レート: %.2f req/sec\n", 
		float64(len(successIds))/elapsed.Seconds())
	
	// 適応的リーキーバケット
	fmt.Println("\n\n3. 適応的リーキーバケット")
	adaptive := NewAdaptiveLeakyBucket(
		20,                    // 容量
		100*time.Millisecond,  // 初期レート
		50*time.Millisecond,   // 最小レート（最速）
		500*time.Millisecond,  // 最大レート（最遅）
	)
	defer adaptive.Stop()
	
	fmt.Println("負荷パターンをシミュレート")
	
	// 高負荷フェーズ
	fmt.Println("\n高負荷フェーズ: 30リクエスト")
	for i := 0; i < 30; i++ {
		go func(id int) {
			done, err := adaptive.Submit(id)
			if err == nil {
				<-done
			}
		}(i + 1)
	}
	
	time.Sleep(3 * time.Second)
	fmt.Printf("キューサイズ: %d\n", adaptive.GetQueueSize())
	
	// 低負荷フェーズ
	fmt.Println("\n低負荷フェーズ: 5秒間隔で5リクエスト")
	for i := 0; i < 5; i++ {
		go func(id int) {
			done, err := adaptive.Submit(id + 100)
			if err == nil {
				<-done
			}
		}(i)
		time.Sleep(1 * time.Second)
	}
	
	time.Sleep(6 * time.Second)
	
	fmt.Println("\n\nリーキーバケットの特徴:")
	fmt.Println("- リクエストをキューに保存し、一定レートで処理")
	fmt.Println("- バーストを平滑化し、下流システムを保護")
	fmt.Println("- 適応的バージョンは負荷に応じてレートを自動調整")
}