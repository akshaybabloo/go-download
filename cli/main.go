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
		"https://pub-006fd807fb3849f392698256347c1606.r2.dev/OrcaSlicer_Linux_V2.2.0.AppImage",
		"https://cdn.hashnode.com/res/hashnode/image/upload/v1735568898507/db93197c-fbed-454d-8134-49b398c4a5df.png",
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
	d.SetParallel(2)

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
