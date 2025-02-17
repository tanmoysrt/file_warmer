package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const maxWorkers = 10

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
	debugMode := os.Getenv("DEBUG") == "1"

	filePath := os.Args[1]

	// Open file with O_DIRECT and O_RDONLY
	// To prevent going through disk cache + prevent modification to file
	// Disk cache is useless as on the system free memory will be less
	// And reading large file will add/remove cache and slow down system and the whole process
	file, err := os.OpenFile(os.Args[1], os.O_RDONLY|syscall.O_DIRECT, 0)
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

	// Get the disk physical sector block size
	blockSize, err := GetBlockSizeOfDiskByFile(filePath)
	if err != nil {
		fmt.Printf("Error getting block size: %v\n", err)
		return
	}

	var workersCount int

	if fileSize < blockSize || fileSize < 10*1024*1024 {
		// If total file size less than BLOCK_SIZE or less than 10MB
		// then run only one worker
		workersCount = 1
	} else {
		workersCount = maxWorkers
	}

	// Tell the kernel to not cache the file
	// Avoid high memory usage during the block reads
	// https://man7.org/linux/man-pages/man2/posix_fadvise.2.html
	err = unix.Fadvise(int(file.Fd()), 0, 0, unix.FADV_DONTNEED)
	if err != nil {
		fmt.Printf("Error fadvise: %v\n", err)
		return
	}

	// Create buffer pool
	// to avoid allocating/deallocating memory for each block
	// automatically cleared by gc on exit
	var bufferPool = sync.Pool{
		New: func() interface{} {
			arg := make([]byte, blockSize)
			return &arg
		},
	}

	// Monitor Stats
	stats := &Stats{
		startTime: time.Now(),
	}
	done := make(chan bool)
	if debugMode {
		go monitorThroughput(stats, blockSize, done)
	}

	// Create a channel for block numbers
	numBlocks := (fileSize + blockSize - 1) / blockSize
	blockChan := make(chan int64, min(workersCount*2, int(numBlocks)))

	// Create a WaitGroup to wait for all workers to finish
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < workersCount; i++ {
		wg.Add(1)
		go worker(file, blockSize, blockChan, &wg, stats, &bufferPool, debugMode)
	}

	// Print some info
	fmt.Printf("Workers running: %d\n", workersCount)
	fmt.Printf("Block size: %d KB\n", blockSize/1024)
	fmt.Printf("File size: %d bytes\n", fileSize)
	fmt.Printf("Number of blocks: %d\n", numBlocks)

	// Send block numbers to channel to be processed
	for blockNum := int64(0); blockNum < numBlocks; blockNum++ {
		blockChan <- blockNum
	}
	close(blockChan)

	// Wait for all workers to finish
	wg.Wait()

	// Signal to exit the monitor
	if debugMode {
		done <- true
	}

	// Some stats
	duration := time.Since(stats.startTime)
	var totalData float64
	if debugMode {
		totalData = float64(stats.blocksProcessed) * float64(blockSize) / 1024 / 1024 // MB
	} else {
		totalData = (float64(fileSize) / 1024 / 1024) // MB
	}
	avgThroughput := totalData / duration.Seconds()

	fmt.Printf("\n~~~ Overall Stats ~~~ \n")
	fmt.Printf("Total time: %.2f seconds\n", duration.Seconds())
	fmt.Printf("Total data: %.2f MB\n", totalData)
	fmt.Printf("Average throughput: %.2f MB/s\n", avgThroughput)
}

func worker(file *os.File, blockSize int64, blockChan chan int64, wg *sync.WaitGroup, stats *Stats, bufferPool *sync.Pool, debugMode bool) {
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
		if debugMode {
			atomic.AddInt64(&stats.blocksProcessed, 1)
		}
		bufferPool.Put(buffer)
	}
}

func monitorThroughput(stats *Stats, blockSize int64, done chan bool) {
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

// GetBlockSizeOfDiskByFile: Try to find the disk of file and then get the block size in bytes
func GetBlockSizeOfDiskByFile(file_path string) (int64, error) {
	// find disk name by df
	output, err := exec.Command("df", "-h", "--output=source", file_path).Output()
	if err != nil {
		return 0, fmt.Errorf("error getting disk name: %v", err)
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("error getting disk name: %v", err)
	}
	disk_name := strings.TrimSpace(lines[1])

	// get block size by lsblk
	output, err = exec.Command("lsblk", "--output=PHY-SEC", disk_name).Output()
	if err != nil {
		return 0, fmt.Errorf("error getting block size: %v", err)
	}
	lines = strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("error getting block size: %v", err)
	}
	block_size, err := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error converting block size: %v", err)
	}
	return block_size * 1024, nil
}
