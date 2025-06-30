package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// TimeSeriesRateLimiter は時系列分析ベースのレートリミッター
type TimeSeriesRateLimiter struct {
	// 時系列データ
	series      *TimeSeries
	predictor   *Predictor
	anomalyDet  *AnomalyDetector
	
	// 制御パラメータ
	baseRate    float64
	currentRate float64
	
	mu sync.RWMutex
}

// TimeSeries は時系列データを管理
type TimeSeries struct {
	points    []DataPoint
	maxPoints int
	mu        sync.RWMutex
}

// DataPoint は時系列の1点
type DataPoint struct {
	Timestamp time.Time
	Value     float64
	Metadata  map[string]interface{}
}

// Predictor は時系列予測を行う
type Predictor struct {
	model       string // "arima", "exponential", "holt-winters"
	windowSize  int
	seasonality int
}

// AnomalyDetector は異常検知を行う
type AnomalyDetector struct {
	threshold   float64
	sensitivity float64
	baseline    *MovingStats
}

// MovingStats は移動統計を計算
type MovingStats struct {
	window []float64
	size   int
	sum    float64
	sumSq  float64
}

// NewTimeSeriesRateLimiter は新しい時系列ベースのレートリミッターを作成
func NewTimeSeriesRateLimiter(baseRate float64) *TimeSeriesRateLimiter {
	tsrl := &TimeSeriesRateLimiter{
		series: &TimeSeries{
			points:    make([]DataPoint, 0),
			maxPoints: 1000,
		},
		predictor: &Predictor{
			model:       "exponential",
			windowSize:  20,
			seasonality: 24, // 24時間の季節性
		},
		anomalyDet: &AnomalyDetector{
			threshold:   3.0, // 3σ
			sensitivity: 0.8,
			baseline: &MovingStats{
				window: make([]float64, 0, 100),
				size:   100,
			},
		},
		baseRate:    baseRate,
		currentRate: baseRate,
	}
	
	// バックグラウンドで分析を実行
	go tsrl.analyzeLoop()
	
	return tsrl
}

// Allow はリクエストを許可するかチェック
func (tsrl *TimeSeriesRateLimiter) Allow() bool {
	tsrl.mu.RLock()
	rate := tsrl.currentRate
	tsrl.mu.RUnlock()
	
	// 簡易的な実装
	threshold := rate / 1000.0
	return randFloat() < threshold
}

// Record はメトリクスを記録
func (tsrl *TimeSeriesRateLimiter) Record(value float64, metadata map[string]interface{}) {
	point := DataPoint{
		Timestamp: time.Now(),
		Value:     value,
		Metadata:  metadata,
	}
	
	tsrl.series.Add(point)
	tsrl.anomalyDet.baseline.Add(value)
}

// analyzeLoop は定期的に時系列分析を実行
func (tsrl *TimeSeriesRateLimiter) analyzeLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		tsrl.analyze()
	}
}

// analyze は時系列分析を実行してレートを調整
func (tsrl *TimeSeriesRateLimiter) analyze() {
	// 予測を実行
	prediction := tsrl.predictor.Predict(tsrl.series)
	
	// 異常検知
	isAnomaly := tsrl.anomalyDet.Detect(tsrl.series)
	
	// トレンド分析
	trend := tsrl.analyzeTrend()
	
	tsrl.mu.Lock()
	defer tsrl.mu.Unlock()
	
	// レートを調整
	if isAnomaly {
		// 異常時は保守的に
		tsrl.currentRate = tsrl.baseRate * 0.5
		fmt.Println("異常検知: レートを50%に削減")
	} else {
		// 予測とトレンドに基づいて調整
		adjustment := 1.0
		
		if prediction > tsrl.baseRate*1.5 {
			adjustment = 0.8 // 高負荷予測時は削減
		} else if prediction < tsrl.baseRate*0.5 {
			adjustment = 1.2 // 低負荷予測時は増加
		}
		
		// トレンドも考慮
		if trend > 0.1 {
			adjustment *= 0.9 // 上昇トレンドなら慎重に
		} else if trend < -0.1 {
			adjustment *= 1.1 // 下降トレンドなら緩和
		}
		
		tsrl.currentRate = tsrl.baseRate * adjustment
		
		fmt.Printf("時系列分析: 予測=%.2f, トレンド=%.2f, 新レート=%.2f\n",
			prediction, trend, tsrl.currentRate)
	}
}

// analyzeTrend はトレンドを分析
func (tsrl *TimeSeriesRateLimiter) analyzeTrend() float64 {
	points := tsrl.series.GetRecent(20)
	if len(points) < 2 {
		return 0
	}
	
	// 線形回帰でトレンドを計算
	var sumX, sumY, sumXY, sumX2 float64
	n := float64(len(points))
	
	for i, p := range points {
		x := float64(i)
		y := p.Value
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	
	// 傾き = (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	
	return slope
}

// TimeSeries メソッド
func (ts *TimeSeries) Add(point DataPoint) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	ts.points = append(ts.points, point)
	
	// 最大サイズを超えたら古いデータを削除
	if len(ts.points) > ts.maxPoints {
		ts.points = ts.points[len(ts.points)-ts.maxPoints:]
	}
}

