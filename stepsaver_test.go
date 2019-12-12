package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"debug"
)

var testStepName = "testStep"

func mockAPI(t *testing.T, testStepName string) MockAPI {
	return MockAPI{
		updateStepLines: func(stepName string, lineCount int) error {
			if stepName != testStepName {
				t.Errorf("stepName == %s, want %s", stepName, testStepName)
				// Panic to get the stacktrace
				panic(true)
			}

			if lineCount != 1 {
				t.Errorf("lineCount == %d, want %d", lineCount, 1)
				// Panic to get the stacktrace
				panic(true)
			}
			return nil
		},
	}
}

type MockAPI struct {
	updateStepLines   func(stepName string, lineCount int) error
}

func (m MockAPI) UpdateStepLines(stepName string, lineCount int) error {
	if m.updateStepLines != nil {
		return m.updateStepLines(stepName, lineCount)
	}
	return nil
}

func newTestStepSaver() *stepSaver {
	s := &stepSaver{StepName: testStepName, Uploader: &mockSDStoreUploader{}, ScrewdriverAPI: &MockAPI{}, linesPerFile: defaultLinesPerFile}
	e := json.NewEncoder(s)
	s.encoder = e

	fmt.Printf("%v", s);
	return s
}

func TestWrite(t *testing.T) {
	s := newTestStepSaver()
	if s.lineCount != 0 {
		t.Errorf("NewStepSaver.lineCount should be 0. Got %d", s.lineCount)
	}
	if len(s.logFiles) != 0 {
		t.Errorf("StepSaver should start with 0 logFiles. Got %d", len(s.logFiles))
	}

	s.Write([]byte("ABC"))
	if s.lineCount != 1 {
		t.Errorf("stepSaver.lineCount should be 1. Got %d", s.lineCount)
	}
	if s.savedLineCount != 0 {
		t.Errorf("NewStepSaver.savedLineCount should be 0. Got %d", s.savedLineCount)
	}
	if len(s.logFiles) != 1 {
		t.Fatalf("StepSaver should make 1 new logfile. Got %d", len(s.logFiles))
	}

	wantPath := fmt.Sprintf("%s/log.0", testStepName)
	if s.logFiles[0].storePath != wantPath {
		t.Errorf("storePath = %s, want %s", s.logFiles[0].storePath, wantPath)
	}

	for i := 0; i < defaultLinesPerFile-1; i++ {
		s.Write([]byte(fmt.Sprintf("Log #%d", i)))
	}
	if len(s.logFiles) != 1 {
		t.Errorf("StepSaver should have 1 logfile until it fills up. Got %d", len(s.logFiles))
	}

	s.Write([]byte("This should be in a new file"))
	if len(s.logFiles) != 2 {
		t.Errorf("StepSaver should create a second logfile when the first fills up. Got %d", len(s.logFiles))
	}

	wantPath = fmt.Sprintf("%s/log.1", testStepName)
	if s.logFiles[1].storePath != wantPath {
		t.Errorf("storePath = %s, want %s", s.logFiles[1].storePath, wantPath)
	}
}

func TestWriteLog(t *testing.T) {
	s := newTestStepSaver()

	l := &logLine{1234, "TestLine", "step1"}
	s.WriteLog(l)
	if s.lineCount != 1 {
		t.Errorf("stepSaver.lineCount should be 1. Got %d", s.lineCount)
	}
	if s.savedLineCount != 0 {
		t.Errorf("NewStepSaver.savedLineCount should be 0. Got %d", s.savedLineCount)
	}
	if len(s.logFiles) != 1 {
		t.Fatalf("StepSaver should make 1 new logfile. Got %d", len(s.logFiles))
	}

	b := bytes.Buffer{}
	s.encoder = json.NewEncoder(&b)
	l = &logLine{2345, "TestLine2", "step1"}
	wantLine := `{"t":2345,"m":"TestLine2","n":1}` + "\n"
	s.WriteLog(l)
	if b.String() != wantLine {
		t.Errorf("buffer = %s, want %s", b.String(), wantLine)
	}
}

func TestWriteLogLong(t *testing.T) {
	s := newTestStepSaver()
	b := bytes.Buffer{}
	s.encoder = json.NewEncoder(&b)

	msg := strings.Repeat("0", maxLineSize)
	l := &logLine{3456, msg, "step1"}
	wantLine := fmt.Sprintf(`{"t":3456,"m":"%s","n":0,"s":"step1"}`, msg) + "\n"
	s.WriteLog(l)
	if b.String() != wantLine {
		t.Errorf("buffer = %s, want %s", b.String(), wantLine)
	}
}

