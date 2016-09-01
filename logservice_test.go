package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockS3FileUploader struct {
	upload func(string, string, *os.File) error
}

func (m *mockS3FileUploader) Upload(bucket, key string, input *os.File) error {
	if m.upload != nil {
		return m.upload(bucket, key, input)
	}

	return nil
}

func parseLogFile(input *os.File) ([]*s3LogLine, error) {
	newLogs := []*s3LogLine{}

	// Re-open the file so we don't need to seek to the beginning
	input, err := os.Open(input.Name())
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		newLog := &s3LogLine{}
		if err := json.Unmarshal([]byte(line), newLog); err != nil {
			return nil, fmt.Errorf("unmarshaling log line %s: %v", line, err)
		}
		newLogs = append(newLogs, newLog)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return newLogs, nil
}

func TestArchiveLogs(t *testing.T) {
	sdbucket := "logs.screwdriver.cd"
	buildID := "testbuild123"
	uploader := mockS3FileUploader{}
	src, err := os.Open("./data/emitterdata")
	if err != nil {
		t.Fatalf("Failed to open the test input file emitterdata: %v", err)
	}

	wantLogCounts := map[string]int{
		"step0": 2,
		"step1": 5,
		"step2": 7,
		"step3": 1,
		"step4": 5,
	}

	stepCount := 0
	uploader.upload = func(bucket, key string, input *os.File) error {
		stepCount++
		if bucket != sdbucket {
			t.Errorf("bucket=%s, want %s", bucket, sdbucket)
		}

		keystep := filepath.Base(key)
		filestep := filepath.Base(input.Name())
		if !strings.HasPrefix(filestep, keystep) {
			t.Errorf("filestep=%s, should start with %s", filestep, keystep)
		}

		wantkey := path.Join(buildID, keystep)
		if key != wantkey {
			t.Errorf("key=%s, want %s", key, wantkey)
		}

		logs, err := parseLogFile(input)
		if err != nil {
			t.Fatalf("Unexpected error parsing the log file for bucket %s and key %s: %v", bucket, key, err)
		}

		lastline := -1
		for _, log := range logs {
			lastline++
			if log.Line != lastline {
				t.Errorf("log.Line=%d, want %d", log.Line, lastline)
			}
		}

		wantLogCount, ok := wantLogCounts[keystep]
		if !ok {
			t.Errorf("Unexpected step %s logged", keystep)
		}
		if len(logs) != wantLogCount {
			t.Errorf("len(%s)=%d, want %d", keystep, len(logs), wantLogCount)
		}

		return nil
	}

	archiveLogs(&uploader, buildID, src)
	fmt.Println("Count", stepCount)
	if stepCount != len(wantLogCounts) {
		t.Errorf("stepCount=%d, want %d", stepCount, len(wantLogCounts))
	}
}

func TestProcessLogsTime(t *testing.T) {
	buildID := "testbuild123"
	stepName := "step1"
	oldUploadInterval := uploadInterval
	defer func() { uploadInterval = oldUploadInterval }()
	uploadInterval = 20 * time.Millisecond

	callCount := 0
	uploader := mockS3FileUploader{}
	uploaded := make(chan struct{})
	uploader.upload = func(bucket, key string, input *os.File) error {
		callCount++
		uploaded <- struct{}{}
		return nil
	}

	logChan := make(chan *logLine, 100)
	for i := 0; i < 30; i++ {
		logChan <- &logLine{time.Now().UnixNano(), fmt.Sprintf("Msg %d", i), stepName}
	}
	go processLogs(&uploader, buildID, stepName, logChan)
	startTime := time.Now()

	bailTimer := time.NewTimer(2 * time.Second)

	select {
	case <-bailTimer.C:
		t.Errorf("Upload was never called after 2 seconds")
	case <-uploaded:
	}
	bailTimer.Stop()

	elapsed := time.Since(startTime)
	if callCount != 1 {
		t.Errorf("callCount=%d, want 1", callCount)
	}
	if elapsed < time.Duration(uploadInterval) {
		t.Errorf("Only %s elapsed. Want %s", elapsed, time.Duration(uploadInterval))
	}
	if elapsed >= time.Duration(2*uploadInterval) {
		t.Errorf("Waited %s for an upload. Want %s", elapsed, time.Duration(uploadInterval))
	}
}

func TestProcessLogsLine(t *testing.T) {
	buildID := "testbuild123"
	stepName := "step1"
	oldUploadInterval := uploadInterval
	defer func() { uploadInterval = oldUploadInterval }()
	uploader := mockS3FileUploader{}
	uploadInterval = 20 * time.Second

	uploaded := make(chan struct{})
	uploader.upload = func(bucket, key string, input *os.File) error {
		uploaded <- struct{}{}
		return nil
	}

	logChan := make(chan *logLine, 350)

	go processLogs(&uploader, buildID, stepName, logChan)

	for i := 0; i <= 199; i++ {
		logChan <- &logLine{time.Now().UnixNano(), fmt.Sprintf("Msg %d", i), stepName}
	}

	bailTimer := time.NewTimer(20 * time.Millisecond)
	select {
	case <-bailTimer.C:
	case <-uploaded:
		t.Errorf("Upload should not happen")
	}
	bailTimer.Stop()

	logChan <- &logLine{time.Now().UnixNano(), fmt.Sprintf("Msg %d", 200), stepName}
	bailTimer = time.NewTimer(20 * time.Millisecond)
	select {
	case <-bailTimer.C:
		t.Errorf("Upload was never called")
	case <-uploaded:
	}

	logChan <- &logLine{time.Now().UnixNano(), fmt.Sprintf("Msg %d", 200), stepName}
	close(logChan)
	bailTimer = time.NewTimer(20 * time.Millisecond)
	select {
	case <-bailTimer.C:
		t.Errorf("Upload was never called")
	case <-uploaded:
	}
}
