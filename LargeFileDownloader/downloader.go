package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	baseURL     = "https://raw.githubusercontent.com/rhshourav/ideal-fishstick/refs/heads/main/SystemPE-Test"
	totalParts  = 113
	threadCount = 32
	retryCount  = 5
)

type FileJob struct {
	Name string
	URL  string
}

var totalDownloaded int64

func main() {

	downloadPath := "SystemPE"
	os.MkdirAll(downloadPath, os.ModePerm)

	files := generateFiles()

	jobs := make(chan FileJob, len(files))
	var wg sync.WaitGroup

	for i := 0; i < threadCount; i++ {
		wg.Add(1)
		go worker(downloadPath, jobs, &wg)
	}

	for _, f := range files {
		jobs <- f
	}

	close(jobs)
	wg.Wait()

	fmt.Println("\nAll downloads finished")
}

func generateFiles() []FileJob {

	var files []FileJob

	files = append(files, FileJob{
		Name: "SystemPE.part001.exe",
		URL:  baseURL + "/SystemPE.part001.exe",
	})

	for i := 2; i <= totalParts; i++ {

		name := fmt.Sprintf("SystemPE.part%03d.rar", i)

		files = append(files, FileJob{
			Name: name,
			URL:  baseURL + "/" + name,
		})
	}

	return files
}

func worker(path string, jobs <-chan FileJob, wg *sync.WaitGroup) {

	defer wg.Done()

	for job := range jobs {

		for attempt := 1; attempt <= retryCount; attempt++ {

			err := downloadFile(path, job)

			if err == nil {
				break
			}

			fmt.Println("Retry", job.Name, "attempt", attempt)
			time.Sleep(time.Second * time.Duration(attempt))
		}
	}
}

func downloadFile(path string, job FileJob) error {

	filePath := filepath.Join(path, job.Name)

	var start int64 = 0

	if info, err := os.Stat(filePath); err == nil {
		start = info.Size()
	}

	client := &http.Client{
		Timeout: 0,
	}

	req, err := http.NewRequest("GET", job.URL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "GoDownloader")
	req.Header.Set("Connection", "keep-alive")

	if start > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	var file *os.File

	if start > 0 {
		file, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		file, err = os.Create(filePath)
	}

	if err != nil {
		return err
	}

	defer file.Close()

	buf := make([]byte, 32*1024)

	startTime := time.Now()
	var downloaded int64 = start

	for {

		n, err := resp.Body.Read(buf)

		if n > 0 {

			file.Write(buf[:n])

			downloaded += int64(n)

			atomic.AddInt64(&totalDownloaded, int64(n))

			elapsed := time.Since(startTime).Seconds()

			if elapsed > 0 {

				speed := float64(downloaded-start) / elapsed / 1024 / 1024

				fmt.Printf(
					"\rDownloading %-25s %.2f MB  Speed: %.2f MB/s",
					job.Name,
					float64(downloaded)/1024/1024,
					speed,
				)
			}
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}
	}

	fmt.Println("\nCompleted", job.Name)

	return nil
}
