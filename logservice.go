package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/screwdriver-cd/log-service/sdstoreuploader"
)

var (
	uploadInterval = 2 * time.Second
	linesPerFile   = 100
)

const (
	region         = "us-west-2"
	bucket         = "logs.screwdriver.cd"
	startupTimeout = 10 * time.Minute
	logBufferSize  = 200
	maxLineSize    = 1000
)

func main() {
	a := App(parseFlags())
	run(a)
}

// parseFlags returns an App object from CLI flags.
func parseFlags() app {
	a := app{}
	flag.StringVar(&a.url, "api-uri", "", "Base URI for the Screwdriver Store API ($SD_API_URI)")
	flag.StringVar(&a.emitterPath, "emitter", "/var/run/sd/emitter", "Path to the log emitter file")
	flag.StringVar(&a.buildID, "build", "", "ID of the build that is emitting logs ($SD_BUILDID)")
	flag.StringVar(&a.token, "token", "", "JWT for authenticating with the Store API ($SD_TOKEN)")
	flag.IntVar(&a.linesPerFile, "lines-per-file", 100, "Max number of lines per file when uploading ($SD_LINESPERFILE)")
	flag.Parse()

	if len(a.token) == 0 {
		a.token = os.Getenv("SD_TOKEN")
	}

	if len(a.token) == 0 {
		log.Println("No JWT specified. Cannot upload.")
		flag.Usage()
		os.Exit(0)
	}

	if len(os.Getenv("SD_LINESPERFILE")) != 0 {
		l, err := strconv.Atoi(os.Getenv("SD_LINESPERFILE"))
		a.linesPerFile = l
		if err != nil {
			log.Println("Bad value for $SD_LINESPERFILE")
		}
	}

	if len(a.buildID) == 0 {
		a.buildID = os.Getenv("SD_BUILDID")
	}

	if len(a.buildID) == 0 {
		log.Println("No buildID specified. Cannot log.")
		flag.Usage()
		os.Exit(0)
	}

	if len(a.url) == 0 {
		a.url = os.Getenv("SD_API_URI")
	}

	if len(a.url) == 0 {
		log.Println("No API URI specified. Cannot send logs anywhere.")
		flag.Usage()
		os.Exit(0)
	}

	return a
}

// App implements the main App's interface
type App interface {
	LogReader() io.Reader
	Uploader() sdstoreuploader.SDStoreUploader
	BuildID() string
	StepSaver(step string) StepSaver
}

type app struct {
	token,
	emitterPath,
	buildID,
	url string
	linesPerFile int
}

// Uploader returns an Uploader object for the Screwdriver Store
func (a app) Uploader() sdstoreuploader.SDStoreUploader {
	return sdstoreuploader.NewFileUploader(a.buildID, a.url, a.token)
}

// LogReader returns a Reader that is the log source.
func (a app) LogReader() io.Reader {
	// If we can't open the socket in the first 10 minutes, the sender probably
	// exited before transmitting any data. Since we are reading from
	// a FIFO, we will block forever unless we bail. 10 minutes should be enough time
	// to download all relevant docker images before starting.
	t := time.AfterFunc(startupTimeout, func() {
		log.Printf("No data in the first %s. Assuming catastophe.", startupTimeout)
		os.Exit(0)
	})
	source, err := os.Open(a.emitterPath)
	t.Stop()
	if err != nil {
		log.Printf("Failed opening %v: %v", a.emitterPath, err)
		os.Exit(0)
	}

	return source
}

// StepSaver returns a new StepSaver object based on the app config
func (a app) StepSaver(step string) StepSaver {
	return NewStepSaver(step, a.Uploader())
}

// BuildID returns the id of the build being processed.
func (a app) BuildID() string {
	return a.buildID
}

// run is a thin wrapper around ArchiveLogs.
func run(a App) {
	log.Println("Processing logs for build", a.BuildID())
	defer log.Println("Processing complete for build", a.BuildID())

	if err := ArchiveLogs(a); err != nil {
		log.Printf("Error archiving logs: %v", err)
		os.Exit(0)
	}
}

// safeClose is for closing when we might have a nil reference.
func safeClose(c io.Closer) error {
	if c == nil {
		return nil
	}

	return c.Close()
}

// ArchiveLogs copies log lines from src into the Screwdriver Store
// Logs are copied to /builds/:buildId/:stepName/log.N
func ArchiveLogs(a App) error {
	log.Println("Archiver started")
	defer log.Println("Archiver stopped")

	var lastStep string
	var stepSaver StepSaver

	scanner := bufio.NewScanner(a.LogReader())
	for scanner.Scan() {
		line := scanner.Text()

		newLog := &logLine{}
		if err := json.Unmarshal([]byte(line), newLog); err != nil {
			return fmt.Errorf("unmarshaling log line %s: %v", line, err)
		}

		if newLog.Step != lastStep {
			if err := safeClose(stepSaver); err != nil {
				return fmt.Errorf("trying to close the StepSaver for %s: %v", lastStep, err)
			}

			stepSaver = a.StepSaver(newLog.Step)
			log.Println("Starting step processing for", newLog.Step)

			lastStep = newLog.Step
		}

		if err := stepSaver.WriteLog(newLog); err != nil {
			return fmt.Errorf("writing logs for step %s: %v", newLog.Step, err)
		}
	}

	safeClose(stepSaver)
	return nil
}
