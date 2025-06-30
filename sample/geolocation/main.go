package main

import (
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"time"
)

// GeoRateLimiter は地理的位置に基づくレート制限
type GeoRateLimiter struct {
	regions     map[string]*RegionConfig
	ipDatabase  *IPGeoDB
	defaultRate int
	mu          sync.RWMutex
}

// RegionConfig は地域ごとの設定
type RegionConfig struct {
	Name          string
	RateLimit     int
	BurstLimit    int
	TimeWindow    time.Duration
	Restrictions  []TimeRestriction
	limiter       RateLimiter
}

// TimeRestriction は時間帯制限
type TimeRestriction struct {
	StartHour int
	EndHour   int
	Multiplier float64
}

// IPGeoDB はIPアドレスの地理情報データベース（シミュレーション）
type IPGeoDB struct {
	ranges map[string]IPRange
	mu     sync.RWMutex
}

// IPRange はIPアドレス範囲と地域情報
type IPRange struct {
	StartIP   net.IP
	EndIP     net.IP
	Country   string
	Region    string
	City      string
	Latitude  float64
	Longitude float64
}

// RateLimiter インターフェース
type RateLimiter interface {
	Allow(identifier string) bool
	GetStats() map[string]interface{}
}

// NewGeoRateLimiter は新しい地理的レートリミッターを作成
func NewGeoRateLimiter(defaultRate int) *GeoRateLimiter {
	grl := &GeoRateLimiter{
		regions:     make(map[string]*RegionConfig),
		ipDatabase:  NewIPGeoDB(),
		defaultRate: defaultRate,
	}
	
	// 地域設定を初期化
	grl.initializeRegions()
	
	return grl
}

// initializeRegions は地域設定を初期化
func (grl *GeoRateLimiter) initializeRegions() {
	// アジア太平洋
	grl.AddRegion("asia-pacific", &RegionConfig{
		Name:       "Asia Pacific",
		RateLimit:  1000,
		BurstLimit: 2000,
		TimeWindow: time.Minute,
		Restrictions: []TimeRestriction{
			{StartHour: 9, EndHour: 18, Multiplier: 1.5}, // 営業時間は1.5倍
		},
	})
	
	// 北米
	grl.AddRegion("north-america", &RegionConfig{
		Name:       "North America",
		RateLimit:  1500,
		BurstLimit: 3000,
		TimeWindow: time.Minute,
		Restrictions: []TimeRestriction{
			{StartHour: 0, EndHour: 6, Multiplier: 0.5}, // 深夜は半分
		},
	})
	
	// ヨーロッパ
	grl.AddRegion("europe", &RegionConfig{
		Name:       "Europe",
		RateLimit:  1200,
		BurstLimit: 2400,
		TimeWindow: time.Minute,
		Restrictions: []TimeRestriction{
			{StartHour: 20, EndHour: 24, Multiplier: 0.8}, // 夜間は0.8倍
		},
	})
}

// AddRegion は新しい地域設定を追加
func (grl *GeoRateLimiter) AddRegion(id string, config *RegionConfig) {
	grl.mu.Lock()
	defer grl.mu.Unlock()
	
	// 地域ごとにレートリミッターを作成
	config.limiter = NewSimpleTokenBucket(config.RateLimit, config.BurstLimit)
	grl.regions[id] = config
}

// Allow はIPアドレスに基づいてリクエストを許可
func (grl *GeoRateLimiter) Allow(ipAddress string) bool {
	// IPアドレスから地域を特定
	location := grl.ipDatabase.GetLocation(ipAddress)
	if location == nil {
		// 不明な場合はデフォルトレートを使用
		return true // 簡易実装
	}
	
	// 地域設定を取得
	grl.mu.RLock()
	config, exists := grl.regions[location.Region]
	grl.mu.RUnlock()
	
	if !exists {
		return true // デフォルト許可
	}
	
	// 時間帯制限をチェック
	multiplier := grl.getTimeMultiplier(config)
	
	// レート制限をチェック
	return config.limiter.Allow(ipAddress)
}

// getTimeMultiplier は時間帯に基づく倍率を取得
func (grl *GeoRateLimiter) getTimeMultiplier(config *RegionConfig) float64 {
	hour := time.Now().Hour()
	
	for _, restriction := range config.Restrictions {
		if hour >= restriction.StartHour && hour < restriction.EndHour {
			return restriction.Multiplier
		}
	}
	
	return 1.0
}

