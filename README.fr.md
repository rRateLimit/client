# Framework de Test de Limitation de Débit

Un framework Go complet pour implémenter et tester des algorithmes de limitation de débit. Ce projet fournit des exemples d'implémentation de diverses stratégies de limitation de débit ainsi que des outils client-serveur pour la validation et le benchmarking.

## Vue d'ensemble

Ce projet comprend :

- **Client de Test** : Envoie des requêtes à des taux configurables
- **Serveur de Test** : Agit comme un serveur echo
- **Exemples d'Algorithmes** : 4 implémentations différentes de limitation de débit

## Démarrage Rapide

### Utilisation de Base

1. Démarrer le serveur :
```bash
go run server/main.go -protocol tcp -port 8080
```

2. Dans un autre terminal, démarrer le client :
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### Exemples

```bash
# Démarrer le serveur TCP avec journalisation détaillée
go run server/main.go -protocol tcp -port 8080 -verbose

# Test à haut débit (1000 req/s, 10 connexions)
go run main.go -rate 1000 -connections 10 -duration 60s

# Test avec protocole UDP
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## Architecture

### Structure des Répertoires

```
.
├── main.go                    # Client de test
├── server/
│   └── main.go               # Serveur de test
└── sample/
    ├── token_bucket/         # Implémentation seau à jetons
    ├── fixed_window/         # Implémentation fenêtre fixe
    ├── sliding_window/       # Implémentation fenêtre glissante
    └── concurrent/           # Implémentation thread-safe
```

## Détails des Composants

### Client de Test (main.go)

Un client de test de limitation de débit haute performance.

**Fonctionnalités :**
- Support des protocoles TCP/UDP
- Connexions concurrentes multiples (TCP uniquement)
- Taille de message personnalisable
- Affichage des statistiques en temps réel

**Options de Ligne de Commande :**
```bash
-server string     # Adresse du serveur (défaut "localhost:8080")
-protocol string   # Protocole : tcp ou udp (défaut "tcp")
-rate int         # Messages par seconde (défaut 100)
-duration duration # Durée du test (défaut 10s)
-connections int   # Connexions concurrentes, TCP uniquement (défaut 1)
-size int         # Taille du message en octets (défaut 64)
```

**Exemple de Sortie :**
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

### Serveur de Test (server/main.go)

Un serveur simple qui renvoie en écho les messages reçus.

**Fonctionnalités :**
- Support des protocoles TCP/UDP
- Connexions simultanées de plusieurs clients
- Affichage des statistiques toutes les 5 secondes
- Arrêt gracieux (Ctrl+C)

**Options de Ligne de Commande :**
```bash
-protocol string  # Protocole : tcp ou udp (défaut "tcp")
-port int        # Port d'écoute (défaut 8080)
-verbose         # Activer la journalisation détaillée
```

**Exemple d'Affichage des Statistiques :**
```
[15:30:45] Received: 5023, Processed: 5023, Errors: 0, Rate: 1004.60 msg/s
[15:30:50] Received: 10089, Processed: 10089, Errors: 0, Rate: 1013.20 msg/s
```

## Algorithmes de Limitation de Débit

### 1. Seau à Jetons (Token Bucket)

**Caractéristiques :**
- Permet le traitement en rafale
- Maintient un débit moyen tout en permettant des pics à court terme
- Efficace en mémoire

**Exemple d'Utilisation :**
```go
limiter := NewTokenBucket(100, 10) // Capacité 100, remplissage 10/seconde

if limiter.Allow() {
    // Traiter la requête
}
```

**Cas d'Usage :**
- Limitation de débit d'API
- Contrôle de bande passante réseau
- Quand le trafic en rafale est acceptable

### 2. Fenêtre Fixe (Fixed Window)

**Caractéristiques :**
- Implémentation simple
- Utilisation minimale de la mémoire
- Problèmes aux limites de fenêtre

**Exemple d'Utilisation :**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // 1000 requêtes par minute

if limiter.Allow() {
    // Traiter la requête
}
```

**Cas d'Usage :**
- Quand une limitation de débit simple est nécessaire
- La performance est prioritaire sur la précision

### 3. Fenêtre Glissante (Sliding Window)

**Caractéristiques :**
- Limitation de débit plus précise
- Résout les problèmes de limites de fenêtre fixe
- Utilisation de mémoire légèrement supérieure

**Exemple d'Utilisation :**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // 100 requêtes par minute

if limiter.Allow() {
    // Traiter la requête
}
```

**Cas d'Usage :**
- Quand une limitation de débit précise est requise
- L'équité est importante

### 4. Implémentation Concurrente

**Caractéristiques :**
- Haute performance avec opérations atomiques
- Support d'environnement distribué
- Exemple de middleware HTTP

**Utilisation du Middleware HTTP :**
```go
// Limitation de débit par utilisateur
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // 100 requêtes/minute par utilisateur
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// Appliquer le middleware
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## Benchmarks de Performance

Comparaison des algorithmes (valeurs de référence) :

| Algorithme | Débit | Utilisation Mémoire | Caractéristiques |
|------------|-------|---------------------|------------------|
| Seau à Jetons | Élevé | Faible | Support rafale |
| Fenêtre Fixe | Très élevé | Très faible | Simple |
| Fenêtre Glissante | Moyen | Moyenne | Précis |
| Concurrent | Élevé | Faible | Optimisé concurrence |

## Cas d'Usage Pratiques

### 1. Test de Limitation de Débit de Serveur API

```bash
# Tester la limitation de débit du serveur API (1000 req/s, 5 minutes)
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. Scénarios de Test de Charge

```bash
# Test d'augmentation graduelle de charge
for rate in 100 500 1000 2000 5000; do
    echo "Testing rate: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. Mesure de Bande Passante Réseau

```bash
# Mesure de bande passante avec grande taille de message
go run main.go -size 1024 -rate 1000 -protocol udp
```

## Dépannage

### Problèmes Courants

1. **Erreur "Too many open files"**
   ```bash
   # Augmenter la limite de descripteurs de fichiers
   ulimit -n 65536
   ```

2. **Erreurs de connexion à débit élevé**
   - Ajuster le paramètre `-connections`
   - Vérifier les tailles de tampon côté serveur

3. **Perte de paquets UDP**
   - Réduire le débit ou la taille des messages
   - Ajuster la taille du tampon UDP du noyau

## Développement

### Compilation

```bash
# Compiler le client
go build -o rate-limit-client main.go

# Compiler le serveur
go build -o rate-limit-server server/main.go
```

### Tests

```bash
# Exécuter les tests unitaires
go test ./...

# Exécuter les benchmarks
go test -bench=. ./sample/...
```

### Ajouter de Nouveaux Algorithmes

1. Créer un nouveau package dans le répertoire `sample/`
2. Implémenter l'interface `RateLimiter` :
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. Ajouter des tests et des benchmarks

## Licence

Licence MIT

## Contributions

Les pull requests sont les bienvenues. Pour des changements majeurs, veuillez d'abord ouvrir une issue pour discuter de ce que vous souhaitez modifier.

## Références

- [Algorithmes de Limitation de Débit](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [Algorithme du Seau à Jetons](https://fr.wikipedia.org/wiki/Seau_%C3%A0_jetons)
- [Compteur à Fenêtre Glissante](https://blog.logrocket.com/rate-limiting-go-application/)