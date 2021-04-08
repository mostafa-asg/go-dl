package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/schollz/progressbar/v3"
)

type Config struct {
	Url         string
	Concurrency int
	OutputDir   string

	// output filename
	Filename       string
	CopyBufferSize int

	// is in resume mode?
	Resume         bool
	fullOutputPath string
}

// returns filename and it's extention
func getFilenameAndExt(fileName string) (string, string) {
	ext := filepath.Ext(fileName)
	return strings.TrimSuffix(fileName, ext), ext
}

type downloader struct {
	// true if the download has been paused
	Paused bool
	config *Config

	// use to pause the download gracefully
	context context.Context
	cancel  context.CancelFunc

	bar *progressbar.ProgressBar
}

func (d *downloader) Pause() {
	d.Paused = true
	d.cancel()
}

func (d *downloader) Resume() {
	d.config.Resume = true
	d.Download()
}

// Returns the progress bar's state
func (d *downloader) ProgressState() progressbar.State {
	if d.bar != nil {
		return d.bar.State()
	}

	return progressbar.State{}
}

// Add a number to the filename if file already exist
// For instance, if filename `hello.pdf` already exist
// it returns hello(1).pdf
func (d *downloader) renameFilenameIfNecessary() {
	if d.config.Resume {
		return // in resume mode, no need to rename
	}

	fullOutputPath := path.Join(d.config.OutputDir, d.config.Filename)

	if _, err := os.Stat(fullOutputPath); err == nil {
		counter := 1
		filename, ext := getFilenameAndExt(d.config.Filename)

		for err == nil {
			log.Printf("File %s already exist", d.config.Filename)
			d.config.Filename = fmt.Sprintf("%s(%d)%s", filename, counter, ext)
			fullOutputPath = path.Join(d.config.OutputDir, d.config.Filename)
			_, err = os.Stat(fullOutputPath)
			counter += 1
		}
	}
}

func New(url string) (*downloader, error) {
	if url == "" {
		return nil, errors.New("Url is empty")
	}

	config := &Config{
		Url:         url,
		Concurrency: 1,
	}

	return NewFromConfig(config)
}

func NewFromConfig(config *Config) (*downloader, error) {
	if config.Url == "" {
		return nil, errors.New("Url is empty")
	}
	if config.Concurrency < 1 {
		config.Concurrency = 1
		log.Print("Concurrency level: 1")
	}
	if config.OutputDir == "" {
		config.OutputDir = "."
	}
	if config.Filename == "" {
		config.Filename = path.Base(config.Url)
	}
	if config.CopyBufferSize == 0 {
		config.CopyBufferSize = 1024
	}

	d := &downloader{config: config}
	ctx, cancel := context.WithCancel(context.Background())
	d.context = ctx
	d.cancel = cancel

	// rename file if such file already exist
	d.renameFilenameIfNecessary()
	log.Printf("Output file: %s", config.Filename)
	config.fullOutputPath = path.Join(config.OutputDir, config.Filename)
	return d, nil
}

func (d *downloader) getPartFilename(partNum int) string {
	return d.config.fullOutputPath + ".part" + strconv.Itoa(partNum)
}

func (d *downloader) Download() {
	res, err := http.Head(d.config.Url)
	if err != nil {
		log.Fatal(err)
	}

	if res.StatusCode == http.StatusOK && res.Header.Get("Accept-Ranges") == "bytes" {
		contentSize, err := strconv.Atoi(res.Header.Get("Content-Length"))
		if err != nil {
			log.Fatal(err)
		}
		d.multiDownload(contentSize)
	} else {
		d.simpleDownload()
	}
}

// Server does not support partial download for this file
func (d *downloader) simpleDownload() {
	if d.config.Resume {
		log.Fatal("Cannot resume. Must be downloaded again")
	}

	// make a request
	res, err := http.Get(d.config.Url)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	// create the output file
	f, err := os.OpenFile(d.config.fullOutputPath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	d.bar = progressbar.DefaultBytes(int64(res.ContentLength), "downloading")

	// copy to output file
	buffer := make([]byte, d.config.CopyBufferSize)
	_, err = io.CopyBuffer(io.MultiWriter(f, d.bar), res.Body, buffer)
	if err != nil {
		log.Fatal(err)
	}
}

// download concurrently
func (d *downloader) multiDownload(contentSize int) {
	partSize := contentSize / d.config.Concurrency

	startRange := 0
	wg := &sync.WaitGroup{}
	wg.Add(d.config.Concurrency)

	d.bar = progressbar.DefaultBytes(int64(contentSize), "downloading")

	for i := 1; i <= d.config.Concurrency; i++ {

		// handle resume
		downloaded := 0
		if d.config.Resume {
			filePath := d.getPartFilename(i)
			f, err := os.Open(filePath)
			if err == nil {
				fileInfo, err := f.Stat()
				if err == nil {
					downloaded = int(fileInfo.Size())
					// update progress bar
					d.bar.Add64(int64(downloaded))
				}
			}
		}

		if i == d.config.Concurrency {
			go d.downloadPartial(startRange+downloaded, contentSize, i, wg)
		} else {
			go d.downloadPartial(startRange+downloaded, startRange+partSize, i, wg)
		}

		startRange += partSize + 1
	}

	wg.Wait()
	if !d.Paused {
		d.merge()
	}
}

func (d *downloader) merge() {
	destination, err := os.OpenFile(d.config.fullOutputPath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer destination.Close()

	for i := 1; i <= d.config.Concurrency; i++ {
		filename := d.getPartFilename(i)
		source, err := os.OpenFile(filename, os.O_RDONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		io.Copy(destination, source)
		source.Close()
		os.Remove(filename)
	}
}

func (d *downloader) downloadPartial(rangeStart, rangeStop int, partialNum int, wg *sync.WaitGroup) {
	defer wg.Done()
	if rangeStart >= rangeStop {
		// nothing to download
		return
	}

	// create a request
	req, err := http.NewRequest("GET", d.config.Url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeStop))

	// make a request
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	// create the output file
	outputPath := d.getPartFilename(partialNum)
	flags := os.O_CREATE | os.O_WRONLY
	if d.config.Resume {
		flags = flags | os.O_APPEND
	}
	f, err := os.OpenFile(outputPath, flags, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// copy to output file
	for {
		select {
		case <-d.context.Done():
			return
		default:
			_, err = io.CopyN(io.MultiWriter(f, d.bar), res.Body, int64(d.config.CopyBufferSize))
			if err != nil {
				if err == io.EOF {
					return
				} else {
					log.Fatal(err)
				}
			}
		}
	}
}