func (ts *TimeSeries) GetRecent(n int) []DataPoint {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	
	if len(ts.points) <= n {
		result := make([]DataPoint, len(ts.points))
		copy(result, ts.points)
		return result
	}
	
	result := make([]DataPoint, n)
	copy(result, ts.points[len(ts.points)-n:])
	return result
}

// Predictor メソッド
func (p *Predictor) Predict(series *TimeSeries) float64 {
	points := series.GetRecent(p.windowSize)
	if len(points) == 0 {
		return 0
	}
	
	switch p.model {
	case "exponential":
		return p.exponentialSmoothing(points)
	case "holt-winters":
		return p.holtWinters(points)
	default:
		return p.simpleAverage(points)
	}
}

func (p *Predictor) exponentialSmoothing(points []DataPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	
	alpha := 0.3 // スムージング係数
	smoothed := points[0].Value
	
	for i := 1; i < len(points); i++ {
		smoothed = alpha*points[i].Value + (1-alpha)*smoothed
	}
	
	return smoothed
}

func (p *Predictor) holtWinters(points []DataPoint) float64 {
	if len(points) < p.seasonality {
		return p.exponentialSmoothing(points)
	}
	
	// 簡易的なHolt-Winters実装
	alpha, beta, gamma := 0.3, 0.1, 0.3
	
	// 初期値
	level := points[0].Value
	trend := (points[1].Value - points[0].Value)
	seasonal := make([]float64, p.seasonality)
	
	// 季節性の初期化
	for i := 0; i < p.seasonality && i < len(points); i++ {
		seasonal[i] = points[i].Value - level
	}
	
	// 予測
	for i := p.seasonality; i < len(points); i++ {
		prevLevel := level
		level = alpha*(points[i].Value-seasonal[i%p.seasonality]) + (1-alpha)*(level+trend)
		trend = beta*(level-prevLevel) + (1-beta)*trend
		seasonal[i%p.seasonality] = gamma*(points[i].Value-level) + (1-gamma)*seasonal[i%p.seasonality]
	}
	
	// 1期先予測
	return level + trend + seasonal[len(points)%p.seasonality]
}

func (p *Predictor) simpleAverage(points []DataPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	
	sum := 0.0
	for _, p := range points {
		sum += p.Value
	}
	
	return sum / float64(len(points))
}

// AnomalyDetector メソッド
func (ad *AnomalyDetector) Detect(series *TimeSeries) bool {
	points := series.GetRecent(10)
	if len(points) == 0 {
		return false
	}
	
	// 最新値を取得
	latest := points[len(points)-1].Value
	
	// 統計値を取得
	mean, stddev := ad.baseline.GetStats()
	
	// Z-スコアを計算
	if stddev == 0 {
		return false
	}
	
	zScore := math.Abs((latest - mean) / stddev)
	
	// 閾値を超えたら異常
	return zScore > ad.threshold
}

// MovingStats メソッド
func (ms *MovingStats) Add(value float64) {
	if len(ms.window) >= ms.size {
		// 最古の値を削除
		old := ms.window[0]
		ms.window = ms.window[1:]
		ms.sum -= old
		ms.sumSq -= old * old
	}
	
	ms.window = append(ms.window, value)
	ms.sum += value
	ms.sumSq += value * value
}

func (ms *MovingStats) GetStats() (mean, stddev float64) {
	n := float64(len(ms.window))
	if n == 0 {
		return 0, 0
	}
	
	mean = ms.sum / n
	variance := (ms.sumSq / n) - (mean * mean)
	
	if variance < 0 {
		variance = 0
	}
	
	stddev = math.Sqrt(variance)
	return
}

// FourierAnalysis はフーリエ変換で周期性を分析
type FourierAnalysis struct {
	sampleRate float64
	fftSize    int
}

// AnalyzePeriodicity は周期性を分析
func (fa *FourierAnalysis) AnalyzePeriodicity(points []DataPoint) []float64 {
	if len(points) < fa.fftSize {
		return nil
	}
	
	// 値を抽出
	values := make([]float64, fa.fftSize)
	for i := 0; i < fa.fftSize; i++ {
		values[i] = points[i].Value
	}
	
	// 簡易的なDFT（実際の実装ではFFTライブラリを使用）
	frequencies := make([]float64, fa.fftSize/2)
	
	for k := 0; k < fa.fftSize/2; k++ {
		var real, imag float64
		
		for n := 0; n < fa.fftSize; n++ {
			angle := -2 * math.Pi * float64(k) * float64(n) / float64(fa.fftSize)
			real += values[n] * math.Cos(angle)
			imag += values[n] * math.Sin(angle)
		}
		
		frequencies[k] = math.Sqrt(real*real + imag*imag)
	}
	
	return frequencies
}

// ユーティリティ関数
func randFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

