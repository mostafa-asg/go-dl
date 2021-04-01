package downloader

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Config struct {
	Url         string
	Concurrency int
	OutputDir   string

	// output filename
	Filename       string
	fullOutputPath string
}

func getFilenameAndExt(fileName string) (string, string) {
	ext := filepath.Ext(fileName)
	return strings.TrimSuffix(fileName, ext), ext
}

func getFullOutputPath(outputDir, filenameWithExt string) string {
	fullOutputPath := path.Join(outputDir, filenameWithExt)

	// Add a number to the filename if file already exist
	// For instance, if filename `hello.pdf` already exist
	// it returns hello(1).pdf
	if _, err := os.Stat(fullOutputPath); err == nil {
		counter := 1
		filename, ext := getFilenameAndExt(filenameWithExt)

		for err == nil {
			log.Printf("File %s alread exist", filenameWithExt)
			filenameWithExt = fmt.Sprintf("%s(%d)%s", filename, counter, ext)
			fullOutputPath = path.Join(outputDir, filenameWithExt)
			_, err = os.Stat(fullOutputPath)
			counter += 1
		}
	}

	return fullOutputPath
}

type downloader struct {
	config *Config
}

func New(url string) (*downloader, error) {
	if url == "" {
		return nil, errors.New("Url is empty")
	}

	filename := path.Base(url)

	config := &Config{
		Url:         url,
		Concurrency: 1,
		OutputDir:   ".",
		Filename:    filename,
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
	config.fullOutputPath = getFullOutputPath(config.OutputDir, config.Filename)
	log.Printf("Output file: %s", config.Filename)

	return &downloader{config: config}, nil
}

func (d *downloader) Download() {
	if d.config.Concurrency == 1 {
		d.simpleDownload()
	} else {
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
}

// download normally, without concurrency
func (d *downloader) simpleDownload() {
	res, err := http.Get(d.config.Url)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	f, err := os.OpenFile(d.config.fullOutputPath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	bytes, err := ioutil.ReadAll(res.Body)
	_, err = f.Write(bytes)
	if err != nil {
		log.Fatal(err)
	}
}

// download concurrently
func (d *downloader) multiDownload(contentSize int) {
	if d.config.Concurrency <= 1 {
		panic("Invalid concurrency value. Should be greater than 1.")
	}
	partSize := contentSize / d.config.Concurrency

	startRange := 0
	wg := &sync.WaitGroup{}
	wg.Add(d.config.Concurrency)

	for i := 1; i <= d.config.Concurrency; i++ {
		if i == d.config.Concurrency {
			go d.downloadPartial(startRange, contentSize, i, wg)
		} else {
			go d.downloadPartial(startRange, startRange+partSize, i, wg)
		}

		startRange += partSize + 1
	}

	wg.Wait()
	d.merge()
}

func (d *downloader) merge() {
	destination, err := os.OpenFile(d.config.fullOutputPath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer destination.Close()

	for i := 1; i <= d.config.Concurrency; i++ {
		filename := d.config.fullOutputPath + ".part" + strconv.Itoa(i)
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

	req, err := http.NewRequest("GET", d.config.Url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeStop))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	outputPath := d.config.fullOutputPath + ".part" + strconv.Itoa(partialNum)
	f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	bytes, err := ioutil.ReadAll(res.Body)
	_, err = f.Write(bytes)
	if err != nil {
		log.Fatal(err)
	}
}
