package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// SlidingLogRateLimiter はスライディングログアルゴリズムを実装
type SlidingLogRateLimiter struct {
	logs     []LogEntry
	limit    int
	window   time.Duration
	mu       sync.Mutex
	cleanupInterval time.Duration
}

// LogEntry はリクエストのログエントリ
type LogEntry struct {
	ID        string
	Timestamp time.Time
	UserID    string
	Weight    int
}

// NewSlidingLogRateLimiter は新しいスライディングログリミッターを作成
func NewSlidingLogRateLimiter(limit int, window time.Duration) *SlidingLogRateLimiter {
	sl := &SlidingLogRateLimiter{
		logs:     make([]LogEntry, 0),
		limit:    limit,
		window:   window,
		cleanupInterval: window / 10,
	}
	
	// 定期的なクリーンアップ
	go sl.cleanupLoop()
	
	return sl
}

// Allow はリクエストを許可するかチェック
func (sl *SlidingLogRateLimiter) Allow(userID string, weight int) bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	
	now := time.Now()
	windowStart := now.Add(-sl.window)
	
	// 古いエントリを削除
	sl.removeOldEntries(windowStart)
	
	// 現在のウィンドウ内のリクエスト数を計算
	count := 0
	for _, entry := range sl.logs {
		if entry.UserID == userID {
			count += entry.Weight
		}
	}
	
	// リミットチェック
	if count+weight > sl.limit {
		return false
	}
	
	// 新しいエントリを追加
	sl.logs = append(sl.logs, LogEntry{
		ID:        fmt.Sprintf("%s-%d", userID, now.UnixNano()),
		Timestamp: now,
		UserID:    userID,
		Weight:    weight,
	})
	
	return true
}

// removeOldEntries は古いエントリを削除
func (sl *SlidingLogRateLimiter) removeOldEntries(windowStart time.Time) {
	// バイナリサーチで削除位置を見つける
	idx := sort.Search(len(sl.logs), func(i int) bool {
		return sl.logs[i].Timestamp.After(windowStart)
	})
	
	if idx > 0 {
		sl.logs = sl.logs[idx:]
	}
}

// cleanupLoop は定期的にクリーンアップを実行
func (sl *SlidingLogRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(sl.cleanupInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		sl.mu.Lock()
		windowStart := time.Now().Add(-sl.window)
		sl.removeOldEntries(windowStart)
		sl.mu.Unlock()
	}
}

// GetUserStats はユーザーの統計情報を取得
func (sl *SlidingLogRateLimiter) GetUserStats(userID string) (count int, entries []LogEntry) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	
	now := time.Now()
	windowStart := now.Add(-sl.window)
	
	for _, entry := range sl.logs {
		if entry.Timestamp.After(windowStart) && entry.UserID == userID {
			count += entry.Weight
			entries = append(entries, entry)
		}
	}
	
	return
}

// OptimizedSlidingLog は最適化されたスライディングログ
type OptimizedSlidingLog struct {
	buckets  map[int64]*Bucket
	window   time.Duration
	limit    int
	bucketSize time.Duration
	mu       sync.RWMutex
}

// Bucket は時間バケット
type Bucket struct {
	timestamp time.Time
	counts    map[string]int
}

// NewOptimizedSlidingLog は最適化版を作成
func NewOptimizedSlidingLog(limit int, window time.Duration) *OptimizedSlidingLog {
	return &OptimizedSlidingLog{
		buckets:    make(map[int64]*Bucket),
		window:     window,
		limit:      limit,
		bucketSize: window / 60, // 60バケットに分割
	}
}

// Allow はリクエストを許可するかチェック
func (osl *OptimizedSlidingLog) Allow(userID string) bool {
	now := time.Now()
	bucketID := now.Unix() / int64(osl.bucketSize.Seconds())
	
	osl.mu.Lock()
	defer osl.mu.Unlock()
	
	// 現在のバケットを取得または作成
	bucket, exists := osl.buckets[bucketID]
	if !exists {
		bucket = &Bucket{
			timestamp: now,
			counts:    make(map[string]int),
		}
		osl.buckets[bucketID] = bucket
	}
	
	// 古いバケットを削除
	osl.cleanOldBuckets(now)
	
	// ユーザーのカウントを集計
	count := osl.getUserCount(userID, now)
	
	if count >= osl.limit {
		return false
	}
	
	// カウントを増加
	bucket.counts[userID]++
	return true
}

