package delegation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateProposalUsesLocalChatAndReturnsContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Stream || request.Format != "json" || request.Options.Temperature != 0 {
			t.Fatalf("unsafe request: %#v", request)
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{}` + `"}}`))
	}))
	defer server.Close()

	got, err := GenerateProposal(context.Background(), server.Client(), server.URL, "test-model", "safe brief")
	if err != nil {
		t.Fatalf("GenerateProposal() error = %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("GenerateProposal() = %q, want {}", got)
	}
}

func TestGenerateProposalRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, maxProposalBytes+2))
	}))
	defer server.Close()
	if _, err := GenerateProposal(context.Background(), server.Client(), server.URL, "test-model", "safe brief"); err == nil {
		t.Fatal("GenerateProposal() succeeded, want response limit failure")
	}
}
