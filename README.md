# Rate Limit Testing Framework

レート制限（Rate Limiting）の実装と検証のためのGoフレームワークです。様々なレート制限アルゴリズムの実装例と、それらをテストするためのクライアント・サーバーツールを提供します。

## 概要

このプロジェクトは以下の要素で構成されています：

- **テストクライアント**: 設定可能なレートでリクエストを送信
- **テストサーバー**: エコーサーバーとして動作
- **アルゴリズム実装例**: 4つの異なるレート制限方式

## クイックスタート

### 基本的な使用方法

1. サーバーを起動:
```bash
go run server/main.go -protocol tcp -port 8080
```

2. 別のターミナルでクライアントを起動:
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### 実行例

```bash
# TCPサーバーを起動（詳細ログ付き）
go run server/main.go -protocol tcp -port 8080 -verbose

# 高レートでのテスト（1000 req/s、10接続）
go run main.go -rate 1000 -connections 10 -duration 60s

# UDPプロトコルでのテスト
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## アーキテクチャ

### ディレクトリ構造

```
.
├── main.go                    # テストクライアント
├── server/
│   └── main.go               # テストサーバー
└── sample/
    ├── token_bucket/         # トークンバケット実装
    ├── fixed_window/         # 固定ウィンドウ実装
    ├── sliding_window/       # スライディングウィンドウ実装
    └── concurrent/           # 並行処理対応実装
```

## コンポーネント詳細

### テストクライアント (main.go)

高性能なレート制限テストクライアントです。

**機能:**
- TCP/UDP プロトコルサポート
- 複数並行接続（TCPのみ）
- カスタマイズ可能なメッセージサイズ
- リアルタイム統計表示

**コマンドラインオプション:**
```bash
-server string    # サーバーアドレス (default "localhost:8080")
-protocol string  # プロトコル: tcp または udp (default "tcp")
-rate int        # 秒あたりのメッセージ数 (default 100)
-duration duration # テスト実行時間 (default 10s)
-connections int  # 並行接続数、TCPのみ (default 1)
-size int        # メッセージサイズ（バイト） (default 64)
```

**出力例:**
```
Starting rate limit test client
Protocol: tcp
Server: localhost:8080
Rate: 1000 messages/second
Duration: 30s
Connections: 10
Message size: 64 bytes

--- Test Statistics ---
Duration: 30s
Messages sent: 30000
Messages succeeded: 29850
Messages failed: 150
Success rate: 99.50%
Actual rate: 1000.00 messages/second
```

### テストサーバー (server/main.go)

受信したメッセージをエコーバックするシンプルなサーバーです。

**機能:**
- TCP/UDP プロトコルサポート
- 複数クライアント同時接続対応
- 5秒ごとの統計情報表示
- グレースフルシャットダウン（Ctrl+C）

**コマンドラインオプション:**
```bash
-protocol string  # プロトコル: tcp または udp (default "tcp")
-port int        # リスニングポート (default 8080)
-verbose         # 詳細ログを有効化
```

**統計表示例:**
```
[15:30:45] Received: 5023, Processed: 5023, Errors: 0, Rate: 1004.60 msg/s
[15:30:50] Received: 10089, Processed: 10089, Errors: 0, Rate: 1013.20 msg/s
```

## レート制限アルゴリズム

### 1. トークンバケット (Token Bucket)

**特徴:**
- バースト処理が可能
- 平均レートを保ちながら短期的な増加を許容
- メモリ効率が良い

**使用例:**
```go
limiter := NewTokenBucket(100, 10) // 容量100、秒10トークン補充

if limiter.Allow() {
    // リクエストを処理
}
```

**適用場面:**
- APIレート制限
- ネットワーク帯域制御
- バーストトラフィックを許容したい場合

### 2. 固定ウィンドウ (Fixed Window)

**特徴:**
- 実装がシンプル
- メモリ使用量が最小
- ウィンドウ境界での問題あり

**使用例:**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // 1分間に1000リクエスト

if limiter.Allow() {
    // リクエストを処理
}
```

**適用場面:**
- シンプルなレート制限が必要な場合
- 厳密さよりも性能を重視する場合

### 3. スライディングウィンドウ (Sliding Window)

**特徴:**
- より正確なレート制限
- 固定ウィンドウの境界問題を解決
- メモリ使用量がやや多い

**使用例:**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // 1分間に100リクエスト

if limiter.Allow() {
    // リクエストを処理
}
```

**適用場面:**
- 正確なレート制限が必要な場合
- 公平性を重視する場合

### 4. 並行処理対応実装 (Concurrent)

**特徴:**
- アトミック操作による高性能
- 分散環境対応
- HTTPミドルウェア実装例

**HTTPミドルウェアの使用例:**
```go
// ユーザーごとのレート制限
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // ユーザーあたり100リクエスト/分
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// ミドルウェアを適用
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## パフォーマンスベンチマーク

各アルゴリズムの性能比較（参考値）:

| アルゴリズム | スループット | メモリ使用量 | 特徴 |
|------------|------------|------------|------|
| Token Bucket | 高 | 低 | バースト対応 |
| Fixed Window | 最高 | 最低 | シンプル |
| Sliding Window | 中 | 中 | 正確 |
| Concurrent | 高 | 低 | 並行処理最適化 |

## 実践的な使用例

### 1. API サーバーのレート制限テスト

```bash
# APIサーバーのレート制限をテスト（1000 req/s、5分間）
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. 負荷テストシナリオ

```bash
# 段階的な負荷増加テスト
for rate in 100 500 1000 2000 5000; do
    echo "Testing rate: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. ネットワーク帯域測定

```bash
# 大きなメッセージサイズでの帯域測定
go run main.go -size 1024 -rate 1000 -protocol udp
```

## トラブルシューティング

### よくある問題

1. **"Too many open files" エラー**
   ```bash
   # ファイルディスクリプタの上限を増やす
   ulimit -n 65536
   ```

2. **高レートでの接続エラー**
   - `-connections` パラメータを調整
   - サーバー側のバッファサイズを確認

3. **UDPパケットロス**
   - レートを下げるか、メッセージサイズを小さくする
   - カーネルのUDPバッファサイズを調整

## 開発

### ビルド

```bash
# クライアントのビルド
go build -o rate-limit-client main.go

# サーバーのビルド
go build -o rate-limit-server server/main.go
```

### テスト

```bash
# 単体テストの実行
go test ./...

# ベンチマークの実行
go test -bench=. ./sample/...
```

### 新しいアルゴリズムの追加

1. `sample/` ディレクトリに新しいパッケージを作成
2. `RateLimiter` インターフェースを実装:
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. テストとベンチマークを追加

## ライセンス

MIT License

## 貢献

プルリクエストを歓迎します。大きな変更の場合は、まずissueを作成して変更内容について議論してください。

## 参考資料

- [Rate Limiting Algorithms](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [Token Bucket Algorithm](https://en.wikipedia.org/wiki/Token_bucket)
- [Sliding Window Counter](https://blog.logrocket.com/rate-limiting-go-application/)