// cleanOldBuckets は古いバケットを削除
func (osl *OptimizedSlidingLog) cleanOldBuckets(now time.Time) {
	windowStart := now.Add(-osl.window)
	startBucketID := windowStart.Unix() / int64(osl.bucketSize.Seconds())
	
	for bucketID := range osl.buckets {
		if bucketID < startBucketID {
			delete(osl.buckets, bucketID)
		}
	}
}

// getUserCount はユーザーのカウントを集計
func (osl *OptimizedSlidingLog) getUserCount(userID string, now time.Time) int {
	windowStart := now.Add(-osl.window)
	startBucketID := windowStart.Unix() / int64(osl.bucketSize.Seconds())
	endBucketID := now.Unix() / int64(osl.bucketSize.Seconds())
	
	count := 0
	for bucketID := startBucketID; bucketID <= endBucketID; bucketID++ {
		if bucket, exists := osl.buckets[bucketID]; exists {
			count += bucket.counts[userID]
		}
	}
	
	return count
}

// CompressedSlidingLog は圧縮されたスライディングログ
type CompressedSlidingLog struct {
	entries  []CompressedEntry
	window   time.Duration
	limit    int
	mu       sync.Mutex
}

// CompressedEntry は圧縮されたエントリ
type CompressedEntry struct {
	timestamp uint32 // エポックからの秒数
	userHash  uint32 // ユーザーIDのハッシュ
	count     uint16 // カウント
}

// NewCompressedSlidingLog は圧縮版を作成
func NewCompressedSlidingLog(limit int, window time.Duration) *CompressedSlidingLog {
	return &CompressedSlidingLog{
		entries: make([]CompressedEntry, 0),
		window:  window,
		limit:   limit,
	}
}

// Allow はリクエストを許可するかチェック
func (csl *CompressedSlidingLog) Allow(userID string) bool {
	csl.mu.Lock()
	defer csl.mu.Unlock()
	
	now := time.Now()
	nowUnix := uint32(now.Unix())
	windowStart := uint32(now.Add(-csl.window).Unix())
	userHash := hashUserID(userID)
	
	// 古いエントリを削除
	newEntries := make([]CompressedEntry, 0, len(csl.entries))
	count := 0
	
	for _, entry := range csl.entries {
		if entry.timestamp >= windowStart {
			newEntries = append(newEntries, entry)
			if entry.userHash == userHash {
				count += int(entry.count)
			}
		}
	}
	
	csl.entries = newEntries
	
	if count >= csl.limit {
		return false
	}
	
	// 新しいエントリを追加または既存を更新
	found := false
	for i := range csl.entries {
		if csl.entries[i].timestamp == nowUnix && csl.entries[i].userHash == userHash {
			csl.entries[i].count++
			found = true
			break
		}
	}
	
	if !found {
		csl.entries = append(csl.entries, CompressedEntry{
			timestamp: nowUnix,
			userHash:  userHash,
			count:     1,
		})
	}
	
	return true
}

// hashUserID はユーザーIDをハッシュ化
func hashUserID(userID string) uint32 {
	hash := uint32(0)
	for _, c := range userID {
		hash = hash*31 + uint32(c)
	}
	return hash
}

