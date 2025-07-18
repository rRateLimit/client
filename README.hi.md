# रेट लिमिट टेस्टिंग फ्रेमवर्क

रेट लिमिटिंग एल्गोरिदम को लागू करने और परीक्षण करने के लिए एक व्यापक Go फ्रेमवर्क। यह प्रोजेक्ट विभिन्न रेट लिमिटिंग रणनीतियों के नमूना कार्यान्वयन के साथ-साथ सत्यापन और बेंचमार्किंग के लिए क्लाइंट-सर्वर टूल प्रदान करता है।

## अवलोकन

इस प्रोजेक्ट में शामिल है:

- **टेस्ट क्लाइंट**: कॉन्फ़िगर करने योग्य दरों पर अनुरोध भेजता है
- **टेस्ट सर्वर**: एक इको सर्वर के रूप में कार्य करता है
- **एल्गोरिदम उदाहरण**: 4 विभिन्न रेट लिमिटिंग कार्यान्वयन

## त्वरित शुरुआत

### बुनियादी उपयोग

1. सर्वर शुरू करें:
```bash
go run server/main.go -protocol tcp -port 8080
```

2. दूसरे टर्मिनल में, क्लाइंट शुरू करें:
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### उदाहरण

```bash
# विस्तृत लॉगिंग के साथ TCP सर्वर शुरू करें
go run server/main.go -protocol tcp -port 8080 -verbose

# उच्च-दर परीक्षण (1000 req/s, 10 कनेक्शन)
go run main.go -rate 1000 -connections 10 -duration 60s

# UDP प्रोटोकॉल परीक्षण
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## आर्किटेक्चर

### डायरेक्टरी संरचना

```
.
├── main.go                    # टेस्ट क्लाइंट
├── server/
│   └── main.go               # टेस्ट सर्वर
└── sample/
    ├── token_bucket/         # टोकन बकेट कार्यान्वयन
    ├── fixed_window/         # फिक्स्ड विंडो कार्यान्वयन
    ├── sliding_window/       # स्लाइडिंग विंडो कार्यान्वयन
    └── concurrent/           # समकालिक-सुरक्षित कार्यान्वयन
```

## घटक विवरण

### टेस्ट क्लाइंट (main.go)

एक उच्च-प्रदर्शन रेट लिमिटिंग टेस्ट क्लाइंट।

**विशेषताएं:**
- TCP/UDP प्रोटोकॉल समर्थन
- एकाधिक समकालिक कनेक्शन (केवल TCP)
- अनुकूलन योग्य संदेश आकार
- वास्तविक समय सांख्यिकी प्रदर्शन

**कमांड-लाइन विकल्प:**
```bash
-server string     # सर्वर पता (डिफ़ॉल्ट "localhost:8080")
-protocol string   # प्रोटोकॉल: tcp या udp (डिफ़ॉल्ट "tcp")
-rate int         # प्रति सेकंड संदेश (डिफ़ॉल्ट 100)
-duration duration # परीक्षण अवधि (डिफ़ॉल्ट 10s)
-connections int   # समकालिक कनेक्शन, केवल TCP (डिफ़ॉल्ट 1)
-size int         # बाइट्स में संदेश आकार (डिफ़ॉल्ट 64)
```

**आउटपुट उदाहरण:**
```
रेट लिमिट टेस्ट क्लाइंट शुरू हो रहा है
प्रोटोकॉल: tcp
सर्वर: localhost:8080
दर: 1000 संदेश/सेकंड
अवधि: 30s
कनेक्शन: 10
संदेश आकार: 64 बाइट्स

--- परीक्षण सांख्यिकी ---
अवधि: 30s
भेजे गए संदेश: 30000
सफल संदेश: 29850
असफल संदेश: 150
सफलता दर: 99.50%
वास्तविक दर: 1000.00 संदेश/सेकंड
```

### टेस्ट सर्वर (server/main.go)

एक सरल सर्वर जो प्राप्त संदेशों को वापस भेजता है।

**विशेषताएं:**
- TCP/UDP प्रोटोकॉल समर्थन
- एकाधिक समकालिक क्लाइंट कनेक्शन
- हर 5 सेकंड में सांख्यिकी प्रदर्शन
- सुचारू शटडाउन (Ctrl+C)

**कमांड-लाइन विकल्प:**
```bash
-protocol string  # प्रोटोकॉल: tcp या udp (डिफ़ॉल्ट "tcp")
-port int        # सुनने का पोर्ट (डिफ़ॉल्ट 8080)
-verbose         # विस्तृत लॉगिंग सक्षम करें
```

**सांख्यिकी प्रदर्शन उदाहरण:**
```
[15:30:45] प्राप्त: 5023, संसाधित: 5023, त्रुटियां: 0, दर: 1004.60 msg/s
[15:30:50] प्राप्त: 10089, संसाधित: 10089, त्रुटियां: 0, दर: 1013.20 msg/s
```

## रेट लिमिटिंग एल्गोरिदम

### 1. टोकन बकेट

**विशेषताएं:**
- बर्स्ट प्रोसेसिंग की अनुमति देता है
- अल्पकालिक स्पाइक्स की अनुमति देते हुए औसत दर बनाए रखता है
- मेमोरी कुशल

**उपयोग उदाहरण:**
```go
limiter := NewTokenBucket(100, 10) // क्षमता 100, रीफिल 10/सेकंड

