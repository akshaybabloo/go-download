package download

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type Options struct {
	// Headers is the headers to be sent with the request
	Headers map[string]string

	// Retry is the number of times the request should be retried
	// If not provided, 3 will be used
	Retry int

	// ShowProgress is the flag to show the progress of the download
	// If not provided, false will be used
	ShowProgress bool

	// DownloadPath is the path to download the files
	// If not provided, the current directory will be used
	DownloadPath string

	// Parallel is the number of parallel downloads
	// If not provided, 1 will be used
	Parallel int

	// Timeout is the time to wait for the download
	// If not provided, 10 seconds will be used
	Timeout int

	// Title is the title of the download folder
	// If not provided, the title will be empty
	// If a URL is provided, it will try to get the title from the URL webpage, if not, the domain name will be used.
	Title string

	// Log is the logger to be used
	Log *log.Logger

	urls []string
}

// NewDownload creates a new download with some default options
func NewDownload(options ...*Options) *Options {

	if len(options) > 0 {
		options := options[0]
		if options.Log == nil {
			options.Log = log.New(os.Stdout, "", log.LstdFlags)
		}
		return options
	}

	opt := &Options{
		Headers:      map[string]string{},
		Retry:        3,
		ShowProgress: false,
		DownloadPath: ".",
		Parallel:     1,
		Timeout:      10000,
		Title:        "",
	}
	return opt
}

// SetURLs sets the URLs to be downloaded
func (o *Options) SetURLs(u []string) {
	o.urls = u
}

// SetOutput sets the output path
func (o *Options) SetOutput(p string) {
	o.DownloadPath = p
}

// SetTitle sets the title of the download
// If a URL is provided, it will try to get the title from the URL webpage, if not, the domain name will be used.
func (o *Options) SetTitle(t string) error {
	if isUrl(t) {
		res, err := http.Get(t)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			requestURI, _ := url.ParseRequestURI(t)
			o.Title = requestURI.Host
			return nil
		}

		doc, err := goquery.NewDocumentFromReader(res.Body)
		if err != nil {
			return fmt.Errorf("failed to parse document: %w", err)
		}
		s, err := doc.Find("title").First().Html()
		if err != nil {
			requestURI, _ := url.ParseRequestURI(t)
			o.Title = requestURI.Host
			return nil
		}
		o.Title = s
	}

	o.Title = t
	return nil
}

// SetParallel sets the number of parallel downloads. Default is 1
func (o *Options) SetParallel(i int) {
	o.Parallel = i
	if len(o.urls) > 0 && o.Parallel > len(o.urls) {
		o.Parallel = len(o.urls)
	}
}

// Start starts the download
func (o *Options) Start() error {
	// Validate URLs
	if len(o.urls) == 0 {
		return fmt.Errorf("no URLs to download")
	}
	for _, u := range o.urls {
		if !isUrl(u) {
			return fmt.Errorf("invalid URL: %s", u)
		}
	}

	// Validate parallel count
	if o.Parallel < 1 {
		return fmt.Errorf("parallel downloads must be at least 1")
	}
	if o.Parallel > len(o.urls) {
		o.Parallel = len(o.urls)
	}

	// Validate timeout
	if o.Timeout < 1 {
		return fmt.Errorf("timeout must be at least 1 second")
	}

	// Validate retry count
	if o.Retry < 0 {
		return fmt.Errorf("retry count cannot be negative")
	}

	// Start downloads
	if o.Parallel == 1 {
		return o.downloadSerial()
	}
	return o.downloadParallel()
}

func isUrl(str string) bool {
	requestURI, err := url.ParseRequestURI(str)
	if err != nil {
		return false
	}

	address := net.ParseIP(requestURI.Host)
	if address == nil {
		return strings.Contains(requestURI.Host, ".")
	}

	return true
}

func (o *Options) downloadSerial() error {
	if !o.ShowProgress {
		// Simple serial download without progress bars
		for _, u := range o.urls {
			if err := o.downloadFile(u, nil); err != nil {
				return err
			}
		}
		return nil
	}

	// Serial download with progress bars
	var wg sync.WaitGroup
	p := mpb.New(mpb.WithWaitGroup(&wg))

	for _, u := range o.urls {
		parsedURL, _ := url.Parse(u)
		filename := path.Base(parsedURL.Path)

		bar := o.newProgress(p, 0, filename)

		if err := o.downloadFile(u, bar); err != nil {
			return err
		}
	}

	p.Wait()
	return nil
}

