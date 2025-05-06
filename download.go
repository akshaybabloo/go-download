package download

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
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
	"golang.org/x/time/rate"
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

	// ResumeDownload is the flag to enable resuming interrupted downloads
	// If not provided, false will be used
	ResumeDownload bool

	// BandwidthLimit is the maximum download speed in KB/s
	// If not provided or set to 0, no limit will be applied
	BandwidthLimit int

	// CustomFilenames is a map of URL to custom filename
	// If provided, the custom filename will be used instead of the one from the URL
	CustomFilenames map[string]string

	// VerifyChecksum is the flag to enable checksum verification
	// If not provided, false will be used
	VerifyChecksum bool

	// ExpectedChecksums is a map of URL to expected checksum
	// The format is "algorithm:checksum", e.g. "sha256:1234abcd..."
	// Supported algorithms: md5, sha1, sha256
	ExpectedChecksums map[string]string

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
		Headers:           map[string]string{},
		Retry:             3,
		ShowProgress:      false,
		DownloadPath:      ".",
		Parallel:          1,
		Timeout:           10000,
		Title:             "",
		ResumeDownload:    false,
		BandwidthLimit:    0,
		CustomFilenames:   make(map[string]string),
		VerifyChecksum:    false,
		ExpectedChecksums: make(map[string]string),
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

	// Get file sizes in parallel
	sizes := make(map[string]int64)
	var sizesMutex sync.Mutex
	var sizeWg sync.WaitGroup
	sizeErrors := make(chan error, len(o.urls))

	// Create a channel for HEAD request jobs
	headJobs := make(chan string, len(o.urls))

	// Start workers for HEAD requests
	workerCount := o.Parallel
	if workerCount > len(o.urls) {
		workerCount = len(o.urls)
	}

	for i := 0; i < workerCount; i++ {
		sizeWg.Add(1)
		go func() {
			defer sizeWg.Done()
			for u := range headJobs {
				req, err := http.NewRequest("HEAD", u, nil)
				if err != nil {
					sizeErrors <- err
					continue
				}

				// Add headers
				for k, v := range o.Headers {
					req.Header.Add(k, v)
				}

				// Set timeout
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(o.Timeout)*time.Second)
				defer cancel()
				req = req.WithContext(ctx)

				// Perform the request with retries
				var resp *http.Response
				var respErr error
				for j := 0; j <= o.Retry; j++ {
					resp, respErr = http.DefaultClient.Do(req)
					if respErr == nil {
						break
					}
					if j < o.Retry {
						time.Sleep(time.Second * time.Duration(j+1))
					}
				}

				if respErr != nil {
					sizeErrors <- respErr
					continue
				}

				// Store the content length
				if resp.StatusCode == http.StatusOK {
					sizesMutex.Lock()
					if resp.ContentLength > 0 {
						sizes[u] = resp.ContentLength
					} else {
						sizes[u] = 0
					}
					sizesMutex.Unlock()
				} else {
					sizeErrors <- fmt.Errorf("unexpected status code for HEAD request: %d", resp.StatusCode)
				}

				resp.Body.Close()
			}
		}()
	}

	// Queue HEAD jobs
	for _, u := range o.urls {
		headJobs <- u
	}
	close(headJobs)

	// Wait for HEAD requests to complete
	go func() {
		sizeWg.Wait()
		close(sizeErrors)
	}()

	// Check for errors from HEAD requests
	for err := range sizeErrors {
		if err != nil {
			return err
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

// SetCustomFilename sets a custom filename for a URL
func (o *Options) SetCustomFilename(url, filename string) {
	if o.CustomFilenames == nil {
		o.CustomFilenames = make(map[string]string)
	}
	o.CustomFilenames[url] = filename
}

// SetExpectedChecksum sets the expected checksum for a URL
// The format is "algorithm:checksum", e.g. "sha256:1234abcd..."
// Supported algorithms: md5, sha1, sha256
func (o *Options) SetExpectedChecksum(url, checksum string) {
	if o.ExpectedChecksums == nil {
		o.ExpectedChecksums = make(map[string]string)
	}
	o.ExpectedChecksums[url] = checksum
}

// SetBasicAuth sets the basic authentication header
func (o *Options) SetBasicAuth(username, password string) {
	auth := username + ":" + password
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	o.Headers["Authorization"] = "Basic " + encoded
}

// SetBearerToken sets the bearer token authentication header
func (o *Options) SetBearerToken(token string) {
	o.Headers["Authorization"] = "Bearer " + token
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

	// Use custom filename if provided, otherwise use the one from the URL
	var filename string
	if customName, ok := o.CustomFilenames[u]; ok && customName != "" {
		filename = customName
	} else {
		filename = path.Base(parsedURL.Path)
		if filename == "" || filename == "." {
			filename = "download"
		}
	}

	// Full path to the file
	filepath := path.Join(downloadDir, filename)

	// Check if the URL is valid and the file exists on the server before creating a local file
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(o.Timeout)*time.Second)
	defer cancel()

	// Create a HEAD request to check if the file exists
	headReq, err := http.NewRequestWithContext(ctx, "HEAD", u, nil)
	if err != nil {
		return fmt.Errorf("failed to create HEAD request: %w", err)
	}

	// Add headers to HEAD request
	for k, v := range o.Headers {
		headReq.Header.Add(k, v)
	}

	// Perform the HEAD request with retries
	var headResp *http.Response
	var headErr error
	for i := 0; i <= o.Retry; i++ {
		headResp, headErr = http.DefaultClient.Do(headReq)
		if headErr == nil {
			break
		}
		if i < o.Retry {
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}

	// Check if HEAD request was successful
	if headErr != nil {
		return fmt.Errorf("failed to check file existence after %d retries: %w", o.Retry, headErr)
	}
	defer headResp.Body.Close()

	// Check if the file exists on the server
	if headResp.StatusCode != http.StatusOK && headResp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("file not found on server, status code: %d", headResp.StatusCode)
	}

	// Get file size from HEAD response
	contentLength := headResp.ContentLength

	// Check if file exists locally and we should resume
	var file *os.File
	var fileSize int64

	if o.ResumeDownload {
		// Check if file exists and get its size
		fileInfo, err := os.Stat(filepath)
		if err == nil && fileInfo.Size() > 0 {
			// File exists and has content
			fileSize = fileInfo.Size()

			// If the local file is already the same size as the remote file, we're done
			if contentLength > 0 && fileSize >= contentLength {
				if bar != nil {
					bar.SetTotal(contentLength, true)
				}
				return nil
			}

			// Open file for appending
			file, err = os.OpenFile(filepath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to open file for resuming: %w", err)
			}
		} else {
			// File doesn't exist or is empty, create new
			file, err = os.Create(filepath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
		}
	} else {
		// Always create a new file if not resuming
		file, err = os.Create(filepath)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
	}
	defer file.Close()

	// Create GET request for actual download
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for k, v := range o.Headers {
		req.Header.Add(k, v)
	}

	// Add Range header if resuming
	if o.ResumeDownload && fileSize > 0 {
		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", fileSize))
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
	if o.ResumeDownload && fileSize > 0 {
		if resp.StatusCode != http.StatusPartialContent {
			// If the server doesn't support range requests, start over
			if resp.StatusCode == http.StatusOK {
				// Close and recreate the file
				file.Close()
				file, err = os.Create(filepath)
				if err != nil {
					return fmt.Errorf("failed to create file: %w", err)
				}
				fileSize = 0
			} else {
				return fmt.Errorf("unexpected status code for range request: %d", resp.StatusCode)
			}
		}
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Calculate total size for progress bar
	totalSize := resp.ContentLength
	if o.ResumeDownload && resp.StatusCode == http.StatusPartialContent {
		// For resumed downloads, add the existing file size to the content length
		totalSize += fileSize
	}

	// Update progress bar if provided
	if bar != nil && totalSize > 0 {
		bar.SetTotal(totalSize, false)
		if fileSize > 0 {
			bar.SetCurrent(fileSize)
		}
	}

	// Use progress bar if provided
	reader := resp.Body
	if bar != nil {
		reader = bar.ProxyReader(resp.Body)
		defer reader.Close()
	}

	// Apply bandwidth limit if set
	if o.BandwidthLimit > 0 {
		// Convert KB/s to bytes per second
		bytesPerSecond := int64(o.BandwidthLimit * 1024)
		reader = &rateLimitedReader{
			r:       reader,
			limiter: rate.NewLimiter(rate.Limit(bytesPerSecond), int(bytesPerSecond)),
		}
	}

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	// Verify checksum if enabled
	if o.VerifyChecksum {
		if expectedChecksum, ok := o.ExpectedChecksums[u]; ok && expectedChecksum != "" {
			if err := o.verifyFileChecksum(filepath, expectedChecksum); err != nil {
				return fmt.Errorf("checksum verification failed: %w", err)
			}
		}
	}

	return nil
}

// rateLimitedReader implements a rate-limited io.ReadCloser
type rateLimitedReader struct {
	r       io.ReadCloser
	limiter *rate.Limiter
}

func (r *rateLimitedReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	if n > 0 {
		if err := r.limiter.WaitN(context.Background(), n); err != nil {
			return n, err
		}
	}
	return
}

func (r *rateLimitedReader) Close() error {
	if closer, ok := r.r.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// verifyFileChecksum verifies the checksum of a file
func (o *Options) verifyFileChecksum(filepath, expectedChecksum string) error {
	// Parse the checksum format "algorithm:checksum"
	parts := strings.SplitN(expectedChecksum, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid checksum format, expected 'algorithm:checksum'")
	}

	algorithm := strings.ToLower(parts[0])
	checksum := parts[1]

	// Open the file
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum verification: %w", err)
	}
	defer file.Close()

	var h hash.Hash

	// Create the appropriate hasher
	switch algorithm {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	default:
		return fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}

	// Calculate the hash
	if _, err := io.Copy(h, file); err != nil {
		return fmt.Errorf("failed to read file for checksum calculation: %w", err)
	}

	// Compare the checksums
	calculatedChecksum := hex.EncodeToString(h.Sum(nil))
	if calculatedChecksum != checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", checksum, calculatedChecksum)
	}

	return nil
}
