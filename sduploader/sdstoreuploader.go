package sduploader

import (
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// default configs
var maxRetries = 5
var httpTimeout = time.Duration(20) * time.Second

const retryWaitMax = 300
const retryWaitMin = 100

// SDUploader is able to upload the contents of a Reader to the SD Store
type SDUploader interface {
	Upload(path string, filePath string) error
}

type sdStoreUploader struct {
	buildID string
	url     string
	token   string
	client  *retryablehttp.Client
}

// NewStoreUploader returns an SDUploader for a given build.
func NewStoreUploader(buildID, url, token string) SDUploader {
	// read config from env variables
	if strings.TrimSpace(os.Getenv("LOGSERVICE_STOREAPI_TIMEOUT_SECS")) != "" {
		storeTimeout, _ := strconv.Atoi(os.Getenv("LOGSERVICE_STOREAPI_TIMEOUT_SECS"))
		httpTimeout = time.Duration(storeTimeout) * time.Second
	}

	if strings.TrimSpace(os.Getenv("LOGSERVICE_STOREAPI_MAXRETRIES")) != "" {
		maxRetries, _ = strconv.Atoi(os.Getenv("LOGSERVICE_STOREAPI_MAXRETRIES"))
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = maxRetries
	retryClient.RetryWaitMin = time.Duration(retryWaitMin) * time.Millisecond
	retryClient.RetryWaitMax = time.Duration(retryWaitMax) * time.Millisecond
	retryClient.Backoff = retryablehttp.LinearJitterBackoff
	retryClient.HTTPClient.Timeout = httpTimeout

	return &sdStoreUploader{
		buildID,
		url,
		token,
		retryClient,
	}
}

// SDError is an error response from the Screwdriver API
type SDError struct {
	StatusCode int    `json:"statusCode"`
	Reason     string `json:"error"`
	Message    string `json:"message"`
}

// Error implements the error interface for SDError
func (e SDError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.StatusCode, e.Reason, e.Message)
}

// Uploads sends a file to a path within the SD Store. The path is relative to
// the build path within the SD Store, e.g. http://store.screwdriver.cd/builds/abc/<storePath>
func (s *sdStoreUploader) Upload(storePath string, filePath string) error {
	u, err := s.makeURL(storePath)
	if err != nil {
		return fmt.Errorf("generating url for file %q to %s", filePath, storePath)
	}

	err = s.putFile(u, "application/x-ndjson", filePath)
	if err != nil {
		log.Printf("errored:[%v], posting file %q to %s", filePath, storePath, err)
		return err
	}
	return nil
}

// makeURL creates the fully-qualified url for a given Store path
func (s *sdStoreUploader) makeURL(storePath string) (*url.URL, error) {
	u, err := url.Parse(s.url)
	if err != nil {
		return nil, fmt.Errorf("bad url %s: %v", s.url, err)
	}
	version := "v1"
	u.Path = path.Join(u.Path, version, "builds", s.buildID, storePath)

	return u, nil
}

func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

// putFile writes a file at filePath to a url with a PUT request. It streams the data
// from disk to save memory
func (s *sdStoreUploader) putFile(url *url.URL, bodyType string, filePath string) error {
	input, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer input.Close()

	stat, err := input.Stat()
	if err != nil {
		return err
	}
	fsize := stat.Size()

	reader, writer := io.Pipe()

	done := make(chan error)
	go func() {
		_, err := s.put(url, bodyType, reader, fsize)
		if err != nil {
			done <- err
			return
		}

		done <- nil
	}()

	io.Copy(writer, input)
	if err := writer.Close(); err != nil {
		return err
	}

	return <-done
}

func (s *sdStoreUploader) put(url *url.URL, bodyType string, payload io.Reader, size int64) ([]byte, error) {
	req, err := http.NewRequest("PUT", url.String(), payload)
	if err != nil {
		return nil, err
	}

	defer s.client.HTTPClient.CloseIdleConnections()

	req.Header.Set("Authorization", tokenHeader(s.token))
	req.Header.Set("Content-Type", bodyType)
	req.ContentLength = size

	res, err := s.client.StandardClient().Do(req)
	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		return nil, err
	}

	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	return body, nil
}
