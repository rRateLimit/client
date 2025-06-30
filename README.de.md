# Rate Limiting Test-Framework

Ein umfassendes Go-Framework zur Implementierung und zum Testen von Rate-Limiting-Algorithmen. Dieses Projekt bietet Beispielimplementierungen verschiedener Rate-Limiting-Strategien sowie Client-Server-Tools zur Validierung und zum Benchmarking.

## Überblick

Dieses Projekt besteht aus:

- **Test-Client**: Sendet Anfragen mit konfigurierbaren Raten
- **Test-Server**: Fungiert als Echo-Server
- **Algorithmus-Beispiele**: 4 verschiedene Rate-Limiting-Implementierungen

## Schnellstart

### Grundlegende Verwendung

1. Server starten:
```bash
go run server/main.go -protocol tcp -port 8080
```

2. In einem anderen Terminal den Client starten:
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### Beispiele

```bash
# TCP-Server mit ausführlicher Protokollierung starten
go run server/main.go -protocol tcp -port 8080 -verbose

# Hochraten-Test (1000 req/s, 10 Verbindungen)
go run main.go -rate 1000 -connections 10 -duration 60s

# UDP-Protokoll-Test
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## Architektur

### Verzeichnisstruktur

```
.
├── main.go                    # Test-Client
├── server/
│   └── main.go               # Test-Server
└── sample/
    ├── token_bucket/         # Token-Bucket-Implementierung
    ├── fixed_window/         # Fixed-Window-Implementierung
    ├── sliding_window/       # Sliding-Window-Implementierung
    └── concurrent/           # Thread-sichere Implementierung
```

## Komponentendetails

### Test-Client (main.go)

Ein hochleistungsfähiger Rate-Limiting-Test-Client.

**Funktionen:**
- TCP/UDP-Protokoll-Unterstützung
- Mehrere gleichzeitige Verbindungen (nur TCP)
- Anpassbare Nachrichtengröße
- Echtzeit-Statistikanzeige

**Kommandozeilenoptionen:**
```bash
-server string     # Serveradresse (Standard "localhost:8080")
-protocol string   # Protokoll: tcp oder udp (Standard "tcp")
-rate int         # Nachrichten pro Sekunde (Standard 100)
-duration duration # Testdauer (Standard 10s)
-connections int   # Gleichzeitige Verbindungen, nur TCP (Standard 1)
-size int         # Nachrichtengröße in Bytes (Standard 64)
```

**Ausgabebeispiel:**
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

### Test-Server (server/main.go)

Ein einfacher Server, der empfangene Nachrichten zurücksendet.

**Funktionen:**
- TCP/UDP-Protokoll-Unterstützung
- Mehrere gleichzeitige Client-Verbindungen
- Statistikanzeige alle 5 Sekunden
- Graceful Shutdown (Strg+C)

**Kommandozeilenoptionen:**
```bash
-protocol string  # Protokoll: tcp oder udp (Standard "tcp")
-port int        # Listening-Port (Standard 8080)
-verbose         # Ausführliche Protokollierung aktivieren
```

**Statistikanzeige-Beispiel:**
```
[15:30:45] Received: 5023, Processed: 5023, Errors: 0, Rate: 1004.60 msg/s
[15:30:50] Received: 10089, Processed: 10089, Errors: 0, Rate: 1013.20 msg/s
```

## Rate-Limiting-Algorithmen

### 1. Token Bucket

**Eigenschaften:**
- Ermöglicht Burst-Verarbeitung
- Hält durchschnittliche Rate bei und erlaubt kurzfristige Spitzen
- Speichereffizient

**Verwendungsbeispiel:**
```go
limiter := NewTokenBucket(100, 10) // Kapazität 100, Auffüllung 10/Sekunde

