package main

import (
	"container/heap"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// WFQScheduler は重み付き公平キューイングスケジューラー
type WFQScheduler struct {
	queues    map[string]*Queue
	heap      *VirtualTimeHeap
	virtualTime float64
	mu        sync.Mutex
	
	// 処理エンジン
	processor chan *Request
	done      chan struct{}
}

// Queue は各クラス/ユーザーのキュー
type Queue struct {
	id          string
	weight      float64
	virtualFinish float64
	requests    []*Request
	active      bool
	
	// 統計情報
	processed   int64
	totalDelay  int64
	lastService time.Time
}

// Request はキューイングされるリクエスト
type Request struct {
	ID        string
	QueueID   string
	Size      int
	Timestamp time.Time
	Done      chan bool
}

// VirtualTimeHeap は仮想終了時刻でソートされるヒープ
type VirtualTimeHeap []*Queue

// NewWFQScheduler は新しいWFQスケジューラーを作成
func NewWFQScheduler() *WFQScheduler {
	wfq := &WFQScheduler{
		queues:    make(map[string]*Queue),
		heap:      &VirtualTimeHeap{},
		processor: make(chan *Request, 100),
		done:      make(chan struct{}),
	}
	
	heap.Init(wfq.heap)
	
	// 処理ループを開始
	go wfq.processLoop()
	
	return wfq
}

// AddQueue は新しいキューを追加
func (wfq *WFQScheduler) AddQueue(id string, weight float64) {
	wfq.mu.Lock()
	defer wfq.mu.Unlock()
	
	if _, exists := wfq.queues[id]; exists {
		return
	}
	
	queue := &Queue{
		id:          id,
		weight:      weight,
		requests:    make([]*Request, 0),
		lastService: time.Now(),
	}
	
	wfq.queues[id] = queue
}

// Enqueue はリクエストをキューに追加
func (wfq *WFQScheduler) Enqueue(queueID string, requestID string, size int) (chan bool, error) {
	wfq.mu.Lock()
	defer wfq.mu.Unlock()
	
	queue, exists := wfq.queues[queueID]
	if !exists {
		return nil, fmt.Errorf("queue %s not found", queueID)
	}
	
	request := &Request{
		ID:        requestID,
		QueueID:   queueID,
		Size:      size,
		Timestamp: time.Now(),
		Done:      make(chan bool, 1),
	}
	
	queue.requests = append(queue.requests, request)
	
	// キューがアクティブでない場合、ヒープに追加
	if !queue.active {
		queue.active = true
		queue.virtualFinish = wfq.virtualTime + float64(size)/queue.weight
		heap.Push(wfq.heap, queue)
	}
	
	// 処理を促す
	select {
	case wfq.processor <- nil:
	default:
	}
	
	return request.Done, nil
}

// processLoop はリクエストを処理するメインループ
func (wfq *WFQScheduler) processLoop() {
	ticker := time.NewTicker(10 * time.Millisecond) // 処理レート
	defer ticker.Stop()
	
	for {
		select {
		case <-wfq.done:
			return
		case <-ticker.C:
			wfq.processNext()
		case <-wfq.processor:
			// 即座に処理を試みる
			wfq.processNext()
		}
	}
}

// processNext は次のリクエストを処理
func (wfq *WFQScheduler) processNext() {
	wfq.mu.Lock()
	defer wfq.mu.Unlock()
	
	if wfq.heap.Len() == 0 {
		return
	}
	
	// 最小の仮想終了時刻を持つキューを選択
	queue := heap.Pop(wfq.heap).(*Queue)
	
	if len(queue.requests) == 0 {
		queue.active = false
		return
	}
	
	// リクエストを取り出して処理
	request := queue.requests[0]
	queue.requests = queue.requests[1:]
	
	// 仮想時刻を更新
	wfq.virtualTime = math.Max(wfq.virtualTime, queue.virtualFinish)
	
	// 遅延を記録
	delay := time.Since(request.Timestamp)
	atomic.AddInt64(&queue.totalDelay, int64(delay))
	atomic.AddInt64(&queue.processed, 1)
	queue.lastService = time.Now()
	
	// 次のリクエストがある場合、仮想終了時刻を更新してヒープに戻す
	if len(queue.requests) > 0 {
		nextRequest := queue.requests[0]
		queue.virtualFinish = wfq.virtualTime + float64(nextRequest.Size)/queue.weight
		heap.Push(wfq.heap, queue)
	} else {
		queue.active = false
	}
	
	// リクエスト処理完了を通知
	request.Done <- true
	close(request.Done)
	
	fmt.Printf("処理: Queue=%s, Request=%s, Size=%d, Delay=%v\n",
		request.QueueID, request.ID, request.Size, delay)
}

// GetStats は統計情報を取得
func (wfq *WFQScheduler) GetStats() map[string]map[string]interface{} {
	wfq.mu.Lock()
	defer wfq.mu.Unlock()
	
	stats := make(map[string]map[string]interface{})
	
	for id, queue := range wfq.queues {
		processed := atomic.LoadInt64(&queue.processed)
		totalDelay := atomic.LoadInt64(&queue.totalDelay)
		
		avgDelay := time.Duration(0)
		if processed > 0 {
			avgDelay = time.Duration(totalDelay / processed)
		}
		
		stats[id] = map[string]interface{}{
			"weight":       queue.weight,
			"processed":    processed,
			"pending":      len(queue.requests),
			"avgDelay":     avgDelay,
			"lastService":  queue.lastService,
			"active":       queue.active,
		}
	}
	
	return stats
}

// Stop はスケジューラーを停止
func (wfq *WFQScheduler) Stop() {
	close(wfq.done)
}

// Heap インターフェースの実装
func (h VirtualTimeHeap) Len() int { return len(h) }

func (h VirtualTimeHeap) Less(i, j int) bool {
	return h[i].virtualFinish < h[j].virtualFinish
}

func (h VirtualTimeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *VirtualTimeHeap) Push(x interface{}) {
	*h = append(*h, x.(*Queue))
}

func (h *VirtualTimeHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

// DRRScheduler は Deficit Round Robin スケジューラー（WFQの簡易版）
type DRRScheduler struct {
	queues       map[string]*DRRQueue
	activeList   []*DRRQueue
	quantum      int
	currentIndex int
	mu           sync.Mutex
}

// DRRQueue はDRR用のキュー
type DRRQueue struct {
	id       string
	weight   int
	deficit  int
	requests []*Request
}

// NewDRRScheduler は新しいDRRスケジューラーを作成
func NewDRRScheduler(quantum int) *DRRScheduler {
	return &DRRScheduler{
		queues:  make(map[string]*DRRQueue),
		quantum: quantum,
	}
}

// デモンストレーション
func main() {
	fmt.Println("重み付き公平キューイング (WFQ) デモ")
	fmt.Println("===================================")
	
	// WFQスケジューラーを作成
	wfq := NewWFQScheduler()
	defer wfq.Stop()
	
	// 異なる重みを持つキューを作成
	wfq.AddQueue("gold", 4.0)    // ゴールドクラス（重み4）
	wfq.AddQueue("silver", 2.0)  // シルバークラス（重み2）
	wfq.AddQueue("bronze", 1.0)  // ブロンズクラス（重み1）
	
	fmt.Println("\nキュー構成:")
	fmt.Println("- Gold:   重み 4.0 (57%)")
	fmt.Println("- Silver: 重み 2.0 (29%)")
	fmt.Println("- Bronze: 重み 1.0 (14%)")
	
	// テスト1: 同じサイズのリクエストで重み配分を確認
	fmt.Println("\n\n1. 基本的な重み配分テスト")
	fmt.Println("各キューに10個の同じサイズ(100)のリクエストを追加")
	
	var wg sync.WaitGroup
	
	for _, queueID := range []string{"gold", "silver", "bronze"} {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(q string, idx int) {
				defer wg.Done()
				
				requestID := fmt.Sprintf("%s-%d", q, idx)
				done, err := wfq.Enqueue(q, requestID, 100)
				if err != nil {
					fmt.Printf("エラー: %v\n", err)
					return
				}
				
				<-done // 処理完了を待つ
			}(queueID, i)
		}
	}
	
	// 処理完了を待つ
	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	
	// 統計情報を表示
	fmt.Println("\n処理統計:")
	stats := wfq.GetStats()
	for id, stat := range stats {
		fmt.Printf("%s: 処理済み=%d, 平均遅延=%v\n",
			id, stat["processed"], stat["avgDelay"])
	}
	
	// テスト2: 可変サイズのリクエスト
	fmt.Println("\n\n2. 可変サイズリクエストのテスト")
	
	// 新しいスケジューラー
	wfq2 := NewWFQScheduler()
	defer wfq2.Stop()
	
	wfq2.AddQueue("video", 3.0)  // ビデオストリーム（大きいサイズ）
	wfq2.AddQueue("api", 2.0)    // API呼び出し（中サイズ）
	wfq2.AddQueue("ping", 1.0)   // ヘルスチェック（小さいサイズ）
	
	// 異なるサイズのリクエストを生成
	requests := []struct {
		queue string
		size  int
		count int
	}{
		{"video", 1000, 5},  // 大きいリクエスト
		{"api", 100, 10},    // 中サイズ
		{"ping", 10, 20},    // 小さいリクエスト
	}
	
	for _, req := range requests {
		for i := 0; i < req.count; i++ {
			wg.Add(1)
			go func(q string, s int, idx int) {
				defer wg.Done()
				
				requestID := fmt.Sprintf("%s-%d", q, idx)
				done, _ := wfq2.Enqueue(q, requestID, s)
				<-done
			}(req.queue, req.size, i)
		}
	}
	
	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	
	fmt.Println("\n処理統計（可変サイズ）:")
	stats2 := wfq2.GetStats()
	for id, stat := range stats2 {
		fmt.Printf("%s: 処理済み=%d, 平均遅延=%v\n",
			id, stat["processed"], stat["avgDelay"])
	}
	
	// テスト3: バースト処理とフェアネス
	fmt.Println("\n\n3. バースト処理とフェアネステスト")
	
	wfq3 := NewWFQScheduler()
	defer wfq3.Stop()
	
	wfq3.AddQueue("burst", 1.0)
	wfq3.AddQueue("steady", 1.0)
	
	// steadyキューに定常的な負荷
	go func() {
		for i := 0; i < 50; i++ {
			wfq3.Enqueue("steady", fmt.Sprintf("steady-%d", i), 100)
			time.Sleep(20 * time.Millisecond)
		}
	}()
	
	// burstキューにバースト的な負荷
	time.Sleep(200 * time.Millisecond)
	for i := 0; i < 20; i++ {
		wfq3.Enqueue("burst", fmt.Sprintf("burst-%d", i), 100)
	}
	
	time.Sleep(1 * time.Second)
	
	fmt.Println("\nバーストテスト結果:")
	stats3 := wfq3.GetStats()
	for id, stat := range stats3 {
		fmt.Printf("%s: 処理済み=%d, 平均遅延=%v\n",
			id, stat["processed"], stat["avgDelay"])
	}
	
	// テスト4: 優先度逆転の防止
	fmt.Println("\n\n4. 優先度逆転防止のデモ")
	
	wfq4 := NewWFQScheduler()
	defer wfq4.Stop()
	
	wfq4.AddQueue("high", 10.0)
	wfq4.AddQueue("low", 1.0)
	
	// 低優先度の大きなリクエストを先に追加
	fmt.Println("低優先度の大きなリクエスト(size=10000)を追加")
	done1, _ := wfq4.Enqueue("low", "low-big", 10000)
	
	time.Sleep(50 * time.Millisecond)
	
	// 高優先度の小さなリクエストを追加
	fmt.Println("高優先度の小さなリクエスト(size=100)を追加")
	start := time.Now()
	done2, _ := wfq4.Enqueue("high", "high-small", 100)
	
	<-done2
	highDelay := time.Since(start)
	fmt.Printf("高優先度リクエストの遅延: %v\n", highDelay)
	
	<-done1
	fmt.Println("低優先度リクエストも完了")
	
	fmt.Println("\n\nWFQの特徴:")
	fmt.Println("- 重みに基づく公平なリソース配分")
	fmt.Println("- 低遅延保証（小さいリクエストは早く処理）")
	fmt.Println("- 優先度逆転の防止")
	fmt.Println("- 長期的な公平性の保証")
}