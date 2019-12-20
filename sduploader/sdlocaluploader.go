package sduploader

import (
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

	_, err = io.Copy(output, input)
	if err != nil {
		return err
	}

	return nil
}
