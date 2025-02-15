package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	blockSize  = 1 * 1024 * 1024 // 1MB blocks
	maxWorkers = 50
)

// Stats struct to track progress and throughput
type Stats struct {
	blocksProcessed int64
	startTime       time.Time
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide a file path")
		return
	}
	file, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Printf("Error getting file info: %v\n", err)
		return
	}

	fileSize := fileInfo.Size()
	fmt.Printf("File size: %d bytes\n", fileSize)
	numBlocks := (fileSize + blockSize - 1) / blockSize
	fmt.Printf("Number of blocks: %d\n", numBlocks)

	// Monitor Stats
	stats := &Stats{
		startTime: time.Now(),
	}
	done := make(chan bool)
	go monitorThroughput(stats, done)

	// Create a channel for block numbers
	blockChan := make(chan int64, maxWorkers)

	// Create a WaitGroup to wait for all workers to finish
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(file, blockChan, &wg, stats)
	}

	// Send block numbers to channel to be processed
	for blockNum := int64(0); blockNum < numBlocks; blockNum++ {
		blockChan <- blockNum
	}
	close(blockChan)

	// Wait for all workers to finish
	wg.Wait()

	// Signal to exit the goroutines
	done <- true

	// Some stats
	duration := time.Since(stats.startTime)
	totalData := float64(stats.blocksProcessed) * float64(blockSize) / 1024 / 1024 // MB
	avgThroughput := totalData / duration.Seconds()

	fmt.Printf("\n~~~ Overall Stats ~~~ \n")
	fmt.Printf("Total time: %.2f seconds\n", duration.Seconds())
	fmt.Printf("Total data: %.2f MB\n", totalData)
	fmt.Printf("Average throughput: %.2f MB/s\n", avgThroughput)
}

func worker(file *os.File, blockChan chan int64, wg *sync.WaitGroup, stats *Stats) {
	defer wg.Done()

	buffer := make([]byte, blockSize)

	for blockNum := range blockChan {
		offset := blockNum * blockSize
		reader := io.NewSectionReader(file, offset, blockSize)
		_, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			fmt.Printf("Error reading block %d: %v\n", blockNum, err)
			continue
		}
		atomic.AddInt64(&stats.blocksProcessed, 1)
	}
}

func monitorThroughput(stats *Stats, done chan bool) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastBlocks int64
	lastTime := stats.startTime

	for {
		select {
		case <-ticker.C:
			currentBlocks := atomic.LoadInt64(&stats.blocksProcessed)
			currentTime := time.Now()

			blocksDelta := currentBlocks - lastBlocks
			timeDelta := currentTime.Sub(lastTime)

			throughput := float64(blocksDelta) * float64(blockSize) / 1024 / 1024 / timeDelta.Seconds()

			fmt.Printf("Current throughput: %.2f MB/s (Blocks processed: %d)\n",
				throughput, currentBlocks)

			lastBlocks = currentBlocks
			lastTime = currentTime

		case <-done:
			return
		}
	}
}