// ProximityRateLimiter は近接性に基づくレート制限
type ProximityRateLimiter struct {
	servers    []ServerLocation
	maxLatency time.Duration
	cache      *DistanceCache
}

// ServerLocation はサーバーの地理的位置
type ServerLocation struct {
	ID        string
	Latitude  float64
	Longitude float64
	Capacity  int
}

// DistanceCache は距離計算のキャッシュ
type DistanceCache struct {
	distances map[string]float64
	mu        sync.RWMutex
}

// NewProximityRateLimiter は近接性ベースのレートリミッターを作成
func NewProximityRateLimiter(maxLatency time.Duration) *ProximityRateLimiter {
	return &ProximityRateLimiter{
		servers:    make([]ServerLocation, 0),
		maxLatency: maxLatency,
		cache: &DistanceCache{
			distances: make(map[string]float64),
		},
	}
}

// AddServer はサーバーを追加
func (prl *ProximityRateLimiter) AddServer(server ServerLocation) {
	prl.servers = append(prl.servers, server)
}

// GetNearestServer は最も近いサーバーを取得
func (prl *ProximityRateLimiter) GetNearestServer(lat, lon float64) *ServerLocation {
	if len(prl.servers) == 0 {
		return nil
	}
	
	var nearest *ServerLocation
	minDistance := math.MaxFloat64
	
	for i := range prl.servers {
		distance := calculateDistance(lat, lon, prl.servers[i].Latitude, prl.servers[i].Longitude)
		if distance < minDistance {
			minDistance = distance
			nearest = &prl.servers[i]
		}
	}
	
	return nearest
}

// calculateDistance はHaversine公式で距離を計算
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371 // km
	
	dLat := toRadians(lat2 - lat1)
	dLon := toRadians(lon2 - lon1)
	
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRadians(lat1))*math.Cos(toRadians(lat2))*
		math.Sin(dLon/2)*math.Sin(dLon/2)
	
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	
	return earthRadius * c
}

func toRadians(degrees float64) float64 {
	return degrees * math.Pi / 180
}

// CountryBasedLimiter は国別のレート制限
type CountryBasedLimiter struct {
	countryLimits map[string]*CountryLimit
	blacklist     map[string]bool
	whitelist     map[string]bool
	mu            sync.RWMutex
}

// CountryLimit は国別の制限設定
type CountryLimit struct {
	Country      string
	DailyLimit   int64
	HourlyLimit  int64
	MinuteLimit  int64
	CurrentUsage map[string]int64
	mu           sync.Mutex
}

// NewCountryBasedLimiter は国別レートリミッターを作成
func NewCountryBasedLimiter() *CountryBasedLimiter {
	return &CountryBasedLimiter{
		countryLimits: make(map[string]*CountryLimit),
		blacklist:     make(map[string]bool),
		whitelist:     make(map[string]bool),
	}
}

// SetCountryLimit は国別の制限を設定
func (cbl *CountryBasedLimiter) SetCountryLimit(country string, daily, hourly, minute int64) {
	cbl.mu.Lock()
	defer cbl.mu.Unlock()
	
	cbl.countryLimits[country] = &CountryLimit{
		Country:     country,
		DailyLimit:  daily,
		HourlyLimit: hourly,
		MinuteLimit: minute,
		CurrentUsage: map[string]int64{
			"daily":  0,
			"hourly": 0,
			"minute": 0,
		},
	}
}

// IPGeoDB の実装
func NewIPGeoDB() *IPGeoDB {
	db := &IPGeoDB{
		ranges: make(map[string]IPRange),
	}
	
	// サンプルデータを追加
	db.AddRange("192.168.0.0/16", IPRange{
		Country:   "JP",
		Region:    "asia-pacific",
		City:      "Tokyo",
		Latitude:  35.6762,
		Longitude: 139.6503,
	})
	
	db.AddRange("10.0.0.0/8", IPRange{
		Country:   "US",
		Region:    "north-america",
		City:      "New York",
		Latitude:  40.7128,
		Longitude: -74.0060,
	})
	
	db.AddRange("172.16.0.0/12", IPRange{
		Country:   "DE",
		Region:    "europe",
		City:      "Berlin",
		Latitude:  52.5200,
		Longitude: 13.4050,
	})
	
	return db
}

