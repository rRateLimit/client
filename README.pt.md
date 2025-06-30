# Framework de Teste de Rate Limiting

Um framework Go abrangente para implementar e testar algoritmos de rate limiting. Este projeto fornece implementações de exemplo de várias estratégias de rate limiting, juntamente com ferramentas cliente-servidor para validação e benchmarking.

## Visão Geral

Este projeto consiste em:

- **Cliente de Teste**: Envia requisições a taxas configuráveis
- **Servidor de Teste**: Atua como um servidor de eco
- **Exemplos de Algoritmos**: 4 implementações diferentes de rate limiting

## Início Rápido

### Uso Básico

1. Iniciar o servidor:
```bash
go run server/main.go -protocol tcp -port 8080
```

2. Em outro terminal, iniciar o cliente:
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### Exemplos

```bash
# Iniciar servidor TCP com log detalhado
go run server/main.go -protocol tcp -port 8080 -verbose

# Teste de alta taxa (1000 req/s, 10 conexões)
go run main.go -rate 1000 -connections 10 -duration 60s

# Teste de protocolo UDP
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## Arquitetura

### Estrutura de Diretórios

```
.
├── main.go                    # Cliente de teste
├── server/
│   └── main.go               # Servidor de teste
└── sample/
    ├── token_bucket/         # Implementação token bucket
    ├── fixed_window/         # Implementação janela fixa
    ├── sliding_window/       # Implementação janela deslizante
    └── concurrent/           # Implementação concurrent-safe
```

## Detalhes dos Componentes

### Cliente de Teste (main.go)

Um cliente de teste de rate limiting de alto desempenho.

**Recursos:**
- Suporte a protocolos TCP/UDP
- Múltiplas conexões concorrentes (apenas TCP)
- Tamanho de mensagem personalizável
- Exibição de estatísticas em tempo real

**Opções de Linha de Comando:**
```bash
-server string     # Endereço do servidor (padrão "localhost:8080")
-protocol string   # Protocolo: tcp ou udp (padrão "tcp")
-rate int         # Mensagens por segundo (padrão 100)
-duration duration # Duração do teste (padrão 10s)
-connections int   # Conexões concorrentes, apenas TCP (padrão 1)
-size int         # Tamanho da mensagem em bytes (padrão 64)
```

**Exemplo de Saída:**
```
Iniciando cliente de teste de rate limit
Protocolo: tcp
Servidor: localhost:8080
Taxa: 1000 mensagens/segundo
Duração: 30s
Conexões: 10
Tamanho da mensagem: 64 bytes

--- Estatísticas do Teste ---
Duração: 30s
Mensagens enviadas: 30000
Mensagens bem-sucedidas: 29850
Mensagens falhadas: 150
Taxa de sucesso: 99,50%
Taxa real: 1000,00 mensagens/segundo
```

### Servidor de Teste (server/main.go)

Um servidor simples que ecoa mensagens recebidas.

**Recursos:**
- Suporte a protocolos TCP/UDP
- Múltiplas conexões simultâneas de clientes
- Exibição de estatísticas a cada 5 segundos
- Encerramento gracioso (Ctrl+C)

**Opções de Linha de Comando:**
```bash
-protocol string  # Protocolo: tcp ou udp (padrão "tcp")
-port int        # Porta de escuta (padrão 8080)
-verbose         # Habilitar log detalhado
```

**Exemplo de Exibição de Estatísticas:**
```
[15:30:45] Recebidas: 5023, Processadas: 5023, Erros: 0, Taxa: 1004,60 msg/s
[15:30:50] Recebidas: 10089, Processadas: 10089, Erros: 0, Taxa: 1013,20 msg/s
```

## Algoritmos de Rate Limiting

### 1. Token Bucket

**Recursos:**
- Permite processamento em rajada
- Mantém taxa média permitindo picos de curto prazo
- Eficiente em memória

**Exemplo de Uso:**
```go
limiter := NewTokenBucket(100, 10) // Capacidade 100, recarga 10/segundo

if limiter.Allow() {
    // Processar requisição
}
```

**Casos de Uso:**
- Rate limiting de API
- Controle de largura de banda de rede
- Quando tráfego em rajada é aceitável

### 2. Janela Fixa

**Recursos:**
- Implementação simples
- Uso mínimo de memória
- Problemas de limite nas bordas da janela

**Exemplo de Uso:**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // 1000 requisições por minuto

if limiter.Allow() {
    // Processar requisição
}
```

**Casos de Uso:**
- Quando rate limiting simples é necessário
- Performance é priorizada sobre precisão

### 3. Janela Deslizante

**Recursos:**
- Rate limiting mais preciso
- Resolve problemas de limite da janela fixa
- Uso de memória ligeiramente maior

**Exemplo de Uso:**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // 100 requisições por minuto

if limiter.Allow() {
    // Processar requisição
}
```

**Casos de Uso:**
- Quando rate limiting preciso é necessário
- Justiça é importante

### 4. Implementação Concorrente

**Recursos:**
- Alto desempenho com operações atômicas
- Suporte a ambiente distribuído
- Exemplo de middleware HTTP

**Uso de Middleware HTTP:**
```go
// Rate limiting por usuário
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // 100 requisições/minuto por usuário
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// Aplicar middleware
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## Benchmarks de Desempenho

Comparação de algoritmos (valores de referência):

| Algoritmo | Throughput | Uso de Memória | Recursos |
|-----------|------------|----------------|----------|
| Token Bucket | Alto | Baixo | Suporte a rajada |
| Janela Fixa | Mais Alto | Mais Baixo | Simples |
| Janela Deslizante | Médio | Médio | Preciso |
| Concorrente | Alto | Baixo | Otimizado para concorrência |

## Casos de Uso Práticos

### 1. Teste de Rate Limit de Servidor API

```bash
# Testar rate limiting de servidor API (1000 req/s, 5 minutos)
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. Cenários de Teste de Carga

```bash
# Teste de aumento gradual de carga
for rate in 100 500 1000 2000 5000; do
    echo "Testando taxa: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. Medição de Largura de Banda de Rede

```bash
# Medição de largura de banda com tamanho grande de mensagem
go run main.go -size 1024 -rate 1000 -protocol udp
```

## Solução de Problemas

### Problemas Comuns

1. **Erro "Too many open files"**
   ```bash
   # Aumentar limite de descritores de arquivo
   ulimit -n 65536
   ```

2. **Erros de conexão em altas taxas**
   - Ajustar parâmetro `-connections`
   - Verificar tamanhos de buffer do lado servidor

3. **Perda de pacotes UDP**
   - Reduzir taxa ou tamanho de mensagem
   - Ajustar tamanho do buffer UDP do kernel

## Desenvolvimento

### Compilação

```bash
# Compilar cliente
go build -o rate-limit-client main.go

# Compilar servidor
go build -o rate-limit-server server/main.go
```

### Testes

```bash
# Executar testes unitários
go test ./...

# Executar benchmarks
go test -bench=. ./sample/...
```

### Adicionando Novos Algoritmos

1. Criar novo pacote no diretório `sample/`
2. Implementar interface `RateLimiter`:
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. Adicionar testes e benchmarks

## Licença

Licença MIT

## Contribuindo

Pull requests são bem-vindos. Para mudanças importantes, por favor abra uma issue primeiro para discutir o que você gostaria de mudar.

## Referências

- [Algoritmos de Rate Limiting](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [Algoritmo Token Bucket](https://en.wikipedia.org/wiki/Token_bucket)
- [Contador de Janela Deslizante](https://blog.logrocket.com/rate-limiting-go-application/)