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
	"sync"
	"time"

	"github.com/screwdriver-cd/log-service/screwdriver"
	"github.com/screwdriver-cd/log-service/sduploader"
)

var (
	uploadInterval = 2 * time.Second
)

const (
	defaultLinesPerFile = 1000
	startupTimeout      = 10 * time.Minute
	logBufferSize       = 200
	maxLineSize         = 5000
)

func main() {
	a := App(parseFlags())

	run(a)
}

// parseFlags returns an App object from CLI flags.
func parseFlags() app {
	a := app{}
	flag.StringVar(&a.apiUrl, "api-uri", "", "Base URI for the Screwdriver API ($SD_API_URL)")
	flag.StringVar(&a.storeUrl, "store-uri", "", "Base URI for the Screwdriver Store API ($SD_STORE_URL)")
	flag.StringVar(&a.emitterPath, "emitter", "/var/run/sd/emitter", "Path to the log emitter file")
	flag.StringVar(&a.buildID, "build", "", "ID of the build that is emitting logs ($SD_BUILDID)")
	flag.StringVar(&a.token, "token", "", "JWT for authenticating with the Store API ($SD_TOKEN)")
	flag.IntVar(&a.linesPerFile, "lines-per-file", defaultLinesPerFile, "Max number of lines per file when uploading ($SD_LINESPERFILE)")
	flag.BoolVar(&a.isLocal, "local-mode", false, "Build run in local mode")
	flag.StringVar(&a.artifactsLogFile, "artifacts-log-file", "", "Path to the Artifacts directory in local mode")
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

	if len(a.storeUrl) == 0 {
		a.storeUrl = os.Getenv("SD_STORE_URL")
	}

	if len(a.apiUrl) == 0 {
		a.apiUrl = os.Getenv("SD_API_URL")
	}

	if len(a.apiUrl) == 0 {
		log.Println("No API URI specified. Cannot update lines for step.")
		flag.Usage()
		os.Exit(0)
	}

	if len(a.storeUrl) == 0 {
		log.Println("No STORE API URI specified. Cannot send logs anywhere.")
		flag.Usage()
		os.Exit(0)
	}

	if a.isLocal && len(a.artifactsLogFile) == 0 {
		log.Println("No Artifacts directory specified. Cannot write logs anywhere in local mode.")
		flag.Usage()
		os.Exit(0)
	}

	return a
}

// App implements the main App's interface
type App interface {
	LogReader() io.Reader
	Uploader() sduploader.SDUploader
	ScrewdriverAPI() screwdriver.API
	BuildID() string
	StepSaver(step string) StepSaver
}

type app struct {
	token,
	emitterPath,
	buildID,
	apiUrl,
	storeUrl,
	artifactsLogFile string
	linesPerFile int
	isLocal      bool
}

// Uploader returns an Uploader object for the Screwdriver Store
func (a app) Uploader() sduploader.SDUploader {
	if a.isLocal {
		return sduploader.NewLocalUploader(a.artifactsLogFile)
	} else {
		return sduploader.NewStoreUploader(a.buildID, a.storeUrl, a.token)
	}
}

func (a app) ScrewdriverAPI() screwdriver.API {
	api, err := screwdriver.New(a.buildID, a.apiUrl, a.token)
	if err != nil {
		log.Printf("Error creating Screwdriver API %v: %v", a.buildID, err)
		os.Exit(0)
	}

	return api
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

	// Force blocking IO. This is necessary for readln() to exit on EOF with go 1.9 and
	// above on darwin. See https://github.com/golang/go/issues/24164 for details
	source.Fd()

	return source
}

// StepSaver returns a new StepSaver object based on the app config
func (a app) StepSaver(step string) StepSaver {
	return NewStepSaver(step, a.Uploader(), a.linesPerFile, a.ScrewdriverAPI())
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

// Returns a single line (without the ending \n) from the input buffered reader
// Pulled from https://stackoverflow.com/a/12206365
func readln(r *bufio.Reader) (string, error) {
	var (
		isPrefix = true
		err      error
		line, ln []byte
	)

	for isPrefix && err == nil {
		line, isPrefix, err = r.ReadLine()
		ln = append(ln, line...)
	}

	return string(ln), err
}

// ArchiveLogs copies log lines from src into the Screwdriver Store
// Logs are copied to /builds/:buildId/:stepName/log.N
func ArchiveLogs(a App) error {
	log.Println("Archiver started")
	defer log.Println("Archiver stopped")

	var lastStep string
	var stepSaver StepSaver
	var stepWaitGroup sync.WaitGroup
	var readErr error
	var line string

	reader := bufio.NewReader(a.LogReader())
	line, readErr = readln(reader)

	for readErr == nil {
		newLog := &logLine{}
		if err := json.Unmarshal([]byte(line), newLog); err != nil {
			return fmt.Errorf("unmarshaling log line %s: %v", line, err)
		}

		if newLog.Step != lastStep {
			stepWaitGroup.Add(1)
			go func(stepSaver StepSaver, stepName string) {
				defer stepWaitGroup.Done()
				if err := safeClose(stepSaver); err != nil {
					log.Printf("ERROR: step %s encountered errors on final save: %v", stepName, err)
				}
			}(stepSaver, lastStep)

			stepSaver = a.StepSaver(newLog.Step)
			log.Println("Starting step processing for", newLog.Step)

			lastStep = newLog.Step
		}

		if err := stepSaver.WriteLog(newLog); err != nil {
			return fmt.Errorf("writing logs for step %s: %v", newLog.Step, err)
		}

		line, readErr = readln(reader)
	}

	if readErr != nil && readErr.Error() != "EOF" {
		return fmt.Errorf("reading the line with reader %s: %v", line, readErr)
	}

	safeClose(stepSaver)
	stepWaitGroup.Wait()
	return nil
}
