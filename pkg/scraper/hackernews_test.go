package scraper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestParseHNJobPosting(t *testing.T) {
	tests := []struct {
		name        string
		commentHTML string
		wantOK      bool
		wantCompany string
		wantTitle   string
		wantURL     string
	}{
		{
			name:        "well-formed pipe convention with link",
			commentHTML: `Acme Corp | Senior Backend Engineer | Remote (US) | Full-time<p>Apply at <a href="https://acme.com/careers/123">https://acme.com/careers/123</a>`,
			wantOK:      true,
			wantCompany: "Acme Corp",
			wantTitle:   "Senior Backend Engineer",
			wantURL:     "https://acme.com/careers/123",
		},
		{
			name:        "no link at all (email-only apply) is skipped",
			commentHTML: `Acme Corp | Backend Engineer | Remote<p>Email us at jobs@acme.com`,
			wantOK:      false,
		},
		{
			name:        "HN-internal link only (e.g. a reply link) is skipped",
			commentHTML: `Some discussion <a href="https://news.ycombinator.com/reply?id=123">reply</a>`,
			wantOK:      false,
		},
		{
			name:        "no pipe convention falls back to generic title",
			commentHTML: `Check out our opening, apply here: <a href="https://example.com/jobs/1">link</a>`,
			wantOK:      true,
			wantTitle:   "Who is Hiring posting",
			wantURL:     "https://example.com/jobs/1",
		},
		{
			name:        "HTML-encoded ampersand in URL is unescaped",
			commentHTML: `Acme | Engineer | Remote <a href="https://acme.com/apply?ref=hn&amp;id=1">apply</a>`,
			wantOK:      true,
			wantURL:     "https://acme.com/apply?ref=hn&id=1",
		},
		{
			name:        "internal/unsafe host is skipped",
			commentHTML: `Acme | Engineer <a href="http://169.254.169.254/latest/meta-data">apply</a>`,
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			company, title, jobURL, ok := parseHNJobPosting(tt.commentHTML)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (company=%q title=%q url=%q)", ok, tt.wantOK, company, title, jobURL)
			}
			if !ok {
				return
			}
			if tt.wantCompany != "" && company != tt.wantCompany {
				t.Errorf("company = %q, want %q", company, tt.wantCompany)
			}
			if tt.wantTitle != "" && title != tt.wantTitle {
				t.Errorf("title = %q, want %q", title, tt.wantTitle)
			}
			if jobURL != tt.wantURL {
				t.Errorf("url = %q, want %q", jobURL, tt.wantURL)
			}
		})
	}
}

func TestLatestWhoIsHiringStoryID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := hnStorySearchResponse{
			Hits: []struct {
				ObjectID string `json:"objectID"`
				Title    string `json:"title"`
			}{
				{ObjectID: "111", Title: "Ask HN: Who wants to be hired? (July 2026)"},
				{ObjectID: "222", Title: "Ask HN: Who is hiring? (July 2026)"},
				{ObjectID: "333", Title: "Ask HN: Freelancer? Seeking freelancer? (July 2026)"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	orig := hnAlgoliaBaseURL
	hnAlgoliaBaseURL = ts.URL
	defer func() { hnAlgoliaBaseURL = orig }()

	id, err := latestWhoIsHiringStoryID()
	if err != nil {
		t.Fatalf("latestWhoIsHiringStoryID failed: %v", err)
	}
	if id != 222 {
		t.Errorf("expected the \"Who is hiring\" story ID 222 (not the sibling wants-to-be-hired/freelancer threads), got %d", id)
	}
}

func TestLatestWhoIsHiringStoryID_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(hnStorySearchResponse{})
	}))
	defer ts.Close()

	orig := hnAlgoliaBaseURL
	hnAlgoliaBaseURL = ts.URL
	defer func() { hnAlgoliaBaseURL = orig }()

	if _, err := latestWhoIsHiringStoryID(); err == nil {
		t.Error("expected an error when no \"Who is hiring\" thread is found")
	}
}

func TestFetchHNThreadComments_Pagination(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "0", "":
			json.NewEncoder(w).Encode(hnCommentSearchResponse{
				Hits:    []hnComment{{ObjectID: "1", ParentID: 999, StoryID: 999}},
				NbPages: 2,
			})
		case "1":
			json.NewEncoder(w).Encode(hnCommentSearchResponse{
				Hits:    []hnComment{{ObjectID: "2", ParentID: 999, StoryID: 999}},
				NbPages: 2,
			})
		default:
			json.NewEncoder(w).Encode(hnCommentSearchResponse{})
		}
	}))
	defer ts.Close()

	orig := hnAlgoliaBaseURL
	hnAlgoliaBaseURL = ts.URL
	defer func() { hnAlgoliaBaseURL = orig }()

	comments, err := fetchHNThreadComments(999)
	if err != nil {
		t.Fatalf("fetchHNThreadComments failed: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected comments from both pages (2 total), got %d", len(comments))
	}
}

func TestDiscoverWithHackerNews(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer storage.CloseDB()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tags := r.URL.Query().Get("tags")
		switch {
		case tags == "story,author_whoishiring":
			json.NewEncoder(w).Encode(hnStorySearchResponse{
				Hits: []struct {
					ObjectID string `json:"objectID"`
					Title    string `json:"title"`
				}{{ObjectID: "500", Title: "Ask HN: Who is hiring? (July 2026)"}},
			})
		case tags == "comment,story_500":
			json.NewEncoder(w).Encode(hnCommentSearchResponse{
				NbPages: 1,
				Hits: []hnComment{
					{
						ObjectID:    "501",
						ParentID:    500,
						StoryID:     500,
						CommentText: `TestCorp | Backend Engineer | Remote <a href="https://testcorp.com/apply/1">apply</a>`,
					},
					{
						// A nested reply, not a top-level posting — must be excluded.
						ObjectID:    "502",
						ParentID:    501,
						StoryID:     500,
						CommentText: `Great opportunity! <a href="https://spam.example.com">check this out</a>`,
					},
				},
			})
		default:
			json.NewEncoder(w).Encode(hnCommentSearchResponse{})
		}
	}))
	defer ts.Close()

	orig := hnAlgoliaBaseURL
	hnAlgoliaBaseURL = ts.URL
	defer func() { hnAlgoliaBaseURL = orig }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)
	engine.discoverWithHackerNews(jobChan)
	close(jobChan)

	var jobs []Job
	for j := range jobChan {
		jobs = append(jobs, j)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected exactly 1 job (the top-level posting, excluding the nested reply), got %d: %+v", len(jobs), jobs)
	}
	if jobs[0].CompanyName != "TestCorp" || jobs[0].URL != "https://testcorp.com/apply/1" {
		t.Errorf("unexpected job: %+v", jobs[0])
	}
	source, found, err := storage.GetDiscoverySource(jobs[0].URL)
	if err != nil {
		t.Fatalf("read Hacker News source: %v", err)
	}
	if !found {
		t.Fatal("expected a persisted Hacker News source")
	}
	if source != "hackernews" {
		t.Errorf("Hacker News source = %q, want hackernews", source)
	}
}
