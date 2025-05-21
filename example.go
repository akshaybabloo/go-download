//go:build example

package main

import (
	"fmt"
	"os"

	"github.com/akshaybabloo/go-download"
)

func main() {
	// Create a new download with default options
	d := download.NewDownload()

	// Set URLs to download
	d.SetURLs([]string{
		"https://github.com/SoftFever/OrcaSlicer/releases/download/v2.3.0/OrcaSlicer_Linux_AppImage_V2.3.0.AppImage",
		"https://github.com/SoftFever/OrcaSlicer/releases/download/v2.3.0/OrcaSlicer_Mac_arm64_V2.3.0.dmg",
		"https://github.com/SoftFever/OrcaSlicer/releases/download/v2.3.0/OrcaSlicer_Mac_x86_64_V2.3.0.dmg",
	})

	// Set download folder title
	err := d.SetTitle("downloads")
	if err != nil {
		fmt.Printf("Error setting title: %v\n", err)
		os.Exit(1)
	}

	// Set output directory
	d.SetOutput("./downloaded_files")

	// Enable parallel downloads (2 at a time)
	d.SetParallel(5)

	// Enable progress bar
	d.ShowProgress = true

	// Enable resume download functionality
	d.ResumeDownload = true

	// Set bandwidth limit to 500 KB/s
	d.BandwidthLimit = 500

	// Set custom filename for a specific URL
	d.SetCustomFilename(
		"https://cdn.hashnode.com/res/hashnode/image/upload/v1735568898507/db93197c-fbed-454d-8134-49b398c4a5df.png",
		"hashnode-image.png",
	)

	// Set basic authentication for a protected URL (example)
	// d.SetBasicAuth("username", "password")

	// Set bearer token for a protected URL (example)
	// d.SetBearerToken("your-token-here")

	// Enable checksum verification
	d.VerifyChecksum = true

	// Set expected checksum for a file (example)
	// Format: "algorithm:checksum"
	// d.SetExpectedChecksum(
	//     "https://example.com/file.zip",
	//     "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	// )

	// Set custom headers if needed
	d.Headers["User-Agent"] = "Go-Download-Client/1.0"

	// Start the download
	fmt.Println("Starting downloads...")
	err = d.Start()
	if err != nil {
		fmt.Printf("Error during download: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Downloads completed successfully!")
}
