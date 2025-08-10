package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rRateLimit/client/ratelimit"
)

// Worker represents a concurrent worker
type Worker struct {
	ID      int
	Limiter ratelimit.Limiter
	Jobs    <-chan Job
	Results chan<- Result
	wg      *sync.WaitGroup
}

// Job represents a work item
type Job struct {
	ID   int
	Data string
}

// Result represents the result of processing a job
type Result struct {
	JobID      int
	WorkerID   int
	Success    bool
	Duration   time.Duration
	RateLimited bool
}

// Statistics tracks overall performance
type Statistics struct {
	TotalJobs       int64
	SuccessfulJobs  int64
	RateLimitedJobs int64
	FailedJobs      int64
	TotalDuration   time.Duration
	StartTime       time.Time
}

func main() {
	fmt.Println("=== Concurrent Rate Limiting Example ===\n")

	// Run different scenarios
	scenario1_SharedLimiter()
	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")
	
	scenario2_PerWorkerLimiter()
	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")
	
	scenario3_PriorityQueues()
}

// Scenario 1: Multiple workers sharing a single rate limiter
func scenario1_SharedLimiter() {
	fmt.Println("--- Scenario 1: Shared Rate Limiter ---")
	fmt.Println("10 workers sharing a single limiter (30 req/sec)")
	
	// Create a shared rate limiter
	sharedLimiter := ratelimit.NewTokenBucket(
		ratelimit.WithRate(30),
		ratelimit.WithPeriod(time.Second),
		ratelimit.WithBurst(10),
	)
	
	// Create channels
	jobs := make(chan Job, 100)
	results := make(chan Result, 100)
	
	// Start workers
	var wg sync.WaitGroup
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		worker := &Worker{
			ID:      i,
			Limiter: sharedLimiter,
			Jobs:    jobs,
			Results: results,
			wg:      &wg,
		}
		go worker.Run()
	}
	
	// Generate jobs
	go func() {
		for i := 1; i <= 100; i++ {
			jobs <- Job{
				ID:   i,
				Data: fmt.Sprintf("Job-%d", i),
			}
		}
		close(jobs)
	}()
	
	// Collect results
	stats := &Statistics{StartTime: time.Now()}
	go collectResults(results, stats)
	
	// Wait for completion
	wg.Wait()
	close(results)
	time.Sleep(100 * time.Millisecond) // Allow result collection to finish
	
	// Print statistics
	printStatistics(stats)
}

// Scenario 2: Each worker has its own rate limiter
func scenario2_PerWorkerLimiter() {
	fmt.Println("--- Scenario 2: Per-Worker Rate Limiters ---")
	fmt.Println("5 workers, each with own limiter (10 req/sec)")
	
	// Create channels
	jobs := make(chan Job, 100)
	results := make(chan Result, 100)
	
	// Start workers with individual limiters
	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		
		// Each worker gets its own limiter
		workerLimiter := ratelimit.NewFixedWindow(
			ratelimit.WithRate(10),
			ratelimit.WithPeriod(time.Second),
		)
		
		worker := &Worker{
			ID:      i,
			Limiter: workerLimiter,
			Jobs:    jobs,
			Results: results,
			wg:      &wg,
		}
		go worker.Run()
	}
	
	// Generate jobs
	go func() {
		for i := 1; i <= 100; i++ {
			jobs <- Job{
				ID:   i,
				Data: fmt.Sprintf("Job-%d", i),
			}
		}
		close(jobs)
	}()
	
	// Collect results
	stats := &Statistics{StartTime: time.Now()}
	go collectResults(results, stats)
	
	// Wait for completion
	wg.Wait()
	close(results)
	time.Sleep(100 * time.Millisecond)
	
	// Print statistics
	printStatistics(stats)
}

