package sdstoreuploader

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"
)

// SDStoreUploader is able to upload the contents of a Reader to the SD Store
type SDStoreUploader interface {
	Upload(path string, filePath string) error
}

type sdUploader struct {
	buildID string
	url     string
	token   string
	client  *http.Client
}

// NewFileUploader returns an SDStoreUploader for a given build.
func NewFileUploader(buildID, url, token string) SDStoreUploader {
	return &sdUploader{
		buildID,
		url,
		token,
		&http.Client{Timeout: 10 * time.Second},
	}
}

// SDError is an error response from the Screwdriver API
type SDError struct {
	StatusCode int    `json:"statusCode"`
	Reason     string `json:"error"`
	Message    string `json:"message"`
}

// Error implemetns the error interface for SDError
func (e SDError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.StatusCode, e.Reason, e.Message)
}

// Uploads sends a file to a path within the SD Store. The path is relative to
// the build path within the SD Store, e.g. http://store.screwdriver.cd/builds/abc/<storePath>
func (s *sdUploader) Upload(storePath string, filePath string) error {
	u, err := s.makeURL(storePath)
	if err != nil {
		return fmt.Errorf("generating url for file %q to %s", filePath, storePath)
	}

	err = s.putFile(u, "text/plain", filePath)
	if err != nil {
		return fmt.Errorf("posting file %q to %s: %v", filePath, storePath, err)
	}
	return nil
}

// makeURL creates the fully-qualified url for a given Store path
func (s *sdUploader) makeURL(storePath string) (*url.URL, error) {
	u, err := url.Parse(s.url)
	if err != nil {
		return nil, fmt.Errorf("bad url %s: %v", s.url, err)
	}
	version := "v1"
	u.Path = path.Join(version, u.Path, "builds", s.buildID, storePath)

	return u, nil
}

func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

// handleResponse attempts to parse error objects from Screwdriver
func handleResponse(res *http.Response) ([]byte, error) {
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var err SDError
		parserr := json.Unmarshal(body, &err)
		if parserr != nil {
			return nil, fmt.Errorf("unparseable error response from Screwdriver: %v", parserr)
		}
		return nil, err
	}
	return body, nil
}

// putFile writes a file at filePath to a url with a PUT request. It streams the data
// from disk to save memory
func (s *sdUploader) putFile(url *url.URL, bodyType string, filePath string) error {
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

func (s *sdUploader) put(url *url.URL, bodyType string, payload io.Reader, size int64) ([]byte, error) {
	req, err := http.NewRequest("PUT", url.String(), payload)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", tokenHeader(s.token))
	req.Header.Set("Content-Type", bodyType)
	req.ContentLength = size

	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode/100 == 5 {
		return nil, fmt.Errorf("response code %d", res.StatusCode)
	}

	defer res.Body.Close()
	return handleResponse(res)
}
