package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

var emitterPath = "/var/run/sd/emitter"

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

func uploadLogToS3(content [][]byte, buildID, stepName string) {
	keyid := os.Getenv("AWS_ACCESS_KEY_ID")
	secretkey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	token := ""
	creds := credentials.NewStaticCredentials(keyid, secretkey, token)
	_, err := creds.Get()
	if err != nil {
		fmt.Printf("bad credentials: %s", err)
	}

	cfg := aws.NewConfig().WithRegion("us-west-2").WithCredentials(creds)
	svc := s3.New(session.New(), cfg)

	combined := bytes.Join(content, []byte("\n"))
	data := bytes.NewReader(combined)
	size := int64(data.Len())
	fmt.Println("Uploading this:", string(combined))

	fileType := "application/x-ndjson"

	path := path.Join(buildID, stepName)
	params := &s3.PutObjectInput{
		Bucket:        aws.String("logs.screwdriver.cd"),
		Key:           aws.String(path),
		Body:          data,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(fileType),
	}

	if _, err := svc.PutObject(params); err != nil {
		log.Fatalf("bad response: %s", err)
	}
}

func processLogs(logChan chan *logLine, buildID string) chan struct{} {
	exitChan := make(chan struct{})
	go func() {
		logs := map[string][][]byte{}

		t := time.Tick(2 * time.Second)
		for {
			select {
			case line, ok := <-logChan:
				if !ok {
					for step, lines := range logs {
						log.Println("Uploading...")
						uploadLogToS3(lines, buildID, step)
					}
					exitChan <- struct{}{}
					return
				}

				s3Line := s3LogLine{
					Time:    line.Time,
					Message: line.Message,
					Line:    len(logs[line.Step]),
				}
				j, _ := json.Marshal(s3Line)
				logs[line.Step] = append(logs[line.Step], j)
			case <-t:
				for step, lines := range logs {
					log.Println("Uploading...")
					uploadLogToS3(lines, buildID, step)
				}
			}

		}
	}()
	return exitChan
}

func scanFile(path, buildID string) error {
	log.Println("Starting scan")
	logChan := make(chan *logLine, 50)
	exitChan := processLogs(logChan, buildID)

	file, err := os.Open(path)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		newLine := &logLine{}
		text := scanner.Text()
		if err := json.Unmarshal([]byte(text), newLine); err != nil {
			return fmt.Errorf("Unable to unmarshal log line %q: %v", text, err)
		}

		logChan <- newLine
	}

	close(logChan)
	<-exitChan
	return nil
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("No buildID specified.")
	}

	buildID := os.Args[1]
	log.Println("Processing logs for build", buildID)

	if err := scanFile(emitterPath, buildID); err != nil {
		log.Fatal(err)
	}
}
