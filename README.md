# Go Download

CLI and SDK to download multiple files asynchronously

## Installation

```bash
go get -u github.com/akshaybabloo/go-download
```

or use the CLI from the [releases](https://github.com/akshaybabloo/go-download/releases) page.

## Usage

### CLI

```bash
gdl --url https://example.com/file1.txt https://example.com/file2.txt \
    --output /path/to/output/directory \
    --title title \
    --parallel 5
```

or use the a JSON file

```json
{
    "urls": ["https://example.com/file1.txt", "https://example.com/file2.txt"],
    "output": "/path/to/output/directory",
    "title": "title",
    "parallel": 5
}
```

### SDK

```go
package main

import "github.com/akshaybabloo/go-download"

func main() {
    d := download.NewDownload()
    d.SetURLs([]string{"https://example.com/file1.txt", "https://example.com/file2.txt"})
    d.SetOutput("/path/to/output/directory")
    d.SetTitle("title")
    d.SetParallel(5)
	d.ShowProgress = true
    d.Start()
}
```
