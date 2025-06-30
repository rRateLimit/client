# Framework di Test per Rate Limiting

Un framework Go completo per implementare e testare algoritmi di rate limiting. Questo progetto fornisce implementazioni di esempio di varie strategie di limitazione del traffico insieme a strumenti client-server per la validazione e il benchmarking.

## Panoramica

Questo progetto è composto da:

- **Client di Test**: Invia richieste a frequenze configurabili
- **Server di Test**: Funziona come server echo
- **Esempi di Algoritmi**: 4 diverse implementazioni di rate limiting

## Avvio Rapido

### Utilizzo Base

1. Avviare il server:
```bash
go run server/main.go -protocol tcp -port 8080
```

2. In un altro terminale, avviare il client:
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### Esempi

```bash
# Avviare il server TCP con log dettagliati
go run server/main.go -protocol tcp -port 8080 -verbose

# Test ad alta frequenza (1000 req/s, 10 connessioni)
go run main.go -rate 1000 -connections 10 -duration 60s

# Test con protocollo UDP
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## Architettura

### Struttura delle Directory

```
.
├── main.go                    # Client di test
├── server/
│   └── main.go               # Server di test
└── sample/
    ├── token_bucket/         # Implementazione token bucket
    ├── fixed_window/         # Implementazione finestra fissa
    ├── sliding_window/       # Implementazione finestra scorrevole
    └── concurrent/           # Implementazione thread-safe
```

## Dettagli dei Componenti

### Client di Test (main.go)

Un client di test per rate limiting ad alte prestazioni.

**Funzionalità:**
- Supporto protocolli TCP/UDP
- Connessioni concorrenti multiple (solo TCP)
- Dimensione messaggi personalizzabile
- Visualizzazione statistiche in tempo reale

**Opzioni della Linea di Comando:**
```bash
-server string     # Indirizzo del server (default "localhost:8080")
-protocol string   # Protocollo: tcp o udp (default "tcp")
-rate int         # Messaggi al secondo (default 100)
-duration duration # Durata del test (default 10s)
-connections int   # Connessioni concorrenti, solo TCP (default 1)
-size int         # Dimensione messaggio in byte (default 64)
```

**Esempio di Output:**
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

### Server di Test (server/main.go)

Un server semplice che restituisce in echo i messaggi ricevuti.

**Funzionalità:**
- Supporto protocolli TCP/UDP
- Connessioni simultanee di più client
- Visualizzazione statistiche ogni 5 secondi
- Arresto graceful (Ctrl+C)

**Opzioni della Linea di Comando:**
```bash
-protocol string  # Protocollo: tcp o udp (default "tcp")
-port int        # Porta di ascolto (default 8080)
-verbose         # Abilita log dettagliati
```

**Esempio di Visualizzazione Statistiche:**
```
[15:30:45] Received: 5023, Processed: 5023, Errors: 0, Rate: 1004.60 msg/s
[15:30:50] Received: 10089, Processed: 10089, Errors: 0, Rate: 1013.20 msg/s
```

## Algoritmi di Rate Limiting

### 1. Token Bucket

**Caratteristiche:**
- Permette elaborazione a raffica
- Mantiene frequenza media permettendo picchi a breve termine
- Efficiente in memoria

**Esempio di Utilizzo:**
```go
limiter := NewTokenBucket(100, 10) // Capacità 100, ricarica 10/secondo

if limiter.Allow() {
    // Elabora richiesta
}
```

**Casi d'Uso:**
- Limitazione rate API
- Controllo larghezza di banda
- Quando il traffico a raffica è accettabile

### 2. Finestra Fissa (Fixed Window)

**Caratteristiche:**
- Implementazione semplice
- Utilizzo minimo di memoria
- Problemi ai bordi della finestra

**Esempio di Utilizzo:**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // 1000 richieste al minuto

if limiter.Allow() {
    // Elabora richiesta
}
```

**Casi d'Uso:**
- Quando serve un rate limiting semplice
- Le prestazioni sono prioritarie rispetto all'accuratezza

### 3. Finestra Scorrevole (Sliding Window)

**Caratteristiche:**
- Rate limiting più accurato
- Risolve i problemi dei bordi della finestra fissa
- Utilizzo di memoria leggermente superiore

**Esempio di Utilizzo:**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // 100 richieste al minuto

if limiter.Allow() {
    // Elabora richiesta
}
```

**Casi d'Uso:**
- Quando è richiesto un rate limiting accurato
- L'equità è importante

### 4. Implementazione Concorrente

**Caratteristiche:**
- Alte prestazioni con operazioni atomiche
- Supporto ambiente distribuito
- Esempio di middleware HTTP

**Utilizzo del Middleware HTTP:**
```go
// Rate limiting per utente
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // 100 richieste/minuto per utente
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// Applica middleware
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## Benchmark delle Prestazioni

Confronto algoritmi (valori di riferimento):

| Algoritmo | Throughput | Utilizzo Memoria | Caratteristiche |
|-----------|------------|------------------|-----------------|
| Token Bucket | Alto | Basso | Supporto raffica |
| Finestra Fissa | Altissimo | Bassissimo | Semplice |
| Finestra Scorrevole | Medio | Medio | Accurato |
| Concorrente | Alto | Basso | Ottimizzato concorrenza |

## Casi d'Uso Pratici

### 1. Test Rate Limiting Server API

```bash
# Test rate limiting server API (1000 req/s, 5 minuti)
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. Scenari di Test di Carico

```bash
# Test aumento graduale del carico
for rate in 100 500 1000 2000 5000; do
    echo "Testing rate: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. Misurazione Larghezza di Banda

```bash
# Misurazione larghezza di banda con messaggi grandi
go run main.go -size 1024 -rate 1000 -protocol udp
```

## Risoluzione Problemi

### Problemi Comuni

1. **Errore "Too many open files"**
   ```bash
   # Aumentare il limite dei file descriptor
   ulimit -n 65536
   ```

2. **Errori di connessione ad alta frequenza**
   - Regolare il parametro `-connections`
   - Verificare le dimensioni buffer lato server

3. **Perdita pacchetti UDP**
   - Ridurre frequenza o dimensione messaggi
   - Regolare dimensione buffer UDP del kernel

## Sviluppo

### Compilazione

```bash
# Compilare client
go build -o rate-limit-client main.go

# Compilare server
go build -o rate-limit-server server/main.go
```

### Test

```bash
# Eseguire test unitari
go test ./...

# Eseguire benchmark
go test -bench=. ./sample/...
```

### Aggiungere Nuovi Algoritmi

1. Creare nuovo package nella directory `sample/`
2. Implementare interfaccia `RateLimiter`:
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. Aggiungere test e benchmark

## Licenza

Licenza MIT

## Contribuire

Le pull request sono benvenute. Per modifiche importanti, aprire prima una issue per discutere cosa si vorrebbe cambiare.

## Riferimenti

- [Algoritmi di Rate Limiting](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [Algoritmo Token Bucket](https://it.wikipedia.org/wiki/Token_bucket)
- [Contatore a Finestra Scorrevole](https://blog.logrocket.com/rate-limiting-go-application/)