// AddRange はIP範囲を追加
func (db *IPGeoDB) AddRange(cidr string, info IPRange) {
	db.mu.Lock()
	defer db.mu.Unlock()
	
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return
	}
	
	info.StartIP = ipNet.IP
	info.EndIP = lastIP(ipNet)
	
	db.ranges[cidr] = info
}

// GetLocation はIPアドレスの位置情報を取得
func (db *IPGeoDB) GetLocation(ipStr string) *IPRange {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil
	}
	
	for _, r := range db.ranges {
		if ipInRange(ip, r.StartIP, r.EndIP) {
			return &r
		}
	}
	
	return nil
}

// ユーティリティ関数
func lastIP(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP))
	copy(ip, n.IP)
	
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i] |= ^n.Mask[i]
	}
	
	return ip
}

func ipInRange(ip, start, end net.IP) bool {
	return bytes2int(ip) >= bytes2int(start) && bytes2int(ip) <= bytes2int(end)
}

func bytes2int(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// SimpleTokenBucket は簡易トークンバケット実装
type SimpleTokenBucket struct {
	tokens   int64
	capacity int64
	mu       sync.Mutex
}

func NewSimpleTokenBucket(rate, burst int) *SimpleTokenBucket {
	return &SimpleTokenBucket{
		tokens:   int64(burst),
		capacity: int64(burst),
	}
}

func (tb *SimpleTokenBucket) Allow(identifier string) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

func (tb *SimpleTokenBucket) GetStats() map[string]interface{} {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	return map[string]interface{}{
		"tokens":   tb.tokens,
		"capacity": tb.capacity,
	}
}

// デモンストレーション
func main() {
	fmt.Println("地理的レート制限デモ")
	fmt.Println("===================")
	
	// 1. 基本的な地理的レート制限
	fmt.Println("\n1. IPアドレスベースの地域制限")
	geoLimiter := NewGeoRateLimiter(100)
	
	testIPs := []struct {
		ip      string
		desc    string
	}{
		{"192.168.1.1", "日本（アジア太平洋）"},
		{"10.0.0.1", "米国（北米）"},
		{"172.16.0.1", "ドイツ（ヨーロッパ）"},
		{"8.8.8.8", "不明な地域"},
	}
	
	for _, test := range testIPs {
		location := geoLimiter.ipDatabase.GetLocation(test.ip)
		allowed := geoLimiter.Allow(test.ip)
		
		if location != nil {
			fmt.Printf("%s (%s): %s, %s - %v\n",
				test.ip, test.desc, location.Country, location.City,
				map[bool]string{true: "許可", false: "拒否"}[allowed])
		} else {
			fmt.Printf("%s (%s): 位置不明 - %v\n",
				test.ip, test.desc,
				map[bool]string{true: "許可", false: "拒否"}[allowed])
		}
	}
	
	// 2. 近接性ベースの制限
	fmt.Println("\n\n2. サーバー近接性によるルーティング")
	proximityLimiter := NewProximityRateLimiter(50 * time.Millisecond)
	
	// サーバーを配置
	servers := []ServerLocation{
		{ID: "tokyo-1", Latitude: 35.6762, Longitude: 139.6503, Capacity: 1000},
		{ID: "singapore-1", Latitude: 1.3521, Longitude: 103.8198, Capacity: 800},
		{ID: "sydney-1", Latitude: -33.8688, Longitude: 151.2093, Capacity: 600},
		{ID: "london-1", Latitude: 51.5074, Longitude: -0.1278, Capacity: 900},
		{ID: "newyork-1", Latitude: 40.7128, Longitude: -74.0060, Capacity: 1200},
	}
	
	for _, server := range servers {
		proximityLimiter.AddServer(server)
		fmt.Printf("サーバー %s: 緯度=%.4f, 経度=%.4f, 容量=%d\n",
			server.ID, server.Latitude, server.Longitude, server.Capacity)
	}
	
	// クライアントの位置から最適なサーバーを選択
	clients := []struct {
		city string
		lat  float64
		lon  float64
	}{
		{"東京", 35.6762, 139.6503},
		{"シドニー", -33.8688, 151.2093},
		{"ロンドン", 51.5074, -0.1278},
		{"サンパウロ", -23.5505, -46.6333},
	}
	
	fmt.Println("\n最適サーバーの選択:")
	for _, client := range clients {
		nearest := proximityLimiter.GetNearestServer(client.lat, client.lon)
		if nearest != nil {
			distance := calculateDistance(client.lat, client.lon, nearest.Latitude, nearest.Longitude)
			fmt.Printf("%s → %s (距離: %.0f km)\n", client.city, nearest.ID, distance)
		}
	}
	
	// 3. 国別制限
	fmt.Println("\n\n3. 国別アクセス制限")
	countryLimiter := NewCountryBasedLimiter()
	
	// 国別制限を設定
	countryLimiter.SetCountryLimit("JP", 100000, 10000, 1000)
	countryLimiter.SetCountryLimit("US", 200000, 20000, 2000)
	countryLimiter.SetCountryLimit("CN", 50000, 5000, 500)
	
	// ブラックリスト/ホワイトリスト
	countryLimiter.blacklist["XX"] = true // 架空の国をブロック
	countryLimiter.whitelist["JP"] = true
	countryLimiter.whitelist["US"] = true
	
	fmt.Println("\n国別設定:")
	for country, limit := range countryLimiter.countryLimits {
		fmt.Printf("%s: 日次=%d, 時間=%d, 分=%d\n",
			country, limit.DailyLimit, limit.HourlyLimit, limit.MinuteLimit)
	}
	
	// 4. 動的ジオフェンシング
	fmt.Println("\n\n4. 動的ジオフェンシング")
	
	type GeoFence struct {
		Name      string
		CenterLat float64
		CenterLon float64
		RadiusKM  float64
		RateLimit int
	}
	
	fences := []GeoFence{
		{"東京都心", 35.6762, 139.6503, 50, 2000},
		{"大阪", 34.6937, 135.5023, 30, 1500},
		{"ニューヨーク", 40.7128, -74.0060, 40, 1800},
	}
	
	// テスト位置
	testLocations := []struct {
		name string
		lat  float64
		lon  float64
	}{
		{"渋谷", 35.6580, 139.7016},
		{"横浜", 35.4437, 139.6380},
		{"大阪城", 34.6873, 135.5262},
		{"マンハッタン", 40.7831, -73.9712},
	}
	
	fmt.Println("\nジオフェンス判定:")
	for _, loc := range testLocations {
		fmt.Printf("\n%s (%.4f, %.4f):\n", loc.name, loc.lat, loc.lon)
		
		for _, fence := range fences {
			distance := calculateDistance(loc.lat, loc.lon, fence.CenterLat, fence.CenterLon)
			inside := distance <= fence.RadiusKM
			
			if inside {
				fmt.Printf("  ✓ %s内 (距離: %.1f km) - レート: %d/分\n",
					fence.Name, distance, fence.RateLimit)
			} else {
				fmt.Printf("  ✗ %s外 (距離: %.1f km)\n",
					fence.Name, distance)
			}
		}
	}
	
	// 5. レイテンシベース制限
	fmt.Println("\n\n5. レイテンシベースの動的調整")
	
	type LatencyBasedLimiter struct {
		targetLatency time.Duration
		measurements  map[string][]time.Duration
		mu            sync.Mutex
	}
	
	lbl := &LatencyBasedLimiter{
		targetLatency: 100 * time.Millisecond,
		measurements:  make(map[string][]time.Duration),
	}
	
	// レイテンシを記録
	regions := []string{"asia", "europe", "americas"}
	for _, region := range regions {
		lbl.measurements[region] = []time.Duration{
			80 * time.Millisecond,
			120 * time.Millisecond,
			90 * time.Millisecond,
			150 * time.Millisecond,
			70 * time.Millisecond,
		}
	}
	
	fmt.Println("地域別レイテンシと推奨レート:")
	for region, latencies := range lbl.measurements {
		var sum time.Duration
		for _, l := range latencies {
			sum += l
		}
		avg := sum / time.Duration(len(latencies))
		
		// レイテンシに基づいてレートを調整
		var recommendedRate int
		if avg < lbl.targetLatency {
			recommendedRate = 1000
		} else if avg < lbl.targetLatency*2 {
			recommendedRate = 500
		} else {
			recommendedRate = 200
		}
		
		fmt.Printf("%s: 平均レイテンシ=%v, 推奨レート=%d req/min\n",
			strings.Title(region), avg, recommendedRate)
	}
	
	fmt.Println("\n\n地理的レート制限の特徴:")
	fmt.Println("- 地域特性に応じた柔軟な制御")
	fmt.Println("- レイテンシ最適化")
	fmt.Println("- 規制やコンプライアンス対応")
	fmt.Println("- DDoS攻撃の地理的分析")
}