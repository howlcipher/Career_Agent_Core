package tracker

import "testing"

func TestClassifyEmail(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		body    string
		want    string
	}{
		// Bug #20's live false positives must classify as nothing
		{"google payment receipt", "google: we've received your payment for 7552-6381-4439", "thank you. next steps: no action needed.", ""},
		{"linkedin sent confirmation", "william, your application was sent to clearlyagile", "prepare for interviews with these tips", ""},
		{"automated message", "interview scheduling", "this is an automated message about your interview", ""},
		// Genuine signals must still classify
		{"real rejection", "glimpse - senior edge infrastructure engineer - next steps", "unfortunately we will not be moving forward", "REJECTED"},
		{"real interview", "your upcoming call with glimpse", "we would like to schedule an interview, what is your availability?", "INTERVIEW_REQUESTED"},
		{"unrelated newsletter", "weekly go digest", "generics deep dive", ""},
	}
	for _, tt := range tests {
		if got := classifyEmail(tt.subject, tt.body); got != tt.want {
			t.Errorf("%s: classifyEmail = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMatchTrackedCompany(t *testing.T) {
	companies := []string{"glimpse", "en-US", "en", "Unknown Company", "jobgether", "ClearlyAgile", "Remote"}
	tests := []struct {
		name         string
		senderDomain string
		subject      string
		want         string
	}{
		{"direct company domain", "glimpse.io", "your upcoming call", "glimpse"},
		{"ats relay, company in subject", "greenhouse.io", "update on your jobgether application", "jobgether"},
		{"case-insensitive stored name", "linkedin.com", "your application to clearlyagile", "ClearlyAgile"},
		// Bug #20: generic/short labels must never match
		{"google receipt never matches en", "google.com", "we've received your payment", ""},
		{"generic label ignored even if contained", "example.com", "en-us update", ""},
		{"no match at all", "randomcorp.com", "hello", ""},
		// Confirmed live 2026-07-22: "Remote" (remote.com) must not match
		// the word "remote" in a recruiter subject line
		{"common-word company not matched via subject", "theswifthire.com", "re: rtr/rc || software product engineer || contract || remote ||", ""},
		{"common-word company matched via its own domain", "remote.com", "update on your application", "Remote"},
	}
	for _, tt := range tests {
		if got := matchTrackedCompany(companies, tt.senderDomain, tt.subject); got != tt.want {
			t.Errorf("%s: matchTrackedCompany = %q, want %q", tt.name, got, tt.want)
		}
	}
}
