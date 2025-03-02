package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var logger = log.New(os.Stdout, "", log.LstdFlags)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide file paths")
		return
	}

	filePathsStr := os.Args[1]
	warmupFiles(strings.Split(filePathsStr, ","), true)
}

func warmupFiles(filePaths []string, showStats bool) {
	var totalFileSize int64
	var startTime time.Time

	if showStats {
		startTime = time.Now()
	}

	var files []*os.File
	for _, filePath := range filePaths {
		// Open file with O_DIRECT and O_RDONLY
		// To prevent going through disk cache + prevent modification to file
		// Disk cache is useless as on the system free memory will be less
		// And reading large file will add/remove cache and slow down system and the whole process
		file, err := os.OpenFile(filePath, os.O_RDONLY|syscall.O_DIRECT, 0)
		if err != nil {
			logger.Printf("Error opening file: %v\n", err)
			return
		}
		defer file.Close()
		files = append(files, file)

		if showStats {
			fileInfo, err := file.Stat()
			if err != nil {
				logger.Printf("Error getting file info: %v\n", err)
				return
			}
			totalFileSize += fileInfo.Size()
		}
	}

	warmupFilesUsingPsync(files)

	if showStats {
		totalData := (float64(totalFileSize) / 1024 / 1024) // MB
		duration := time.Since(startTime)
		fmt.Printf("\n~~~ Overall Stats ~~~ \n")
		fmt.Printf("Total time: %.2f seconds\n", duration.Seconds())
		fmt.Printf("Total data: %.2f MB\n", totalData)
		fmt.Printf("Average throughput: %.2f MB/s\n", totalData/duration.Seconds())
	}
}

func warmupFilesUsingPsync(files []*os.File) {
	if len(files) == 0 {
		logger.Println("No files to warmup")
		return
	}
	const blockSize int64 = 1024 * 296 // 296 KB
	const workersCount int = 4         // Number of workers

	// Find the largest file size
	var largestFileSize int64
	for _, file := range files {
		fileInfo, err := file.Stat()
		if err == nil {
			fileSize := fileInfo.Size()
			if fileSize > largestFileSize {
				largestFileSize = fileSize
			}
		}
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

	// Create a channel for block numbers
	numBlocks := (largestFileSize + blockSize - 1) / blockSize

	for _, file := range files {
		logger.Printf("Warming up file: %s\n", file.Name())
		blockChan := make(chan int64, min(workersCount*2, int(numBlocks)))

		// Tell the kernel to not cache the file
		// Avoid high memory usage during the block reads
		// https://man7.org/linux/man-pages/man2/posix_fadvise.2.html
		err := unix.Fadvise(int(file.Fd()), 0, 0, unix.FADV_DONTNEED)
		if err != nil {
			logger.Printf("Error fadvise: %v\n", err)
			continue
		}

		// Create a WaitGroup to wait for all workers to finish
		var wg sync.WaitGroup

		// Start workers
		for i := 0; i < workersCount; i++ {
			wg.Add(1)
			go psyncWorker(file, blockSize, blockChan, &wg, &bufferPool)
		}

		// Send block numbers to channel to be processed
		for blockNum := int64(0); blockNum < numBlocks; blockNum++ {
			blockChan <- blockNum
		}

		// Close the channel
		close(blockChan)

		// Wait for all workers to finish
		wg.Wait()

	}

}

func psyncWorker(file *os.File, blockSize int64, blockChan chan int64, wg *sync.WaitGroup, bufferPool *sync.Pool) {
	defer wg.Done()

	for blockNum := range blockChan {
		offset := blockNum * blockSize
		reader := io.NewSectionReader(file, offset, blockSize)
		buffer := bufferPool.Get().(*[]byte)
		_, err := reader.Read(*buffer)
		if err != nil && err != io.EOF {
			logger.Printf("Error reading block %d: %v\n", blockNum, err)
			bufferPool.Put(buffer)
			continue
		}
		bufferPool.Put(buffer)
	}
}
