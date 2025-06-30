package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// RedisTokenBucket はRedisベースの分散トークンバケット（シミュレーション）
type RedisTokenBucket struct {
	key      string
	capacity int64
	rate     int64
	redis    *RedisSimulator
}

// RedisSimulator はRedisの動作をシミュレート
type RedisSimulator struct {
	data       map[string]interface{}
	expiry     map[string]time.Time
	mu         sync.RWMutex
	scripts    map[string]*LuaScript
}

// LuaScript はLuaスクリプトを表現
type LuaScript struct {
	hash string
	code string
}

// TokenBucketData はトークンバケットのデータ
type TokenBucketData struct {
	Tokens       int64     `json:"tokens"`
	LastRefill   time.Time `json:"last_refill"`
	Capacity     int64     `json:"capacity"`
	RefillRate   int64     `json:"refill_rate"`
}

// NewRedisTokenBucket は新しいRedisベースのトークンバケットを作成
func NewRedisTokenBucket(key string, capacity, rate int64, redis *RedisSimulator) *RedisTokenBucket {
	return &RedisTokenBucket{
		key:      key,
		capacity: capacity,
		rate:     rate,
		redis:    redis,
	}
}

// Allow はトークンを消費してリクエストを許可
func (rtb *RedisTokenBucket) Allow() bool {
	// Luaスクリプトで原子的に実行
	script := rtb.getTokenBucketScript()
	
	result, err := rtb.redis.EvalScript(script, []string{rtb.key}, 
		rtb.capacity, rtb.rate, time.Now().Unix())
	
	if err != nil {
		return false
	}
	
	allowed, ok := result.(bool)
	return ok && allowed
}

// getTokenBucketScript はトークンバケットのLuaスクリプトを取得
func (rtb *RedisTokenBucket) getTokenBucketScript() string {
	return `
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local rate = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		
		local bucket = redis.call('GET', key)
		local data
		
		if bucket then
			data = cjson.decode(bucket)
		else
			data = {
				tokens = capacity,
				last_refill = now,
				capacity = capacity,
				refill_rate = rate
			}
		end
		
		-- トークンを補充
		local elapsed = now - data.last_refill
		local tokens_to_add = math.floor(elapsed * rate)
		data.tokens = math.min(data.tokens + tokens_to_add, capacity)
		data.last_refill = now
		
		-- トークンを消費
		if data.tokens >= 1 then
			data.tokens = data.tokens - 1
			redis.call('SET', key, cjson.encode(data))
			redis.call('EXPIRE', key, 3600)
			return true
		else
			redis.call('SET', key, cjson.encode(data))
			redis.call('EXPIRE', key, 3600)
			return false
		end
	`
}

// DistributedRateLimiter は分散レート制限の調整役
type DistributedRateLimiter struct {
	nodes       []string
	localLimit  int64
	globalLimit int64
	syncPeriod  time.Duration
	redis       *RedisSimulator
}

// NewDistributedRateLimiter は分散レートリミッターを作成
func NewDistributedRateLimiter(nodes []string, globalLimit int64, redis *RedisSimulator) *DistributedRateLimiter {
	localLimit := globalLimit / int64(len(nodes))
	
	return &DistributedRateLimiter{
		nodes:       nodes,
		localLimit:  localLimit,
		globalLimit: globalLimit,
		syncPeriod:  5 * time.Second,
		redis:       redis,
	}
}

// RequestQuota はノードがクォータを要求
func (drl *DistributedRateLimiter) RequestQuota(nodeID string) int64 {
	// グローバルな使用状況を確認
	usage := drl.getGlobalUsage()
	available := drl.globalLimit - usage
	
	// 公平に分配
	nodeCount := int64(len(drl.nodes))
	fairShare := available / nodeCount
	
	// 使用率に基づいて調整
	nodeUsage := drl.getNodeUsage(nodeID)
	if nodeUsage < drl.localLimit/2 {
		// 使用率が低い場合は少なめに
		return fairShare / 2
	} else if nodeUsage > drl.localLimit*3/4 {
		// 使用率が高い場合は多めに
		return fairShare * 3 / 2
	}
	
	return fairShare
}

// getGlobalUsage はグローバルな使用量を取得
func (drl *DistributedRateLimiter) getGlobalUsage() int64 {
	total := int64(0)
	for _, node := range drl.nodes {
		usage, _ := drl.redis.Get(fmt.Sprintf("usage:%s", node))
		if u, ok := usage.(int64); ok {
			total += u
		}
	}
	return total
}

// getNodeUsage はノードの使用量を取得
func (drl *DistributedRateLimiter) getNodeUsage(nodeID string) int64 {
	usage, _ := drl.redis.Get(fmt.Sprintf("usage:%s", nodeID))
	if u, ok := usage.(int64); ok {
		return u
	}
	return 0
}

