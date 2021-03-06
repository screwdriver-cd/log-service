package sduploader

import (
	"bytes"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

func TestNewLocalUploader(t *testing.T) {
	expected := &sdLocalUploader{
		logFile: "test/.sd-local",
	}

	actual := NewLocalUploader("test/.sd-local")

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf(
			"There are something wrong with sdLocalUploader\nexpected: %v \nactual: %v",
			expected,
			actual,
		)
	}
}

func TestWriteLog(t *testing.T) {
	testLogFile, err := ioutil.TempFile("", "build.log")
	if err != nil {
		panic(err)
	}

	logFileName := testLogFile.Name()
	uploader := &sdLocalUploader{
		logFile: logFileName,
	}
	defer os.Remove(logFileName)

	testPath := "dummy"
	logFileExpected := testFile().Name()

	uploader.Upload(testPath, logFileExpected)

	expected, err := ioutil.ReadFile(logFileExpected)
	if err != nil {
		panic(err)
	}

	actual, err := ioutil.ReadFile(logFileName)
	if err != nil {
		t.Fatalf("Couldn't read log file: %v", err)
	}

	if !bytes.Equal(expected, actual) {
		t.Errorf(
			"There are something wrong with written logs\nexpected: %s \nactual: %s",
			expected,
			actual,
		)
	}

}

func TestOverwriteLog(t *testing.T) {
	testLogFile, err := ioutil.TempFile("", "build.log")
	if err != nil {
		panic(err)
	}

	logFileName := testLogFile.Name()
	uploader := &sdLocalUploader{
		logFile: logFileName,
	}
	defer os.Remove(logFileName)

	testPath := "dummy"
	logFileExpected := testFile().Name()

	uploader.Upload(testPath, logFileExpected)

	expectedLastLine := "{\"t\":158380,\"m\":\"msg 20\",\"s\":\"step4\"}"
	actualLastLine, err := getLastLine(logFileExpected)
	if err != nil {
		t.Fatalf("Couldn't get lastLine of log file: %v", err)
	}
	if expectedLastLine != actualLastLine {
		t.Errorf(
			"There are something wrong with written logs\nexpected: %s \nactual: %s",
			expectedLastLine,
			actualLastLine,
		)
	}

	uploader.Upload(testPath, logFileExpected)

	expected, err := ioutil.ReadFile(logFileExpected)
	if err != nil {
		panic(err)
	}

	actual, err := ioutil.ReadFile(logFileName)
	if err != nil {
		t.Fatalf("Couldn't read log file: %v", err)
	}

	if !bytes.Equal(expected, actual) {
		t.Errorf(
			"There are something wrong with written logs\nexpected: %s \nactual: %s",
			expected,
			actual,
		)
	}

}
