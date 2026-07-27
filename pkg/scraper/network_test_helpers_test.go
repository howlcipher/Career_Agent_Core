package scraper

import (
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMain(main *testing.M) {
	productionFactory := newHTTPClient
	newHTTPClient = func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout}
	}
	code := main.Run()
	newHTTPClient = productionFactory
	os.Exit(code)
}
