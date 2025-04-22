# Go Download

CLI and SDK to download multiple files asynchronously with advanced features like resume support, bandwidth limiting, and checksum verification.

## Features

- Download multiple files in parallel
- Resume interrupted downloads
- Bandwidth limiting
- Progress bars with ETA and speed
- Custom filenames
- Authentication support (Basic Auth and Bearer Token)
- Checksum verification (MD5, SHA1, SHA256)
- Configurable retries and timeouts
- JSON configuration file support

## Installation

```bash
go get -u github.com/akshaybabloo/go-download
```

or use the CLI from the [releases](https://github.com/akshaybabloo/go-download/releases) page.

## Usage

### CLI

The CLI tool `gdl` provides a convenient way to download files from the command line:

```bash
# Basic usage
gdl --url https://example.com/file1.txt --url https://example.com/file2.txt \
    --output /path/to/output/directory \
    --title "My Downloads" \
    --parallel 5

# With resume and bandwidth limiting
gdl --url https://example.com/large-file.zip \
    --resume \
    --bandwidth 500 \
    --progress

# With custom filename
gdl --url https://example.com/file-with-long-name.txt \
    --filename "https://example.com/file-with-long-name.txt:short-name.txt"

# With authentication
gdl --url https://protected-site.com/file.zip \
    --basic-auth "username:password"

# With checksum verification
gdl --url https://example.com/important-file.iso \
    --verify-checksum \
    --checksum "https://example.com/important-file.iso:sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
```

#### Using a Configuration File

For more complex downloads, you can use a JSON configuration file:

```bash
gdl --config download-config.json
```

Example configuration file:

```json
{
  "urls": [
    "https://example.com/file1.zip",
    "https://example.com/file2.pdf"
  ],
  "output": "./downloads",
  "title": "my-downloads",
  "parallel": 2,
  "show_progress": true,
  "resume_download": true,
  "bandwidth_limit": 500,
  "timeout": 30,
  "retry": 3,
  "headers": {
    "User-Agent": "Go-Download-Client/1.0"
  },
  "custom_filenames": {
    "https://example.com/file1.zip": "custom-name.zip"
  },
  "verify_checksum": true,
  "expected_checksums": {
    "https://example.com/file1.zip": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
  }
}
```

### SDK

The Go Download SDK provides a flexible way to integrate download functionality into your Go applications:

```go
package main

import (
    "fmt"
    "github.com/akshaybabloo/go-download"
)

func main() {
    // Create a new download with default options
    d := download.NewDownload()
    
    // Set URLs to download
    d.SetURLs([]string{
        "https://example.com/file1.txt",
        "https://example.com/file2.txt",
    })
    
    // Configure download options
    d.SetOutput("/path/to/output/directory")
    d.SetTitle("My Downloads")
    d.SetParallel(5)
    
    // Enable advanced features
    d.ShowProgress = true
    d.ResumeDownload = true
    d.BandwidthLimit = 500 // KB/s
    
    // Set custom filename for a specific URL
    d.SetCustomFilename(
        "https://example.com/file2.txt",
        "custom-name.txt",
    )
    
    // Set authentication if needed
    d.SetBasicAuth("username", "password")
    // or
    d.SetBearerToken("your-token-here")
    
    // Enable checksum verification
    d.VerifyChecksum = true
    d.SetExpectedChecksum(
        "https://example.com/file1.txt",
        "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    )
    
    // Start the download
    err := d.Start()
    if err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

## Advanced Features

### Resume Downloads

The resume functionality allows interrupted downloads to be continued from where they left off, saving bandwidth and time:

```go
d := download.NewDownload()
d.ResumeDownload = true
```

### Bandwidth Limiting

Control the download speed to avoid saturating your network connection:

```go
d := download.NewDownload()
d.BandwidthLimit = 500 // Limit to 500 KB/s
```

### Checksum Verification

Verify the integrity of downloaded files using MD5, SHA1, or SHA256 checksums:

```go
d := download.NewDownload()
d.VerifyChecksum = true
d.SetExpectedChecksum(
    "https://example.com/file.zip",
    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
)
```

### Authentication

Access protected resources with authentication:

```go
d := download.NewDownload()
// Basic authentication
d.SetBasicAuth("username", "password")
// Or Bearer token
d.SetBearerToken("your-token-here")
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