// デモンストレーション
func main() {
	fmt.Println("スライディングログアルゴリズムデモ")
	fmt.Println("==================================")
	
	// 1. 基本的なスライディングログ
	fmt.Println("\n1. 基本的なスライディングログ (5 req/10s)")
	sl := NewSlidingLogRateLimiter(5, 10*time.Second)
	
	users := []string{"alice", "bob", "charlie"}
	
	// 各ユーザーがリクエスト
	for _, user := range users {
		fmt.Printf("\n%s のリクエスト:\n", user)
		for i := 0; i < 7; i++ {
			allowed := sl.Allow(user, 1)
			if allowed {
				fmt.Printf("  リクエスト %d: 許可\n", i+1)
			} else {
				fmt.Printf("  リクエスト %d: 拒否\n", i+1)
			}
			time.Sleep(1 * time.Second)
		}
		
		count, entries := sl.GetUserStats(user)
		fmt.Printf("  統計: %d リクエスト in window\n", count)
		for _, e := range entries {
			fmt.Printf("    - %s at %s\n", e.ID, e.Timestamp.Format("15:04:05"))
		}
	}
	
	// 2. 重み付きリクエスト
	fmt.Println("\n\n2. 重み付きリクエスト")
	sl2 := NewSlidingLogRateLimiter(10, 5*time.Second)
	
	requests := []struct {
		user   string
		weight int
		desc   string
	}{
		{"premium", 1, "通常リクエスト"},
		{"premium", 3, "重いリクエスト"},
		{"premium", 5, "非常に重いリクエスト"},
		{"premium", 2, "中程度のリクエスト"},
		{"premium", 1, "通常リクエスト"},
	}
	
	for _, req := range requests {
		allowed := sl2.Allow(req.user, req.weight)
		status := "拒否"
		if allowed {
			status = "許可"
		}
		fmt.Printf("%s (重み %d): %s\n", req.desc, req.weight, status)
		
		count, _ := sl2.GetUserStats(req.user)
		fmt.Printf("  現在の使用量: %d/10\n", count)
	}
	
	// 3. 最適化されたスライディングログ
	fmt.Println("\n\n3. 最適化版スライディングログ")
	osl := NewOptimizedSlidingLog(100, 60*time.Second)
	
	// 大量のリクエストをシミュレート
	var wg sync.WaitGroup
	start := time.Now()
	
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(userID string) {
			defer wg.Done()
			
			allowed := 0
			denied := 0
			
			for j := 0; j < 200; j++ {
				if osl.Allow(userID) {
					allowed++
				} else {
					denied++
				}
				time.Sleep(5 * time.Millisecond)
			}
			
			fmt.Printf("ユーザー %s: 許可=%d, 拒否=%d\n", userID, allowed, denied)
		}(fmt.Sprintf("user%d", i))
	}
	
	wg.Wait()
	fmt.Printf("処理時間: %v\n", time.Since(start))
	
	// 4. 圧縮版スライディングログ
	fmt.Println("\n\n4. 圧縮版スライディングログ（メモリ効率）")
	csl := NewCompressedSlidingLog(50, 30*time.Second)
	
	// メモリ使用量を比較
	fmt.Println("メモリ効率の比較:")
	fmt.Printf("通常版: LogEntry = %d bytes × エントリ数\n", 
		64) // approximate size
	fmt.Printf("圧縮版: CompressedEntry = %d bytes × エントリ数\n", 
		10) // 4 + 4 + 2 bytes
	
	// パフォーマンステスト
	users2 := make([]string, 100)
	for i := range users2 {
		users2[i] = fmt.Sprintf("user_%d", i)
	}
	
	start2 := time.Now()
	operations := 0
	
	for i := 0; i < 1000; i++ {
		user := users2[i%len(users2)]
		csl.Allow(user)
		operations++
	}
	
	elapsed := time.Since(start2)
	fmt.Printf("\n圧縮版パフォーマンス: %d ops in %v (%.2f ops/sec)\n",
		operations, elapsed, float64(operations)/elapsed.Seconds())
	
	// 5. 精度の比較
	fmt.Println("\n\n5. 各実装の精度比較")
	
	// 同じパターンで3つの実装をテスト
	sl3 := NewSlidingLogRateLimiter(10, 5*time.Second)
	osl3 := NewOptimizedSlidingLog(10, 5*time.Second)
	csl3 := NewCompressedSlidingLog(10, 5*time.Second)
	
	testUser := "testuser"
	
	fmt.Println("同じリクエストパターンでテスト:")
	for i := 0; i < 15; i++ {
		r1 := sl3.Allow(testUser, 1)
		r2 := osl3.Allow(testUser)
		r3 := csl3.Allow(testUser)
		
		fmt.Printf("リクエスト %2d: 基本=%v, 最適化=%v, 圧縮=%v\n",
			i+1, r1, r2, r3)
		
		if i == 7 {
			fmt.Println("  (5秒待機...)")
			time.Sleep(5 * time.Second)
		}
	}
	
	fmt.Println("\n\nスライディングログの特徴:")
	fmt.Println("- 最も正確なレート制限")
	fmt.Println("- メモリ使用量が多い")
	fmt.Println("- 実装の最適化が重要")
	fmt.Println("- 監査ログとしても使用可能")
}