// ConsistentHashRateLimiter はコンシステントハッシュを使用
type ConsistentHashRateLimiter struct {
	ring    *HashRing
	buckets map[string]*RedisTokenBucket
	redis   *RedisSimulator
}

// HashRing はコンシステントハッシュリング
type HashRing struct {
	nodes        []string
	virtualNodes int
	ring         map[uint32]string
	sortedKeys   []uint32
}

// NewConsistentHashRateLimiter は新しいコンシステントハッシュリミッターを作成
func NewConsistentHashRateLimiter(nodes []string, capacity, rate int64, redis *RedisSimulator) *ConsistentHashRateLimiter {
	ring := &HashRing{
		nodes:        nodes,
		virtualNodes: 150,
		ring:         make(map[uint32]string),
	}
	
	// リングを構築
	for _, node := range nodes {
		for i := 0; i < ring.virtualNodes; i++ {
			hash := hashString(fmt.Sprintf("%s:%d", node, i))
			ring.ring[hash] = node
		}
	}
	
	// ソート済みキーを作成
	ring.sortedKeys = make([]uint32, 0, len(ring.ring))
	for k := range ring.ring {
		ring.sortedKeys = append(ring.sortedKeys, k)
	}
	
	// バケットを作成
	buckets := make(map[string]*RedisTokenBucket)
	for _, node := range nodes {
		buckets[node] = NewRedisTokenBucket(
			fmt.Sprintf("bucket:%s", node),
			capacity,
			rate,
			redis,
		)
	}
	
	return &ConsistentHashRateLimiter{
		ring:    ring,
		buckets: buckets,
		redis:   redis,
	}
}

// Allow はユーザーのリクエストを許可
func (chrl *ConsistentHashRateLimiter) Allow(userID string) bool {
	// ユーザーIDからノードを決定
	node := chrl.ring.GetNode(userID)
	
	// 対応するバケットでチェック
	if bucket, exists := chrl.buckets[node]; exists {
		return bucket.Allow()
	}
	
	return false
}

// GetNode はキーに対応するノードを取得
func (hr *HashRing) GetNode(key string) string {
	if len(hr.sortedKeys) == 0 {
		return ""
	}
	
	hash := hashString(key)
	
	// 二分探索で最も近いノードを見つける
	idx := 0
	for i := range hr.sortedKeys {
		if hr.sortedKeys[i] >= hash {
			idx = i
			break
		}
	}
	
	return hr.ring[hr.sortedKeys[idx]]
}

// hashString は文字列をハッシュ化
func hashString(s string) uint32 {
	h := uint32(0)
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}

// RedisSimulator のメソッド
func NewRedisSimulator() *RedisSimulator {
	return &RedisSimulator{
		data:    make(map[string]interface{}),
		expiry:  make(map[string]time.Time),
		scripts: make(map[string]*LuaScript),
	}
}

func (r *RedisSimulator) Get(key string) (interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// 有効期限チェック
	if exp, exists := r.expiry[key]; exists && time.Now().After(exp) {
		delete(r.data, key)
		delete(r.expiry, key)
		return nil, fmt.Errorf("key expired")
	}
	
	val, exists := r.data[key]
	if !exists {
		return nil, fmt.Errorf("key not found")
	}
	
	return val, nil
}

func (r *RedisSimulator) Set(key string, value interface{}, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.data[key] = value
	if ttl > 0 {
		r.expiry[key] = time.Now().Add(ttl)
	}
	
	return nil
}

