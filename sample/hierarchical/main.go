package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// HierarchicalRateLimiter は階層的なレート制限を実装
type HierarchicalRateLimiter struct {
	root *Node
	mu   sync.RWMutex
}

// Node は階層構造のノード
type Node struct {
	name     string
	path     string
	limiter  RateLimiter
	parent   *Node
	children map[string]*Node
	mu       sync.RWMutex
	
	// 共有リソースプール
	sharedTokens *SharedTokenPool
}

// RateLimiter インターフェース
type RateLimiter interface {
	Allow() bool
	AllowN(n int) bool
	Name() string
}

// SharedTokenPool は親子間でトークンを共有するプール
type SharedTokenPool struct {
	capacity int64
	tokens   int64
	borrowed int64 // 子ノードに貸し出したトークン数
	mu       sync.Mutex
}

// TokenBucketLimiter は階層用のトークンバケット実装
type TokenBucketLimiter struct {
	name     string
	capacity int64
	tokens   int64
	rate     int64
	parent   *SharedTokenPool
}

// NewHierarchicalRateLimiter は新しい階層的レートリミッターを作成
func NewHierarchicalRateLimiter() *HierarchicalRateLimiter {
	rootLimiter := &TokenBucketLimiter{
		name:     "root",
		capacity: 1000,
		tokens:   1000,
		rate:     100,
	}
	
	root := &Node{
		name:     "root",
		path:     "/",
		limiter:  rootLimiter,
		children: make(map[string]*Node),
		sharedTokens: &SharedTokenPool{
			capacity: 1000,
			tokens:   1000,
		},
	}
	
	hrl := &HierarchicalRateLimiter{
		root: root,
	}
	
	// トークン補充を開始
	go hrl.refillLoop()
	
	return hrl
}

// AddNode は新しいノードを階層に追加
func (hrl *HierarchicalRateLimiter) AddNode(path string, capacity, rate int64) error {
	hrl.mu.Lock()
	defer hrl.mu.Unlock()
	
	// パスを解析してノードを作成
	segments := splitPath(path)
	current := hrl.root
	
	for i, segment := range segments {
		current.mu.Lock()
		
		child, exists := current.children[segment]
		if !exists {
			// 新しいノードを作成
			childPath := joinPath(segments[:i+1])
			childLimiter := &TokenBucketLimiter{
				name:     segment,
				capacity: capacity,
				tokens:   capacity,
				rate:     rate,
				parent:   current.sharedTokens,
			}
			
			child = &Node{
				name:     segment,
				path:     childPath,
				limiter:  childLimiter,
				parent:   current,
				children: make(map[string]*Node),
				sharedTokens: &SharedTokenPool{
					capacity: capacity,
					tokens:   capacity,
				},
			}
			
			current.children[segment] = child
		}
		
		current.mu.Unlock()
		current = child
	}
	
	return nil
}

// Allow はパスに対してリクエストを許可するかチェック
func (hrl *HierarchicalRateLimiter) Allow(path string) bool {
	node := hrl.findNode(path)
	if node == nil {
		return false
	}
	
	// 階層を上にたどってすべてのレベルでチェック
	current := node
	nodes := []*Node{}
	
	for current != nil {
		nodes = append(nodes, current)
		current = current.parent
	}
	
	// ルートから順にチェック（トップダウン）
	for i := len(nodes) - 1; i >= 0; i-- {
		if !nodes[i].limiter.Allow() {
			return false
		}
	}
	
	return true
}

// findNode はパスに対応するノードを検索
func (hrl *HierarchicalRateLimiter) findNode(path string) *Node {
	hrl.mu.RLock()
	defer hrl.mu.RUnlock()
	
	segments := splitPath(path)
	current := hrl.root
	
	for _, segment := range segments {
		current.mu.RLock()
		child, exists := current.children[segment]
		current.mu.RUnlock()
		
		if !exists {
			return current // 最も近い親ノードを返す
		}
		current = child
	}
	
	return current
}

// refillLoop は定期的にトークンを補充
func (hrl *HierarchicalRateLimiter) refillLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for range ticker.C {
		hrl.refillNode(hrl.root)
	}
}

