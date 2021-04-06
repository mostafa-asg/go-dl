package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	downloader "github.com/mostafa-asg/go-dl"
)

func main() {
	url := flag.String("u", "", "* Download url")
	concurrency := flag.Int("n", 1, "Concurrency level")
	outputDir := flag.String("o", ".", "Output directory")
	filename := flag.String("f", "", "Output file name")
	bufferSize := flag.Int("buffer-size", 32*1024, "The buffer size to copy from http response body")
	resume := flag.Bool("resume", false, "Resume the download")

	flag.Parse()
	if *url == "" {
		log.Fatal("Please specify the url using -u parameter")
	}

	config := &downloader.Config{
		Url:            *url,
		Concurrency:    *concurrency,
		OutputDir:      *outputDir,
		Filename:       *filename,
		CopyBufferSize: *bufferSize,
		Resume:         *resume,
	}
	d, err := downloader.NewFromConfig(config)
	if err != nil {
		log.Fatal(err.Error())
	}

	termCh := make(chan os.Signal)
	signal.Notify(termCh, os.Interrupt)
	go func() {
		<-termCh
		println("\nExiting ...")
		d.Pause()
	}()

	d.Download()
	if d.Paused {
		println("\nDownload has paused. Resume it again with -resume=true parameter.")
	} else {
		println("Downloadd completed.")
	}
}
