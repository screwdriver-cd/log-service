package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sync"
	"time"

	"github.com/screwdriver-cd/log-service/s3fileuploader"
)

var (
	emitterPath    = "/var/run/sd/emitter"
	uploadInterval = 2 * time.Second
)

const (
	region         = "us-west-2"
	bucket         = "logs.screwdriver.cd"
	startupTimeout = time.Minute
)

func main() {
	if len(os.Args) != 2 {
		log.Println("No buildID specified. Cannot log.")
		os.Exit(0)
	}

	buildID := os.Args[1]
	log.Println("Processing logs for build", buildID)

	// If we can't open the socket in the first 60s, the sender probably
	// exited before transmitting any data. Since we are reading from
	// a FIFO, we will block forever unless we bail.
	t := time.AfterFunc(startupTimeout, func() {
		log.Printf("No data in the first %s. Assuming catastophe.", startupTimeout)
		os.Exit(0)
	})
	source, err := os.Open(emitterPath)
	t.Stop()
	if err != nil {
		log.Printf("Failed opening %v: %v", emitterPath, err)
		os.Exit(0)
	}

	uploader := s3fileuploader.NewS3FileUploader(region)
	if err := archiveLogs(uploader, buildID, source); err != nil {
		log.Printf("Error moving logs to S3: %v", err)
		os.Exit(0)
	}
}

// archiveLogs copies log lines from src into an S3 bucket.
// Logs are copied to /buildID/stepName
func archiveLogs(uploader s3fileuploader.S3FileUploader, buildID string, src io.Reader) error {
	log.Println("Archiver started")

	var wg sync.WaitGroup
	steps := make(map[string]chan *logLine)
	var lastStep string

	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		line := scanner.Text()

		newLog := &logLine{}
		if err := json.Unmarshal([]byte(line), newLog); err != nil {
			return fmt.Errorf("unmarshaling log line %s: %v", line, err)
		}

		// Get a channel to send the log to
		c, ok := steps[newLog.Step]
		if !ok {
			if steps[lastStep] != nil {
				close(steps[lastStep])
				delete(steps, lastStep)
			}
			lastStep = newLog.Step

			c = make(chan *logLine, 50)
			steps[newLog.Step] = c

			// Process the logs for this step in the background
			wg.Add(1)
			go func(buildID, step string, c chan *logLine) {
				defer wg.Done()
				processLogs(uploader, buildID, step, c)
			}(buildID, newLog.Step, c)
		}

		c <- newLog
	}

	// Close any channel that might be open still
	for _, v := range steps {
		close(v)
	}

	wg.Wait()

	return nil
}

type logLine struct {
	Time    int64  `json:"t"`
	Message string `json:"m"`
	Step    string `json:"s"`
}

type s3LogLine struct {
	Time    int64  `json:"t"`
	Message string `json:"m"`
	Line    int    `json:"n"`
}

func processLogs(u s3fileuploader.S3FileUploader, buildID, stepName string, logs chan *logLine) error {
	log.Println("Starting step processing for", stepName)

	cache, err := ioutil.TempFile("", stepName)
	if err != nil {
		return err
	}
	defer os.Remove(cache.Name())
	encoder := json.NewEncoder(cache)

	s3key := path.Join(buildID, stepName)

	// Every upload is identical
	lineCount := 0
	savedLineCount := 0
	upload := func() {
		if lineCount == savedLineCount {
			return
		}
		log.Println("Uploading", cache.Name())
		u.Upload(bucket, s3key, cache)
		savedLineCount = lineCount
	}

	// Ship the logs every uploadInterval.
	ticker := time.NewTicker(uploadInterval)
	defer ticker.Stop()
	t := ticker.C

	for {
		select {
		case l, ok := <-logs:
			if !ok {
				// End of logs. Send to S3 one last time
				upload()
				return nil
			}

			// Write the new line to our cache file
			s3Line := s3LogLine{
				Time:    l.Time,
				Message: l.Message,
				Line:    lineCount,
			}

			if err := encoder.Encode(s3Line); err != nil {
				return fmt.Errorf("marshaling log line %v: %v", s3Line, err)
			}
			lineCount++

			if lineCount-savedLineCount > 200 {
				upload()
			}

		case <-t:
			upload()
		}
	}
}
