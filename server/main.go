package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Config struct {
	Protocol string
	Port     int
	Verbose  bool
}

type Stats struct {
	Received   int64
	Processed  int64
	Errors     int64
	StartTime  time.Time
	mu         sync.Mutex
	LastPrint  time.Time
}

func main() {
	config := parseFlags()
	
	fmt.Printf("Starting rate limit test server\n")
	fmt.Printf("Protocol: %s\n", config.Protocol)
	fmt.Printf("Port: %d\n\n", config.Port)
	
	stats := &Stats{
		StartTime: time.Now(),
		LastPrint: time.Now(),
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	var wg sync.WaitGroup
	
	// Start stats printer
	wg.Add(1)
	go func() {
		defer wg.Done()
		statsPrinter(ctx, stats)
	}()
	
	// Start server
	wg.Add(1)
	go func() {
		defer wg.Done()
		switch config.Protocol {
		case "tcp":
			runTCPServer(ctx, config, stats)
		case "udp":
			runUDPServer(ctx, config, stats)
		default:
			log.Fatalf("Invalid protocol: %s", config.Protocol)
		}
	}()
	
	// Wait for signal
	<-sigChan
	fmt.Println("\nShutting down server...")
	cancel()
	
	// Wait for graceful shutdown
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		fmt.Println("Server shut down gracefully")
	case <-time.After(5 * time.Second):
		fmt.Println("Shutdown timeout exceeded")
	}
	
	printFinalStats(stats)
}

func parseFlags() *Config {
	config := &Config{}
	
	flag.StringVar(&config.Protocol, "protocol", "tcp", "Protocol (tcp or udp)")
	flag.IntVar(&config.Port, "port", 8080, "Port to listen on")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()
	
	return config
}

func runTCPServer(ctx context.Context, config *Config, stats *Stats) {
	addr := fmt.Sprintf(":%d", config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}
	defer listener.Close()
	
	fmt.Printf("TCP server listening on %s\n", addr)
	
	// Accept connections in a separate goroutine
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("Accept error: %v", err)
					continue
				}
			}
			
			go handleTCPConnection(ctx, conn, config, stats)
		}
	}()
	
	<-ctx.Done()
}

func handleTCPConnection(ctx context.Context, conn net.Conn, config *Config, stats *Stats) {
	defer conn.Close()
	
	if config.Verbose {
		log.Printf("New TCP connection from %s", conn.RemoteAddr())
	}
	
	buf := make([]byte, 65536)
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if err.Error() != "EOF" && config.Verbose {
					log.Printf("Read error from %s: %v", conn.RemoteAddr(), err)
				}
				return
			}
			
			atomic.AddInt64(&stats.Received, 1)
			
			// Echo back
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			_, err = conn.Write(buf[:n])
			if err != nil {
				atomic.AddInt64(&stats.Errors, 1)
				if config.Verbose {
					log.Printf("Write error to %s: %v", conn.RemoteAddr(), err)
				}
				return
			}
			
			atomic.AddInt64(&stats.Processed, 1)
		}
	}
}

func runUDPServer(ctx context.Context, config *Config, stats *Stats) {
	addr := fmt.Sprintf(":%d", config.Port)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}
	
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}
	defer conn.Close()
	
	fmt.Printf("UDP server listening on %s\n", addr)
	
	buf := make([]byte, 65536)
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, clientAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				atomic.AddInt64(&stats.Errors, 1)
				if config.Verbose {
					log.Printf("UDP read error: %v", err)
				}
				continue
			}
			
			atomic.AddInt64(&stats.Received, 1)
			
			// Echo back
			_, err = conn.WriteToUDP(buf[:n], clientAddr)
			if err != nil {
				atomic.AddInt64(&stats.Errors, 1)
				if config.Verbose {
					log.Printf("UDP write error to %s: %v", clientAddr, err)
				}
				continue
			}
			
			atomic.AddInt64(&stats.Processed, 1)
		}
	}
}

func statsPrinter(ctx context.Context, stats *Stats) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			printCurrentStats(stats)
		}
	}
}

func printCurrentStats(stats *Stats) {
	received := atomic.LoadInt64(&stats.Received)
	processed := atomic.LoadInt64(&stats.Processed)
	errors := atomic.LoadInt64(&stats.Errors)
	
	stats.mu.Lock()
	now := time.Now()
	duration := now.Sub(stats.LastPrint)
	stats.LastPrint = now
	stats.mu.Unlock()
	
	rate := float64(received) / duration.Seconds()
	
	fmt.Printf("[%s] Received: %d, Processed: %d, Errors: %d, Rate: %.2f msg/s\n",
		time.Now().Format("15:04:05"),
		received, processed, errors, rate)
}

func printFinalStats(stats *Stats) {
	duration := time.Since(stats.StartTime)
	received := atomic.LoadInt64(&stats.Received)
	processed := atomic.LoadInt64(&stats.Processed)
	errors := atomic.LoadInt64(&stats.Errors)
	
	fmt.Println("\n--- Final Statistics ---")
	fmt.Printf("Total duration: %s\n", duration.Round(time.Millisecond))
	fmt.Printf("Messages received: %d\n", received)
	fmt.Printf("Messages processed: %d\n", processed)
	fmt.Printf("Errors: %d\n", errors)
	fmt.Printf("Success rate: %.2f%%\n", float64(processed)/float64(received)*100)
	fmt.Printf("Average rate: %.2f messages/second\n", float64(received)/duration.Seconds())
}