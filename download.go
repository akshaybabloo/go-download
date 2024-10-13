package download

import (
	"context"
	"github.com/gocolly/colly"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type Options struct {
	// Headers is the headers to be sent with the request
	Headers map[string]string

	// Context is the context to be used with the request
	// If not provided, context.Background() will be used
	Context context.Context

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
	// If an URL is provided, it will try to get the title from the URL webpage, if not, the domain name will be used.
	Title string

	// Log is the logger to be used
	Log *log.Logger
}

func NewDownload(options ...*Options) *Options {

	if len(options) > 0 {
		options := options[0]
		if options.Log == nil {
			options.Log = log.New(os.Stdout, "", log.LstdFlags)
		}
		return options
	}

	return &Options{
		Headers:      map[string]string{},
		Context:      context.Background(),
		Retry:        3,
		ShowProgress: false,
		DownloadPath: ".",
		Parallel:     1,
		Timeout:      10,
		Title:        "",
	}
}

func (o *Options) httpClient() *http.Client {
	c := &http.Client{
		Timeout: time.Duration(o.Timeout) * time.Second,
	}
	return c
}

func (o *Options) Download(urls []string) {
	if o.Parallel == 1 {
		err := o.downloadSerial(urls)
		if err != nil {
			return
		}
	} else {
		err := o.downloadParallel(urls)
		if err != nil {
			return
		}
	}
}

func (o *Options) downloadSerial(urls []string) error {
	if isUrl(o.Title) {
		err := o.setTitle()
		if err != nil {
			return err
		}
	}

	for _, u := range urls {
		err := o.downloadFile(u)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *Options) downloadParallel(urls []string) error {
	return nil
}

func (o *Options) downloadFile(u string) error {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}

	for k, v := range o.Headers {
		req.Header.Add(k, v)
	}

	resp, err := o.httpClient().Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	file, err := os.Create(path.Join(o.DownloadPath, o.Title, u))
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

func (o *Options) setTitle() error {
	c := colly.NewCollector()
	c.OnHTML("title", func(e *colly.HTMLElement) {
		o.Title = e.Text
	})
	err := c.Visit(o.Title)
	if err != nil {
		return err
	}
	return nil
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
