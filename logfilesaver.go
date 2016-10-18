package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/screwdriver-cd/log-service/sdstoreuploader"
)

// storedLogLine is a representation of logs for permanent storage in the Store
type storedLogLine struct {
	Time    int64  `json:"t"`
	Message string `json:"m"`
	Line    int    `json:"n"`
}

type logFile struct {
	lineCount      int
	savedLineCount int
	mutex          *sync.RWMutex
	storePath      string
	uploader       sdstoreuploader.SDStoreUploader
	file           *os.File
}

// newLogFile returns a logFile object for saving a single file to the Store.
func newLogFile(uploader sdstoreuploader.SDStoreUploader, storePath string) (*logFile, error) {
	file, err := ioutil.TempFile("", filepath.Base(storePath))
	if err != nil {
		return &logFile{}, fmt.Errorf("creating temporary file for %s: %v", storePath, err)
	}

	return &logFile{
		mutex:     &sync.RWMutex{},
		storePath: storePath,
		uploader:  uploader,
		file:      file,
	}, nil
}

// Save synchronously saves the logfile to the data store
func (l *logFile) Save() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.lineCount == l.savedLineCount {
		return nil
	}

	log.Println("Uploading", l.file.Name())
	err := l.uploader.Upload(l.storePath, l.file.Name())
	if err == nil {
		l.savedLineCount = l.lineCount
	}

	return err
}

// Write is an io.Writer that writes to the logfile.
func (l *logFile) Write(p []byte) (int, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	n, err := l.file.Write(p)
	if err == nil {
		l.lineCount++
	}
	return n, err
}

// Close closes the underlying file handle in the logFile object
func (l *logFile) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// If the file is already closed, make this a no-op
	if l.file == nil {
		return nil
	}

	f := l.file
	if err := f.Close(); err != nil {
		return err
	}

	l.file = nil

	return os.Remove(f.Name())
}
