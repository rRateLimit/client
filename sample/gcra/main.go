package main

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// GCRA (Generic Cell Rate Algorithm) は高精度なレート制限を実現します
// ATMネットワークで使用されるアルゴリズムをHTTPレート制限に適用
type GCRA struct {
	// τ (tau): 発信間隔（emission interval）
	tau float64
	
	// T: バースト許容値（tolerance）
	burst float64
	
	// TAT: 理論到着時刻（Theoretical Arrival Time）
	tat atomic.Value // float64として保存
	
	// 時計の精度向上のためのナノ秒単位の基準時刻
	startTime time.Time
	
	mu sync.Mutex
}

// NewGCRA は新しいGCRAリミッターを作成します
// rate: 1秒あたりのリクエスト数
// burst: バーストサイズ
func NewGCRA(rate float64, burst int) *GCRA {
	gcra := &GCRA{
		tau:       1.0 / rate,
		burst:     float64(burst),
		startTime: time.Now(),
	}
	gcra.tat.Store(0.0)
	return gcra
}

// Allow はリクエストを許可するかどうかを判定します
func (g *GCRA) Allow() bool {
	return g.AllowN(1)
}

// AllowN はn個のセルを許可するかどうかを判定します
func (g *GCRA) AllowN(n int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	now := g.now()
	tat := g.tat.Load().(float64)
	
	// 新しいTATを計算
	newTat := math.Max(now, tat) + float64(n)*g.tau
	
	// バースト制限チェック
	if newTat-now > g.burst*g.tau {
		return false
	}
	
	// TATを更新
	g.tat.Store(newTat)
	return true
}

// AllowAt は指定時刻でのリクエストを許可するかチェックします（テスト用）
func (g *GCRA) AllowAt(t time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	now := float64(t.Sub(g.startTime).Nanoseconds()) / 1e9
	tat := g.tat.Load().(float64)
	
	newTat := math.Max(now, tat) + g.tau
	
	if newTat-now > g.burst*g.tau {
		return false
	}
	
	g.tat.Store(newTat)
	return true
}

// now は現在時刻を秒単位で返します
func (g *GCRA) now() float64 {
	return float64(time.Since(g.startTime).Nanoseconds()) / 1e9
}

// GetInfo は現在の状態情報を返します
func (g *GCRA) GetInfo() (nextAllowedTime time.Time, availableBurst int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	now := g.now()
	tat := g.tat.Load().(float64)
	
	// 次に許可される時刻
	if tat > now {
		nextAllowedTime = g.startTime.Add(time.Duration(tat * 1e9))
	} else {
		nextAllowedTime = time.Now()
	}
	
	// 利用可能なバースト
	availableBurst = int((g.burst*g.tau - (tat - now)) / g.tau)
	if availableBurst < 0 {
		availableBurst = 0
	} else if availableBurst > int(g.burst) {
		availableBurst = int(g.burst)
	}
	
	return
}

// MultiTierGCRA は複数の時間枠でレート制限を行います
type MultiTierGCRA struct {
	limiters map[string]*GCRA
	mu       sync.RWMutex
}

// NewMultiTierGCRA は階層的なレート制限を作成します
func NewMultiTierGCRA() *MultiTierGCRA {
	return &MultiTierGCRA{
		limiters: map[string]*GCRA{
			"second": NewGCRA(10, 20),    // 10 req/sec, burst 20
			"minute": NewGCRA(300, 50),   // 300 req/min (5/sec avg), burst 50
			"hour":   NewGCRA(10000, 100), // 10000 req/hour, burst 100
		},
	}
}

// Allow はすべての階層でチェックを行います
func (m *MultiTierGCRA) Allow() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, limiter := range m.limiters {
		if !limiter.Allow() {
			return false
		}
	}
	return true
}

// GetStatus は各階層の状態を返します
func (m *MultiTierGCRA) GetStatus() map[string]struct {
	NextAllowed    time.Time
	AvailableBurst int
} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	status := make(map[string]struct {
		NextAllowed    time.Time
		AvailableBurst int
	})
	
	for name, limiter := range m.limiters {
		next, burst := limiter.GetInfo()
		status[name] = struct {
			NextAllowed    time.Time
			AvailableBurst int
		}{next, burst}
	}
	
	return status
}

