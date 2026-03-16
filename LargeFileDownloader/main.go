package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	_ "modernc.org/sqlite"
)

type Config struct {
	BaseURL            string `json:"base_url"`
	DownloadPath       string `json:"download_path"`
	ThreadsTotal       int    `json:"threads_total"`
	SegmentsPerFile    int    `json:"segments_per_file"`
	ConcurrentFiles    int    `json:"concurrent_files"`
	MinSegmentSizeMB   int64  `json:"min_segment_size_mb"`
	RetryCount         int    `json:"retry_count"`
	HTTPTimeoutSeconds int    `json:"http_timeout_seconds"`
	HeadTimeoutSeconds int    `json:"head_timeout_seconds"`
	UIRefreshMs        int    `json:"ui_refresh_ms"`
	TotalParts         int    `json:"total_parts"`
	DBPath             string `json:"db_path"`
}

func loadConfig() Config {

	f, err := os.Open("config.json")
	if err != nil {
		panic(err)
	}

	defer f.Close()

	cfg := Config{}
	json.NewDecoder(f).Decode(&cfg)

	if cfg.ThreadsTotal == 0 {
		cfg.ThreadsTotal = runtime.NumCPU() * 6
	}

	if cfg.SegmentsPerFile == 0 {
		cfg.SegmentsPerFile = 4
	}

	if cfg.ConcurrentFiles == 0 {
		cfg.ConcurrentFiles = 4
	}

	if cfg.MinSegmentSizeMB == 0 {
		cfg.MinSegmentSizeMB = 4
	}

	if cfg.RetryCount == 0 {
		cfg.RetryCount = 4
	}

	if cfg.DBPath == "" {
		cfg.DBPath = "resume.db"
	}

	if cfg.UIRefreshMs == 0 {
		cfg.UIRefreshMs = 300
	}

	return cfg
}

type ResumeDB struct {
	db *sql.DB
	mu sync.Mutex
}

func openDB(path string) *ResumeDB {

	db, err := sql.Open("sqlite", path)
	if err != nil {
		panic(err)
	}

	r := &ResumeDB{db: db}

	schema := `
CREATE TABLE IF NOT EXISTS segments(
file TEXT,
idx INTEGER,
completed INTEGER,
PRIMARY KEY(file,idx)
);`

	db.Exec(schema)

	return r
}

func (r *ResumeDB) done(file string, idx int) {

	r.mu.Lock()
	defer r.mu.Unlock()

	r.db.Exec("INSERT OR REPLACE INTO segments VALUES(?,?,1)", file, idx)
}

func (r *ResumeDB) completed(file string) map[int]bool {

	r.mu.Lock()
	defer r.mu.Unlock()

	rows, _ := r.db.Query("SELECT idx FROM segments WHERE file=?", file)

	out := map[int]bool{}

	for rows.Next() {
		var i int
		rows.Scan(&i)
		out[i] = true
	}

	return out
}

type Segment struct {
	idx   int
	start int64
	end   int64
}

type FileJob struct {
	Name string
	URL  string
	Size int64

	Segments []Segment

	Downloaded int64
	DoneSeg    int32
	Completed  bool
}

var totalBytes int64

func httpClient(timeout int) *http.Client {

	tr := &http.Transport{
		MaxIdleConns:        512,
		MaxIdleConnsPerHost: 128,
		IdleConnTimeout:     60 * time.Second,
		DialContext: (&net.Dialer{
			Timeout: 20 * time.Second,
		}).DialContext,
	}

	return &http.Client{
		Transport: tr,
		Timeout:   time.Duration(timeout) * time.Second,
	}
}

func probeSize(client *http.Client, url string) int64 {

	req, _ := http.NewRequest("HEAD", url, nil)

	resp, err := client.Do(req)
	if err != nil {
		return 0
	}

	defer resp.Body.Close()

	cl := resp.Header.Get("Content-Length")

	size, _ := strconv.ParseInt(cl, 10, 64)

	return size
}

func partition(size int64, cfg Config) []Segment {

	min := cfg.MinSegmentSizeMB * 1024 * 1024

	segments := cfg.SegmentsPerFile

	maxAllowed := int(size / min)

	if maxAllowed < segments {
		segments = maxAllowed
	}

	if segments < 1 {
		segments = 1
	}

	block := size / int64(segments)

	var out []Segment

	start := int64(0)

	for i := 0; i < segments; i++ {

		end := start + block - 1

		if i == segments-1 {
			end = size - 1
		}

		out = append(out, Segment{i, start, end})

		start = end + 1
	}

	return out
}

