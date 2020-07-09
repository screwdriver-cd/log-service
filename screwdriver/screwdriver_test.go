package screwdriver

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/assert"
)

func makeValidatedFakeHTTPClient(t *testing.T, code int, body string, v func(r *http.Request)) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		fmt.Println("in api")
		validateHeader(t, "Authorization", wantTokenHeader)
		v(r)

		w.WriteHeader(code)
		if code == 500 {
			time.Sleep(time.Duration(2) * time.Second)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func validateHeader(t *testing.T, key, value string) func(r *http.Request) {
	return func(r *http.Request) {
		headers, ok := r.Header[key]
		if !ok {
			t.Fatalf("No %s header sent in Screwdriver request", key)
		}
		header := headers[0]
		if header != value {
			t.Errorf("%s header = %q, want %q", key, header, value)
		}
	}
}

func TestUpdateStepLines(t *testing.T) {
	var client *retryablehttp.Client
	client = retryablehttp.NewClient()
	http := makeValidatedFakeHTTPClient(t, 200, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)

		want := regexp.MustCompile(`{"lines":2000}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	client.HTTPClient = http
	testAPI := api{"123", "http://fakeurl", "faketoken", client}

	err := testAPI.UpdateStepLines("step1", 2000)

	if err != nil {
		t.Errorf("Unexpected error from UpdateStepStart: %v", err)
	}
}

func TestUpdateStepLinesRetry(t *testing.T) {
	var client *retryablehttp.Client
	client = retryablehttp.NewClient()
	http := makeValidatedFakeHTTPClient(t, 500, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)

		want := regexp.MustCompile(`{"lines":2000}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	client.HTTPClient = http
	maxRetries = 2
	httpTimeout = time.Duration(1) * time.Second
	client.RetryMax = maxRetries
	client.HTTPClient.Timeout = httpTimeout

	testAPI := api{"123", "http://fakeurl", "faketoken", client}

	err := testAPI.UpdateStepLines("step1", 2000)
	assert.Contains(t, err.Error(), "giving up after 3 attempts")
}

func TestNewDefaults(t *testing.T) {
	maxRetries = 5
	httpTimeout = time.Duration(20) * time.Second

	os.Setenv("SDAPI_TIMEOUT_SECS", "")
	os.Setenv("SDAPI_MAXRETRIES", "")
	_, _ = New("1", "http://fakeurl", "fake")
	assert.Equal(t, httpTimeout, time.Duration(20)*time.Second)
	assert.Equal(t, maxRetries, 5)
}

func TestNew(t *testing.T) {
	os.Setenv("SDAPI_TIMEOUT_SECS", "10")
	os.Setenv("SDAPI_MAXRETRIES", "1")
	_, _ = New("1", "http://fakeurl", "fake")
	assert.Equal(t, httpTimeout, time.Duration(10)*time.Second)
	assert.Equal(t, maxRetries, 1)
}
