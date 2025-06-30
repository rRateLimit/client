# 속도 제한 테스트 프레임워크

속도 제한 알고리즘을 구현하고 테스트하기 위한 포괄적인 Go 프레임워크입니다. 이 프로젝트는 다양한 속도 제한 전략의 샘플 구현과 검증 및 벤치마킹을 위한 클라이언트-서버 도구를 제공합니다.

## 개요

이 프로젝트의 구성:

- **테스트 클라이언트**: 구성 가능한 속도로 요청 전송
- **테스트 서버**: 에코 서버로 작동
- **알고리즘 예제**: 4가지 다른 속도 제한 구현

## 빠른 시작

### 기본 사용법

1. 서버 시작:
```bash
go run server/main.go -protocol tcp -port 8080
```

2. 다른 터미널에서 클라이언트 시작:
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### 예제

```bash
# 상세 로깅으로 TCP 서버 시작
go run server/main.go -protocol tcp -port 8080 -verbose

# 고속 테스트 (1000 req/s, 10개 연결)
go run main.go -rate 1000 -connections 10 -duration 60s

# UDP 프로토콜 테스트
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## 아키텍처

### 디렉토리 구조

```
.
├── main.go                    # 테스트 클라이언트
├── server/
│   └── main.go               # 테스트 서버
└── sample/
    ├── token_bucket/         # 토큰 버킷 구현
    ├── fixed_window/         # 고정 윈도우 구현
    ├── sliding_window/       # 슬라이딩 윈도우 구현
    └── concurrent/           # 스레드 안전 구현
```

## 컴포넌트 상세

### 테스트 클라이언트 (main.go)

고성능 속도 제한 테스트 클라이언트입니다.

**기능:**
- TCP/UDP 프로토콜 지원
- 다중 동시 연결 (TCP 전용)
- 사용자 정의 메시지 크기
- 실시간 통계 표시

**명령줄 옵션:**
```bash
-server string     # 서버 주소 (기본값 "localhost:8080")
-protocol string   # 프로토콜: tcp 또는 udp (기본값 "tcp")
-rate int         # 초당 메시지 수 (기본값 100)
-duration duration # 테스트 기간 (기본값 10s)
-connections int   # 동시 연결 수, TCP 전용 (기본값 1)
-size int         # 메시지 크기 (바이트) (기본값 64)
```

**출력 예시:**
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

### 테스트 서버 (server/main.go)

수신된 메시지를 에코하는 간단한 서버입니다.

**기능:**
- TCP/UDP 프로토콜 지원
- 다중 클라이언트 동시 연결
- 5초마다 통계 표시
- 우아한 종료 (Ctrl+C)

**명령줄 옵션:**
```bash
-protocol string  # 프로토콜: tcp 또는 udp (기본값 "tcp")
-port int        # 리스닝 포트 (기본값 8080)
-verbose         # 상세 로깅 활성화
```

**통계 표시 예시:**
```
[15:30:45] Received: 5023, Processed: 5023, Errors: 0, Rate: 1004.60 msg/s
[15:30:50] Received: 10089, Processed: 10089, Errors: 0, Rate: 1013.20 msg/s
```

## 속도 제한 알고리즘

### 1. 토큰 버킷 (Token Bucket)

**특징:**
- 버스트 처리 가능
- 평균 속도 유지하며 단기 급증 허용
- 메모리 효율적

**사용 예시:**
```go
limiter := NewTokenBucket(100, 10) // 용량 100, 초당 10개 보충

if limiter.Allow() {
    // 요청 처리
}
```

**사용 사례:**
- API 속도 제한
- 네트워크 대역폭 제어
- 버스트 트래픽이 허용될 때

### 2. 고정 윈도우 (Fixed Window)

**특징:**
- 간단한 구현
- 최소 메모리 사용
- 윈도우 경계 문제 존재

**사용 예시:**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // 분당 1000개 요청

if limiter.Allow() {
    // 요청 처리
}
```

**사용 사례:**
- 간단한 속도 제한이 필요할 때
- 정확도보다 성능이 우선일 때

### 3. 슬라이딩 윈도우 (Sliding Window)

**특징:**
- 더 정확한 속도 제한
- 고정 윈도우 경계 문제 해결
- 약간 높은 메모리 사용

**사용 예시:**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // 분당 100개 요청

if limiter.Allow() {
    // 요청 처리
}
```

**사용 사례:**
- 정확한 속도 제한이 필요할 때
- 공정성이 중요할 때

### 4. 동시성 구현 (Concurrent)

**특징:**
- 원자 연산을 통한 고성능
- 분산 환경 지원
- HTTP 미들웨어 예제

**HTTP 미들웨어 사용:**
```go
// 사용자별 속도 제한
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // 사용자당 분당 100개 요청
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// 미들웨어 적용
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## 성능 벤치마크

알고리즘 비교 (참조값):

| 알고리즘 | 처리량 | 메모리 사용 | 특징 |
|---------|--------|------------|------|
| 토큰 버킷 | 높음 | 낮음 | 버스트 지원 |
| 고정 윈도우 | 최고 | 최저 | 단순함 |
| 슬라이딩 윈도우 | 중간 | 중간 | 정확함 |
| 동시성 | 높음 | 낮음 | 동시성 최적화 |

## 실제 사용 사례

### 1. API 서버 속도 제한 테스트

```bash
# API 서버 속도 제한 테스트 (1000 req/s, 5분간)
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. 부하 테스트 시나리오

```bash
# 점진적 부하 증가 테스트
for rate in 100 500 1000 2000 5000; do
    echo "Testing rate: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. 네트워크 대역폭 측정

```bash
# 큰 메시지로 대역폭 측정
go run main.go -size 1024 -rate 1000 -protocol udp
```

## 문제 해결

### 일반적인 문제

1. **"Too many open files" 오류**
   ```bash
   # 파일 디스크립터 제한 증가
   ulimit -n 65536
   ```

2. **고속에서 연결 오류**
   - `-connections` 매개변수 조정
   - 서버 측 버퍼 크기 확인

3. **UDP 패킷 손실**
   - 속도나 메시지 크기 감소
   - 커널 UDP 버퍼 크기 조정

## 개발

### 빌드

```bash
# 클라이언트 빌드
go build -o rate-limit-client main.go

# 서버 빌드
go build -o rate-limit-server server/main.go
```

### 테스트

```bash
# 단위 테스트 실행
go test ./...

# 벤치마크 실행
go test -bench=. ./sample/...
```

### 새 알고리즘 추가

1. `sample/` 디렉토리에 새 패키지 생성
2. `RateLimiter` 인터페이스 구현:
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. 테스트와 벤치마크 추가

## 라이선스

MIT 라이선스

## 기여

Pull Request를 환영합니다. 주요 변경사항의 경우 먼저 이슈를 열어 변경하고자 하는 내용을 논의해 주세요.

## 참고 자료

- [속도 제한 알고리즘](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [토큰 버킷 알고리즘](https://ko.wikipedia.org/wiki/토큰_버킷)
- [슬라이딩 윈도우 카운터](https://blog.logrocket.com/rate-limiting-go-application/)