package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

/*
128KB blocks
3000 IOPS

Throughput required to achieve 3000 IOPS:

(128KB*3000)/1024 = 375MB/s

EBS throughput Required:
~400 MB/s
*/

const (
	blockSize  = 1 * 1024 * 128 // 128KB blocks
	maxWorkers = 7              // 1 worker can do ~45MB/s
)

// Stats struct to track progress and throughput
type Stats struct {
	blocksProcessed int64
	startTime       time.Time
}

var bufferPool = sync.Pool{ // Use a pointer to sync.Pool
	New: func() interface{} {
		arg := make([]byte, blockSize)
		return &arg
	},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide a file path")
		return
	}
	file, err := os.OpenFile(os.Args[1], os.O_RDONLY|syscall.O_DIRECT, 0)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	// Tell the kernel to not cache the file
	// Avoid high memory usage during the block reads
	// https://man7.org/linux/man-pages/man2/posix_fadvise.2.html
	err = unix.Fadvise(int(file.Fd()), 0, 0, unix.FADV_DONTNEED)
	if err != nil {
		fmt.Printf("Error fadvise: %v\n", err)
		return
	}

	fmt.Printf("Workers running: %d\n", maxWorkers)
	fmt.Printf("Block size: %d KB\n", blockSize/1024)

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
	blockChan := make(chan int64, min(maxWorkers*2, int(numBlocks)))

	// Create a WaitGroup to wait for all workers to finish
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(file, blockChan, &wg, stats, &bufferPool)
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

func worker(file *os.File, blockChan chan int64, wg *sync.WaitGroup, stats *Stats, bufferPool *sync.Pool) {
	defer wg.Done()

	for blockNum := range blockChan {
		offset := blockNum * blockSize
		reader := io.NewSectionReader(file, offset, blockSize)
		buffer := bufferPool.Get().(*[]byte)
		_, err := reader.Read(*buffer)
		if err != nil && err != io.EOF {
			fmt.Printf("Error reading block %d: %v\n", blockNum, err)
			bufferPool.Put(buffer)
			continue
		}
		atomic.AddInt64(&stats.blocksProcessed, 1)
		bufferPool.Put(buffer)
	}
}

func monitorThroughput(stats *Stats, done chan bool) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastBlocks := atomic.LoadInt64(&stats.blocksProcessed)
	lastTime := time.Now()

	var throughput float64
	for {
		select {
		case <-ticker.C:
			currentBlocks := atomic.LoadInt64(&stats.blocksProcessed)
			now := time.Now()

			throughput = float64(currentBlocks-lastBlocks) * float64(blockSize) /
				(1024 * 1024 * now.Sub(lastTime).Seconds())

			fmt.Printf("Current throughput: %.2f MB/s (Blocks processed: %d)\n",
				throughput, currentBlocks)

			lastBlocks = currentBlocks
			lastTime = now

		case <-done:
			return
		}
	}
}