type ProgressReader struct {
	r       io.Reader
	counter *int64
}

func (p *ProgressReader) Read(b []byte) (int, error) {

	n, err := p.r.Read(b)

	atomic.AddInt64(p.counter, int64(n))
	atomic.AddInt64(&totalBytes, int64(n))

	return n, err
}

func downloadSegment(client *http.Client, job *FileJob, seg Segment, cfg Config, db *ResumeDB) error {

	req, _ := http.NewRequest("GET", job.URL, nil)

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", seg.start, seg.end))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	path := filepath.Join(cfg.DownloadPath, job.Name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	defer f.Close()

	f.Seek(seg.start, 0)

	pr := &ProgressReader{resp.Body, &job.Downloaded}

	_, err = io.Copy(f, pr)
	if err != nil {
		return err
	}

	db.done(job.Name, seg.idx)

	atomic.AddInt32(&job.DoneSeg, 1)

	return nil
}

func workerPool(cfg Config, jobs []*FileJob, db *ResumeDB) {

	client := httpClient(cfg.HTTPTimeoutSeconds)

	connSem := make(chan struct{}, cfg.ThreadsTotal)
	fileSem := make(chan struct{}, cfg.ConcurrentFiles)

	var wg sync.WaitGroup

	for _, job := range jobs {

		wg.Add(1)

		go func(j *FileJob) {

			defer wg.Done()

			fileSem <- struct{}{}
			defer func() { <-fileSem }()

			for _, seg := range j.Segments {

				connSem <- struct{}{}

				go func(s Segment) {

					defer func() { <-connSem }()

					for r := 0; r < cfg.RetryCount; r++ {

						err := downloadSegment(client, j, s, cfg, db)

						if err == nil {
							break
						}

						time.Sleep(time.Second * time.Duration(r+1))
					}

				}(seg)
			}

		}(job)
	}

	wg.Wait()
}

func buildJobs(cfg Config, client *http.Client) []*FileJob {

	var jobs []*FileJob

	for i := 1; i <= cfg.TotalParts; i++ {

		var name string

		if i == 1 {
			name = "SystemPE.part001.exe"
		} else {
			name = fmt.Sprintf("SystemPE.part%03d.rar", i)
		}

		url := strings.TrimRight(cfg.BaseURL, "/") + "/" + name

		size := probeSize(client, url)

		job := &FileJob{
			Name: name,
			URL:  url,
			Size: size,
		}

		job.Segments = partition(size, cfg)

		jobs = append(jobs, job)
	}

	return jobs
}

type UI struct {
	files []*FileJob

	table table.Model

	progress progress.Model

	spin spinner.Model

	last int64

	history []float64

	refresh time.Duration
}

func newUI(cfg Config, files []*FileJob) UI {

	cols := []table.Column{
		{"File", 30},
		{"Progress", 10},
		{"Downloaded", 12},
	}

	t := table.New(table.WithColumns(cols))

	p := progress.New(progress.WithDefaultGradient())

	s := spinner.New()
	s.Spinner = spinner.Line

	return UI{
		files:    files,
		table:    t,
		progress: p,
		spin:     s,
		refresh:  time.Millisecond * time.Duration(cfg.UIRefreshMs),
	}
}

func (u UI) Init() tea.Cmd {
	return tea.Tick(u.refresh, func(t time.Time) tea.Msg { return t })
}

func percent(f *FileJob) int {

	if f.Size == 0 {
		return 0
	}

	d := atomic.LoadInt64(&f.Downloaded)

	return int(float64(d) / float64(f.Size) * 100)
}

func (u UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg.(type) {

	case time.Time:

		rows := []table.Row{}

		for _, f := range u.files {

			rows = append(rows, table.Row{
				f.Name,
				fmt.Sprintf("%d%%", percent(f)),
				fmt.Sprintf("%.2fMB", float64(f.Downloaded)/1024/1024),
			})
		}

		u.table.SetRows(rows)

		return u, tea.Tick(u.refresh, func(t time.Time) tea.Msg { return t })
	}

	return u, nil
}

func (u UI) View() string {

	title := lipgloss.NewStyle().Bold(true).Render("Downloader")

	return title + "\n\n" + u.table.View()
}

func main() {

	cfg := loadConfig()

	db := openDB(cfg.DBPath)

	client := httpClient(cfg.HTTPTimeoutSeconds)

	jobs := buildJobs(cfg, client)

	os.MkdirAll(cfg.DownloadPath, 0755)

	ui := newUI(cfg, jobs)

	go workerPool(cfg, jobs, db)

	p := tea.NewProgram(ui)

	p.Start()
}
