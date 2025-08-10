# rRateLimit Client Library

高性能で柔軟なGoレート制限ライブラリ。Token Bucket、Fixed Window、Sliding Windowアルゴリズムを実装し、HTTPミドルウェアサポートも提供します。

## 特徴

- 🚀 **複数のアルゴリズム**: Token Bucket、Fixed Window、Sliding Window
- 🔒 **スレッドセーフ**: 並行処理環境での安全な動作
- 🌐 **HTTPミドルウェア**: 簡単なWeb API統合
- ⚡ **高性能**: 最小限のオーバーヘッド
- 🎯 **柔軟な設定**: カスタマイズ可能なオプション
- 📊 **統計情報**: リアルタイムモニタリング

## インストール

```bash
go get github.com/rRateLimit/client/ratelimit
```

## クイックスタート

### 基本的な使用法

```go
package main

import (
    "fmt"
    "time"
    "github.com/rRateLimit/client/ratelimit"
)

func main() {
    // Token Bucketレートリミッターを作成
    limiter := ratelimit.NewTokenBucket(
        ratelimit.WithRate(100),           // 100リクエスト
        ratelimit.WithPeriod(time.Second),  // 1秒あたり
        ratelimit.WithBurst(10),            // バースト10まで許可
    )

    // リクエストを許可するかチェック
    if limiter.Allow() {
        fmt.Println("Request allowed")
    } else {
        fmt.Println("Rate limit exceeded")
    }

    // 複数のトークンを一度に使用
    if limiter.AllowN(5) {
        fmt.Println("5 tokens consumed")
    }

    // 利用可能なトークン数を確認
    fmt.Printf("Available tokens: %d\n", limiter.Available())
}
```

### HTTPミドルウェア

```go
package main

import (
    "net/http"
    "time"
    "github.com/rRateLimit/client/ratelimit"
)

func main() {
    // レート制限ミドルウェアを作成
    middleware := ratelimit.NewMiddleware(&ratelimit.MiddlewareConfig{
        LimiterFactory: func() ratelimit.Limiter {
            return ratelimit.NewTokenBucket(
                ratelimit.WithRate(1000),
                ratelimit.WithPeriod(time.Minute),
                ratelimit.WithBurst(50),
            )
        },
        KeyFunc: ratelimit.IPKeyFunc,  // IP別の制限
    })

    // ハンドラーに適用
    mux := http.NewServeMux()
    mux.HandleFunc("/api/", apiHandler)
    
    // ミドルウェアを適用
    http.ListenAndServe(":8080", middleware.Handler(mux))
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("API Response"))
}
```

## アルゴリズム

### Token Bucket

トークンバケットアルゴリズムは、バーストトラフィックを許可しながら平均レートを維持します。

```go
limiter := ratelimit.NewTokenBucket(
    ratelimit.WithRate(100),           // レート: 100リクエスト
    ratelimit.WithPeriod(time.Second),  // 期間: 1秒
    ratelimit.WithBurst(20),            // バースト容量: 20
)
```

**特徴:**
- バーストトラフィックの許可
- スムーズなレート制限
- APIレート制限に最適

### Fixed Window

固定ウィンドウアルゴリズムは、固定時間枠内でリクエストをカウントします。

```go
limiter := ratelimit.NewFixedWindow(
    ratelimit.WithRate(1000),
    ratelimit.WithPeriod(time.Minute),
)
```

**特徴:**
- シンプルな実装
- 低メモリ使用量
- 基本的なレート制限に適合

### Sliding Window

スライディングウィンドウアルゴリズムは、より正確なレート制限を提供します。

```go
limiter := ratelimit.NewSlidingWindow(
    ratelimit.WithRate(100),
    ratelimit.WithPeriod(time.Minute),
)
```

**特徴:**
- 正確なレート制限
- 固定ウィンドウの境界問題を解決
- 公平性を重視する場合に最適

## 高度な使用法

### Wait機能

レート制限が利用可能になるまで待機：

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := limiter.Wait(ctx); err != nil {
    log.Printf("Rate limit wait failed: %v", err)
    return
}

// リクエストを処理
processRequest()
```

### カスタムキー関数

独自のキー抽出ロジックを実装：

```go
config := &ratelimit.MiddlewareConfig{
    KeyFunc: func(r *http.Request) string {
        // APIキーベースの制限
        apiKey := r.Header.Get("X-API-Key")
        if apiKey != "" {
            return "api:" + apiKey
        }
        return ratelimit.IPKeyFunc(r)
    },
    // ...
}
```

### 統計情報の取得

```go
middleware := ratelimit.NewMiddleware(config)

// 統計情報を取得
stats := middleware.Stats()
for key, available := range stats {
    fmt.Printf("Key: %s, Available: %d\n", key, available)
}
```

## 並行処理

すべての実装はスレッドセーフで、並行環境で安全に使用できます：

```go
var wg sync.WaitGroup
limiter := ratelimit.NewTokenBucket(
    ratelimit.WithRate(1000),
    ratelimit.WithPeriod(time.Second),
)

// 複数のゴルーチンから同時にアクセス
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        for j := 0; j < 100; j++ {
            if limiter.Allow() {
                // リクエストを処理
            }
        }
    }(i)
}

wg.Wait()
```

## ベストプラクティス

1. **適切なアルゴリズムの選択**
   - バーストが必要: Token Bucket
   - シンプルさ重視: Fixed Window
   - 正確性重視: Sliding Window

2. **キー戦略**
   - IP別: 基本的な保護
   - ユーザー別: 認証済みAPI
   - パス別: エンドポイント固有の制限

3. **エラーハンドリング**
   ```go
   if !limiter.Allow() {
       w.Header().Set("Retry-After", "60")
       http.Error(w, "Rate limit exceeded", 429)
       return
   }
   ```

4. **モニタリング**
   - 統計情報を定期的に記録
   - アラートの設定
   - 制限の調整

## パフォーマンス

ベンチマーク結果（参考値）：

```
BenchmarkTokenBucket-8      10000000    105 ns/op    0 B/op    0 allocs/op
BenchmarkFixedWindow-8      15000000     85 ns/op    0 B/op    0 allocs/op
BenchmarkSlidingWindow-8     8000000    145 ns/op   16 B/op    1 allocs/op
```

## 例

詳細な例は `examples/` ディレクトリを参照：

- [基本的な使用法](../examples/basic/main.go)
- [Webサーバー統合](../examples/webserver/main.go)
- [並行処理](../examples/concurrent/main.go)

## テスト

```bash
# 単体テストの実行
go test ./ratelimit/...

# ベンチマークの実行
go test -bench=. ./ratelimit/...

# カバレッジレポート
go test -cover ./ratelimit/...
```

## ライセンス

MIT License

## 貢献

プルリクエストを歓迎します。大きな変更の場合は、まずissueを作成して変更内容について議論してください。