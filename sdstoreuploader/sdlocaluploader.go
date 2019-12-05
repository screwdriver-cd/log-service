package sdstoreuploader

import (
	"io"
	"os"
)

const logFile string = "/sd/workspace/artifacts/build.log"

type sdLocalUploader struct {
}

func NewFileUploader() SDStoreUploader {
	return &sdLocalUploader{}
}

func (s *sdLocalUploader) Upload(path string, filePath string) error {
	input, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
