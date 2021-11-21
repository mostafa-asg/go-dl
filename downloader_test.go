package downloader

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestDetectingFilename(t *testing.T) {
	testCases := []struct {
		URL      string
		Filename string
	}{
		{
			URL:      "http://www.yahoo.com/index.html",
			Filename: "index.html",
		},
		{
			URL:      "http://movie.com/a/k1.mkv?auth=1",
			Filename: "k1.mkv",
		},
	}

	for _, testCase := range testCases {
		actual := detectFilename(testCase.URL)
		if actual != testCase.Filename {
			t.Errorf("Expected filename to be %s, got %s", testCase.Filename, actual)
		}
	}
}

func TestDownload(t *testing.T) {
	files := http.Dir("./files/")
	portCh := make(chan int, 1)

	go func() {
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			log.Fatal(err)
		}
		// notify the port to others
		portCh <- listener.Addr().(*net.TCPAddr).Port
		log.Fatal(http.Serve(listener, http.FileServer(files)))
	}()

	port := <-portCh

	// wait for fileserver to initialize
	time.Sleep(2 * time.Second)

	outFile, err := ioutil.TempFile("", "go_dl_temp_file")
	if err != nil {
		t.Fatal("Coudn't create the output file")
	}
	outFile.Close()
	// We just want to use this temp filename, so we delete the file,
	// otherwise downloader creates a new file
	os.Remove(outFile.Name())

	downloadConfig := Config{
		Url:         fmt.Sprintf("http://localhost:%d/book.pdf", port),
		Concurrency: 1,
		OutFilename: outFile.Name(),
	}
	d, err := NewFromConfig(&downloadConfig)
	if err != nil {
		t.Fatal("Coudn't initialize downloader")
	}
	d.Download()

	original, err := ioutil.ReadFile("./files/book.pdf")
	if err != nil {
		t.Fatal("Cannot read ./files/book.pdf")
	}

	downloaded, err := ioutil.ReadFile(outFile.Name())
	if err != nil {
		t.Fatalf("Cannot read %s", outFile.Name())
	}

	equal := bytes.Equal(original, downloaded)
	if !equal {
		t.Error("Downloaded file is not the same as original file")
	}

	os.Remove(outFile.Name())
}

func TestParallelDownload(t *testing.T) {
	files := http.Dir("./files/")
	portCh := make(chan int, 1)
	downloadCompleted := make(chan bool, 1)

	go func() {
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			log.Fatal(err)
		}
		// notify the port to others
		portCh <- listener.Addr().(*net.TCPAddr).Port
		log.Fatal(http.Serve(listener, http.FileServer(files)))
	}()

	port := <-portCh

	// wait for fileserver to initialize
	time.Sleep(2 * time.Second)

	outFile, err := ioutil.TempFile("", "go_dl_temp_file")
	if err != nil {
		t.Fatal("Coudn't create the output file")
	}
	outFile.Close()
	// We just want to use this temp filename, so we delete the file,
	// otherwise downloader creates a new file
	os.Remove(outFile.Name())

	downloadConfig := Config{
		Url:            fmt.Sprintf("http://localhost:%d/book.pdf", port),
		Concurrency:    4,
		OutFilename:    outFile.Name(),
		CopyBufferSize: 1, // in order to download it very slowly
	}
	d, err := NewFromConfig(&downloadConfig)
	if err != nil {
		t.Fatal("Coudn't initialize downloader")
	}

	go func() {
		d.Download()
		fmt.Printf("\nInterupted ...\n")

		// first interupt, continue download again
		d.Resume()
		fmt.Printf("\nInterupted ...\n")

		// second interupt, continue download again
		d.Resume()
		fmt.Printf("\nInterupted ...\n")

		// third interupt, continue download again
		d.Resume()

		downloadCompleted <- true
	}()

	go func() {
		// paused at 20 percent
		p20 := false

		// paused at 50 percent
		p50 := false

		// paused at 80 percent
		p80 := false

		for {
			percent := d.ProgressState().CurrentPercent

			if p20 == false && percent >= 0.2 && percent <= 0.3 {
				d.Pause()
				p20 = true
			}

			if p50 == false && percent >= 0.5 && percent <= 0.6 {
				d.Pause()
				p50 = true
			}

			if p80 == false && percent >= 0.8 && percent <= 0.9 {
				d.Pause()
				p80 = true
			}

			if p20 && p50 && p80 {
				break
			}
		}
	}()

	// wait for download
	<-downloadCompleted

	original, err := ioutil.ReadFile("./files/book.pdf")
	if err != nil {
		t.Fatal("Cannot read ./files/book.pdf")
	}

	downloaded, err := ioutil.ReadFile(outFile.Name())
	if err != nil {
		t.Fatalf("Cannot read %s", outFile.Name())
	}

	equal := bytes.Equal(original, downloaded)
	if !equal {
		t.Error("Downloaded file is not the same as original file")
	}

	os.Remove(outFile.Name())
}
