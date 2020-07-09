package screwdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// default configs
var maxRetries = 5
var httpTimeout = time.Duration(20) * time.Second

const retryWaitMin = 100
const retryWaitMax = 300

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
	client  *retryablehttp.Client
}

// New returns a new API object
func New(buildID, url, token string) (API, error) {
	// read config from env variables
	if strings.TrimSpace(os.Getenv("SDAPI_TIMEOUT_SECS")) != "" {
		apiTimeout, _ := strconv.Atoi(os.Getenv("SDAPI_TIMEOUT_SECS"))
		httpTimeout = time.Duration(apiTimeout) * time.Second
	}

	if strings.TrimSpace(os.Getenv("SDAPI_MAXRETRIES")) != "" {
		maxRetries, _ = strconv.Atoi(os.Getenv("SDAPI_MAXRETRIES"))
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = maxRetries
	retryClient.RetryWaitMin = time.Duration(retryWaitMin) * time.Millisecond
	retryClient.RetryWaitMax = time.Duration(retryWaitMax) * time.Millisecond
	retryClient.Backoff = retryablehttp.LinearJitterBackoff
	retryClient.HTTPClient.Timeout = httpTimeout

	newAPI := api{
		buildID,
		url,
		token,
		retryClient,
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

func (a api) write(url *url.URL, requestType string, bodyType string, payload io.Reader) ([]byte, error) {
	req := &http.Request{}
	buf := new(bytes.Buffer)

	size, err := buf.ReadFrom(payload)
	if err != nil {
		log.Printf("WARNING: error:[%v], not able to read payload: %v", err, payload)
		return nil, fmt.Errorf("WARNING: error:[%v], not able to read payload: %v", err, payload)
	}
	p := buf.String()

	req, err = http.NewRequest(requestType, url.String(), strings.NewReader(p))
	if err != nil {
		log.Printf("WARNING: received error generating new request for %s(%s): %v ", requestType, url.String(), err)
		return nil, fmt.Errorf("WARNING: received error generating new request for %s(%s): %v ", requestType, url.String(), err)
	}

	defer a.client.HTTPClient.CloseIdleConnections()

	req.Header.Set("Authorization", tokenHeader(a.token))
	req.Header.Set("Content-Type", bodyType)
	req.ContentLength = size

	res, err := a.client.StandardClient().Do(req)
	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		log.Printf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
		return nil, fmt.Errorf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("reading response Body from Screwdriver: %v", err)
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var errParse SDError
		parseError := json.Unmarshal(body, &errParse)
		if parseError != nil {
			log.Printf("unparseable error response from Screwdriver: %v", parseError)
			return nil, fmt.Errorf("unparseable error response from Screwdriver: %v", parseError)
		}

		log.Printf("WARNING: received response %d from %s ", res.StatusCode, url.String())
		return nil, fmt.Errorf("WARNING: received response %d from %s ", res.StatusCode, url.String())
	}

	return body, nil
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
