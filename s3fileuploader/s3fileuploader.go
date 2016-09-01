package s3fileuploader

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// S3FileUploader is able to upload a Reader to a bucket in S3.
type S3FileUploader interface {
	// Send a File to an S3 bucket at a specific key.
	Upload(bucket, key string, input *os.File) error
}

type s3Uploader struct {
	api *s3.S3
}

// NewS3FileUploader returns an S3FileUploader for a given region using AWS EnvCredentials.
func NewS3FileUploader(region string) S3FileUploader {
	creds := credentials.NewEnvCredentials()
	conf := aws.NewConfig().WithRegion(region).WithCredentials(creds)
	return &s3Uploader{s3.New(session.New(), conf)}
}

func (s *s3Uploader) Upload(bucket, key string, input *os.File) error {
	fileInfo, err := input.Stat()
	if err != nil {
		return fmt.Errorf("attempting to read %s: %v", input.Name(), err)
	}
	size := fileInfo.Size()
	fileType := "application/x-ndjson"

	params := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          input,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(fileType),
	}
	if _, err := s.api.PutObject(params); err != nil {
		return fmt.Errorf("writing %s to bucket %s: %v", key, bucket, err)
	}

	return nil
}
