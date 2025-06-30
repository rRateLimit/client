package main

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// ProbabilisticRateLimiter は確率的なレート制限を実装
type ProbabilisticRateLimiter struct {
	targetRate     float64 // 目標レート（req/sec）
	currentLoad    atomic.Value // 現在の負荷
	acceptanceProb atomic.Value // 受け入れ確率
	
	// メトリクス
	requests  int64
	accepted  int64
	rejected  int64
	lastReset time.Time
	
	mu sync.RWMutex
}

// NewProbabilisticRateLimiter は新しい確率的レートリミッターを作成
func NewProbabilisticRateLimiter(targetRate float64) *ProbabilisticRateLimiter {
	prl := &ProbabilisticRateLimiter{
		targetRate: targetRate,
		lastReset:  time.Now(),
	}
	
	prl.currentLoad.Store(0.0)
	prl.acceptanceProb.Store(1.0)
	
	// 定期的に統計をリセットして確率を調整
	go prl.adjustmentLoop()
	
	return prl
}

// Allow は確率的にリクエストを許可
func (prl *ProbabilisticRateLimiter) Allow() bool {
	atomic.AddInt64(&prl.requests, 1)
	
	// 現在の受け入れ確率を取得
	prob := prl.acceptanceProb.Load().(float64)
	
	// 確率的な判定
	if rand.Float64() < prob {
		atomic.AddInt64(&prl.accepted, 1)
		return true
	}
	
	atomic.AddInt64(&prl.rejected, 1)
	return false
}

// adjustmentLoop は定期的に受け入れ確率を調整
func (prl *ProbabilisticRateLimiter) adjustmentLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		prl.adjust()
	}
}

// adjust は負荷に基づいて確率を調整
func (prl *ProbabilisticRateLimiter) adjust() {
	prl.mu.Lock()
	defer prl.mu.Unlock()
	
	elapsed := time.Since(prl.lastReset).Seconds()
	if elapsed == 0 {
		return
	}
	
	// 現在のレートを計算
	currentRate := float64(atomic.LoadInt64(&prl.accepted)) / elapsed
	prl.currentLoad.Store(currentRate)
	
	// 目標レートとの比率で確率を調整
	ratio := prl.targetRate / (currentRate + 1) // +1 でゼロ除算を防ぐ
	
	// 現在の確率を取得
	currentProb := prl.acceptanceProb.Load().(float64)
	
	// 新しい確率を計算（指数移動平均でスムージング）
	alpha := 0.7 // スムージング係数
	newProb := alpha*currentProb*ratio + (1-alpha)*currentProb
	
	// 確率を0-1の範囲に制限
	newProb = math.Max(0.0, math.Min(1.0, newProb))
	
	prl.acceptanceProb.Store(newProb)
	
	// 統計をリセット
	atomic.StoreInt64(&prl.requests, 0)
	atomic.StoreInt64(&prl.accepted, 0)
	atomic.StoreInt64(&prl.rejected, 0)
	prl.lastReset = time.Now()
	
	fmt.Printf("調整: レート=%.2f/%.2f, 確率=%.2f%% → %.2f%%\n",
		currentRate, prl.targetRate, currentProb*100, newProb*100)
}

// GetStats は統計情報を取得
func (prl *ProbabilisticRateLimiter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"targetRate":     prl.targetRate,
		"currentLoad":    prl.currentLoad.Load().(float64),
		"acceptanceProb": prl.acceptanceProb.Load().(float64),
		"requests":       atomic.LoadInt64(&prl.requests),
		"accepted":       atomic.LoadInt64(&prl.accepted),
		"rejected":       atomic.LoadInt64(&prl.rejected),
	}
}

// BloomFilterRateLimiter はBloomフィルタを使用した確率的レート制限
type BloomFilterRateLimiter struct {
	filters      []*BloomFilter
	currentIndex int
	windowSize   time.Duration
	maxRequests  int
	mu           sync.Mutex
}

// BloomFilter は簡易的なBloomフィルタ実装
type BloomFilter struct {
	bits     []uint64
	size     int
	hashFunc int
}

// NewBloomFilter は新しいBloomフィルタを作成
func NewBloomFilter(size int) *BloomFilter {
	return &BloomFilter{
		bits:     make([]uint64, (size+63)/64),
		size:     size,
		hashFunc: 3, // ハッシュ関数の数
	}
}

// Add は要素を追加
func (bf *BloomFilter) Add(item string) {
	for i := 0; i < bf.hashFunc; i++ {
		hash := bf.hash(item, i)
		idx := hash / 64
		bit := hash % 64
		bf.bits[idx] |= 1 << bit
	}
}

