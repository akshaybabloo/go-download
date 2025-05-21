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

	return &Options{
		Headers:           map[string]string{},
		Retry:             3,
		ShowProgress:      false,
		DownloadPath:      ".",
		Parallel:          5,
		Timeout:           10000,
		Title:             "",
		ResumeDownload:    false,
		BandwidthLimit:    0,
		CustomFilenames:   make(map[string]string),
		VerifyChecksum:    false,
		ExpectedChecksums: make(map[string]string),
	}
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
	totalFiles := len(o.urls)

	overallBar := p.AddBar(int64(totalFiles),
		mpb.PrependDecorators(
			decor.Name("Overall Progress: ", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
			decor.CountersNoUnit("%d / %d files", decor.WCSyncWidth),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WCSyncWidth),
		),
	)

	for _, u := range o.urls {
		parsedURL, _ := url.Parse(u)
		filename := path.Base(parsedURL.Path)
		if filename == "" || filename == "." {
			filename = "download"
		}

		var fileSize int64
		headResp, headErr := http.Head(u)
		if headErr == nil && headResp.StatusCode == http.StatusOK {
			fileSize = headResp.ContentLength
			if headResp.Body != nil {
				headResp.Body.Close()
			}
		}

		fileBar := o.newIndividualProgress(p, fileSize, filename)

		err := o.downloadFile(u, fileBar)
		if err != nil {
			if fileBar != nil {
				fileBar.Abort(true)
			}
			if o.Log != nil {
				o.Log.Printf("Error downloading %s: %v", u, err)
			}
		}
		if fileBar != nil && !fileBar.Completed() && !fileBar.Aborted() {
			if fileSize > 0 {
				fileBar.SetCurrent(fileSize)
			}
			fileBar.SetTotal(fileBar.Current(), true)
		}

		overallBar.Increment()
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
	totalFiles := len(o.urls)
	errors := make(chan error, totalFiles)

	overallBar := p.AddBar(int64(totalFiles),
		mpb.PrependDecorators(
			decor.Name("Overall Progress: ", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
			decor.CountersNoUnit("%d / %d files", decor.WCSyncWidth),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WCSyncWidth),
		),
	)

	// Get file sizes in parallel (existing logic, slightly adapted)
	sizes := make(map[string]int64)
	var sizesMutex sync.Mutex
	var sizeWg sync.WaitGroup
	sizeErrors := make(chan error, totalFiles)
	headJobs := make(chan string, totalFiles)

	headWorkerCount := min(o.Parallel, totalFiles)
	if headWorkerCount == 0 && totalFiles > 0 {
		headWorkerCount = 1
	}

	for range headWorkerCount {
		sizeWg.Add(1)
		go func() {
			defer sizeWg.Done()
			for u := range headJobs {
				req, err := http.NewRequest("HEAD", u, nil)
				if err != nil {
					sizeErrors <- fmt.Errorf("failed to create HEAD request for %s: %w", u, err)
					continue
				}
				for k, v := range o.Headers {
					req.Header.Add(k, v)
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(o.Timeout)*time.Second)
				var resp *http.Response
				var respErr error
				for j := 0; j <= o.Retry; j++ {
					resp, respErr = http.DefaultClient.Do(req.WithContext(ctx))
					if respErr == nil {
						break
					}
					if j < o.Retry {
						time.Sleep(time.Second * time.Duration(j+1))
					}
				}
				cancel()

				if respErr != nil {
					sizeErrors <- fmt.Errorf("HEAD request for %s failed after %d retries: %w", u, o.Retry, respErr)
					continue
				}

				if resp.StatusCode == http.StatusOK {
					sizesMutex.Lock()
					sizes[u] = resp.ContentLength
					sizesMutex.Unlock()
				} else if resp.StatusCode != http.StatusNotFound {
					sizeErrors <- fmt.Errorf("unexpected status code %d for HEAD request: %s", resp.StatusCode, u)
				} else {
					sizesMutex.Lock()
					sizes[u] = -1
					sizesMutex.Unlock()
				}
				if resp.Body != nil {
					resp.Body.Close()
				}
			}
		}()
	}

	for _, u := range o.urls {
		headJobs <- u
	}
	close(headJobs)

	sizeWg.Wait()
	close(sizeErrors)

	for err := range sizeErrors {
		if err != nil {
			if o.Log != nil {
				o.Log.Printf("Warning during size fetch: %v", err)
			}
		}
	}

	// Create a channel for download jobs
	downloadJobs := make(chan struct {
		url  string
		size int64
	}, totalFiles)

	// Populate download jobs
	for _, u := range o.urls {
		downloadJobs <- struct {
			url  string
			size int64
		}{url: u, size: sizes[u]}
	}
	close(downloadJobs)

	// Start limited number of workers for actual downloads
	actualParallel := min(o.Parallel, totalFiles)
	if actualParallel == 0 && totalFiles > 0 {
		actualParallel = 1
	}

	for range actualParallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range downloadJobs {
				parsedURL, _ := url.Parse(job.url)
				filename := path.Base(parsedURL.Path)
				if filename == "" || filename == "." {
					filename = "download"
				}

				fileBar := o.newIndividualProgress(p, job.size, filename)

				err := o.downloadFile(job.url, fileBar)
				if err != nil {
					if fileBar != nil {
						fileBar.Abort(true)
					}
					errors <- fmt.Errorf("failed to download %s: %w", job.url, err)
				} else {
					if fileBar != nil && !fileBar.Completed() && !fileBar.Aborted() {
						currentSize := fileBar.Current()
						if job.size > 0 && currentSize < job.size {
							fileBar.SetCurrent(job.size)
						}
						fileBar.SetTotal(fileBar.Current(), true)
					}
				}
				overallBar.Increment()
			}
		}()
	}

	p.Wait()

	close(errors)

	var lastErr error
	for err := range errors {
		if err != nil {
			if o.Log != nil {
				o.Log.Println(err)
			}
			if lastErr == nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// newIndividualProgress creates a progress bar for an individual file download
func (o *Options) newIndividualProgress(p *mpb.Progress, size int64, name string) *mpb.Bar {
	if size < 0 {
		size = 0
	}
	return p.AddBar(size,
		mpb.BarFillerClearOnComplete(),
		mpb.PrependDecorators(
			decor.Name(name+": ", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
			decor.CountersKibiByte("% .2f / % .2f"),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WCSyncWidth),
			decor.EwmaSpeed(decor.SizeB1024(0), "% .2f/s", 60),
			decor.Name(" ETA: ", decor.WCSyncWidth),
			decor.EwmaETA(decor.ET_STYLE_GO, 60, decor.WCSyncWidth),
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
	downloadDir := path.Join(o.DownloadPath, o.Title)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	parsedURL, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	var filename string
	if customName, ok := o.CustomFilenames[u]; ok && customName != "" {
		filename = customName
	} else {
		filename = path.Base(parsedURL.Path)
		if filename == "" || filename == "." {
			filename = "download"
		}
	}

	filepath := path.Join(downloadDir, filename)

	headCtx, headCancel := context.WithTimeout(context.Background(), time.Duration(o.Timeout)*time.Second)
	defer headCancel()

	headReq, err := http.NewRequestWithContext(headCtx, "HEAD", u, nil)
	if err != nil {
		return fmt.Errorf("failed to create HEAD request: %w", err)
	}

	for k, v := range o.Headers {
		headReq.Header.Add(k, v)
	}

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

	if headErr != nil {
		return fmt.Errorf("failed to check file existence after %d retries: %w", o.Retry, headErr)
	}
	if headResp != nil {
		defer headResp.Body.Close()
	}

	if headResp == nil {
		return fmt.Errorf("no response received from server")
	}

	if headResp.StatusCode != http.StatusOK && headResp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("file not found on server, status code: %d", headResp.StatusCode)
	}

	contentLength := headResp.ContentLength

	var file *os.File
	var fileSize int64

	if o.ResumeDownload {
		fileInfo, err := os.Stat(filepath)
		if err == nil && fileInfo.Size() > 0 {
			fileSize = fileInfo.Size()
			if contentLength > 0 && fileSize >= contentLength {
				if bar != nil {
					bar.SetTotal(contentLength, true)
				}
				return nil
			}
			file, err = os.OpenFile(filepath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to open file for resuming: %w", err)
			}
		} else {
			file, err = os.Create(filepath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
		}
	} else {
		file, err = os.Create(filepath)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
	}
	defer file.Close()

	downloadCtx, downloadCancel := context.WithTimeout(context.Background(), time.Duration(o.Timeout*10)*time.Second)
	defer downloadCancel()

	req, err := http.NewRequestWithContext(downloadCtx, "GET", u, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range o.Headers {
		req.Header.Add(k, v)
	}

	if o.ResumeDownload && fileSize > 0 {
		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", fileSize))
	}

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
	if resp == nil {
		return fmt.Errorf("no response received from server")
	}
	defer resp.Body.Close()

	if o.ResumeDownload && fileSize > 0 {
		if resp.StatusCode != http.StatusPartialContent {
			if resp.StatusCode == http.StatusOK {
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

	totalSize := resp.ContentLength
	if o.ResumeDownload && resp.StatusCode == http.StatusPartialContent {
		totalSize += fileSize
	}

	var actualReader io.ReadCloser = resp.Body
	if bar != nil {
		if totalSize > 0 {
			bar.SetTotal(totalSize, false)
		}
		if fileSize > 0 {
			bar.SetCurrent(fileSize)
		}
		actualReader = bar.ProxyReader(resp.Body)
	}

	if o.BandwidthLimit > 0 {
		bytesPerSecond := int64(o.BandwidthLimit * 1024)
		actualReader = &rateLimitedReader{
			r:       actualReader,
			limiter: rate.NewLimiter(rate.Limit(bytesPerSecond), int(bytesPerSecond)),
		}
	}

	_, err = io.Copy(file, actualReader)
	if actualReaderCloser, ok := actualReader.(io.Closer); ok {
		actualReaderCloser.Close()
	}

	if err != nil {
		if bar != nil {
			bar.Abort(true)
		}
		return fmt.Errorf("failed to save file: %w", err)
	}

	if bar != nil {
		if !bar.Completed() {
			bar.SetTotal(bar.Current(), true)
		} else if bar.Completed() && totalSize > 0 {
			bar.SetTotal(totalSize, true)
		}
	}

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
	parts := strings.SplitN(expectedChecksum, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid checksum format, expected 'algorithm:checksum'")
	}

	algorithm := strings.ToLower(parts[0])
	checksum := parts[1]

	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum verification: %w", err)
	}
	if file != nil {
		defer file.Close()
	}

	var h hash.Hash

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

	if file == nil {
		return fmt.Errorf("file is nil, cannot calculate checksum")
	}

	if _, err := io.Copy(h, file); err != nil {
		return fmt.Errorf("failed to read file for checksum calculation: %w", err)
	}

	calculatedChecksum := hex.EncodeToString(h.Sum(nil))
	if calculatedChecksum != checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", checksum, calculatedChecksum)
	}

	return nil
}
