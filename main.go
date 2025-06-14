package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	ServerAddr   string
	Protocol     string
	Rate         int
	Duration     time.Duration
	Connections  int
	MessageSize  int
}

type Stats struct {
	Sent      int64
	Succeeded int64
	Failed    int64
	StartTime time.Time
}

func main() {
	config := parseFlags()
	
	fmt.Printf("Starting rate limit test client\n")
	fmt.Printf("Protocol: %s\n", config.Protocol)
	fmt.Printf("Server: %s\n", config.ServerAddr)
	fmt.Printf("Rate: %d messages/second\n", config.Rate)
	fmt.Printf("Duration: %s\n", config.Duration)
	fmt.Printf("Connections: %d\n", config.Connections)
	fmt.Printf("Message size: %d bytes\n\n", config.MessageSize)
	
	stats := &Stats{StartTime: time.Now()}
	
	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)
	defer cancel()
	
	switch config.Protocol {
	case "tcp":
		runTCPTest(ctx, config, stats)
	case "udp":
		runUDPTest(ctx, config, stats)
	default:
		log.Fatalf("Invalid protocol: %s", config.Protocol)
	}
	
	printStats(stats)
}

func parseFlags() *Config {
	config := &Config{}
	
	flag.StringVar(&config.ServerAddr, "server", "localhost:8080", "Server address")
	flag.StringVar(&config.Protocol, "protocol", "tcp", "Protocol (tcp or udp)")
	flag.IntVar(&config.Rate, "rate", 100, "Messages per second")
	flag.DurationVar(&config.Duration, "duration", 10*time.Second, "Test duration")
	flag.IntVar(&config.Connections, "connections", 1, "Number of concurrent connections (TCP only)")
	flag.IntVar(&config.MessageSize, "size", 64, "Message size in bytes")
	flag.Parse()
	
	return config
}

func runTCPTest(ctx context.Context, config *Config, stats *Stats) {
	var wg sync.WaitGroup
	
	for i := 0; i < config.Connections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tcpWorker(ctx, id, config, stats)
		}(i)
	}
	
	wg.Wait()
}

func tcpWorker(ctx context.Context, id int, config *Config, stats *Stats) {
	conn, err := net.Dial("tcp", config.ServerAddr)
	if err != nil {
		log.Printf("Worker %d: Failed to connect: %v", id, err)
		return
	}
	defer conn.Close()
	
	message := make([]byte, config.MessageSize)
	for i := range message {
		message[i] = byte('A' + (i % 26))
	}
	
	ticker := time.NewTicker(time.Second / time.Duration(config.Rate/config.Connections))
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			atomic.AddInt64(&stats.Sent, 1)
			
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			_, err := conn.Write(message)
			if err != nil {
				atomic.AddInt64(&stats.Failed, 1)
				log.Printf("Worker %d: Write error: %v", id, err)
				continue
			}
			
			buf := make([]byte, 1024)
			conn.SetReadDeadline(time.Now().Add(time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				atomic.AddInt64(&stats.Failed, 1)
				log.Printf("Worker %d: Read error: %v", id, err)
				continue
			}
			
			if n > 0 {
				atomic.AddInt64(&stats.Succeeded, 1)
			}
		}
	}
}

func runUDPTest(ctx context.Context, config *Config, stats *Stats) {
	conn, err := net.Dial("udp", config.ServerAddr)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	
	message := make([]byte, config.MessageSize)
	for i := range message {
		message[i] = byte('A' + (i % 26))
	}
	
	var wg sync.WaitGroup
	
	// Sender
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Second / time.Duration(config.Rate))
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				atomic.AddInt64(&stats.Sent, 1)
				_, err := conn.Write(message)
				if err != nil {
					atomic.AddInt64(&stats.Failed, 1)
					log.Printf("UDP write error: %v", err)
				}
			}
		}
	}()
	
	// Receiver
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				n, err := conn.Read(buf)
				if err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						continue
					}
					log.Printf("UDP read error: %v", err)
					continue
				}
				if n > 0 {
					atomic.AddInt64(&stats.Succeeded, 1)
				}
			}
		}
	}()
	
	wg.Wait()
}

func printStats(stats *Stats) {
	duration := time.Since(stats.StartTime)
	sent := atomic.LoadInt64(&stats.Sent)
	succeeded := atomic.LoadInt64(&stats.Succeeded)
	failed := atomic.LoadInt64(&stats.Failed)
	
	fmt.Println("\n--- Test Statistics ---")
	fmt.Printf("Duration: %s\n", duration.Round(time.Millisecond))
	fmt.Printf("Messages sent: %d\n", sent)
	fmt.Printf("Messages succeeded: %d\n", succeeded)
	fmt.Printf("Messages failed: %d\n", failed)
	fmt.Printf("Success rate: %.2f%%\n", float64(succeeded)/float64(sent)*100)
	fmt.Printf("Actual rate: %.2f messages/second\n", float64(sent)/duration.Seconds())
}