if limiter.Allow() {
    // अनुरोध संसाधित करें
}
```

**उपयोग के मामले:**
- API रेट लिमिटिंग
- नेटवर्क बैंडविड्थ नियंत्रण
- जब बर्स्ट ट्रैफिक स्वीकार्य हो

### 2. फिक्स्ड विंडो

**विशेषताएं:**
- सरल कार्यान्वयन
- न्यूनतम मेमोरी उपयोग
- विंडो किनारों पर सीमा समस्याएं

**उपयोग उदाहरण:**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // प्रति मिनट 1000 अनुरोध

if limiter.Allow() {
    // अनुरोध संसाधित करें
}
```

**उपयोग के मामले:**
- जब सरल रेट लिमिटिंग की आवश्यकता हो
- प्रदर्शन को सटीकता से अधिक प्राथमिकता दी जाती है

### 3. स्लाइडिंग विंडो

**विशेषताएं:**
- अधिक सटीक रेट लिमिटिंग
- फिक्स्ड विंडो सीमा समस्याओं को हल करता है
- थोड़ा अधिक मेमोरी उपयोग

**उपयोग उदाहरण:**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // प्रति मिनट 100 अनुरोध

if limiter.Allow() {
    // अनुरोध संसाधित करें
}
```

**उपयोग के मामले:**
- जब सटीक रेट लिमिटिंग आवश्यक हो
- निष्पक्षता महत्वपूर्ण है

### 4. समकालिक कार्यान्वयन

**विशेषताएं:**
- परमाणु संचालन के साथ उच्च प्रदर्शन
- वितरित वातावरण समर्थन
- HTTP मिडलवेयर उदाहरण

**HTTP मिडलवेयर उपयोग:**
```go
// प्रति-उपयोगकर्ता रेट लिमिटिंग
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // प्रति उपयोगकर्ता 100 अनुरोध/मिनट
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// मिडलवेयर लागू करें
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## प्रदर्शन बेंचमार्क

एल्गोरिदम तुलना (संदर्भ मान):

| एल्गोरिदम | थ्रूपुट | मेमोरी उपयोग | विशेषताएं |
|-----------|---------|--------------|-----------|
| टोकन बकेट | उच्च | कम | बर्स्ट समर्थन |
| फिक्स्ड विंडो | सर्वोच्च | सबसे कम | सरल |
| स्लाइडिंग विंडो | मध्यम | मध्यम | सटीक |
| समकालिक | उच्च | कम | समकालिकता के लिए अनुकूलित |

## व्यावहारिक उपयोग के मामले

### 1. API सर्वर रेट लिमिट परीक्षण

```bash
# API सर्वर रेट लिमिटिंग का परीक्षण करें (1000 req/s, 5 मिनट)
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. लोड टेस्ट परिदृश्य

```bash
# क्रमिक लोड वृद्धि परीक्षण
for rate in 100 500 1000 2000 5000; do
    echo "परीक्षण दर: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. नेटवर्क बैंडविड्थ मापन

```bash
# बड़े संदेश आकार के साथ बैंडविड्थ मापन
go run main.go -size 1024 -rate 1000 -protocol udp
```

## समस्या निवारण

### सामान्य समस्याएं

1. **"Too many open files" त्रुटि**
   ```bash
   # फ़ाइल डिस्क्रिप्टर सीमा बढ़ाएं
   ulimit -n 65536
   ```

2. **उच्च दरों पर कनेक्शन त्रुटियां**
   - `-connections` पैरामीटर समायोजित करें
   - सर्वर-साइड बफ़र आकार की जांच करें

3. **UDP पैकेट हानि**
   - दर या संदेश आकार कम करें
   - कर्नेल UDP बफ़र आकार समायोजित करें

## विकास

### बिल्डिंग

```bash
# क्लाइंट बनाएं
go build -o rate-limit-client main.go

# सर्वर बनाएं
go build -o rate-limit-server server/main.go
```

### परीक्षण

```bash
# यूनिट टेस्ट चलाएं
go test ./...

# बेंचमार्क चलाएं
go test -bench=. ./sample/...
```

### नए एल्गोरिदम जोड़ना

1. `sample/` डायरेक्टरी में नया पैकेज बनाएं
2. `RateLimiter` इंटरफ़ेस लागू करें:
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. परीक्षण और बेंचमार्क जोड़ें

## लाइसेंस

MIT लाइसेंस

## योगदान

पुल अनुरोध का स्वागत है। बड़े परिवर्तनों के लिए, कृपया पहले एक इश्यू खोलें जिसमें आप चर्चा करें कि आप क्या बदलना चाहते हैं।

## संदर्भ

- [रेट लिमिटिंग एल्गोरिदम](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [टोकन बकेट एल्गोरिदम](https://en.wikipedia.org/wiki/Token_bucket)
- [स्लाइडिंग विंडो काउंटर](https://blog.logrocket.com/rate-limiting-go-application/)