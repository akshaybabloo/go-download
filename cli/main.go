package main

import "github.com/akshaybabloo/go-download"

func main() {
	d := download.NewDownload()
	d.SetURLs([]string{"https://pub-006fd807fb3849f392698256347c1606.r2.dev/OrcaSlicer_Linux_V2.2.0.AppImage", "https://cdn.hashnode.com/res/hashnode/image/upload/v1735568898507/db93197c-fbed-454d-8134-49b398c4a5df.png", "https://pub-006fd807fb3849f392698256347c1606.r2.dev/debian-12.8.0-amd64-netinst.iso", "https://pub-006fd807fb3849f392698256347c1606.r2.dev/ubuntu-24.04.1-desktop-amd64.iso"})
	err := d.SetTitle("temp")
	if err != nil {
		panic(err)
	}
	d.SetParallel(5)
	d.ShowProgress = true
	err = d.Start()
	if err != nil {
		panic(err)
	}
}