// Scenario 3: Priority-based rate limiting
func scenario3_PriorityQueues() {
	fmt.Println("--- Scenario 3: Priority-Based Rate Limiting ---")
	fmt.Println("High priority: 20 req/sec, Low priority: 5 req/sec")
	
	// Create separate limiters for different priorities
	highPriorityLimiter := ratelimit.NewSlidingWindow(
		ratelimit.WithRate(20),
		ratelimit.WithPeriod(time.Second),
	)
	
	lowPriorityLimiter := ratelimit.NewSlidingWindow(
		ratelimit.WithRate(5),
		ratelimit.WithPeriod(time.Second),
	)
	
	// Create priority channels
	highPriorityJobs := make(chan Job, 50)
	lowPriorityJobs := make(chan Job, 50)
	results := make(chan Result, 100)
	
	// Start high priority workers
	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		worker := &Worker{
			ID:      i,
			Limiter: highPriorityLimiter,
			Jobs:    highPriorityJobs,
			Results: results,
			wg:      &wg,
		}
		go worker.RunWithLabel("HIGH")
	}
	
	// Start low priority workers
	for i := 4; i <= 6; i++ {
		wg.Add(1)
		worker := &Worker{
			ID:      i,
			Limiter: lowPriorityLimiter,
			Jobs:    lowPriorityJobs,
			Results: results,
			wg:      &wg,
		}
		go worker.RunWithLabel("LOW")
	}
	
	// Generate mixed priority jobs
	go func() {
		for i := 1; i <= 60; i++ {
			job := Job{
				ID:   i,
				Data: fmt.Sprintf("Job-%d", i),
			}
			
			// 30% high priority, 70% low priority
			if i%10 < 3 {
				highPriorityJobs <- job
			} else {
				lowPriorityJobs <- job
			}
		}
		close(highPriorityJobs)
		close(lowPriorityJobs)
	}()
	
	// Collect results
	stats := &Statistics{StartTime: time.Now()}
	go collectResults(results, stats)
	
	// Wait for completion
	wg.Wait()
	close(results)
	time.Sleep(100 * time.Millisecond)
	
	// Print statistics
	printStatistics(stats)
}

// Worker methods

func (w *Worker) Run() {
	defer w.wg.Done()
	
	for job := range w.Jobs {
		start := time.Now()
		
		// Check rate limit
		if !w.Limiter.Allow() {
			w.Results <- Result{
				JobID:       job.ID,
				WorkerID:    w.ID,
				Success:     false,
				Duration:    time.Since(start),
				RateLimited: true,
			}
			continue
		}
		
		// Process job
		success := w.processJob(job)
		
		w.Results <- Result{
			JobID:       job.ID,
			WorkerID:    w.ID,
			Success:     success,
			Duration:    time.Since(start),
			RateLimited: false,
		}
	}
}

func (w *Worker) RunWithLabel(priority string) {
	defer w.wg.Done()
	
	for job := range w.Jobs {
		start := time.Now()
		
		// Wait for rate limit with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := w.Limiter.Wait(ctx)
		cancel()
		
		if err != nil {
			fmt.Printf("[%s] Worker %d: Job %d rate limited (timeout)\n", 
				priority, w.ID, job.ID)
			w.Results <- Result{
				JobID:       job.ID,
				WorkerID:    w.ID,
				Success:     false,
				Duration:    time.Since(start),
				RateLimited: true,
			}
			continue
		}
		
		// Process job
		success := w.processJob(job)
		fmt.Printf("[%s] Worker %d: Processed job %d\n", priority, w.ID, job.ID)
		
		w.Results <- Result{
			JobID:       job.ID,
			WorkerID:    w.ID,
			Success:     success,
			Duration:    time.Since(start),
			RateLimited: false,
		}
	}
}

func (w *Worker) processJob(job Job) bool {
	// Simulate job processing
	time.Sleep(10 * time.Millisecond)
	return true // Simulate success
}

// Helper functions

func collectResults(results <-chan Result, stats *Statistics) {
	for result := range results {
		atomic.AddInt64(&stats.TotalJobs, 1)
		
		if result.RateLimited {
			atomic.AddInt64(&stats.RateLimitedJobs, 1)
		} else if result.Success {
			atomic.AddInt64(&stats.SuccessfulJobs, 1)
		} else {
			atomic.AddInt64(&stats.FailedJobs, 1)
		}
	}
}

func printStatistics(stats *Statistics) {
	duration := time.Since(stats.StartTime)
	
	fmt.Println("\n📊 Statistics:")
	fmt.Printf("  Total Jobs:        %d\n", atomic.LoadInt64(&stats.TotalJobs))
	fmt.Printf("  Successful:        %d\n", atomic.LoadInt64(&stats.SuccessfulJobs))
	fmt.Printf("  Rate Limited:      %d\n", atomic.LoadInt64(&stats.RateLimitedJobs))
	fmt.Printf("  Failed:            %d\n", atomic.LoadInt64(&stats.FailedJobs))
	fmt.Printf("  Total Duration:    %v\n", duration)
	fmt.Printf("  Throughput:        %.2f jobs/sec\n", 
		float64(atomic.LoadInt64(&stats.SuccessfulJobs))/duration.Seconds())
}

// strings package function replacement
var strings = struct {
	Repeat func(s string, count int) string
}{
	Repeat: func(s string, count int) string {
		result := ""
		for i := 0; i < count; i++ {
			result += s
		}
		return result
	},
}