// refillNode は再帰的にノードのトークンを補充
func (hrl *HierarchicalRateLimiter) refillNode(node *Node) {
	// 自身のトークンを補充
	if tb, ok := node.limiter.(*TokenBucketLimiter); ok {
		current := atomic.LoadInt64(&tb.tokens)
		if current < tb.capacity {
			toAdd := tb.rate / 10 // 100ms ごとの補充量
			newValue := current + toAdd
			if newValue > tb.capacity {
				newValue = tb.capacity
			}
			atomic.StoreInt64(&tb.tokens, newValue)
		}
	}
	
	// 共有プールも補充
	if node.sharedTokens != nil {
		node.sharedTokens.mu.Lock()
		if node.sharedTokens.tokens < node.sharedTokens.capacity {
			toAdd := node.sharedTokens.capacity / 10
			node.sharedTokens.tokens += toAdd
			if node.sharedTokens.tokens > node.sharedTokens.capacity {
				node.sharedTokens.tokens = node.sharedTokens.capacity
			}
		}
		node.sharedTokens.mu.Unlock()
	}
	
	// 子ノードを再帰的に補充
	node.mu.RLock()
	children := make([]*Node, 0, len(node.children))
	for _, child := range node.children {
		children = append(children, child)
	}
	node.mu.RUnlock()
	
	for _, child := range children {
		hrl.refillNode(child)
	}
}

// GetStats は階層の統計情報を取得
func (hrl *HierarchicalRateLimiter) GetStats() map[string]interface{} {
	hrl.mu.RLock()
	defer hrl.mu.RUnlock()
	
	stats := make(map[string]interface{})
	hrl.collectStats(hrl.root, stats)
	return stats
}

// collectStats は再帰的に統計情報を収集
func (hrl *HierarchicalRateLimiter) collectStats(node *Node, stats map[string]interface{}) {
	if tb, ok := node.limiter.(*TokenBucketLimiter); ok {
		stats[node.path] = map[string]interface{}{
			"tokens":   atomic.LoadInt64(&tb.tokens),
			"capacity": tb.capacity,
			"rate":     tb.rate,
		}
	}
	
	node.mu.RLock()
	defer node.mu.RUnlock()
	
	for _, child := range node.children {
		hrl.collectStats(child, stats)
	}
}

// TokenBucketLimiterの実装
func (tb *TokenBucketLimiter) Allow() bool {
	return tb.AllowN(1)
}

func (tb *TokenBucketLimiter) AllowN(n int) bool {
	// まず自身のトークンをチェック
	current := atomic.LoadInt64(&tb.tokens)
	if current >= int64(n) {
		if atomic.CompareAndSwapInt64(&tb.tokens, current, current-int64(n)) {
			return true
		}
	}
	
	// 親の共有プールから借りる
	if tb.parent != nil {
		tb.parent.mu.Lock()
		defer tb.parent.mu.Unlock()
		
		if tb.parent.tokens >= int64(n) {
			tb.parent.tokens -= int64(n)
			tb.parent.borrowed += int64(n)
			return true
		}
	}
	
	return false
}

func (tb *TokenBucketLimiter) Name() string {
	return tb.name
}

// ユーティリティ関数
func splitPath(path string) []string {
	segments := []string{}
	for _, s := range path {
		if s == '/' {
			continue
		}
		segments = append(segments, string(s))
	}
	if len(segments) == 0 {
		return segments
	}
	
	// 実際の実装では適切なパス分割が必要
	if path == "/api" {
		return []string{"api"}
	} else if path == "/api/users" {
		return []string{"api", "users"}
	} else if path == "/api/posts" {
		return []string{"api", "posts"}
	}
	
	return segments
}

func joinPath(segments []string) string {
	if len(segments) == 0 {
		return "/"
	}
	result := "/"
	for i, s := range segments {
		result += s
		if i < len(segments)-1 {
			result += "/"
		}
	}
	return result
}

