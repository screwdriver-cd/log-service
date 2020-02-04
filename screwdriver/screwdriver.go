package screwdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var sleep = time.Sleep

const maxAttempts = 5

// API is a Screwdriver API endpoint
type API interface {
	UpdateStepLines(stepName string, lineCount int) error
}

// SDError is an error response from the Screwdriver API
type SDError struct {
	StatusCode int    `json:"statusCode"`
	Reason     string `json:"error"`
	Message    string `json:"message"`
}

func (e SDError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.StatusCode, e.Reason, e.Message)
}

type api struct {
	buildID string
	baseURL string
	token   string
	client  *http.Client
}

// New returns a new API object
func New(buildID, url, token string) (API, error) {
	newAPI := api{
		buildID,
		url,
		token,
		&http.Client{Timeout: 20 * time.Second},
	}
	return API(newAPI), nil
}

// StepStartPayload is a Screwdriver Step Start payload.
type StepLinesPayload struct {
	Lines int `json:"lines"`
}

func (a api) makeURL(path string) (*url.URL, error) {
	version := "v4"
	fullpath := fmt.Sprintf("%s/%s/%s", a.baseURL, version, path)
	return url.Parse(fullpath)
}

func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

func handleResponse(res *http.Response) ([]byte, error) {
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var err SDError
		parserr := json.Unmarshal(body, &err)
		if parserr != nil {
			return nil, fmt.Errorf("Unparseable error response from Screwdriver: %v", parserr)
		}
		return nil, err
	}
	return body, nil
}

func retry(attempts int, callback func() error) (err error) {
	for i := 0; ; i++ {
		err = callback()
		if err == nil {
			return nil
		}

		if i >= (attempts - 1) {
			break
		}

		//Exponential backoff of 2 seconds
		duration := time.Duration(math.Pow(2, float64(i+1)))
		sleep(duration * time.Second)
	}
	return fmt.Errorf("After %d attempts, Last error: %s", attempts, err)
}

func (a api) write(url *url.URL, requestType string, bodyType string, payload io.Reader) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.ReadFrom(payload)
	p := buf.String()

	res := &http.Response{}
	req := &http.Request{}
	attemptNumber := 0

	err := retry(maxAttempts, func() error {
		attemptNumber++
		var err error
		req, err = http.NewRequest(requestType, url.String(), strings.NewReader(p))
		if err != nil {
			log.Printf("WARNING: received error generating new request for %s(%s): %v "+
				"(attempt %v of %v)", requestType, url.String(), err, attemptNumber, maxAttempts)
			return err
		}

		req.Header.Set("Authorization", tokenHeader(a.token))
		req.Header.Set("Content-Type", bodyType)

		res, err = a.client.Do(req)
		if err != nil {
			log.Printf("WARNING: received error from %s(%s): %v "+
				"(attempt %d of %d)", requestType, url.String(), err, attemptNumber, maxAttempts)
			return err
		}

		if res.StatusCode/100 == 5 {
			log.Printf("WARNING: received response %d from %s "+
				"(attempt %d of %d)", res.StatusCode, url.String(), attemptNumber, maxAttempts)
			return fmt.Errorf("retries exhausted: %d returned from %s",
				res.StatusCode, url.String())
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	return handleResponse(res)
}

func (a api) put(url *url.URL, bodyType string, payload io.Reader) ([]byte, error) {
	return a.write(url, "PUT", bodyType, payload)
}

func (a api) UpdateStepLines(stepName string, lineCount int) error {
	u, err := a.makeURL(fmt.Sprintf("builds/%s/steps/%s", a.buildID, stepName))
	if err != nil {
		return fmt.Errorf("Creating url: %v", err)
	}

	bs := StepLinesPayload{
		Lines: lineCount,
	}
	payload, err := json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("Marshaling JSON for Step lines: %v", err)
	}

	_, err = a.put(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("Posting to Step lines: %v", err)
	}

	return nil
}