// Contains は要素の存在をチェック
func (bf *BloomFilter) Contains(item string) bool {
	for i := 0; i < bf.hashFunc; i++ {
		hash := bf.hash(item, i)
		idx := hash / 64
		bit := hash % 64
		if bf.bits[idx]&(1<<bit) == 0 {
			return false
		}
	}
	return true
}

// hash はハッシュ値を計算
func (bf *BloomFilter) hash(item string, seed int) int {
	hash := seed
	for _, c := range item {
		hash = hash*31 + int(c)
	}
	return abs(hash) % bf.size
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// NewBloomFilterRateLimiter は新しいBloomフィルタベースのレートリミッターを作成
func NewBloomFilterRateLimiter(maxRequests int, windowSize time.Duration) *BloomFilterRateLimiter {
	bfrl := &BloomFilterRateLimiter{
		filters:     make([]*BloomFilter, 2),
		windowSize:  windowSize,
		maxRequests: maxRequests,
	}
	
	// 2つのフィルタを初期化（ローテーション用）
	for i := range bfrl.filters {
		bfrl.filters[i] = NewBloomFilter(maxRequests * 10)
	}
	
	// フィルタローテーション
	go bfrl.rotateFilters()
	
	return bfrl
}

// Allow はユーザーのリクエストを許可するかチェック
func (bfrl *BloomFilterRateLimiter) Allow(userID string) bool {
	bfrl.mu.Lock()
	defer bfrl.mu.Unlock()
	
	// 現在のフィルタをチェック
	current := bfrl.filters[bfrl.currentIndex]
	
	// すでに記録されている場合は確率的に拒否
	if current.Contains(userID) {
		// 誤検出率を考慮して一定確率で許可
		if rand.Float64() < 0.1 { // 10%の確率で許可
			return true
		}
		return false
	}
	
	// 新規リクエストを記録
	current.Add(userID)
	return true
}

// rotateFilters は定期的にフィルタを切り替え
func (bfrl *BloomFilterRateLimiter) rotateFilters() {
	ticker := time.NewTicker(bfrl.windowSize / 2)
	defer ticker.Stop()
	
	for range ticker.C {
		bfrl.mu.Lock()
		// インデックスを切り替え
		bfrl.currentIndex = (bfrl.currentIndex + 1) % 2
		// 新しいフィルタをクリア
		bfrl.filters[bfrl.currentIndex] = NewBloomFilter(bfrl.maxRequests * 10)
		bfrl.mu.Unlock()
	}
}

// HyperLogLogRateLimiter はHyperLogLogを使用したカーディナリティベースの制限
type HyperLogLogRateLimiter struct {
	hll         *HyperLogLog
	maxUnique   int
	windowStart time.Time
	windowSize  time.Duration
	mu          sync.Mutex
}

// HyperLogLog は簡易的な実装
type HyperLogLog struct {
	registers []uint8
	m         int // レジスタ数（2の冪）
}

// NewHyperLogLog は新しいHyperLogLogを作成
func NewHyperLogLog(precision int) *HyperLogLog {
	m := 1 << precision
	return &HyperLogLog{
		registers: make([]uint8, m),
		m:         m,
	}
}

// Add は要素を追加
func (hll *HyperLogLog) Add(item string) {
	hash := hll.hash(item)
	j := hash & uint32(hll.m-1)        // レジスタインデックス
	w := hash >> uint(len(fmt.Sprintf("%b", hll.m-1))) // 残りのビット
	
	// Leading zerosを数える
	rho := leadingZeros(w) + 1
	
	if rho > hll.registers[j] {
		hll.registers[j] = rho
	}
}

// EstimateCardinality はユニーク要素数を推定
func (hll *HyperLogLog) EstimateCardinality() int {
	sum := 0.0
	zeros := 0
	
	for _, val := range hll.registers {
		if val == 0 {
			zeros++
		}
		sum += math.Pow(2, -float64(val))
	}
	
	estimate := hll.alpha() * float64(hll.m*hll.m) / sum
	
	// 小さい値の補正
	if estimate <= 2.5*float64(hll.m) && zeros != 0 {
		estimate = float64(hll.m) * math.Log(float64(hll.m)/float64(zeros))
	}
	
	return int(estimate)
}

// alpha は補正係数
func (hll *HyperLogLog) alpha() float64 {
	switch hll.m {
	case 16:
		return 0.673
	case 32:
		return 0.697
	case 64:
		return 0.709
	default:
		return 0.7213 / (1 + 1.079/float64(hll.m))
	}
}

// hash はハッシュ値を計算
func (hll *HyperLogLog) hash(item string) uint32 {
	h := uint32(0)
	for _, c := range item {
		h = h*31 + uint32(c)
	}
	return h
}

// leadingZeros は先頭の0の数を数える
func leadingZeros(x uint32) uint8 {
	if x == 0 {
		return 32
	}
	n := uint8(0)
	for x&0x80000000 == 0 {
		n++
		x <<= 1
	}
	return n
}

// デモンストレーション
func main() {
	fmt.Println("確率的レートリミッターデモ")
	fmt.Println("==========================")
	
	// 1. 基本的な確率的レートリミッター
	fmt.Println("\n1. 基本的な確率的レートリミッター")
	prl := NewProbabilisticRateLimiter(50) // 50 req/sec
	
	// 負荷テスト
	simulate := func(rate int, duration time.Duration, name string) {
		fmt.Printf("\n%s: %d req/sec で %v 間テスト\n", name, rate, duration)
		
		start := time.Now()
		accepted := int64(0)
		rejected := int64(0)
		
		ticker := time.NewTicker(time.Second / time.Duration(rate))
		defer ticker.Stop()
		
		done := time.After(duration)
		
		for {
			select {
			case <-done:
				elapsed := time.Since(start).Seconds()
				fmt.Printf("結果: 許可=%d (%.2f req/sec), 拒否=%d\n",
					accepted, float64(accepted)/elapsed, rejected)
				return
			case <-ticker.C:
				if prl.Allow() {
					accepted++
				} else {
					rejected++
				}
			}
		}
	}
	
	// 異なる負荷でテスト
	simulate(30, 3*time.Second, "低負荷")
	simulate(100, 3*time.Second, "高負荷")
	simulate(50, 3*time.Second, "目標負荷")
	
	// 2. Bloomフィルタベースのレートリミッター
	fmt.Println("\n\n2. Bloomフィルタベースのレートリミッター")
	bfrl := NewBloomFilterRateLimiter(100, 10*time.Second)
	
	users := []string{"user1", "user2", "user3", "user4", "user5"}
	
	fmt.Println("\n各ユーザーが複数回リクエスト:")
	for round := 0; round < 3; round++ {
		fmt.Printf("\nラウンド %d:\n", round+1)
		for _, user := range users {
			allowed := 0
			for i := 0; i < 5; i++ {
				if bfrl.Allow(user) {
					allowed++
				}
			}
			fmt.Printf("%s: %d/5 リクエスト許可\n", user, allowed)
		}
		time.Sleep(1 * time.Second)
	}
	
	// 3. HyperLogLogベースのユニークユーザー制限
	fmt.Println("\n\n3. HyperLogLogベースのカーディナリティ制限")
	
	hll := NewHyperLogLog(10) // 2^10 = 1024 レジスタ
	
	// ユニークユーザーを追加
	uniqueUsers := 0
	for i := 0; i < 1000; i++ {
		userID := fmt.Sprintf("user_%d", rand.Intn(100))
		hll.Add(userID)
		if i%100 == 99 {
			estimate := hll.EstimateCardinality()
			fmt.Printf("追加: %d, 推定ユニーク数: %d\n", i+1, estimate)
		}
	}
	
	// 4. 確率的サンプリング
	fmt.Println("\n\n4. 確率的サンプリングによるレート制限")
	
	// リザーバーサンプリング
	reservoir := make([]string, 10)
	seen := 0
	
	fmt.Println("\n1000リクエストから10個をサンプリング:")
	for i := 0; i < 1000; i++ {
		seen++
		requestID := fmt.Sprintf("req_%d", i)
		
		if seen <= len(reservoir) {
			reservoir[seen-1] = requestID
		} else {
			// 確率的に既存の要素を置き換え
			j := rand.Intn(seen)
			if j < len(reservoir) {
				reservoir[j] = requestID
			}
		}
	}
	
	fmt.Println("サンプル結果:")
	for i, req := range reservoir {
		fmt.Printf("  %d: %s\n", i+1, req)
	}
	
	// 5. 確率的ドロップによる優先度制御
	fmt.Println("\n\n5. 優先度ベースの確率的ドロップ")
	
	priorities := map[string]float64{
		"high":   0.9,  // 90%の確率で許可
		"medium": 0.5,  // 50%の確率で許可
		"low":    0.1,  // 10%の確率で許可
	}
	
	for priority, prob := range priorities {
		allowed := 0
		total := 100
		
		for i := 0; i < total; i++ {
			if rand.Float64() < prob {
				allowed++
			}
		}
		
		fmt.Printf("%s優先度: %d/%d (%.0f%%) 許可\n",
			priority, allowed, total, float64(allowed)/float64(total)*100)
	}
	
	fmt.Println("\n\n確率的レートリミッターの特徴:")
	fmt.Println("- メモリ効率的（確率的データ構造）")
	fmt.Println("- 分散環境での実装が容易")
	fmt.Println("- 厳密さと効率のトレードオフ")
	fmt.Println("- 大規模システムに適している")
}