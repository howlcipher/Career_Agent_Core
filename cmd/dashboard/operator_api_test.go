package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecodeBoundedJSON(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		method  string
		wantErr bool
	}{
		{"valid", `{"minimum_fit_score": 70, "application_mode": "automatic"}`, "POST", false},
		{"trailing whitespace", `{"minimum_fit_score": 70, "application_mode": "automatic"}   ` + "\n\t", "POST", false},
		{"trailing data", `{"minimum_fit_score": 70, "application_mode": "automatic"} {}`, "POST", true},
		{"trailing non-whitespace", `{"minimum_fit_score": 70, "application_mode": "automatic"} foo`, "POST", true},
		{"unknown field", `{"minimum_fit_score": 70, "application_mode": "automatic", "foo": "bar"}`, "POST", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			var out struct {
				MinimumFitScore int    `json:"minimum_fit_score"`
				ApplicationMode string `json:"application_mode"`
			}
			err := decodeBoundedJSON(rec, req, &out)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeBoundedJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServeQualifiedJobs_MethodCheck(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/qualified-jobs", nil)
	rec := httptest.NewRecorder()
	serveQualifiedJobs(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("serveQualifiedJobs POST returned %v, want 405", rec.Code)
	}
}