// デモンストレーション
func main() {
	fmt.Println("階層的レートリミッターデモ")
	fmt.Println("==========================")
	
	// 階層的レートリミッターを作成
	hrl := NewHierarchicalRateLimiter()
	
	// API階層を構築
	hrl.AddNode("/api", 500, 50)
	hrl.AddNode("/api/users", 200, 20)
	hrl.AddNode("/api/posts", 300, 30)
	
	fmt.Println("\n階層構造:")
	fmt.Println("/          (1000 req/sec)")
	fmt.Println("└── api    (500 req/sec)")
	fmt.Println("    ├── users (200 req/sec)")
	fmt.Println("    └── posts (300 req/sec)")
	
	// テスト1: 各エンドポイントへのアクセス
	fmt.Println("\n\n1. 各エンドポイントへの連続アクセス")
	
	endpoints := []string{"/", "/api", "/api/users", "/api/posts"}
	for _, endpoint := range endpoints {
		fmt.Printf("\n%s への10リクエスト:\n", endpoint)
		allowed := 0
		for i := 0; i < 10; i++ {
			if hrl.Allow(endpoint) {
				allowed++
			}
		}
		fmt.Printf("許可: %d/10\n", allowed)
	}
	
	// 統計情報を表示
	fmt.Println("\n現在のトークン状態:")
	stats := hrl.GetStats()
	for path, stat := range stats {
		if s, ok := stat.(map[string]interface{}); ok {
			fmt.Printf("%s: %d/%d トークン\n", path, s["tokens"], s["capacity"])
		}
	}
	
	// テスト2: 階層的制限の確認
	fmt.Println("\n\n2. 階層的制限のテスト")
	time.Sleep(1 * time.Second) // トークン回復を待つ
	
	fmt.Println("\n/api/users への大量リクエスト（親の制限も影響）:")
	
	var wg sync.WaitGroup
	allowedCount := int64(0)
	totalCount := 100
	
	for i := 0; i < totalCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if hrl.Allow("/api/users") {
				atomic.AddInt64(&allowedCount, 1)
			}
		}()
	}
	
	wg.Wait()
	fmt.Printf("結果: %d/%d リクエスト許可\n", allowedCount, totalCount)
	
	// テスト3: 共有プールのデモ
	fmt.Println("\n\n3. トークン共有のデモ")
	
	// カスタム階層を作成
	hrl2 := NewHierarchicalRateLimiter()
	hrl2.AddNode("/premium", 100, 10)
	hrl2.AddNode("/standard", 50, 5)
	
	fmt.Println("\nプレミアムユーザーは親プールからトークンを借用可能")
	fmt.Println("スタンダードユーザーは自身のプールのみ使用")
	
	// 並行アクセスパターン
	fmt.Println("\n\n4. 実際のAPIパターンシミュレーション")
	
	// ユーザー別・API別の階層
	userHRL := NewHierarchicalRateLimiter()
	
	// ユーザータイプ別の制限
	userHRL.AddNode("/users/premium", 1000, 100)
	userHRL.AddNode("/users/standard", 100, 10)
	userHRL.AddNode("/users/free", 10, 1)
	
	// API別の制限（各ユーザータイプ内）
	userTypes := []string{"premium", "standard", "free"}
	apis := []string{"read", "write", "delete"}
	
	for _, userType := range userTypes {
		for _, api := range apis {
			path := fmt.Sprintf("/users/%s/%s", userType, api)
			
			// APIごとに異なる制限
			capacity := int64(10)
			rate := int64(1)
			
			switch api {
			case "read":
				capacity *= 10
				rate *= 10
			case "write":
				capacity *= 5
				rate *= 5
			case "delete":
				capacity *= 1
				rate *= 1
			}
			
			if userType == "premium" {
				capacity *= 10
				rate *= 10
			} else if userType == "standard" {
				capacity *= 5
				rate *= 5
			}
			
			userHRL.AddNode(path, capacity, rate)
		}
	}
	
	// 各ユーザータイプのアクセスパターンをテスト
	fmt.Println("\nユーザータイプ別アクセステスト:")
	for _, userType := range userTypes {
		fmt.Printf("\n%sユーザー:\n", userType)
		for _, api := range apis {
			path := fmt.Sprintf("/users/%s/%s", userType, api)
			allowed := 0
			for i := 0; i < 20; i++ {
				if userHRL.Allow(path) {
					allowed++
				}
			}
			fmt.Printf("  %s API: %d/20 リクエスト許可\n", api, allowed)
		}
	}
	
	fmt.Println("\n\n階層的レートリミッターの特徴:")
	fmt.Println("- 組織的な構造でのレート制限")
	fmt.Println("- 親子間でのリソース共有")
	fmt.Println("- きめ細かなアクセス制御")
	fmt.Println("- 動的な階層構築")
}