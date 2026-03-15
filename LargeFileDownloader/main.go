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
	"github.com/charmbracelet/bubbles/spinner"
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

type FileState struct {
	Name      string
	Total     int64
	Current   int64
	Completed bool
}

type model struct {
	files    []*FileState
	spinner  spinner.Model
	progress progress.Model
	speed    float64
	eta      string
	threads  int
	history  []float64
}

var totalBytes int64
var prevBytes int64

func loadConfig() Config {
	f, _ := os.Open("config.json")
	defer f.Close()
	var c Config
	json.NewDecoder(f).Decode(&c)
	return c
}

func generateFiles(cfg Config) []string {

	files := []string{"SystemPE.part001.exe"}

	for i := 2; i <= cfg.TotalParts; i++ {
		files = append(files, fmt.Sprintf("SystemPE.part%03d.rar", i))
	}

	return files
}

func downloadFile(cfg Config, state *FileState) {

	url := cfg.BaseURL + "/" + state.Name
	path := filepath.Join(cfg.DownloadPath, state.Name)

	client := &http.Client{}

	req, _ := http.NewRequest("GET", url, nil)

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	state.Total = resp.ContentLength

	file, _ := os.Create(path)
	defer file.Close()

	buf := make([]byte, 32768)

	for {

		n, err := resp.Body.Read(buf)

		if n > 0 {

			file.Write(buf[:n])

			state.Current += int64(n)

			atomic.AddInt64(&totalBytes, int64(n))
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			break
		}
	}

	state.Completed = true
}

func worker(cfg Config, jobs <-chan *FileState, wg *sync.WaitGroup) {

	defer wg.Done()

	for state := range jobs {

		for i := 0; i < cfg.RetryCount; i++ {

			downloadFile(cfg, state)

			if state.Completed {
				break
			}

			time.Sleep(time.Second)
		}
	}
}

func startDownloads(cfg Config, states []*FileState) {

	os.MkdirAll(cfg.DownloadPath, os.ModePerm)

	jobs := make(chan *FileState)

	var wg sync.WaitGroup

	for i := 0; i < cfg.Threads; i++ {

		wg.Add(1)

		go worker(cfg, jobs, &wg)
	}

	for _, s := range states {
		jobs <- s
	}

	close(jobs)

	wg.Wait()
}

func initialModel(states []*FileState, threads int) model {

	sp := spinner.New()
	sp.Spinner = spinner.Line

	return model{
		files:    states,
		spinner:  sp,
		progress: progress.New(progress.WithDefaultGradient()),
		threads:  threads,
	}
}

func tick() tea.Cmd {

	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return t
	})
}

func (m model) Init() tea.Cmd {
	return tick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg.(type) {

	case time.Time:

		current := atomic.LoadInt64(&totalBytes)

		diff := current - prevBytes

		prevBytes = current

		m.speed = float64(diff) / 1024 / 1024

		m.history = append(m.history, m.speed)

		if len(m.history) > 30 {
			m.history = m.history[1:]
		}

		var total int64
		var done int64

		for _, f := range m.files {

			total += f.Total
			done += f.Current
		}

		if total > 0 {

			remaining := total - done

			if m.speed > 0 {

				sec := float64(remaining) / (m.speed * 1024 * 1024)

				m.eta = (time.Duration(sec) * time.Second).String()
			}
		}

		return m, tick()
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)

	return m, cmd
}

func graph(history []float64) string {

	g := ""

	for _, v := range history {

		bars := int(v * 2)

		line := ""

		for i := 0; i < bars; i++ {
			line += "█"
		}

		g += line + "\n"
	}

	return g
}

func (m model) View() string {

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Render("SystemPE Downloader")

	out := title + "\n\n"

	out += fmt.Sprintf("Threads: %d\nSpeed: %.2f MB/s\nETA: %s\n\n", m.threads, m.speed, m.eta)

	for _, f := range m.files {

		pct := float64(f.Current) / float64(f.Total)

		if f.Total == 0 {
			pct = 0
		}

		bar := m.progress.ViewAs(pct)

		spin := m.spinner.View()

		status := spin

		if f.Completed {
			status = "✔"
		}

		out += fmt.Sprintf("%s %s %s\n", status, f.Name, bar)
	}

	out += "\nThroughput Graph\n"

	out += graph(m.history)

	out += "\nAuthor: rhshourav\nGitHub: https://github.com/rhshourav\n"

	return out
}

func main() {

	cfg := loadConfig()

	names := generateFiles(cfg)

	var states []*FileState

	for _, n := range names {
		states = append(states, &FileState{Name: n})
	}

	go startDownloads(cfg, states)

	p := tea.NewProgram(initialModel(states, cfg.Threads))

	p.Start()
}
