package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckJobAlive_Live(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	err := checkJobAlive(context.Background(), ts.URL)
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
}

func TestCheckJobAlive_Dead(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	err := checkJobAlive(context.Background(), ts.URL)
	if !errors.Is(err, errDeadRedirect) {
		t.Errorf("Expected errDeadRedirect, got %v", err)
	}
}

func TestCheckJobAlive_Gone(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer ts.Close()

	err := checkJobAlive(context.Background(), ts.URL)
	if !errors.Is(err, errDeadRedirect) {
		t.Errorf("Expected errDeadRedirect for 410, got %v", err)
	}
}

func TestCheckJobAlive_TransientReturnsNilSoItContinues(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	err := checkJobAlive(context.Background(), ts.URL)
	if err != nil {
		t.Errorf("Expected nil for 500 (transient), got %v", err)
	}
}

func TestCheckJobAlive_RedirectLoopReturnsError(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ts.URL, http.StatusFound)
	}))
	defer ts.Close()

	err := checkJobAlive(context.Background(), ts.URL)
	if err == nil {
		t.Error("Expected error for redirect loop, got nil")
	} else if errors.Is(err, errDeadRedirect) {
		t.Errorf("Expected generic error, got errDeadRedirect: %v", err)
	}
}

func TestCheckJobAlive_Cancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := checkJobAlive(ctx, ts.URL)
	if err == nil {
		t.Error("Expected context deadline exceeded error, got nil")
	}
}
