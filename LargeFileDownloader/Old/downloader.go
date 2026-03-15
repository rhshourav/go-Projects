package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Config struct {
	Author       string `json:"author"`
	Github       string `json:"github"`
	BaseURL      string `json:"base_url"`
	DownloadPath string `json:"download_path"`
	Threads      int    `json:"threads"`
	RetryCount   int    `json:"retry_count"`
	TotalParts   int    `json:"total_parts"`
}

type model struct {
	progress progress.Model
	percent  float64
	speed    float64
	done     int
	total    int
}

var totalDownloaded int64

func loadConfig() Config {

	file, err := os.Open("config.json")
	if err != nil {
		panic(err)
	}

	defer file.Close()

	var cfg Config
	json.NewDecoder(file).Decode(&cfg)

	return cfg
}

func generateFiles(cfg Config) []string {

	files := []string{"SystemPE.part001.exe"}

	for i := 2; i <= cfg.TotalParts; i++ {

		name := fmt.Sprintf("SystemPE.part%03d.rar", i)
		files = append(files, name)
	}

	return files
}

func downloadFile(cfg Config, name string) error {

	url := cfg.BaseURL + "/" + name
	path := filepath.Join(cfg.DownloadPath, name)

	var start int64

	if info, err := os.Stat(path); err == nil {
		start = info.Size()
	}

	client := &http.Client{}

	req, _ := http.NewRequest("GET", url, nil)

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
		file, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		file, _ = os.Create(path)
	}

	defer file.Close()

	buf := make([]byte, 32768)

	for {

		n, err := resp.Body.Read(buf)

		if n > 0 {

			file.Write(buf[:n])
			atomic.AddInt64(&totalDownloaded, int64(n))
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func worker(cfg Config, jobs <-chan string, wg *sync.WaitGroup) {

	defer wg.Done()

	for name := range jobs {

		for i := 0; i < cfg.RetryCount; i++ {

			err := downloadFile(cfg, name)

			if err == nil {
				break
			}

			time.Sleep(time.Second)
		}
	}
}

func startDownload(cfg Config) {

	os.MkdirAll(cfg.DownloadPath, os.ModePerm)

	files := generateFiles(cfg)

	jobs := make(chan string, len(files))

	var wg sync.WaitGroup

	for i := 0; i < cfg.Threads; i++ {

		wg.Add(1)
		go worker(cfg, jobs, &wg)
	}

	for _, f := range files {
		jobs <- f
	}

	close(jobs)

	wg.Wait()
}

func initialModel(total int) model {

	p := progress.New(progress.WithDefaultGradient())

	return model{
		progress: p,
		total:    total,
	}
}

func (m model) Init() tea.Cmd {
	return tick()
}

func tick() tea.Cmd {

	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return t
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg.(type) {

	case time.Time:

		speed := float64(atomic.LoadInt64(&totalDownloaded)) / 1024 / 1024

		m.speed = speed

		if m.percent < 1.0 {
			m.percent += 0.01
		}

		return m, tick()
	}

	return m, nil
}

func (m model) View() string {

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("63")).
		Render("SystemPE Downloader")

	info := fmt.Sprintf(
		"Author: rhshourav\nGitHub: https://github.com/rhshourav\n\nSpeed: %.2f MB/s\n",
		m.speed,
	)

	bar := m.progress.ViewAs(m.percent)

	return fmt.Sprintf("\n%s\n\n%s\n%s\n", title, info, bar)
}

func main() {

	cfg := loadConfig()

	go startDownload(cfg)

	p := tea.NewProgram(initialModel(cfg.TotalParts))

	if err := p.Start(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
