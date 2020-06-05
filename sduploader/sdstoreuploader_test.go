package sduploader

import (
	"bytes"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

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

func makeFakeHTTPClient(t *testing.T, code int, body string, v func(r *http.Request)) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		validateHeader(t, "Authorization", wantTokenHeader)(r)
		if v != nil {
			v(r)
		}

		w.WriteHeader(code)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, body)
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func testFile() *os.File {
	f, err := os.Open("../data/emitterdata")
	if err != nil {
		panic(err)
	}
	return f
}

func TestFileUpload(t *testing.T) {
	var retryHttpClient *retryablehttp.Client
	retryHttpClient = retryablehttp.NewClient()
	testBuildID := "testbuild"
	url := "http://fakeurl"
	token := "faketoken"
	testPath := "test/path/1"
	uploader := &sdStoreUploader{
		testBuildID,
		url,
		token,
		retryHttpClient,
	}
	called := false

	want := bytes.NewBuffer(nil)
	f := testFile()
	io.Copy(want, f)
	f.Close()

	http := makeFakeHTTPClient(t, 200, "OK", func(r *http.Request) {
		called = true
		got := bytes.NewBuffer(nil)
		io.Copy(got, r.Body)
		r.Body.Close()

		if got.String() != want.String() {
			t.Errorf("Received payload %s, want %s", got, want)
		}

		wantURL := fmt.Sprintf("%s/v1/builds/%s/%s", url, testBuildID, testPath)
		if r.URL.String() != wantURL {
			t.Errorf("Wrong URL for the uploader. Got %s, want %s", r.URL.String(), wantURL)
		}

		if r.Method != "PUT" {
			t.Errorf("Uploaded with method %s, want PUT", r.Method)
		}

		stat, err := testFile().Stat()
		if err != nil {
			t.Fatalf("Couldn't stat test file: %v", err)
		}

		fsize := stat.Size()
		if r.ContentLength != fsize {
			t.Errorf("Wrong Content-Length sent to uploader. Got %d, want %d", r.ContentLength, fsize)
		}
	})
	uploader.client.HTTPClient = http
	uploader.Upload(testPath, testFile().Name())

	if !called {
		t.Fatalf("The HTTP client was never used.")
	}
}

func TestFileUploadRetry(t *testing.T) {
	var retryHttpClient *retryablehttp.Client
	retryHttpClient = retryablehttp.NewClient()
	retryHttpClient.RetryMax = 2
	retryHttpClient.HTTPClient.Timeout = time.Duration(1) * time.Second
	testBuildID := "testbuild"
	url := "http://fakeurl"
	token := "faketoken"
	testPath := "test/path/1"
	uploader := &sdStoreUploader{
		testBuildID,
		url,
		token,
		retryHttpClient,
	}
	callCount := 0
	http := makeFakeHTTPClient(t, 500, "ERROR", func(r *http.Request) {
		callCount++
	})
	uploader.client.HTTPClient = http
	err := uploader.Upload(testPath, testFile().Name())
	if err == nil {
		t.Error("Expected error from uploader.Upload(), got nil")
	}
	if callCount != 3 {
		t.Errorf("Expected 3 retries, got %d", callCount)
	}
}

func TestNewStoreUploaderDefaults(t *testing.T) {
	maxRetries = 5
	httpTimeout = time.Duration(20) * time.Second
	os.Setenv("LOGSERVICE_STOREAPI_TIMEOUT_SECS", "")
	os.Setenv("LOGSERVICE_STOREAPI_MAXRETRIES", "")
	_ = NewStoreUploader("1", "http://fakeurl", "fake")
	assert.Equal(t, httpTimeout, time.Duration(20)*time.Second)
	assert.Equal(t, maxRetries, 5)
}

func TestNewStoreUploader(t *testing.T) {
	os.Setenv("LOGSERVICE_STOREAPI_TIMEOUT_SECS", "10")
	os.Setenv("LOGSERVICE_STOREAPI_MAXRETRIES", "1")
	_ = NewStoreUploader("1", "http://fakeurl", "fake")
	assert.Equal(t, httpTimeout, time.Duration(10)*time.Second)
	assert.Equal(t, maxRetries, 1)
}