// デモンストレーション
func main() {
	fmt.Println("時系列分析ベースレートリミッターデモ")
	fmt.Println("====================================")
	
	// 時系列レートリミッターを作成
	tsrl := NewTimeSeriesRateLimiter(100) // 基本レート: 100 req/sec
	
	// 1. 通常パターンのシミュレーション
	fmt.Println("\n1. 通常の負荷パターン")
	
	// 正弦波パターンで負荷を生成
	for i := 0; i < 60; i++ {
		// 時間によって変動する負荷
		hour := float64(i % 24)
		load := 50 + 30*math.Sin(2*math.Pi*hour/24) + randFloat()*10
		
		tsrl.Record(load, map[string]interface{}{
			"hour": hour,
			"type": "normal",
		})
		
		if i%10 == 9 {
			fmt.Printf("時刻 %02.0f:00 - 負荷: %.2f req/sec\n", hour, load)
		}
		
		time.Sleep(100 * time.Millisecond)
	}
	
	// 2. 異常パターンの注入
	fmt.Println("\n\n2. 異常パターンの検出")
	
	// スパイクを生成
	fmt.Println("スパイクを注入...")
	for i := 0; i < 5; i++ {
		spike := 200 + randFloat()*50
		tsrl.Record(spike, map[string]interface{}{
			"type": "spike",
		})
		fmt.Printf("スパイク %d: %.2f req/sec\n", i+1, spike)
		time.Sleep(200 * time.Millisecond)
	}
	
	// 3. 季節性分析
	fmt.Println("\n\n3. 季節性パターンの分析")
	
	fa := &FourierAnalysis{
		sampleRate: 1.0, // 1 sample/hour
		fftSize:    64,
	}
	
	// 季節性データを生成
	seasonalData := make([]DataPoint, 0)
	for i := 0; i < 168; i++ { // 1週間分
		hour := i % 24
		day := i / 24
		
		// 日中は高負荷、夜間は低負荷
		dayPattern := 50 + 30*math.Cos(2*math.Pi*float64(hour-12)/24)
		
		// 週末は全体的に低負荷
		weekendFactor := 1.0
		if day >= 5 {
			weekendFactor = 0.7
		}
		
		value := dayPattern * weekendFactor + randFloat()*10
		
		seasonalData = append(seasonalData, DataPoint{
			Timestamp: time.Now().Add(time.Duration(i) * time.Hour),
			Value:     value,
		})
	}
	
	// 周期性を分析
	if len(seasonalData) >= fa.fftSize {
		frequencies := fa.AnalyzePeriodicity(seasonalData)
		
		fmt.Println("主要な周期:")
		// 最も強い周期を検出
		for i := 1; i < len(frequencies) && i < 5; i++ {
			period := float64(fa.fftSize) / float64(i)
			strength := frequencies[i] / frequencies[0]
			if strength > 0.1 {
				fmt.Printf("  周期: %.1f 時間, 強度: %.2f\n", period, strength)
			}
		}
	}
	
	// 4. 予測精度の評価
	fmt.Println("\n\n4. 予測精度の評価")
	
	predictor := &Predictor{
		model:      "holt-winters",
		windowSize: 24,
		seasonality: 24,
	}
	
	// テストデータで予測精度を評価
	testSeries := &TimeSeries{
		points:    make([]DataPoint, 0),
		maxPoints: 100,
	}
	
	errors := make([]float64, 0)
	
	for i := 0; i < 48; i++ {
		actual := 100 + 20*math.Sin(2*math.Pi*float64(i)/24)
		
		if i > 24 {
			// 予測を実行
			predicted := predictor.Predict(testSeries)
			error := math.Abs(predicted - actual)
			errors = append(errors, error)
			
			if i%6 == 0 {
				fmt.Printf("時刻 %02d:00 - 実際: %.2f, 予測: %.2f, 誤差: %.2f\n",
					i%24, actual, predicted, error)
			}
		}
		
		testSeries.Add(DataPoint{
			Timestamp: time.Now(),
			Value:     actual,
		})
	}
	
	// 平均誤差を計算
	if len(errors) > 0 {
		avgError := 0.0
		for _, e := range errors {
			avgError += e
		}
		avgError /= float64(len(errors))
		fmt.Printf("\n平均予測誤差: %.2f req/sec\n", avgError)
	}
	
	// 5. 適応的制御のデモ
	fmt.Println("\n\n5. 時系列に基づく適応的制御")
	
	// 実際の使用例
	allowed := 0
	total := 1000
	
	for i := 0; i < total; i++ {
		if tsrl.Allow() {
			allowed++
		}
		
		// 定期的に負荷を記録
		if i%100 == 0 {
			currentLoad := float64(allowed) / (float64(i+1) / float64(total)) * 100
			tsrl.Record(currentLoad, nil)
		}
	}
	
	fmt.Printf("\n最終結果: %d/%d リクエスト許可 (%.2f%%)\n",
		allowed, total, float64(allowed)/float64(total)*100)
	
	fmt.Println("\n\n時系列分析ベースの特徴:")
	fmt.Println("- 過去のパターンから未来を予測")
	fmt.Println("- 季節性や周期性を自動検出")
	fmt.Println("- 異常パターンの早期発見")
	fmt.Println("- 長期的な最適化")
}