// デモンストレーション
func main() {
	fmt.Println("GCRA (Generic Cell Rate Algorithm) デモ")
	fmt.Println("=======================================")
	
	// 基本的なGCRA
	fmt.Println("\n1. 基本的なGCRA (10 req/sec, burst 5)")
	gcra := NewGCRA(10, 5)
	
	// バースト処理
	fmt.Println("\nバーストテスト: 8リクエストを即座に送信")
	successCount := 0
	for i := 0; i < 8; i++ {
		if gcra.Allow() {
			successCount++
			next, burst := gcra.GetInfo()
			fmt.Printf("リクエスト %d: 許可 (次回可能時刻: %v, 残バースト: %d)\n",
				i+1, next.Format("15:04:05.000"), burst)
		} else {
			fmt.Printf("リクエスト %d: 拒否\n", i+1)
		}
	}
	fmt.Printf("成功: %d/8\n", successCount)
	
	// レート制限の確認
	fmt.Println("\n100ms間隔で追加リクエスト")
	time.Sleep(100 * time.Millisecond)
	
	for i := 0; i < 5; i++ {
		if gcra.Allow() {
			fmt.Printf("時刻 %s: 許可\n", time.Now().Format("15:04:05.000"))
		} else {
			next, _ := gcra.GetInfo()
			fmt.Printf("時刻 %s: 拒否 (次回: %s)\n",
				time.Now().Format("15:04:05.000"),
				next.Format("15:04:05.000"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	// 高精度テスト
	fmt.Println("\n\n2. 高精度テスト (100 req/sec)")
	highRate := NewGCRA(100, 10)
	
	start := time.Now()
	allowed := 0
	total := 0
	
	// 1秒間テスト
	for time.Since(start) < time.Second {
		total++
		if highRate.Allow() {
			allowed++
		}
		time.Sleep(5 * time.Millisecond) // 200 req/sec のペースで送信
	}
	
	elapsed := time.Since(start)
	fmt.Printf("結果: %d/%d リクエスト許可 (%.2f req/sec)\n",
		allowed, total, float64(allowed)/elapsed.Seconds())
	
	// マルチティアGCRA
	fmt.Println("\n\n3. 階層的レート制限")
	multi := NewMultiTierGCRA()
	
	fmt.Println("初期状態:")
	for tier, info := range multi.GetStatus() {
		fmt.Printf("  %s: バースト残 %d\n", tier, info.AvailableBurst)
	}
	
	// バーストテスト
	fmt.Println("\n30リクエストのバースト:")
	allowed = 0
	for i := 0; i < 30; i++ {
		if multi.Allow() {
			allowed++
		}
	}
	fmt.Printf("成功: %d/30\n", allowed)
	
	fmt.Println("\n各階層の状態:")
	for tier, info := range multi.GetStatus() {
		fmt.Printf("  %s: バースト残 %d, 次回可能 %v\n",
			tier, info.AvailableBurst, info.NextAllowed.Format("15:04:05.000"))
	}
	
	// 並行アクセステスト
	fmt.Println("\n\n4. 並行アクセステスト")
	gcra2 := NewGCRA(50, 10)
	
	var wg sync.WaitGroup
	successAtomic := int64(0)
	totalAtomic := int64(0)
	
	// 10ゴルーチンで1秒間アクセス
	testDuration := time.Second
	numGoroutines := 10
	
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			localSuccess := 0
			localTotal := 0
			start := time.Now()
			
			for time.Since(start) < testDuration {
				localTotal++
				if gcra2.Allow() {
					localSuccess++
				}
				time.Sleep(time.Millisecond) // 各ゴルーチンは1000 req/sec
			}
			
			atomic.AddInt64(&successAtomic, int64(localSuccess))
			atomic.AddInt64(&totalAtomic, int64(localTotal))
		}(i)
	}
	
	wg.Wait()
	
	fmt.Printf("並行テスト結果: %d/%d リクエスト許可 (%.2f req/sec)\n",
		successAtomic, totalAtomic,
		float64(successAtomic)/testDuration.Seconds())
	
	// アルゴリズムの特徴
	fmt.Println("\n\nGCRAの特徴:")
	fmt.Println("- 高精度なレート制限（ナノ秒単位）")
	fmt.Println("- メモリ効率的（タイムスタンプ1つのみ保存）")
	fmt.Println("- 公平性が高い（到着順序を保持）")
	fmt.Println("- ATMネットワークで実証済みの信頼性")
}