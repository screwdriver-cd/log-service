package sduploader

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

type sdLocalUploader struct {
	logFile string
}

func NewLocalUploader(logFile string) SDUploader {
	return &sdLocalUploader{
		logFile: logFile,
	}
}

func getLastLine(filePath string) (string, error) {
	lastLine := ""
	p, err := os.Open(filePath)
	if err != nil {
		return lastLine, err
	}
	defer p.Close()

	scanner := bufio.NewScanner(p)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 {
			lastLine = line
		}
	}

	return lastLine, nil
}

func (s *sdLocalUploader) Upload(path string, filePath string) error {
	input, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(s.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer output.Close()

	// Skip lines that have already been logged
	lastLine, err := getLastLine(s.logFile)
	if err != nil {
		return err
	}
	inputScanner := bufio.NewScanner(input)
	matched := false
	if len(lastLine) > 0 {
		for inputScanner.Scan() {
			if matched {
				_, err = output.Write(([]byte)(fmt.Sprintf("%s\n", inputScanner.Text())))
				if err != nil {
					return err
				}
			} else if lastLine == inputScanner.Text() {
				matched = true
			}
		}
	}

	// Output all if there are no lines already logged
	if !matched {
		input.Seek(0, 0)
		_, err = io.Copy(output, input)
		if err != nil {
			return err
		}
	}

	return nil
}
