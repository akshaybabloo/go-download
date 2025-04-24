package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/akshaybabloo/go-download"
)

// Config represents the JSON configuration file structure
type Config struct {
	URLs            []string          `json:"urls"`
	Output          string            `json:"output"`
	Title           string            `json:"title"`
	Parallel        int               `json:"parallel"`
	ShowProgress    bool              `json:"show_progress"`
	ResumeDownload  bool              `json:"resume_download"`
	BandwidthLimit  int               `json:"bandwidth_limit"`
	Timeout         int               `json:"timeout"`
	Retry           int               `json:"retry"`
	Headers         map[string]string `json:"headers"`
	CustomFilenames map[string]string `json:"custom_filenames"`
	BasicAuth       struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"basic_auth"`
	BearerToken       string            `json:"bearer_token"`
	VerifyChecksum    bool              `json:"verify_checksum"`
	ExpectedChecksums map[string]string `json:"expected_checksums"`
}

func main() {
	app := &cli.Command{
		Name:  "gdl",
		Usage: "Download multiple files asynchronously",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:    "url",
				Aliases: []string{"u"},
				Usage:   "URLs to download (can be specified multiple times)",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output directory",
				Value:   ".",
			},
			&cli.StringFlag{
				Name:    "title",
				Aliases: []string{"t"},
				Usage:   "Title of the download folder (can be a URL to extract title from)",
			},
			&cli.IntFlag{
				Name:    "parallel",
				Aliases: []string{"p"},
				Usage:   "Number of parallel downloads",
				Value:   1,
			},
			&cli.BoolFlag{
				Name:  "progress",
				Usage: "Show progress bars",
				Value: true,
			},
			&cli.BoolFlag{
				Name:  "resume",
				Usage: "Enable resuming interrupted downloads",
				Value: false,
			},
			&cli.IntFlag{
				Name:  "bandwidth",
				Usage: "Bandwidth limit in KB/s (0 for unlimited)",
				Value: 0,
			},
			&cli.IntFlag{
				Name:  "timeout",
				Usage: "Timeout in seconds",
				Value: 10,
			},
			&cli.IntFlag{
				Name:  "retry",
				Usage: "Number of retries",
				Value: 3,
			},
			&cli.StringSliceFlag{
				Name:  "header",
				Usage: "Custom headers in format 'Key:Value' (can be specified multiple times)",
			},
			&cli.StringSliceFlag{
				Name:  "filename",
				Usage: "Custom filenames in format 'URL:Filename' (can be specified multiple times)",
			},
			&cli.StringFlag{
				Name:  "basic-auth",
				Usage: "Basic authentication in format 'username:password'",
			},
			&cli.StringFlag{
				Name:  "bearer",
				Usage: "Bearer token for authentication",
			},
			&cli.BoolFlag{
				Name:  "verify-checksum",
				Usage: "Enable checksum verification",
				Value: false,
			},
			&cli.StringSliceFlag{
				Name:  "checksum",
				Usage: "Expected checksums in format 'URL:algorithm:checksum' (can be specified multiple times)",
			},
			&cli.StringFlag{
				Name:  "config",
				Usage: "Path to JSON configuration file",
			},
		},

		Action: func(cli context.Context, c *cli.Command) error {
			d := download.NewDownload()

			// Load from config file if specified
			if configPath := c.String("config"); configPath != "" {
				if err := loadConfig(configPath, d); err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}
			}

			// Command line flags override config file

			// Set URLs
			urls := c.StringSlice("url")
			if len(urls) > 0 {
				d.SetURLs(urls)
			}

			// Set output directory
			if output := c.String("output"); output != "" {
				d.SetOutput(output)
			}

			// Set title
			if title := c.String("title"); title != "" {
				if err := d.SetTitle(title); err != nil {
					return fmt.Errorf("failed to set title: %w", err)
				}
			}

			// Set parallel downloads
			if parallel := c.Int("parallel"); parallel > 0 {
				d.SetParallel(parallel)
			}

			// Set progress bar
			d.ShowProgress = c.Bool("progress")

			// Set resume download
			d.ResumeDownload = c.Bool("resume")

			// Set bandwidth limit
			if bandwidth := c.Int("bandwidth"); bandwidth > 0 {
				d.BandwidthLimit = bandwidth
			}

			// Set timeout
			if timeout := c.Int("timeout"); timeout > 0 {
				d.Timeout = timeout
			}

			// Set retry count
			if retry := c.Int("retry"); retry >= 0 {
				d.Retry = retry
			}

			// Set custom headers
			for _, header := range c.StringSlice("header") {
				parts := strings.SplitN(header, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					d.Headers[key] = value
				}
			}

			// Set custom filenames
			for _, filename := range c.StringSlice("filename") {
				parts := strings.SplitN(filename, ":", 2)
				if len(parts) == 2 {
					url := strings.TrimSpace(parts[0])
					name := strings.TrimSpace(parts[1])
					d.SetCustomFilename(url, name)
				}
			}

			// Set basic authentication
			if basicAuth := c.String("basic-auth"); basicAuth != "" {
				parts := strings.SplitN(basicAuth, ":", 2)
				if len(parts) == 2 {
					username := parts[0]
					password := parts[1]
					d.SetBasicAuth(username, password)
				}
			}

			// Set bearer token
			if bearer := c.String("bearer"); bearer != "" {
				d.SetBearerToken(bearer)
			}

			// Set checksum verification
			d.VerifyChecksum = c.Bool("verify-checksum")

			// Set expected checksums
			for _, checksumStr := range c.StringSlice("checksum") {
				parts := strings.SplitN(checksumStr, ":", 3)
				if len(parts) == 3 {
					url := strings.TrimSpace(parts[0])
					algorithm := strings.TrimSpace(parts[1])
					checksum := strings.TrimSpace(parts[2])
					d.SetExpectedChecksum(url, algorithm+":"+checksum)
				}
			}

			// Validate URLs
			if len(urls) == 0 && c.String("config") == "" {
				return fmt.Errorf("no URLs specified")
			}

			// Start download
			fmt.Println("Starting downloads...")
			if err := d.Start(); err != nil {
				return fmt.Errorf("download failed: %w", err)
			}

			fmt.Println("Downloads completed successfully!")
			return nil
		},
	}

	err := app.Run(context.Background(), os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

// loadConfig loads configuration from a JSON file
func loadConfig(path string, d *download.Options) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	// Apply configuration
	if len(config.URLs) > 0 {
		d.SetURLs(config.URLs)
	}

	if config.Output != "" {
		d.SetOutput(config.Output)
	}

	if config.Title != "" {
		if err := d.SetTitle(config.Title); err != nil {
			return err
		}
	}

	if config.Parallel > 0 {
		d.SetParallel(config.Parallel)
	}

	d.ShowProgress = config.ShowProgress
	d.ResumeDownload = config.ResumeDownload

	if config.BandwidthLimit > 0 {
		d.BandwidthLimit = config.BandwidthLimit
	}

	if config.Timeout > 0 {
		d.Timeout = config.Timeout
	}

	if config.Retry >= 0 {
		d.Retry = config.Retry
	}

	// Set headers
	for k, v := range config.Headers {
		d.Headers[k] = v
	}

	// Set custom filenames
	for url, filename := range config.CustomFilenames {
		d.SetCustomFilename(url, filename)
	}

	// Set basic auth
	if config.BasicAuth.Username != "" && config.BasicAuth.Password != "" {
		d.SetBasicAuth(config.BasicAuth.Username, config.BasicAuth.Password)
	}

	// Set bearer token
	if config.BearerToken != "" {
		d.SetBearerToken(config.BearerToken)
	}

	// Set checksum verification
	d.VerifyChecksum = config.VerifyChecksum

	// Set expected checksums
	for url, checksum := range config.ExpectedChecksums {
		d.SetExpectedChecksum(url, checksum)
	}

	return nil
}
