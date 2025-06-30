# Framework de Pruebas de Limitación de Velocidad

Un framework completo en Go para implementar y probar algoritmos de limitación de velocidad. Este proyecto proporciona implementaciones de muestra de varias estrategias de limitación junto con herramientas cliente-servidor para validación y benchmarking.

## Resumen

Este proyecto consta de:

- **Cliente de Prueba**: Envía solicitudes a velocidades configurables
- **Servidor de Prueba**: Actúa como servidor eco
- **Ejemplos de Algoritmos**: 4 implementaciones diferentes de limitación de velocidad

## Inicio Rápido

### Uso Básico

1. Iniciar el servidor:
```bash
go run server/main.go -protocol tcp -port 8080
```

2. En otra terminal, iniciar el cliente:
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### Ejemplos

```bash
# Iniciar servidor TCP con registro detallado
go run server/main.go -protocol tcp -port 8080 -verbose

# Prueba de alta velocidad (1000 req/s, 10 conexiones)
go run main.go -rate 1000 -connections 10 -duration 60s

# Prueba con protocolo UDP
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## Arquitectura

### Estructura de Directorios

```
.
├── main.go                    # Cliente de prueba
├── server/
│   └── main.go               # Servidor de prueba
└── sample/
    ├── token_bucket/         # Implementación token bucket
    ├── fixed_window/         # Implementación ventana fija
    ├── sliding_window/       # Implementación ventana deslizante
    └── concurrent/           # Implementación thread-safe
```

## Detalles de Componentes

### Cliente de Prueba (main.go)

Un cliente de prueba de limitación de velocidad de alto rendimiento.

**Características:**
- Soporte de protocolos TCP/UDP
- Múltiples conexiones concurrentes (solo TCP)
- Tamaño de mensaje personalizable
- Visualización de estadísticas en tiempo real

**Opciones de Línea de Comandos:**
```bash
-server string     # Dirección del servidor (por defecto "localhost:8080")
-protocol string   # Protocolo: tcp o udp (por defecto "tcp")
-rate int         # Mensajes por segundo (por defecto 100)
-duration duration # Duración de la prueba (por defecto 10s)
-connections int   # Conexiones concurrentes, solo TCP (por defecto 1)
-size int         # Tamaño del mensaje en bytes (por defecto 64)
```

**Ejemplo de Salida:**
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

### Servidor de Prueba (server/main.go)

Un servidor simple que devuelve eco de los mensajes recibidos.

**Características:**
- Soporte de protocolos TCP/UDP
- Conexiones simultáneas de múltiples clientes
- Visualización de estadísticas cada 5 segundos
- Apagado elegante (Ctrl+C)

**Opciones de Línea de Comandos:**
```bash
-protocol string  # Protocolo: tcp o udp (por defecto "tcp")
-port int        # Puerto de escucha (por defecto 8080)
-verbose         # Habilitar registro detallado
```

**Ejemplo de Visualización de Estadísticas:**
```
[15:30:45] Received: 5023, Processed: 5023, Errors: 0, Rate: 1004.60 msg/s
[15:30:50] Received: 10089, Processed: 10089, Errors: 0, Rate: 1013.20 msg/s
```

## Algoritmos de Limitación de Velocidad

### 1. Token Bucket

**Características:**
- Permite procesamiento en ráfaga
- Mantiene velocidad promedio permitiendo picos a corto plazo
- Eficiente en memoria

**Ejemplo de Uso:**
```go
limiter := NewTokenBucket(100, 10) // Capacidad 100, recarga 10/segundo

if limiter.Allow() {
    // Procesar solicitud
}
```

**Casos de Uso:**
- Limitación de velocidad de API
- Control de ancho de banda
- Cuando el tráfico en ráfaga es aceptable

### 2. Ventana Fija (Fixed Window)

**Características:**
- Implementación simple
- Uso mínimo de memoria
- Problemas en los límites de ventana

**Ejemplo de Uso:**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // 1000 solicitudes por minuto

if limiter.Allow() {
    // Procesar solicitud
}
```

**Casos de Uso:**
- Cuando se necesita limitación simple
- Se prioriza rendimiento sobre precisión

### 3. Ventana Deslizante (Sliding Window)

**Características:**
- Limitación más precisa
- Resuelve problemas de límites de ventana fija
- Uso de memoria ligeramente mayor

**Ejemplo de Uso:**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // 100 solicitudes por minuto

if limiter.Allow() {
    // Procesar solicitud
}
```

**Casos de Uso:**
- Cuando se requiere limitación precisa
- La equidad es importante

### 4. Implementación Concurrente

**Características:**
- Alto rendimiento con operaciones atómicas
- Soporte de entorno distribuido
- Ejemplo de middleware HTTP

**Uso del Middleware HTTP:**
```go
// Limitación por usuario
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // 100 solicitudes/minuto por usuario
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// Aplicar middleware
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## Benchmarks de Rendimiento

Comparación de algoritmos (valores de referencia):

| Algoritmo | Rendimiento | Uso de Memoria | Características |
|-----------|-------------|----------------|-----------------|
| Token Bucket | Alto | Bajo | Soporte ráfaga |
| Ventana Fija | Muy alto | Muy bajo | Simple |
| Ventana Deslizante | Medio | Medio | Preciso |
| Concurrente | Alto | Bajo | Optimizado concurrencia |

## Casos de Uso Prácticos

### 1. Prueba de Limitación de API

```bash
# Probar limitación del servidor API (1000 req/s, 5 minutos)
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. Escenarios de Prueba de Carga

```bash
# Prueba de aumento gradual de carga
for rate in 100 500 1000 2000 5000; do
    echo "Testing rate: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. Medición de Ancho de Banda

```bash
# Medición con mensajes grandes
go run main.go -size 1024 -rate 1000 -protocol udp
```

## Solución de Problemas

### Problemas Comunes

1. **Error "Too many open files"**
   ```bash
   # Aumentar límite de descriptores de archivo
   ulimit -n 65536
   ```

2. **Errores de conexión a alta velocidad**
   - Ajustar parámetro `-connections`
   - Verificar tamaños de búfer del servidor

3. **Pérdida de paquetes UDP**
   - Reducir velocidad o tamaño de mensaje
   - Ajustar tamaño del búfer UDP del kernel

## Desarrollo

### Compilación

```bash
# Compilar cliente
go build -o rate-limit-client main.go

# Compilar servidor
go build -o rate-limit-server server/main.go
```

### Pruebas

```bash
# Ejecutar pruebas unitarias
go test ./...

# Ejecutar benchmarks
go test -bench=. ./sample/...
```

### Agregar Nuevos Algoritmos

1. Crear nuevo paquete en directorio `sample/`
2. Implementar interfaz `RateLimiter`:
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. Agregar pruebas y benchmarks

## Licencia

Licencia MIT

## Contribuciones

Las pull requests son bienvenidas. Para cambios importantes, primero abra un issue para discutir lo que desea cambiar.

## Referencias

- [Algoritmos de Limitación de Velocidad](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [Algoritmo Token Bucket](https://es.wikipedia.org/wiki/Token_bucket)
- [Contador de Ventana Deslizante](https://blog.logrocket.com/rate-limiting-go-application/)