func (r *RedisSimulator) EvalScript(script string, keys []string, args ...interface{}) (interface{}, error) {
	// 簡易的なLuaスクリプト実行シミュレーション
	// 実際の実装では適切なLuaインタープリタを使用
	
	if len(keys) > 0 && len(args) >= 3 {
		key := keys[0]
		capacity := args[0].(int64)
		rate := args[1].(int64)
		now := args[2].(int64)
		
		// トークンバケットのロジックをシミュレート
		dataRaw, _ := r.Get(key)
		
		var data TokenBucketData
		if dataRaw != nil {
			if jsonData, ok := dataRaw.(string); ok {
				json.Unmarshal([]byte(jsonData), &data)
			}
		} else {
			data = TokenBucketData{
				Tokens:     capacity,
				LastRefill: time.Unix(now, 0),
				Capacity:   capacity,
				RefillRate: rate,
			}
		}
		
		// トークンを補充
		elapsed := now - data.LastRefill.Unix()
		tokensToAdd := elapsed * rate
		data.Tokens = min(data.Tokens+tokensToAdd, capacity)
		data.LastRefill = time.Unix(now, 0)
		
		// トークンを消費
		allowed := false
		if data.Tokens >= 1 {
			data.Tokens--
			allowed = true
		}
		
		// データを保存
		jsonData, _ := json.Marshal(data)
		r.Set(key, string(jsonData), time.Hour)
		
		return allowed, nil
	}
	
	return nil, fmt.Errorf("invalid script execution")
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// デモンストレーション
func main() {
	fmt.Println("Redis分散トークンバケットデモ")
	fmt.Println("=============================")
	
	redis := NewRedisSimulator()
	
	// 1. 基本的なRedisトークンバケット
	fmt.Println("\n1. 基本的なRedisトークンバケット")
	rtb := NewRedisTokenBucket("user:alice", 10, 2, redis)
	
	fmt.Println("20リクエストを送信 (容量10, レート2/秒):")
	allowed := 0
	for i := 0; i < 20; i++ {
		if rtb.Allow() {
			allowed++
			fmt.Printf("リクエスト %d: 許可\n", i+1)
		} else {
			fmt.Printf("リクエスト %d: 拒否\n", i+1)
		}
		
		if i == 9 {
			fmt.Println("  (1秒待機...)")
			time.Sleep(1 * time.Second)
		}
	}
	fmt.Printf("結果: %d/20 許可\n", allowed)
	
	// 2. 分散レート制限
	fmt.Println("\n\n2. 分散レート制限 (3ノード)")
	nodes := []string{"node1", "node2", "node3"}
	drl := NewDistributedRateLimiter(nodes, 100, redis)
	
	// 各ノードがクォータを要求
	fmt.Println("\n各ノードのクォータ要求:")
	for _, node := range nodes {
		// 使用量をシミュレート
		usage := int64(20 + len(node)*5)
		redis.Set(fmt.Sprintf("usage:%s", node), usage, time.Hour)
		
		quota := drl.RequestQuota(node)
		fmt.Printf("%s: 使用量=%d, 割当=%d\n", node, usage, quota)
	}
	
	// 3. コンシステントハッシュ
	fmt.Println("\n\n3. コンシステントハッシュによる分散")
	chrl := NewConsistentHashRateLimiter(nodes, 30, 5, redis)
	
	// ユーザーを各ノードに分散
	users := []string{"alice", "bob", "charlie", "david", "eve", "frank"}
	userNodes := make(map[string]string)
	
	fmt.Println("\nユーザーのノード割当:")
	for _, user := range users {
		node := chrl.ring.GetNode(user)
		userNodes[user] = node
		fmt.Printf("%s → %s\n", user, node)
	}
	
	// 各ユーザーがリクエスト
	fmt.Println("\n各ユーザーのリクエスト:")
	for _, user := range users {
		allowed := 0
		for i := 0; i < 5; i++ {
			if chrl.Allow(user) {
				allowed++
			}
		}
		fmt.Printf("%s (%s): %d/5 許可\n", user, userNodes[user], allowed)
	}
	
	// 4. フェイルオーバーシミュレーション
	fmt.Println("\n\n4. ノード障害とフェイルオーバー")
	
	// node2を削除
	fmt.Println("\nnode2が障害...")
	remainingNodes := []string{"node1", "node3"}
	chrl2 := NewConsistentHashRateLimiter(remainingNodes, 45, 7, redis)
	
	fmt.Println("\n再割当後:")
	for _, user := range users {
		oldNode := userNodes[user]
		newNode := chrl2.ring.GetNode(user)
		fmt.Printf("%s: %s → %s\n", user, oldNode, newNode)
	}
	
	// 5. グローバルレート制限
	fmt.Println("\n\n5. グローバルレート制限の調整")
	
	// 中央調整器
	type GlobalCoordinator struct {
		redis       *RedisSimulator
		globalLimit int64
		nodes       []string
	}
	
	gc := &GlobalCoordinator{
		redis:       redis,
		globalLimit: 1000,
		nodes:       nodes,
	}
	
	// 定期的な同期をシミュレート
	fmt.Println("\nグローバル同期:")
	for i := 0; i < 3; i++ {
		fmt.Printf("\n同期ラウンド %d:\n", i+1)
		
		totalUsage := int64(0)
		for _, node := range gc.nodes {
			usage := int64(100 + i*50 + len(node)*10)
			redis.Set(fmt.Sprintf("global:usage:%s", node), usage, time.Hour)
			totalUsage += usage
			fmt.Printf("  %s: %d req/sec\n", node, usage)
		}
		
		fmt.Printf("  合計: %d/%d req/sec\n", totalUsage, gc.globalLimit)
		
		if totalUsage > gc.globalLimit {
			fmt.Println("  警告: グローバル制限を超過！")
			// 各ノードに削減を指示
			reduction := float64(gc.globalLimit) / float64(totalUsage)
			for _, node := range gc.nodes {
				fmt.Printf("    %s: %.0f%%に削減\n", node, reduction*100)
			}
		}
		
		time.Sleep(100 * time.Millisecond)
	}
	
	fmt.Println("\n\nRedis分散トークンバケットの特徴:")
	fmt.Println("- 原子的操作による一貫性")
	fmt.Println("- 水平スケーリング対応")
	fmt.Println("- ノード障害への耐性")
	fmt.Println("- グローバル制限の実現")
}