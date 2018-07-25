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
)

func makeFakeHTTPClient(t *testing.T, code int, body string) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		validateHeader(t, "Authorization", wantTokenHeader)
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

func makeValidatedFakeHTTPClient(t *testing.T, code int, body string, v func(r *http.Request)) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		validateHeader(t, "Authorization", wantTokenHeader)
		v(r)

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

func TestMain(m *testing.M) {
	sleep = func(d time.Duration) {}
	os.Exit(m.Run())
}

func TestUpdateStepLines(t *testing.T) {
	http := makeValidatedFakeHTTPClient(t, 200, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)

		want := regexp.MustCompile(`{"lines":2000}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	testAPI := api{"123", "http://fakeurl", "faketoken", http}

	err := testAPI.UpdateStepLines("step1", 2000)

	if err != nil {
		t.Errorf("Unexpected error from UpdateStepStart: %v", err)
	}
}

