package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"path"
	"sync"
	"time"

	"github.com/screwdriver-cd/log-service/sdstoreuploader"
	"github.com/screwdriver-cd/log-service/screwdriver"
)

// logLine is a representation of log lines coming from the Screwdriver launcher
type logLine struct {
	Time    int64  `json:"t"`
	Message string `json:"m"`
	Step    string `json:"s"`
}

// String stringifies the logLine for humans to read.
func (l *logLine) String() string {
	return fmt.Sprintf("{t:%d, m:\"%s\", s:\"%s\"}", l.Time, l.Message, l.Step)
}

// StepSaver deals with saving the logs for a single step
type StepSaver interface {
	Close() error
	WriteLog(line *logLine) error
	Write(p []byte) (int, error)
}

type stepSaver struct {
	StepName       string
	Uploader       sdstoreuploader.SDStoreUploader
	ScrewdriverAPI screwdriver.API
	lineCount      int
	savedLineCount int
	logFiles       []*logFile
	encoder        *json.Encoder
	ticker         *time.Ticker
	mutex          sync.Mutex
	linesPerFile   int
}

// Close cancels the save ticker, saves the logs for this step, and closes the logFiles.
// If it gets an error while closing, it stops immediately and returns the error.
func (s *stepSaver) Close() error {
	s.ticker.Stop()
	err := s.Save()
	if err != nil {
		return fmt.Errorf("saving on stepSaver Close: %v", err)
	}

	for _, f := range s.logFiles {
		if err := f.Close(); err != nil {
			return err
		}
	}

	log.Println("Completed step processing for", s.StepName)

	if err = s.ScrewdriverAPI.UpdateStepLines(s.StepName, s.lineCount); err != nil {
		return fmt.Errorf("Updating step meta lines: %v", err)
	}

	log.Println("Set step lines to", s.lineCount)

	return nil
}

// WriteLog takes a logLine, converts it for storage, and uploads to the SD Store with its uploader.
// It splits logs into pieces and uploads them separately and incrementally.
func (s *stepSaver) WriteLog(l *logLine) error {
	storedLine := storedLogLine{
		Time:       l.Time,
		Message:    l.Message,
		Line:       s.lineCount,
		StepName:   l.Step,
	}

	if len(storedLine.Message) > maxLineSize {
		var buffer bytes.Buffer
		buffer.WriteString(storedLine.Message[:maxLineSize])
		buffer.WriteString(fmt.Sprintf(" [line truncated after %d characters]", maxLineSize))
		storedLine.Message = buffer.String()
	}
	if err := s.encoder.Encode(storedLine); err != nil {
		return fmt.Errorf("marshaling log line %v: %v", storedLine, err)
	}

	return nil
}

// newLogFile is a helper for adding a logFile to the internal collection of logFiles.
func (s *stepSaver) newLogFile(storePath string) error {
	lf, err := newLogFile(s.Uploader, storePath)
	if err != nil {
		return err
	}
	s.mutex.Lock()
	s.logFiles = append(s.logFiles, lf)
	s.mutex.Unlock()
	return nil
}

// LogFiles returns the logFile slice in a concurrency-safe fashion
func (s *stepSaver) LogFiles() []*logFile {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.logFiles
}

// Write implements io.Writer for writing raw text to logFiles. It selects the logFile
// to write to based on the current line count, making new logFiles as necessary.
func (s *stepSaver) Write(p []byte) (int, error) {
	defer func() { s.lineCount++ }()

	fileNum := s.lineCount / s.linesPerFile

	// We have passed the linePerFile limit and need to create a new file
	if fileNum >= len(s.LogFiles()) {
		log.Println("Making a new log file:", fileNum, s.StepName)

		// Save the old file one last time before proceeding
		if fileNum > 0 {
			log.Println("About to save log file:", fileNum-1, s.StepName)
			go func() {
				err := s.LogFiles()[fileNum-1].Save()
				if err != nil {
					log.Printf("Error encountered saving logs: %v", err)
				}
			}()
		}

		logpath := fmt.Sprintf("log.%d", fileNum)
		destination := path.Join(s.StepName, logpath)
		err := s.newLogFile(destination)
		if err != nil {
			return 0, fmt.Errorf("creating log #%d for step %s: %v", fileNum, s.StepName, err)
		}
	}

	n, err := s.LogFiles()[fileNum].Write(p)
	if err != nil {
		err = fmt.Errorf("writing to log #%d for step %s: %v", fileNum, s.StepName, err)
	}
	return n, err
}

// Save concurrently saves all logFiles, waiting for them all to complete.
func (s *stepSaver) Save() error {
	var wg sync.WaitGroup
	for _, f := range s.LogFiles() {
		wg.Add(1)
		go func(f *logFile) {
			defer wg.Done()

			err := f.Save()
			if err != nil {
				log.Println("ERROR saving logfile:", err)
			}
		}(f)
	}

	wg.Wait()
	return nil
}

// NewStepSaver creates a StepSaver out of a name and sdstoreuploader.SDStoreUploader
func NewStepSaver(name string, uploader sdstoreuploader.SDStoreUploader, linesPerFile int, screwdriverAPI screwdriver.API) StepSaver {
	s := &stepSaver{StepName: name, Uploader: uploader, ticker: time.NewTicker(uploadInterval), linesPerFile: linesPerFile, ScrewdriverAPI: screwdriverAPI}
	e := json.NewEncoder(s)
	s.encoder = e

	go func(s *stepSaver) {
		for range s.ticker.C {
			err := s.Save()
			if err != nil {
				log.Println("Error saving logs: ", err)
			}
		}
	}(s)

	return s
}