func (o *Options) downloadParallel() error {
	if !o.ShowProgress {
		return o.downloadParallelSimple()
	}
	return o.downloadParallelWithProgress()
}

func (o *Options) downloadParallelSimple() error {
	var wg sync.WaitGroup
	errors := make(chan error, len(o.urls))

	wg.Add(len(o.urls))
	for _, u := range o.urls {
		go func(url string) {
			defer wg.Done()
			if err := o.downloadFile(url, nil); err != nil {
				errors <- err
			}
		}(u)
	}

	go func() {
		wg.Wait()
		close(errors)
	}()

	var lastErr error
	for err := range errors {
		if err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (o *Options) downloadParallelWithProgress() error {
	var wg sync.WaitGroup
	p := mpb.New(mpb.WithWaitGroup(&wg))
	errors := make(chan error, len(o.urls))

	// Get file sizes first
	sizes := make(map[string]int64)
	for _, u := range o.urls {
		req, err := http.NewRequest("HEAD", u, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.ContentLength > 0 {
			sizes[u] = resp.ContentLength
		} else {
			sizes[u] = 0
		}
	}

	// Create a channel for jobs
	jobs := make(chan downloadJob, len(o.urls))

	// Start limited number of workers
	for i := 0; i < o.Parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if err := o.downloadFile(job.url, job.bar); err != nil {
					job.bar.Abort(true)
					errors <- err
				}
			}
		}()
	}

	// Queue jobs
	for _, u := range o.urls {
		parsedURL, _ := url.Parse(u)
		filename := path.Base(parsedURL.Path)
		bar := o.newProgress(p, int(sizes[u]), filename)
		jobs <- downloadJob{
			url: u,
			bar: bar,
		}
	}
	close(jobs)

	// Use a channel to signal completion
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Wait for either completion or error
	var lastErr error
	remaining := len(o.urls)

	for remaining > 0 {
		select {
		case err := <-errors:
			if err != nil {
				lastErr = err
			}
			remaining--
		case <-done:
			remaining = 0
		}
	}

	close(errors)
	return lastErr
}

// downloadJob represents a download task
type downloadJob struct {
	url string
	bar *mpb.Bar
}

func (o *Options) newProgress(p *mpb.Progress, size int, name string) *mpb.Bar {
	return p.AddBar(int64(size),
		mpb.BarFillerClearOnComplete(),
		mpb.PrependDecorators(
			decor.Name(name+": ", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
			decor.OnComplete(
				decor.CountersKibiByte("% .2f / % .2f"), "done!",
			),
		),
		mpb.AppendDecorators(
			decor.OnComplete(
				decor.EwmaSpeed(decor.SizeB1024(2), "% .2f", 60), "",
			),
			decor.OnComplete(
				decor.EwmaETA(decor.ET_STYLE_GO, 60), "",
			),
		),
	)
}

func (o *Options) downloadFile(u string, bar *mpb.Bar) error {
	// Create the download directory if it doesn't exist
	downloadDir := path.Join(o.DownloadPath, o.Title)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	// Parse the URL to get a safe filename
	parsedURL, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	filename := path.Base(parsedURL.Path)
	if filename == "" || filename == "." {
		filename = "download"
	}

	// Create the file with a safe path
	filepath := path.Join(downloadDir, filename)
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(o.Timeout)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for k, v := range o.Headers {
		req.Header.Add(k, v)
	}

	// Perform the request with retries
	var resp *http.Response
	for i := 0; i <= o.Retry; i++ {
		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			break
		}
		if i < o.Retry {
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to download after %d retries: %w", o.Retry, err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Update progress bar if provided
	if bar != nil && resp.ContentLength > 0 {
		bar.SetTotal(resp.ContentLength, false)
	}

	// Use progress bar if provided
	reader := resp.Body
	if bar != nil {
		reader = bar.ProxyReader(resp.Body)
		defer reader.Close()
	}

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	return nil
}