if limiter.Allow() {
    // Anfrage verarbeiten
}
```

**Anwendungsfälle:**
- API-Rate-Limiting
- Netzwerk-Bandbreitenkontrolle
- Wenn Burst-Traffic akzeptabel ist

### 2. Fixed Window

**Eigenschaften:**
- Einfache Implementierung
- Minimaler Speicherverbrauch
- Grenzprobleme an Fensterkanten

**Verwendungsbeispiel:**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // 1000 Anfragen pro Minute

if limiter.Allow() {
    // Anfrage verarbeiten
}
```

**Anwendungsfälle:**
- Wenn einfaches Rate-Limiting benötigt wird
- Leistung hat Vorrang vor Genauigkeit

### 3. Sliding Window

**Eigenschaften:**
- Genaueres Rate-Limiting
- Löst Fixed-Window-Grenzprobleme
- Etwas höherer Speicherverbrauch

**Verwendungsbeispiel:**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // 100 Anfragen pro Minute

if limiter.Allow() {
    // Anfrage verarbeiten
}
```

**Anwendungsfälle:**
- Wenn genaues Rate-Limiting erforderlich ist
- Fairness ist wichtig

### 4. Concurrent-Implementierung

**Eigenschaften:**
- Hohe Leistung mit atomaren Operationen
- Unterstützung verteilter Umgebungen
- HTTP-Middleware-Beispiel

**HTTP-Middleware-Verwendung:**
```go
// Rate-Limiting pro Benutzer
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // 100 Anfragen/Minute pro Benutzer
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// Middleware anwenden
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## Performance-Benchmarks

Algorithmenvergleich (Referenzwerte):

| Algorithmus | Durchsatz | Speichernutzung | Eigenschaften |
|-------------|-----------|-----------------|---------------|
| Token Bucket | Hoch | Niedrig | Burst-Unterstützung |
| Fixed Window | Höchste | Niedrigste | Einfach |
| Sliding Window | Mittel | Mittel | Genau |
| Concurrent | Hoch | Niedrig | Nebenläufigkeit optimiert |

## Praktische Anwendungsfälle

### 1. API-Server Rate-Limit-Test

```bash
# API-Server Rate-Limiting testen (1000 req/s, 5 Minuten)
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. Lasttest-Szenarien

```bash
# Schrittweiser Lastanstiegstest
for rate in 100 500 1000 2000 5000; do
    echo "Testing rate: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. Netzwerk-Bandbreitenmessung

```bash
# Bandbreitenmessung mit großer Nachrichtengröße
go run main.go -size 1024 -rate 1000 -protocol udp
```

## Fehlerbehebung

### Häufige Probleme

1. **"Too many open files" Fehler**
   ```bash
   # Dateideskriptor-Limit erhöhen
   ulimit -n 65536
   ```

2. **Verbindungsfehler bei hohen Raten**
   - Parameter `-connections` anpassen
   - Server-seitige Puffergrößen überprüfen

3. **UDP-Paketverlust**
   - Rate oder Nachrichtengröße reduzieren
   - Kernel-UDP-Puffergröße anpassen

## Entwicklung

### Kompilierung

```bash
# Client kompilieren
go build -o rate-limit-client main.go

# Server kompilieren
go build -o rate-limit-server server/main.go
```

### Tests

```bash
# Unit-Tests ausführen
go test ./...

# Benchmarks ausführen
go test -bench=. ./sample/...
```

### Neue Algorithmen hinzufügen

1. Neues Paket im `sample/`-Verzeichnis erstellen
2. `RateLimiter`-Interface implementieren:
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. Tests und Benchmarks hinzufügen

## Lizenz

MIT-Lizenz

## Beiträge

Pull Requests sind willkommen. Bei größeren Änderungen bitte zuerst ein Issue öffnen, um zu besprechen, was Sie ändern möchten.

## Referenzen

- [Rate-Limiting-Algorithmen](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [Token-Bucket-Algorithmus](https://de.wikipedia.org/wiki/Token-Bucket-Algorithmus)
- [Sliding-Window-Counter](https://blog.logrocket.com/rate-limiting-go-application/)