func TestWriteLogTruncate(t *testing.T) {
	s := newTestStepSaver()
	b := bytes.Buffer{}
	s.encoder = json.NewEncoder(&b)

	msg := strings.Repeat("0", maxLineSize+1)
	wantMsg := msg[:5000] + fmt.Sprintf(" [line truncated after %d characters]", maxLineSize)
	l := &logLine{3456, msg, "step1"}
	wantLine := fmt.Sprintf(`{"t":3456,"m":"%s","n":0,"s":"step1"}`, wantMsg) + "\n"
	s.WriteLog(l)
	if b.String() != wantLine {
		t.Errorf("buffer = %s, want %s", b.String(), wantLine)
	}
}

func TestSaverUploadOnNewFile(t *testing.T) {
	type upload struct {
		storePath string
		localFile string
	}
	gotUploads := []upload{}
	uploadChan := make(chan upload, 10)
	uploader := &mockSDStoreUploader{
		upload: func(storePath string, localFile string) error {
			// gotUploads = append(gotUploads, upload{storePath, localFile})
			uploadChan <- upload{storePath, localFile}
			return nil
		},
	}
	screwdriverAPI := mockAPI(t, testStepName)

	s := NewStepSaver(testStepName, uploader, defaultLinesPerFile, screwdriverAPI)
	for i := 0; i < defaultLinesPerFile; i++ {
		l := &logLine{3456, fmt.Sprintf("LogMsg #%d", i), "step1"}
		s.WriteLog(l)
	}

	if len(gotUploads) != 0 {
		t.Errorf("Unexpected call to Upload(). len(gotUploads) = %d, want 0", len(gotUploads))
	}

	l := &logLine{3456, fmt.Sprintf("LogMsg #%d", defaultLinesPerFile), "step1"}
	s.WriteLog(l)

	// Wait just a moment to let other goroutines do their thing
	time.Sleep(10 * time.Millisecond)
	for i := 0; i < len(uploadChan); i++ {
		gotUploads = append(gotUploads, <-uploadChan)
	}

	if len(gotUploads) != 1 {
		t.Errorf("Upload not called before writing new file. len(gotUploads) = %d, want 1", len(gotUploads))
	}
}

func TestSaverUploadOnTimeElapsed(t *testing.T) {
	oldUploadInterval := uploadInterval
	uploadInterval = 500 * time.Millisecond
	defer func() { uploadInterval = oldUploadInterval }()

	type upload struct {
		storePath string
		localFile string
	}

	uploadChan := make(chan upload, 1)
	uploader := &mockSDStoreUploader{
		upload: func(storePath string, localFile string) error {
			uploadChan <- upload{storePath, localFile}
			return nil
		},
	}
	screwdriverAPI := mockAPI(t, testStepName)

	gotUploads := []upload{}
	s := NewStepSaver(testStepName, uploader, defaultLinesPerFile, screwdriverAPI)
	for i := 0; i < defaultLinesPerFile; i++ {
		l := &logLine{3456, fmt.Sprintf("LogMsg #%d", i), "step1"}
		s.WriteLog(l)
		select {
		case u := <-uploadChan:
			gotUploads = append(gotUploads, u)
		default:
			// Continue on if nothing is in the upload channel
		}
	}

	if len(gotUploads) != 0 {
		t.Errorf("Unexpected call to Upload(). len(gotUploads) = %d, want 0", len(gotUploads))
	}

	// We should get a single upload after the interval passes
	time.Sleep(time.Duration(float32(uploadInterval) * 1.5))
	for i := 0; i < len(uploadChan); i++ {
		gotUploads = append(gotUploads, <-uploadChan)
	}

	if len(gotUploads) != 1 {
		t.Errorf("Upload not called after interval. len(gotUploads) = %d, want 1", len(gotUploads))
	}
}

func TestSaverUploadOnClose(t *testing.T) {
	type upload struct {
		storePath string
		localFile string
	}
	gotUploads := []upload{}
	uploader := &mockSDStoreUploader{
		upload: func(storePath string, localFile string) error {
			gotUploads = append(gotUploads, upload{storePath, localFile})
			return nil
		},
	}
	screwdriverAPI := mockAPI(t, testStepName)

	s := NewStepSaver(testStepName, uploader, defaultLinesPerFile, screwdriverAPI)
	l := &logLine{4567, fmt.Sprintf("LogMsg #1"), "step1"}
	s.WriteLog(l)

	if len(gotUploads) != 0 {
		t.Errorf("Unexpected call to Upload(). len(gotUploads) = %d, want 0", len(gotUploads))
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Unexpected error closng the StepSaver: %v", err)
	}

	if len(gotUploads) != 1 {
		t.Errorf("Upload not called after close. len(gotUploads) = %d, want 1", len(gotUploads))
	}
}

func TestLogStringer(t *testing.T) {
	l := &logLine{123, "TestMSG", "TestStep"}
	wantString := `{t:123, m:"TestMSG", s:"TestStep"}`
	if l.String() != wantString {
		t.Errorf("Bad stringification of logs. Got %s, want %s", l, wantString)

	}
}
