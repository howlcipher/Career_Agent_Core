package submitter

import (
	"errors"
	"fmt"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/mxschmitt/playwright-go"
)

type MockBrowser struct {
	playwright.Browser
	newContextFunc func(options ...playwright.BrowserNewContextOptions) (playwright.BrowserContext, error)
}

func (m *MockBrowser) NewContext(options ...playwright.BrowserNewContextOptions) (playwright.BrowserContext, error) {
	if m.newContextFunc != nil {
		return m.newContextFunc(options...)
	}
	return nil, fmt.Errorf("mock error")
}

func TestAttemptSubmit_NewContextFails(t *testing.T) {
	mockBrowser := &MockBrowser{
		newContextFunc: func(options ...playwright.BrowserNewContextOptions) (playwright.BrowserContext, error) {
			return nil, fmt.Errorf("context creation failed")
		},
	}

	err := AttemptSubmit(mockBrowser, nil, nil, nil, "TestCompany", "https://example.com/apply", nil, nil, "", false, false)

	if err == nil {
		t.Errorf("Expected error when NewContext fails, got nil")
	}

	expectedErr := "could not create context: context creation failed"
	if err != nil && err.Error() != expectedErr {
		t.Errorf("Expected error message %q, got %q", expectedErr, err.Error())
	}
}

type MockContext struct {
	playwright.BrowserContext
	newPageFunc func() (playwright.Page, error)
	closeFunc   func(options ...playwright.BrowserContextCloseOptions) error
}

func (m *MockContext) NewPage() (playwright.Page, error) {
	if m.newPageFunc != nil {
		return m.newPageFunc()
	}
	return nil, fmt.Errorf("mock error")
}

func (m *MockContext) Close(options ...playwright.BrowserContextCloseOptions) error {
	if m.closeFunc != nil {
		return m.closeFunc(options...)
	}
	return nil
}

func TestAttemptSubmit_NewPageFails(t *testing.T) {
	mockCtx := &MockContext{
		newPageFunc: func() (playwright.Page, error) {
			return nil, fmt.Errorf("page creation failed")
		},
		closeFunc: func(options ...playwright.BrowserContextCloseOptions) error { return nil },
	}

	mockBrowser := &MockBrowser{
		newContextFunc: func(options ...playwright.BrowserNewContextOptions) (playwright.BrowserContext, error) {
			return mockCtx, nil
		},
	}

	err := AttemptSubmit(mockBrowser, nil, nil, nil, "TestCompany", "https://example.com/apply", nil, nil, "", false, false)

	if err == nil {
		t.Errorf("Expected error when NewPage fails, got nil")
	}

	expectedErr := "could not create page: page creation failed"
	if err != nil && err.Error() != expectedErr {
		t.Errorf("Expected error message %q, got %q", expectedErr, err.Error())
	}
}

// Test edge cases of safeFill using a nil Page or mock Page
type MockPage struct {
	playwright.Page
	locatorFunc          func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator
	getByLabelFunc       func(text any) playwright.Locator
	getByPlaceholderFunc func(text any) playwright.Locator
	mainFrame            playwright.Frame
	frames               []playwright.Frame
	urlValue             string
	contentValue         string
	screenshotFunc       func() ([]byte, error)
	// evaluateFunc backs page-level JS probes (bugs.md #99/#101).
	evaluateFunc func(expression string) (interface{}, error)
}

func (m *MockPage) Evaluate(expression string, arg ...interface{}) (interface{}, error) {
	if m.evaluateFunc != nil {
		return m.evaluateFunc(expression)
	}
	return nil, nil
}

func (m *MockPage) MainFrame() playwright.Frame { return m.mainFrame }
func (m *MockPage) Frames() []playwright.Frame  { return m.frames }
func (m *MockPage) URL() string                 { return m.urlValue }
func (m *MockPage) Content() (string, error)    { return m.contentValue, nil }
func (m *MockPage) WaitForLoadState(options ...playwright.PageWaitForLoadStateOptions) error {
	return nil
}

// pwFrame aliases playwright.Frame so it can be embedded under a field name
// other than "Frame" — same collision-avoidance reason as pwLocator above.
type pwFrame = playwright.Frame

type MockFrame struct {
	pwFrame
	url         string
	locatorFunc func(selector string, options ...playwright.FrameLocatorOptions) playwright.Locator
}

func (m *MockFrame) URL() string { return m.url }
func (m *MockFrame) Locator(selector string, options ...playwright.FrameLocatorOptions) playwright.Locator {
	if m.locatorFunc != nil {
		return m.locatorFunc(selector, options...)
	}
	return nil
}

func (m *MockPage) Locator(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
	if m.locatorFunc != nil {
		return m.locatorFunc(selector, options...)
	}
	return nil
}

func (m *MockPage) WaitForTimeout(timeout float64) {}

func (m *MockPage) WaitForSelector(selector string, options ...playwright.PageWaitForSelectorOptions) (playwright.ElementHandle, error) {
	return nil, nil
}

// Goto/Route/SetDefaultTimeout/AddInitScript/Close/Screenshot are no-op
// overrides needed to drive a full AttemptSubmit call end to end (bugs #8,
// #10, #14's closing tests) -- earlier tests only ever reached the very top
// of AttemptSubmit (NewContext/NewPage failures), so these were never
// exercised on MockPage before.
func (m *MockPage) Goto(url string, options ...playwright.PageGotoOptions) (playwright.Response, error) {
	return nil, nil
}
func (m *MockPage) Route(url any, handler func(playwright.Route), times ...int) error { return nil }
func (m *MockPage) SetDefaultTimeout(timeout float64)                                 {}
func (m *MockPage) AddInitScript(script playwright.Script) error                      { return nil }
func (m *MockPage) Close(options ...playwright.PageCloseOptions) error                { return nil }
func (m *MockPage) Screenshot(options ...playwright.PageScreenshotOptions) ([]byte, error) {
	if m.screenshotFunc != nil {
		return m.screenshotFunc()
	}
	return []byte("fake-png-bytes"), nil
}

func (m *MockPage) GetByLabel(text any, options ...playwright.PageGetByLabelOptions) playwright.Locator {
	if m.getByLabelFunc != nil {
		return m.getByLabelFunc(text)
	}
	return nil
}

func (m *MockPage) GetByPlaceholder(text any, options ...playwright.PageGetByPlaceholderOptions) playwright.Locator {
	if m.getByPlaceholderFunc != nil {
		return m.getByPlaceholderFunc(text)
	}
	return nil
}

// pwLocator aliases playwright.Locator so it can be embedded under a field
// name other than "Locator" — the interface has its own Locator(...)
// chaining method, which otherwise collides with the embedded field name.
type pwLocator = playwright.Locator

type MockLocator struct {
	pwLocator
	countFunc     func() (int, error)
	clickFunc     func(options ...playwright.LocatorClickOptions) error
	clickCalls    int
	fillFunc      func(value string) error
	fillCalls     int
	nthFunc       func(index int) playwright.Locator
	isVisibleFunc func() (bool, error)
	// evaluateFunc backs the tagName/type probe fillCoverLetterIfPresent uses
	// to tell an upload control apart from a paste textarea (bugs.md #61).
	evaluateFunc func(expression string) (interface{}, error)
	// setInputFilesFunc records an upload; uploadedFiles captures what was
	// actually sent so a test can assert on the letter's real content.
	setInputFilesFunc func(files any) error
	uploadedFiles     []playwright.InputFile
	selectOptionFunc  func(values playwright.SelectOptionValues) ([]string, error)
	checkCalls        int
	uncheckCalls      int
	// pressFunc backs the keypress that commits an autocomplete selection
	// (bugs.md #74).
	pressFunc func(key string) error
	// typeFunc backs the real keystrokes an autocomplete needs to open and
	// filter its menu (bugs.md #78).
	typeFunc func(text string) error
}

func (m *MockLocator) Type(text string, options ...playwright.LocatorTypeOptions) error {
	if m.typeFunc != nil {
		return m.typeFunc(text)
	}
	return nil
}

func (m *MockLocator) Press(key string, options ...playwright.LocatorPressOptions) error {
	if m.pressFunc != nil {
		return m.pressFunc(key)
	}
	return nil
}

func (m *MockLocator) First() playwright.Locator { return m }

func (m *MockLocator) Count() (int, error) {
	if m.countFunc != nil {
		return m.countFunc()
	}
	return 0, nil
}

// Nth defaults to returning the receiver itself (matching First()'s
// existing default) for tests that don't care about multi-match ordering;
// tests exercising firstVisibleLocator set nthFunc explicitly to return a
// distinct MockLocator per index.
func (m *MockLocator) Nth(index int) playwright.Locator {
	if m.nthFunc != nil {
		return m.nthFunc(index)
	}
	return m
}

func (m *MockLocator) IsVisible(options ...playwright.LocatorIsVisibleOptions) (bool, error) {
	if m.isVisibleFunc != nil {
		return m.isVisibleFunc()
	}
	return true, nil
}

func (m *MockLocator) Fill(value string, options ...playwright.LocatorFillOptions) error {
	m.fillCalls++
	if m.fillFunc != nil {
		return m.fillFunc(value)
	}
	return nil
}

func (m *MockLocator) Click(options ...playwright.LocatorClickOptions) error {
	m.clickCalls++
	if m.clickFunc != nil {
		return m.clickFunc(options...)
	}
	return nil
}

func (m *MockLocator) Evaluate(expression string, arg interface{}, options ...playwright.LocatorEvaluateOptions) (interface{}, error) {
	if m.evaluateFunc != nil {
		return m.evaluateFunc(expression)
	}
	// Default to "not a file input" so the text-fill path is what an
	// unconfigured mock exercises.
	return false, nil
}

func (m *MockLocator) SelectOption(values playwright.SelectOptionValues, options ...playwright.LocatorSelectOptionOptions) ([]string, error) {
	if m.selectOptionFunc != nil {
		return m.selectOptionFunc(values)
	}
	return nil, nil
}

func (m *MockLocator) Check(options ...playwright.LocatorCheckOptions) error {
	m.checkCalls++
	return nil
}

func (m *MockLocator) Uncheck(options ...playwright.LocatorUncheckOptions) error {
	m.uncheckCalls++
	return nil
}

func (m *MockLocator) SetInputFiles(files any, options ...playwright.LocatorSetInputFilesOptions) error {
	if typed, ok := files.([]playwright.InputFile); ok {
		m.uploadedFiles = append(m.uploadedFiles, typed...)
	}
	if m.setInputFilesFunc != nil {
		return m.setInputFilesFunc(files)
	}
	return nil
}

// MockMapper implements FormMapper for tests that need to exercise the full
// AttemptSubmit orchestration (bugs #8, #10, #14's closing tests), where the
// exact DOM/screenshot passed in doesn't matter -- only the mapping (or
// failure) each stage returns.
type MockMapper struct {
	extractFormMappingFunc       func(domHTML, profileContext string) (string, error)
	extractFormMappingVisionFunc func(screenshotBytes []byte) (string, error)
	solveValidationErrorsFunc    func(domHTML, profileContext string) (map[string]string, error)
}

func (m *MockMapper) ExtractFormMapping(domHTML, profileContext string) (string, error) {
	if m.extractFormMappingFunc != nil {
		return m.extractFormMappingFunc(domHTML, profileContext)
	}
	return "", fmt.Errorf("not implemented")
}

func (m *MockMapper) ExtractFormMappingVision(screenshotBytes []byte) (string, error) {
	if m.extractFormMappingVisionFunc != nil {
		return m.extractFormMappingVisionFunc(screenshotBytes)
	}
	return "", fmt.Errorf("not implemented")
}

func (m *MockMapper) SolveValidationErrors(domHTML, profileContext string) (map[string]string, error) {
	if m.solveValidationErrorsFunc != nil {
		return m.solveValidationErrorsFunc(domHTML, profileContext)
	}
	return nil, fmt.Errorf("not implemented")
}

func TestIsDeadJobPage(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"<html><body>Apply now for this Senior Engineer role</body></html>", false},
		{"<html><body>Sorry, the job listing no longer exists</body></html>", true},
		{"<html><body>This position has been filled</body></html>", true},
		{"<html><body>404 Not Found</body></html>", true},
		{"<html><body>We are no longer accepting applications for this role</body></html>", true},
		// Lever's expired-posting shell, confirmed live (bugs.md #15)
		{"<html><head><title>Not found – 404 error</title></head><body>Sorry, we couldn't find anything here</body></html>", true},
		// SmartRecruiters expired banner, confirmed live (Arista, 2026-07-22)
		{"<html><body><button>Sorry, this job has expired</button></body></html>", true},
		// ApplyToJob/JazzHR expiry wording, root cause of bugs.md #39
		// (Bright Vision Technologies, confirmed live 2026-07-23)
		{"<html><body>This position is no longer available.</body></html>", true},
		{"", false},
	}

	for _, tt := range tests {
		got := isDeadJobPage(tt.content)
		if got != tt.want {
			t.Errorf("isDeadJobPage(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}

func TestDeadRedirectReason(t *testing.T) {
	tests := []struct {
		name     string
		applyURL string
		finalURL string
		wantDead bool
	}{
		{"no redirect", "https://jobs.lever.co/acme/abc-123", "https://jobs.lever.co/acme/abc-123", false},
		{"tracking params added", "https://jobs.lever.co/acme/abc-123", "https://jobs.lever.co/acme/abc-123?lever-origin=applied", false},
		// Confirmed live (bugs.md #15/#7): Greenhouse expired-posting redirect
		{"greenhouse error redirect", "https://job-boards.greenhouse.io/remotecom/jobs/7778860003", "https://job-boards.greenhouse.io/remotecom?error=true", true},
		// Confirmed live (bugs.md #9): Jobvite expired-posting redirect
		{"jobvite error redirect", "https://jobs.jobvite.com/dwt/job/o79Qzfwp/apply", "https://jobs.jobvite.com/careers/dwt/jobs?error=404", true},
		// Confirmed live (bugs.md #15): board migrated off the ATS entirely
		{"off-ATS migration redirect", "https://boards.eu.greenhouse.io/nebius/jobs/4558243101", "https://careers.nebius.com/", true},
		// Same registrable domain: board migration within the ATS may keep the posting
		{"within-ATS board migration", "https://boards.greenhouse.io/acme/jobs/123", "https://job-boards.greenhouse.io/acme/jobs/123", false},
		{"unparseable final URL", "https://jobs.lever.co/acme/abc-123", "", false},
	}

	for _, tt := range tests {
		reason := deadRedirectReason(tt.applyURL, tt.finalURL)
		if gotDead := reason != ""; gotDead != tt.wantDead {
			t.Errorf("%s: deadRedirectReason(%q, %q) = %q, wantDead=%v", tt.name, tt.applyURL, tt.finalURL, reason, tt.wantDead)
		}
	}
}

func TestRegistrableDomain(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"jobs.lever.co", "lever.co"},
		{"boards.eu.greenhouse.io", "greenhouse.io"},
		{"gdit.wd5.myworkdayjobs.com", "myworkdayjobs.com"},
		{"careers.nebius.com", "nebius.com"},
		{"lever.co", "lever.co"},
		{"localhost", "localhost"},
	}
	for _, tt := range tests {
		if got := registrableDomain(tt.host); got != tt.want {
			t.Errorf("registrableDomain(%q) = %q, want %q", tt.host, got, tt.want)
		}
	}
}

func TestIsCaptchaContent(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		// DataDome interstitial copy, confirmed live (bugs.md #23)
		{"<html><body>Access is temporarily restricted. We detected unusual activity from your device or network.</body></html>", true},
		{"<html><body>Attention Required! | Cloudflare</body></html>", true},
		{"<html><body>Please verify you are human</body></html>", true},
		{"<html><body><form><label>First Name</label><input/></form></body></html>", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isCaptchaContent(tt.content); got != tt.want {
			t.Errorf("isCaptchaContent(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}

func TestIsKnownAuthGatedHost(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://gdit.wd5.myworkdayjobs.com/External_Career_Site/job/Any-Location--Remote/Site-Reliability-Engineer_RQ219922-1", true},
		{"https://redhat.wd5.myworkdayjobs.com/en-US/jobs", true},
		{"https://myworkdayjobs.com/whatever", true},
		{"https://boards.greenhouse.io/acme/jobs/12345", false},
		{"https://jobs.lever.co/acme/abc-def", false},
		// developer.workday.com is a docs site, not the job-posting domain,
		// and is already filtered by the FunnelEngine (bug #5) — it must not
		// match here either.
		{"https://developer.workday.com/welcome", false},
		{"https://evil.example.com/myworkdayjobs.com", false},
		{"not a url at all ://", false},
		{"", false},
		{"https://jobs.workable.com/view/abc123/senior-engineer-at-acme", true},
		{"https://apply.workable.com/acme/j/ABC123/", true},
		{"https://workable.com/whatever", true},
		{"https://evil.example.com/workable.com", false},
	}

	for _, tt := range tests {
		got := isKnownAuthGatedHost(tt.url)
		if got != tt.want {
			t.Errorf("isKnownAuthGatedHost(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestLooksLikeAuthWallContent(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"<html><body><h1>Sign In to Apply</h1><input type='password'/></body></html>", true},
		{"<html><body>Already have an account? Log in here.</body></html>", true},
		{"<html><body>Create Account to start your application</body></html>", true},
		{"<html><body>Returning Candidate? Welcome back.</body></html>", true},
		{"<html><body><form><label>First Name</label><input name='first_name'/></form></body></html>", false},
		{"", false},
	}

	for _, tt := range tests {
		got := looksLikeAuthWallContent(tt.content)
		if got != tt.want {
			t.Errorf("looksLikeAuthWallContent(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}

func TestClickApplyIfPresent_NoApplyButton(t *testing.T) {
	mockLocator := &MockLocator{countFunc: func() (int, error) { return 0, nil }}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return mockLocator
		},
	}

	clickApplyIfPresent(mockPage)

	if mockLocator.clickCalls != 0 {
		t.Errorf("expected Click to not be called when no Apply element is found, got %d calls", mockLocator.clickCalls)
	}
}

func TestClickApplyIfPresent_ClicksWhenFound(t *testing.T) {
	mockLocator := &MockLocator{countFunc: func() (int, error) { return 1, nil }}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return mockLocator
		},
	}

	clickApplyIfPresent(mockPage)

	if mockLocator.clickCalls != 1 {
		t.Errorf("expected Click to be called exactly once when an Apply element is found, got %d calls", mockLocator.clickCalls)
	}
}

func TestSafeFill_Empty(t *testing.T) {
	mockPage := &MockPage{}
	target := pageTarget{mockPage}
	err := safeFill(target, "", "text")
	if err != ErrEmptySelector {
		t.Errorf("safeFill with empty selector should return ErrEmptySelector, got %v", err)
	}

	err = safeFill(target, "selector", "")
	if err != nil {
		t.Errorf("safeFill with empty text should return nil, got %v", err)
	}
}

// TestFirstVisibleLocator_SkipsHiddenCaptchaButton reproduces a real live
// failure (2026-07-24, a Lever posting "Nova"): the submit-button selector
// matched a hidden <button type="submit" id="hcaptchaSubmitBtn"> before the
// real, visible submit button later in the DOM, and blindly clicking
// .First() hung for the full click timeout on an element Playwright would
// never consider clickable.
func TestFirstVisibleLocator_SkipsHiddenCaptchaButton(t *testing.T) {
	hidden := &MockLocator{isVisibleFunc: func() (bool, error) { return false, nil }}
	visible := &MockLocator{isVisibleFunc: func() (bool, error) { return true, nil }}

	root := &MockLocator{
		nthFunc: func(index int) playwright.Locator {
			if index == 0 {
				return hidden
			}
			return visible
		},
	}

	got := firstVisibleLocator(root, 2)
	if got != playwright.Locator(visible) {
		t.Error("expected firstVisibleLocator to skip the hidden index-0 match and return the visible index-1 match")
	}
}

func TestFirstVisibleLocator_FallsBackToFirstWhenNoneVisible(t *testing.T) {
	hiddenA := &MockLocator{isVisibleFunc: func() (bool, error) { return false, nil }}
	hiddenB := &MockLocator{isVisibleFunc: func() (bool, error) { return false, nil }}

	root := &MockLocator{
		nthFunc: func(index int) playwright.Locator {
			if index == 0 {
				return hiddenA
			}
			return hiddenB
		},
	}

	got := firstVisibleLocator(root, 2)
	if got != playwright.Locator(root) {
		t.Error("expected firstVisibleLocator to fall back to loc.First() (the root locator) when no match reports visible")
	}
}

func TestSafeFillWithLabelFallback_LabelTriedFirstWhenAvailable(t *testing.T) {
	labelLocator := &MockLocator{fillFunc: func(value string) error { return nil }}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			t.Fatalf("CSS selector should not be tried when the label fill succeeds")
			return nil
		},
		getByLabelFunc: func(text any) playwright.Locator {
			if text != "First Name" {
				t.Errorf("expected label lookup for %q, got %q", "First Name", text)
			}
			return labelLocator
		},
		getByPlaceholderFunc: func(text any) playwright.Locator {
			t.Fatalf("placeholder should not be tried when the label fill succeeds")
			return nil
		},
	}
	target := pageTarget{mockPage}

	err := safeFillWithLabelFallback(target, "input#first_name", "First Name", "Ada")
	if err != nil {
		t.Errorf("expected nil error when label fill succeeds, got %v", err)
	}
	if labelLocator.fillCalls != 1 {
		t.Errorf("expected the label locator to be filled once, got %d calls", labelLocator.fillCalls)
	}
}

func TestSafeFillWithLabelFallback_FallsBackToPlaceholderWhenLabelFails(t *testing.T) {
	placeholderLocator := &MockLocator{fillFunc: func(value string) error { return nil }}
	labelLocator := &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("timeout: label not found") }}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			t.Fatalf("CSS selector should not be tried when the placeholder fallback succeeds")
			return nil
		},
		getByLabelFunc: func(text any) playwright.Locator {
			return labelLocator
		},
		getByPlaceholderFunc: func(text any) playwright.Locator {
			if text != "First Name" {
				t.Errorf("expected placeholder lookup for %q, got %q", "First Name", text)
			}
			return placeholderLocator
		},
	}
	target := pageTarget{mockPage}

	err := safeFillWithLabelFallback(target, "input#first_name", "First Name", "Ada")
	if err != nil {
		t.Errorf("expected nil error when placeholder fallback succeeds, got %v", err)
	}
	if placeholderLocator.fillCalls != 1 {
		t.Errorf("expected the placeholder locator to be filled once, got %d calls", placeholderLocator.fillCalls)
	}
}

func TestSafeFillWithLabelFallback_FallsBackToSelectorWhenLabelAndPlaceholderFail(t *testing.T) {
	// countFunc added with bugs.md #67: the CSS tier now resolves the element
	// (verifying it exists) before filling, which safeFill never did.
	selLocator := &MockLocator{countFunc: func() (int, error) { return 1, nil }, fillFunc: func(value string) error { return nil }}
	labelLocator := &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("timeout: label not found") }}
	placeholderLocator := &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("timeout: placeholder not found") }}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return selLocator
		},
		getByLabelFunc: func(text any) playwright.Locator {
			return labelLocator
		},
		getByPlaceholderFunc: func(text any) playwright.Locator {
			return placeholderLocator
		},
	}
	target := pageTarget{mockPage}

	err := safeFillWithLabelFallback(target, "input#first_name", "First Name", "Ada")
	if err != nil {
		t.Errorf("expected nil error when selector fallback succeeds, got %v", err)
	}
	if selLocator.fillCalls != 1 {
		t.Errorf("expected the selector locator to be filled once, got %d calls", selLocator.fillCalls)
	}
}

func TestSafeFillWithLabelFallback_AllThreeTiersFail(t *testing.T) {
	selLocator := &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("selector timeout") }}
	labelLocator := &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("label timeout") }}
	placeholderLocator := &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("placeholder timeout") }}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return selLocator
		},
		getByLabelFunc: func(text any) playwright.Locator {
			return labelLocator
		},
		getByPlaceholderFunc: func(text any) playwright.Locator {
			return placeholderLocator
		},
	}
	target := pageTarget{mockPage}

	err := safeFillWithLabelFallback(target, "input#wrong-guess", "First Name", "Ada")
	if err == nil {
		t.Error("expected an error when the label, placeholder, and selector fallbacks all fail")
	}
}

func TestSafeFillWithLabelFallback_NoLabelAvailable(t *testing.T) {
	selLocator := &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("selector timeout") }}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return selLocator
		},
		getByLabelFunc: func(text any) playwright.Locator {
			t.Fatalf("GetByLabel should not be called when no label text is available")
			return nil
		},
	}
	target := pageTarget{mockPage}

	err := safeFillWithLabelFallback(target, "input#wrong-guess", "", "Ada")
	if err == nil {
		t.Error("expected an error when the selector fails and no label is available")
	}
}

// TestResolveFillTarget_PrefersMainPageWhenItHasInputs covers the common
// case: a normal, non-embedded application form. No frame should even be
// consulted once the main page reports a nonzero input count.
func TestResolveFillTarget_PrefersMainPageWhenItHasInputs(t *testing.T) {
	mainLocator := &MockLocator{countFunc: func() (int, error) { return 3, nil }}
	frameLocatorCalled := false
	mockFrame := &MockFrame{
		url: "https://example.com/unrelated-iframe",
		locatorFunc: func(selector string, options ...playwright.FrameLocatorOptions) playwright.Locator {
			frameLocatorCalled = true
			return &MockLocator{countFunc: func() (int, error) { return 5, nil }}
		},
	}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return mainLocator
		},
		frames: []playwright.Frame{mockFrame},
	}
	mockPage.mainFrame = mockFrame // irrelevant here since the main-page check short-circuits first

	target := resolveFillTarget(mockPage)

	if _, ok := target.(pageTarget); !ok {
		t.Errorf("expected pageTarget when the main page has inputs, got %T", target)
	}
	if frameLocatorCalled {
		t.Error("frame should never be consulted once the main page already has inputs")
	}
}

// TestResolveFillTarget_FallsBackToIframeWithInputs is bug #4's exact
// repro shape (SmartRecruiters' TechnologyNavigators posting): zero inputs
// on the main page, one real iframe holding the actual application form.
func TestResolveFillTarget_FallsBackToIframeWithInputs(t *testing.T) {
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		},
	}
	// A distinct object identity from any entry in frames, matching how a
	// real playwright.Page returns its main frame vs. child frames.
	mainFrame := &MockFrame{url: "https://example.com/apply"}
	childFrame := &MockFrame{
		url: "https://example.com/embedded-form",
		locatorFunc: func(selector string, options ...playwright.FrameLocatorOptions) playwright.Locator {
			return &MockLocator{countFunc: func() (int, error) { return 4, nil }}
		},
	}
	mockPage.mainFrame = mainFrame
	mockPage.frames = []playwright.Frame{mainFrame, childFrame}

	target := resolveFillTarget(mockPage)

	ft, ok := target.(frameTarget)
	if !ok {
		t.Fatalf("expected frameTarget when only a child frame has inputs, got %T", target)
	}
	if ft.frame != childFrame {
		t.Errorf("expected the frame with inputs to be selected, got frame %q", ft.frame.URL())
	}
}

// TestResolveFillTarget_FallsBackToPageWhenNothingHasInputs covers the
// dead-end case (e.g. a captcha-only frame or a genuinely formless page):
// no frame should be picked just because it exists, only if it has inputs.
func TestResolveFillTarget_FallsBackToPageWhenNothingHasInputs(t *testing.T) {
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		},
	}
	mainFrame := &MockFrame{url: "https://example.com/apply"}
	// e.g. a DataDome captcha iframe: present, but has no real form fields
	// the way this test defines "no inputs" (count 0) for this case.
	captchaFrame := &MockFrame{
		url: "https://geo.captcha-delivery.com/captcha/",
		locatorFunc: func(selector string, options ...playwright.FrameLocatorOptions) playwright.Locator {
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		},
	}
	mockPage.mainFrame = mainFrame
	mockPage.frames = []playwright.Frame{mainFrame, captchaFrame}

	target := resolveFillTarget(mockPage)

	if _, ok := target.(pageTarget); !ok {
		t.Errorf("expected pageTarget fallback when no frame has inputs, got %T", target)
	}
}

// TestIsCaptchaBlocked_GenuineInterstitialWithFewMainFields is bug #23's
// original repro shape (AbbVie/SmartRecruiters): the page itself has almost
// no real fields, and its only real frame is a known challenge host.
func TestIsCaptchaBlocked_GenuineInterstitialWithFewMainFields(t *testing.T) {
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		},
		frames: []playwright.Frame{
			&MockFrame{url: "https://geo.captcha-delivery.com/captcha/?cid=abc"},
		},
	}

	if !isCaptchaBlocked(mockPage, "") {
		t.Error("expected a page with zero real fields and a challenge-host frame to be treated as blocked")
	}
}

// TestIsCaptchaBlocked_RealFormWithBenignCaptchaWidget is the false-positive
// this fix targets, confirmed live 2026-07-23 on real Greenhouse/Lever
// postings: a large, genuinely fillable form that also embeds a standard
// invisible reCAPTCHA/hCaptcha anti-spam widget must NOT be treated as
// blocked just because that widget's iframe host matches captchaFrameHosts.
func TestIsCaptchaBlocked_RealFormWithBenignCaptchaWidget(t *testing.T) {
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return &MockLocator{countFunc: func() (int, error) { return 21, nil }}
		},
		frames: []playwright.Frame{
			&MockFrame{url: "https://newassets.hcaptcha.com/captcha/v1/abc/static/hcaptcha-enclave.html"},
		},
	}

	if isCaptchaBlocked(mockPage, "") {
		t.Error("expected a page with a real, large form to not be treated as blocked just because a captcha widget iframe is present")
	}
}

// TestIsCaptchaBlocked_ExplicitBlockWordingStillWins ensures the text-based
// check remains authoritative regardless of field count — a page could in
// principle serve both a genuine interstitial and some unrelated fields.
func TestIsCaptchaBlocked_ExplicitBlockWordingStillWins(t *testing.T) {
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return &MockLocator{countFunc: func() (int, error) { return 21, nil }}
		},
		frames: []playwright.Frame{},
	}

	if !isCaptchaBlocked(mockPage, "Please verify you are human before continuing.") {
		t.Error("expected explicit block wording to be treated as blocked even with a high field count")
	}
}

// TestHandleGreenhouse_SubmitFallsBackWhenLegacySelectorMissing is bug #49's
// repro shape (job-boards.greenhouse.io/alphasense, confirmed live
// 2026-07-23): a modern-board posting has zero elements matching the legacy
// input#submit_app selector, only a plain button[type='submit'].
func TestHandleGreenhouse_SubmitFallsBackWhenLegacySelectorMissing(t *testing.T) {
	legacyLocator := &MockLocator{countFunc: func() (int, error) { return 0, nil }}
	fallbackLocator := &MockLocator{countFunc: func() (int, error) { return 1, nil }}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			switch selector {
			case "input#submit_app":
				return legacyLocator
			case "button[type='submit']":
				return fallbackLocator
			default:
				// first_name/last_name/email/phone fields and the resume
				// file input: not under test here, just need to not panic.
				return &MockLocator{}
			}
		},
	}
	target := pageTarget{mockPage}

	if err := handleGreenhouse(target, "", "", nil, true); err != nil {
		t.Fatalf("expected no error when the fallback submit selector matches, got: %v", err)
	}
	if fallbackLocator.clickCalls != 1 {
		t.Errorf("expected the fallback button[type='submit'] locator to be clicked once, got %d calls", fallbackLocator.clickCalls)
	}
	if legacyLocator.clickCalls != 0 {
		t.Errorf("expected the legacy input#submit_app locator to never be clicked when it has zero matches, got %d calls", legacyLocator.clickCalls)
	}
}

// TestHandleGreenhouse_SubmitUsesLegacySelectorWhenPresent confirms postings
// still on Greenhouse's legacy embed theme (where input#submit_app does
// exist) are unaffected by bug #49's fallback.
func TestHandleGreenhouse_SubmitUsesLegacySelectorWhenPresent(t *testing.T) {
	legacyLocator := &MockLocator{countFunc: func() (int, error) { return 1, nil }}
	fallbackLocator := &MockLocator{countFunc: func() (int, error) { return 1, nil }}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			switch selector {
			case "input#submit_app":
				return legacyLocator
			case "button[type='submit']":
				return fallbackLocator
			default:
				return &MockLocator{}
			}
		},
	}
	target := pageTarget{mockPage}

	if err := handleGreenhouse(target, "", "", nil, true); err != nil {
		t.Fatalf("expected no error when the legacy submit selector matches, got: %v", err)
	}
	if legacyLocator.clickCalls != 1 {
		t.Errorf("expected the legacy input#submit_app locator to be clicked once, got %d calls", legacyLocator.clickCalls)
	}
	if fallbackLocator.clickCalls != 0 {
		t.Errorf("expected the fallback locator to never be consulted when the legacy selector already matches, got %d calls", fallbackLocator.clickCalls)
	}
}

func TestIsSubmissionConfirmed(t *testing.T) {
	applyURL := "https://jobs.lever.co/acme/abc-123"
	tests := []struct {
		name       string
		currentURL string
		content    string
		want       bool
		wantReason submissionConfirmationReason
	}{
		{
			name:       "explicit confirmation text on the same URL (AJAX-style success)",
			currentURL: applyURL,
			content:    "<html><body>Thank you for applying! We'll be in touch.</body></html>",
			want:       true,
			wantReason: reasonConfirmationPhrase,
		},
		{
			name:       "URL itself looks like a confirmation page",
			currentURL: "https://jobs.lever.co/acme/abc-123/thank-you",
			content:    "<html><body>Some generic content</body></html>",
			want:       true,
			wantReason: reasonConfirmationURL,
		},
		{
			name:       "same URL, no confirmation text, no error text -- bug #51's exact false-positive shape avoided",
			currentURL: applyURL,
			content:    "<html><body>Apply for this job</body></html>",
			want:       false,
			wantReason: reasonURLUnchanged,
		},
		{
			name:       "URL changed but the destination is a validation-error page -- bug #51's repro",
			currentURL: "https://jobs.lever.co/acme/abc-123?step=review",
			content:    "<html><body>This field is required: Last Name</body></html>",
			want:       false,
			wantReason: reasonErrorPhrase,
		},
		{
			name:       "URL changed with no error signal -- the original, weaker heuristic, still allowed as a last resort",
			currentURL: "https://jobs.lever.co/acme/careers",
			content:    "<html><body>Browse our other open roles</body></html>",
			want:       true,
			wantReason: reasonURLChangedNoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := isSubmissionConfirmed(applyURL, tt.currentURL, tt.content)
			if got != tt.want {
				t.Errorf("isSubmissionConfirmed(%q, %q, ...) = %v, want %v", applyURL, tt.currentURL, got, tt.want)
			}
			if reason != tt.wantReason {
				t.Errorf("isSubmissionConfirmed(%q, %q, ...) reason = %q, want %q", applyURL, tt.currentURL, reason, tt.wantReason)
			}
		})
	}
}

func TestConfirmOrError_SkipsCheckWhenNotAutoSubmitting(t *testing.T) {
	page := &MockPage{urlValue: "https://jobs.lever.co/acme/abc-123", contentValue: "<html><body>Apply for this job</body></html>"}
	if err := confirmOrError(page, "Acme", "https://jobs.lever.co/acme/abc-123", false); err != nil {
		t.Errorf("expected no error when autoSubmitClick is false, got: %v", err)
	}
}

func TestConfirmOrError_ConfirmsOnStrongEvidence(t *testing.T) {
	page := &MockPage{
		urlValue:     "https://jobs.lever.co/acme/abc-123/apply",
		contentValue: "<html><body>Thank you for applying!</body></html>",
	}
	if err := confirmOrError(page, "Acme", "https://jobs.lever.co/acme/abc-123/apply", true); err != nil {
		t.Errorf("expected no error on confirmation phrase, got: %v", err)
	}
}

// TestConfirmOrError_CatchesNativeValidationBlock is the exact live-repro
// shape found 2026-07-24: a real Lever posting has native HTML5 `required`
// fields with no formnovalidate override, so a blank required field makes
// the browser block the submit client-side -- no navigation, no error text
// rendered anywhere in the DOM. Before this fix, confirmOrError's caller
// compared against the *original job-posting URL* rather than the URL
// right before this click, and since bug #47's click-to-reveal step always
// changes that first, the "URL changed" fallback fired regardless of
// whether the submit click did anything at all. Passing the correct
// pre-click URL (post-reveal-click, e.g. the /apply page) must catch this.
func TestConfirmOrError_CatchesNativeValidationBlock(t *testing.T) {
	urlBeforeClick := "https://jobs.lever.co/acme/abc-123/apply"
	page := &MockPage{
		// Native validation blocked the click: no navigation occurred.
		urlValue:     urlBeforeClick,
		contentValue: "<html><body>Apply for this job</body></html>",
	}
	// bugs.md #95: with no evidence either way this now legitimately waits out
	// the settle budget before ruling. Shorten it rather than sit through 15s.
	defer withShortSubmitOutcomeTiming()()
	err := confirmOrError(page, "Acme", urlBeforeClick, true)
	if err == nil {
		t.Fatal("expected an error when the URL never changed from the pre-click baseline, got nil")
	}
}

// withShortSubmitOutcomeTiming compresses #95's settle timings for tests that
// exercise the full polling loop. Returns a restore func.
func withShortSubmitOutcomeTiming() func() {
	floor, budget, interval := submitOutcomeSettleFloor, submitOutcomeBudget, submitOutcomePollInterval
	submitOutcomeSettleFloor = 10 * time.Millisecond
	submitOutcomeBudget = 40 * time.Millisecond
	submitOutcomePollInterval = 5 * time.Millisecond
	return func() {
		submitOutcomeSettleFloor, submitOutcomeBudget, submitOutcomePollInterval = floor, budget, interval
	}
}

// TestHandleDynamic_FillsCustomQuestionAnswers covers improvements.md #16:
// a mapping with an "answers" entry (a custom screening question the
// proactive first-pass mapper found and generated a grounded response for)
// should get filled the same way a standard field does, via the label
// fallback chain.
func TestHandleDynamic_FillsCustomQuestionAnswers(t *testing.T) {
	var filledWith string
	mockPage := &MockPage{
		getByLabelFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error {
				filledWith = value
				return nil
			}}
		},
	}

	mappingJSON := `{
		"fields": {"custom_q_1": "#q1"},
		"labels": {"custom_q_1": "Why do you want to work here?"},
		"answers": {"custom_q_1": "I've followed the team's infrastructure work for years and want to contribute directly."}
	}`

	if err := handleDynamic(pageTarget{page: mockPage}, "", "", nil, mappingJSON, false); err != nil {
		t.Fatalf("handleDynamic returned an unexpected error: %v", err)
	}
	if filledWith != "I've followed the team's infrastructure work for years and want to contribute directly." {
		t.Errorf("expected the custom question's answer to be filled, got %q", filledWith)
	}
}

// TestHandleDynamic_CustomQuestionFillFailureDoesNotAbort ensures a bad
// selector on one optional custom question doesn't fail the whole
// submission -- the ATS's own validation (or SolveValidationErrors' retry
// pass) is the backstop, same as for a question this pass never found.
func TestHandleDynamic_CustomQuestionFillFailureDoesNotAbort(t *testing.T) {
	mockPage := &MockPage{
		getByLabelFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("no such element") }}
		},
		getByPlaceholderFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("no such element") }}
		},
	}

	mappingJSON := `{
		"fields": {"custom_q_1": ""},
		"labels": {"custom_q_1": "Why do you want to work here?"},
		"answers": {"custom_q_1": "Some generated answer."}
	}`

	if err := handleDynamic(pageTarget{page: mockPage}, "", "", nil, mappingJSON, false); err != nil {
		t.Errorf("expected a failed custom-question fill to not abort the submission, got error: %v", err)
	}
}

func TestConfirmOrError_ErrorsOnValidationErrorText(t *testing.T) {
	page := &MockPage{
		urlValue:     "https://jobs.lever.co/acme/abc-123/apply?step=review",
		contentValue: "<html><body>This field is required: Last Name</body></html>",
	}
	err := confirmOrError(page, "Acme", "https://jobs.lever.co/acme/abc-123/apply", true)
	if err == nil {
		t.Fatal("expected an error on validation-error page content, got nil")
	}
}

// TestLikelyExceedsModelContext covers the live-confirmed shapes from
// bugs.md #52/#57/#60: Reddit's 54,917-char prompt originally needed 18,572
// tokens against the then-6,144-token context window (~2.96 chars/token).
// After #60 raised the Ollama server to a 32,768-token context (with
// OLLAMA_KV_CACHE_TYPE=q8_0), Reddit's real repro size now comfortably fits
// -- confirmed here so a regression back to a too-small threshold would be
// caught by this same test rather than only rediscovered live.
func TestLikelyExceedsModelContext(t *testing.T) {
	tests := []struct {
		name    string
		dom     string
		profile string
		want    bool
	}{
		{name: "small form, small profile", dom: strings.Repeat("x", 5000), profile: strings.Repeat("y", 1000), want: false},
		{name: "combined length just over the threshold", dom: strings.Repeat("x", 79000), profile: strings.Repeat("y", 1001), want: true},
		// bugs.md #83 corrects this case. #60 raised the *context* ceiling and
		// this size does fit it — but fitting the context is not the same as
		// finishing in time. Measured live 2026-07-25: a 50,501-char payload
		// passed this check and then burned the full 45-minute Ollama timeout.
		// At ~17.5 chars/s of prompt processing, anything past ~47k characters
		// is mathematically doomed, so 54,917 must now trip the breaker — on
		// the time budget, not the context one.
		{name: "Reddit's real repro size (54,917 chars) fits the context window but cannot finish in time (bugs.md #60 -> #83)", dom: strings.Repeat("x", 54917), profile: "", want: true},
		{name: "a form genuinely larger than even the raised budget still trips the breaker", dom: strings.Repeat("x", 90000), profile: "", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := likelyExceedsModelContext(tt.dom, tt.profile); got != tt.want {
				t.Errorf("likelyExceedsModelContext(len=%d) = %v, want %v", len(tt.dom)+len(tt.profile), got, tt.want)
			}
		})
	}
}

// TestAttemptSubmit_ClickToRevealPlusLabelFallback_EndToEndSuccess closes
// bugs #8 and #14 together, by driving the real AttemptSubmit orchestration
// end to end instead of just their individual helper functions in
// isolation (clickApplyIfPresent, safeFillWithLabelFallback are each already
// covered alone). bugs.md records both fixes firing correctly live but
// never producing the "full end-to-end outcome" -- a genuine confirmed
// submission through this specific path -- despite four days of real batch
// runs. This reproduces the original repro shape directly (bug #8: zero
// form fields until an "Apply" element is clicked, on Breezy.hr/
// SmartRecruiters; bug #14: the Learner Module's CSS-selector guesses are
// wrong but its identified accessible labels are correct) and confirms the
// fix handles it, the same "verify the mechanism directly" fallback bug #4
// used once its own live repro became structurally unreachable.
func TestAttemptSubmit_ClickToRevealPlusLabelFallback_EndToEndSuccess(t *testing.T) {
	const applyURL = "https://jway-group.breezy.hr/p/419b44576d64-backend-developer"
	revealed := false

	mockPage := &MockPage{
		urlValue:     applyURL,
		contentValue: "<html><body>Backend Developer role. Apply below.</body></html>",
	}
	mockPage.locatorFunc = func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
		switch selector {
		case "button:has-text('Apply'), a:has-text('Apply'), button:has-text(\"I'm interested\"), a:has-text(\"I'm interested\")":
			return &MockLocator{
				countFunc: func() (int, error) { return 1, nil },
				clickFunc: func(options ...playwright.LocatorClickOptions) error {
					revealed = true
					return nil
				},
			}
		case "input, textarea, select", "input":
			return &MockLocator{countFunc: func() (int, error) {
				if revealed {
					return 6, nil
				}
				return 0, nil
			}}
		case "input[type='password']":
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		case "input#zzz-wrong-first", "input#zzz-wrong-last", "input#zzz-wrong-email", "input#zzz-wrong-phone":
			return &MockLocator{fillFunc: func(value string) error {
				return fmt.Errorf("no such element (the Learner Module's CSS-selector guess was wrong, as bug #14 assumes it might be)")
			}}
		case "button#apply-submit":
			return &MockLocator{clickFunc: func(options ...playwright.LocatorClickOptions) error {
				mockPage.urlValue = applyURL + "/thank-you"
				return nil
			}}
		default:
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		}
	}
	mockPage.getByLabelFunc = func(text any) playwright.Locator {
		return &MockLocator{fillFunc: func(value string) error { return nil }}
	}
	mockPage.getByPlaceholderFunc = func(text any) playwright.Locator {
		t.Fatalf("placeholder tier should never be tried when the label fill succeeds (field %v)", text)
		return nil
	}

	mockCtx := &MockContext{newPageFunc: func() (playwright.Page, error) { return mockPage, nil }}
	mockBrowser := &MockBrowser{newContextFunc: func(options ...playwright.BrowserNewContextOptions) (playwright.BrowserContext, error) {
		return mockCtx, nil
	}}

	mapper := &MockMapper{
		extractFormMappingFunc: func(domHTML, profileContext string) (string, error) {
			return `{
				"fields": {
					"first_name": "input#zzz-wrong-first",
					"last_name": "input#zzz-wrong-last",
					"email": "input#zzz-wrong-email",
					"phone": "input#zzz-wrong-phone",
					"submit_button": "button#apply-submit"
				},
				"labels": {
					"first_name": "First Name",
					"last_name": "Last Name",
					"email": "Email",
					"phone": "Phone"
				}
			}`, nil
		},
	}

	pii := &config.PII{FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com", Phone: "555-0100"}
	resumePath := t.TempDir() + "/resume.pdf"
	generateDocs := func() (string, string, error) { return resumePath, resumePath, nil }

	err := AttemptSubmit(mockBrowser, nil, mapper, nil, "Jway Group", applyURL, generateDocs, pii, "profile context", true, true)

	if err != nil {
		t.Fatalf("expected a confirmed successful submission, got error: %v", err)
	}
	if !revealed {
		t.Error("expected clickApplyIfPresent to have clicked the Apply element (bug #8)")
	}
}

// TestAttemptSubmit_VisionFallback_EndToEndSuccess closes bug #10: a genuine
// Learner Module fill failure (not just an outright mapping-generation
// error) must trigger AttemptVisionSubmit, and that fallback must be able to
// carry the submission all the way to a confirmed success. bugs.md #10
// records the trigger firing live (e.g. the GDIT Workday case) but never
// the "outcome half" -- a real job actually completing via this path --
// since every live case observed so far happened to land on a page with no
// fillable form at all. Written for the same reason as the paired #8/#14
// test above: a direct structural reproduction in lieu of scarce, hard-to-
// control live traffic.
func TestAttemptSubmit_VisionFallback_EndToEndSuccess(t *testing.T) {
	const applyURL = "https://jobs.example-ats.com/acme/senior-engineer"
	resumePath := t.TempDir() + "/resume.pdf"

	mockPage := &MockPage{
		urlValue:     applyURL,
		contentValue: "<html><body>Senior Engineer role. <form><input id='first_name'></form></body></html>",
	}
	mockPage.locatorFunc = func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
		switch selector {
		case "input, textarea, select":
			return &MockLocator{countFunc: func() (int, error) { return 6, nil }}
		case "input[type='password']":
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		case "input#totally-wrong-first":
			return &MockLocator{countFunc: func() (int, error) { return 1, nil }, fillFunc: func(value string) error {
				return fmt.Errorf("no such element (the Learner Module's mapping guess was wrong, as bug #10 assumes it might be)")
			}}
		case "input#real-first", "input#real-last", "input#real-email", "input#real-phone":
			// countFunc added with bugs.md #67: fills now resolve the element
			// first, so a present field must report a non-zero count.
			return &MockLocator{countFunc: func() (int, error) { return 1, nil }, fillFunc: func(value string) error { return nil }}
		case "button#real-submit":
			return &MockLocator{clickFunc: func(options ...playwright.LocatorClickOptions) error {
				mockPage.urlValue = applyURL + "/success"
				return nil
			}}
		default:
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		}
	}

	visionCalled := false
	mapper := &MockMapper{
		extractFormMappingFunc: func(domHTML, profileContext string) (string, error) {
			// A plausible-looking mapping that is simply wrong against the
			// real page -- bugs.md #10's exact observed shape: the Learner
			// Module "succeeds" (no error, non-empty JSON) but the fill
			// itself fails.
			return `{"fields": {"first_name": "input#totally-wrong-first"}, "labels": {}}`, nil
		},
		extractFormMappingVisionFunc: func(screenshotBytes []byte) (string, error) {
			visionCalled = true
			if len(screenshotBytes) == 0 {
				t.Error("expected a non-empty screenshot to be passed to the Vision mapper")
			}
			return `{
				"fields": {
					"first_name": "input#real-first",
					"last_name": "input#real-last",
					"email": "input#real-email",
					"phone": "input#real-phone",
					"submit_button": "button#real-submit"
				},
				"labels": {}
			}`, nil
		},
	}

	mockCtx := &MockContext{newPageFunc: func() (playwright.Page, error) { return mockPage, nil }}
	mockBrowser := &MockBrowser{newContextFunc: func(options ...playwright.BrowserNewContextOptions) (playwright.BrowserContext, error) {
		return mockCtx, nil
	}}

	pii := &config.PII{FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com", Phone: "555-0100"}
	generateDocs := func() (string, string, error) { return resumePath, resumePath, nil }

	err := AttemptSubmit(mockBrowser, nil, mapper, nil, "Acme", applyURL, generateDocs, pii, "profile context", true, true)

	if err != nil {
		t.Fatalf("expected the Vision fallback to carry the submission to a confirmed success, got error: %v", err)
	}
	if !visionCalled {
		t.Error("expected ExtractFormMappingVision to be called after the Learner Module fill genuinely failed (bug #10)")
	}
}

// --- bugs.md #61: the cover letter must actually reach the form ---
//
// Before this fix, handleDynamic/handleGreenhouse/handleLever filled name,
// email, phone, resume and custom questions but had no cover_letter step at
// all, so every application went out resume-only while the pipeline still
// paid full LLM cost to write a letter that was then discarded.

func TestFillCoverLetter_PastesIntoTextarea(t *testing.T) {
	letter := "Dear Hiring Manager,\n\nI build automation in Go and Python.\n"
	path := writeTempCoverLetter(t, letter)

	var filledWith string
	loc := &MockLocator{
		countFunc: func() (int, error) { return 1, nil },
		evaluateFunc: func(expression string) (interface{}, error) {
			return false, nil // a textarea, not a file input
		},
	}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator { return loc },
		getByLabelFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error {
				filledWith = value
				return nil
			}}
		},
	}

	fillCoverLetterIfPresent(pageTarget{page: mockPage}, path, "#cover", "Cover Letter")

	if filledWith != letter {
		t.Errorf("expected the cover letter text to be filled into the form, got %q", filledWith)
	}
}

func TestFillCoverLetter_UploadsToFileInput(t *testing.T) {
	letter := "Dear Hiring Manager,\n\nI build automation in Go and Python.\n"
	path := writeTempCoverLetter(t, letter)

	loc := &MockLocator{
		countFunc: func() (int, error) { return 1, nil },
		evaluateFunc: func(expression string) (interface{}, error) {
			return true, nil // input[type=file]
		},
	}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator { return loc },
	}

	fillCoverLetterIfPresent(pageTarget{page: mockPage}, path, "input[type='file'][name='cover_letter']", "Cover Letter")

	if len(loc.uploadedFiles) != 1 {
		t.Fatalf("expected exactly one uploaded cover letter, got %d", len(loc.uploadedFiles))
	}
	if got := string(loc.uploadedFiles[0].Buffer); got != letter {
		t.Errorf("uploaded the wrong content: got %q, want %q", got, letter)
	}
}

// A cover letter is optional on most real postings, so a failure to place one
// must never abort an otherwise complete, submittable application -- same
// best-effort contract as the custom-screening-question pass.
func TestFillCoverLetter_FailureDoesNotAbortSubmission(t *testing.T) {
	path := writeTempCoverLetter(t, "Dear Hiring Manager,\n")

	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return &MockLocator{
				countFunc: func() (int, error) { return 0, nil },
				fillFunc:  func(value string) error { return fmt.Errorf("no such element") },
			}
		},
		getByLabelFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("no such element") }}
		},
		getByPlaceholderFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error { return fmt.Errorf("no such element") }}
		},
	}

	mappingJSON := `{"fields": {"cover_letter": "#cover"}, "labels": {"cover_letter": "Cover Letter"}}`
	if err := handleDynamic(pageTarget{page: mockPage}, "", path, nil, mappingJSON, false); err != nil {
		t.Errorf("a failed cover-letter fill must not abort the submission, got: %v", err)
	}
}

// A missing master_cover_letter.txt must degrade to "apply without a letter"
// rather than taking down the submission.
func TestFillCoverLetter_MissingFileIsTolerated(t *testing.T) {
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return &MockLocator{countFunc: func() (int, error) { return 1, nil }}
		},
	}
	mappingJSON := `{"fields": {"cover_letter": "#cover"}, "labels": {"cover_letter": "Cover Letter"}}`
	if err := handleDynamic(pageTarget{page: mockPage}, "", "/nonexistent/master_cover_letter.txt", nil, mappingJSON, false); err != nil {
		t.Errorf("a missing cover letter file must not abort the submission, got: %v", err)
	}
}

func writeTempCoverLetter(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "master_cover_letter.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp cover letter: %v", err)
	}
	return path
}

// A PDF master cover letter must upload byte-for-byte under a .pdf name, not
// be renamed .txt — the employer should receive the formatted document.
func TestFillCoverLetter_UploadsPDFUnderPDFName(t *testing.T) {
	pdfBytes := []byte("%PDF-1.4\nfake pdf body\n%%EOF")
	path := filepath.Join(t.TempDir(), "Omni_CoverLetter.pdf")
	if err := os.WriteFile(path, pdfBytes, 0644); err != nil {
		t.Fatalf("failed to write temp pdf: %v", err)
	}

	loc := &MockLocator{
		countFunc:    func() (int, error) { return 1, nil },
		evaluateFunc: func(expression string) (interface{}, error) { return true, nil },
	}
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return loc
		},
	}

	fillCoverLetterIfPresent(pageTarget{page: mockPage}, path, "input[type='file']", "Cover Letter")

	if len(loc.uploadedFiles) != 1 {
		t.Fatalf("expected one uploaded file, got %d", len(loc.uploadedFiles))
	}
	if loc.uploadedFiles[0].Name != "cover_letter.pdf" {
		t.Errorf("uploaded name = %q, want cover_letter.pdf", loc.uploadedFiles[0].Name)
	}
	if string(loc.uploadedFiles[0].Buffer) != string(pdfBytes) {
		t.Error("the PDF was not uploaded byte-for-byte")
	}
}

// The inverse risk: a PDF must never have its raw bytes pasted into a
// textarea. Extraction failing means skip, not send the employer binary.
func TestFillCoverLetter_NeverPastesRawPDFBytes(t *testing.T) {
	pdfBytes := []byte("%PDF-1.4\nfake pdf body that is not extractable text\n%%EOF")
	path := filepath.Join(t.TempDir(), "cover.pdf")
	if err := os.WriteFile(path, pdfBytes, 0644); err != nil {
		t.Fatalf("failed to write temp pdf: %v", err)
	}

	var filledWith string
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			return &MockLocator{
				countFunc:    func() (int, error) { return 1, nil },
				evaluateFunc: func(expression string) (interface{}, error) { return false, nil },
			}
		},
		getByLabelFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error {
				filledWith = value
				return nil
			}}
		},
	}

	fillCoverLetterIfPresent(pageTarget{page: mockPage}, path, "#cover", "Cover Letter")

	if strings.Contains(filledWith, "%PDF") {
		t.Errorf("raw PDF bytes were pasted into the form: %q", filledWith)
	}
}

// Upload must win over paste even when the mapper pointed cover_letter at a
// textarea — the employer should get the real formatted document whenever the
// form offers an upload control, the same treatment the resume gets.
func TestFillCoverLetter_PrefersUploadOverPasteWhenMappingPointsAtTextarea(t *testing.T) {
	pdfBytes := []byte("%PDF-1.4\nfake pdf\n%%EOF")
	path := filepath.Join(t.TempDir(), "Omni_CoverLetter.pdf")
	if err := os.WriteFile(path, pdfBytes, 0644); err != nil {
		t.Fatalf("failed to write temp pdf: %v", err)
	}

	fileInput := &MockLocator{
		countFunc:    func() (int, error) { return 1, nil },
		evaluateFunc: func(expression string) (interface{}, error) { return true, nil },
	}
	textarea := &MockLocator{
		countFunc:    func() (int, error) { return 1, nil },
		evaluateFunc: func(expression string) (interface{}, error) { return false, nil },
	}

	var pasted string
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			if strings.Contains(selector, "type='file'") {
				return fileInput
			}
			return textarea // what the mapper guessed
		},
		getByLabelFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error {
				pasted = value
				return nil
			}}
		},
	}

	fillCoverLetterIfPresent(pageTarget{page: mockPage}, path, "#coverLetterTextarea", "Cover Letter")

	if len(fileInput.uploadedFiles) != 1 {
		t.Fatalf("expected the cover letter to be uploaded, got %d uploads", len(fileInput.uploadedFiles))
	}
	if pasted != "" {
		t.Errorf("expected no paste fallback once upload succeeded, but pasted %q", pasted)
	}
}

// The upload-search selectors must never match a bare resume file input:
// SetInputFiles replaces a file input's contents outright, so a loose selector
// would overwrite the resume and send the employer no resume at all.
func TestCoverLetterFileInputSelectorsNeverMatchBareResumeInput(t *testing.T) {
	for _, sel := range coverLetterFileInputSelectors {
		if sel == "input[type='file']" {
			t.Fatalf("bare file-input selector %q would overwrite the resume", sel)
		}
		if !strings.Contains(strings.ToLower(sel), "cover") && !strings.Contains(strings.ToLower(sel), "letter") {
			t.Errorf("selector %q is not scoped to a cover-letter attribute; it risks matching the resume input", sel)
		}
	}
}

// With no upload control anywhere, pasting is still the correct fallback.
func TestFillCoverLetter_FallsBackToPasteWhenNoFileInputExists(t *testing.T) {
	const letter = "Dear Hiring Manager,\n\nI build automation.\n"
	path := writeTempCoverLetter(t, letter)

	var pasted string
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			// No file inputs on this form at all.
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		},
		getByLabelFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error {
				pasted = value
				return nil
			}}
		},
	}

	fillCoverLetterIfPresent(pageTarget{page: mockPage}, path, "#cover", "Cover Letter")

	if pasted != letter {
		t.Errorf("expected the letter to be pasted when no upload control exists, got %q", pasted)
	}
}

// An empty coverPath is how "cover letters are off" reaches the submitter
// (profile.yaml's send_cover_letter: false). It must touch no control at all
// -- not the mapped selector, not the upload search, not the paste fallback.
func TestFillCoverLetter_EmptyPathTouchesNothing(t *testing.T) {
	locatorCalls := 0
	pasted := false
	mockPage := &MockPage{
		locatorFunc: func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
			locatorCalls++
			return &MockLocator{countFunc: func() (int, error) { return 1, nil }}
		},
		getByLabelFunc: func(text any) playwright.Locator {
			return &MockLocator{fillFunc: func(value string) error {
				pasted = true
				return nil
			}}
		},
	}

	fillCoverLetterIfPresent(pageTarget{page: mockPage}, "", "#cover", "Cover Letter")

	if locatorCalls != 0 {
		t.Errorf("expected no control lookups when cover letters are off, got %d", locatorCalls)
	}
	if pasted {
		t.Error("expected no paste when cover letters are off")
	}
}

// bugs.md #65: a validation fix must be applied according to the control's
// real type. Fill() is rejected by Playwright on a <select>, and required
// dropdowns are routine on Greenhouse-style forms -- so treating everything
// as a text input made those fields impossible to satisfy.
func TestApplyValidationFix_UsesSelectOptionForDropdowns(t *testing.T) {
	var gotLabels []string
	loc := &MockLocator{
		countFunc:    func() (int, error) { return 1, nil },
		evaluateFunc: func(string) (interface{}, error) { return "select|select-one", nil },
		selectOptionFunc: func(v playwright.SelectOptionValues) ([]string, error) {
			if v.Labels != nil {
				gotLabels = *v.Labels
			}
			return []string{"ok"}, nil
		},
		fillFunc: func(string) error { return fmt.Errorf("Fill() must never be used on a <select>") },
	}
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}

	if err := applyValidationFix(pageTarget{page: page}, "#work_auth", "Yes"); err != nil {
		t.Fatalf("applyValidationFix: %v", err)
	}
	if len(gotLabels) != 1 || gotLabels[0] != "Yes" {
		t.Errorf("expected the option to be chosen by visible label, got %v", gotLabels)
	}
	if loc.fillCalls != 0 {
		t.Error("Fill() was called on a <select>; Playwright rejects that")
	}
}

func TestApplyValidationFix_ChecksCheckboxes(t *testing.T) {
	mk := func(val string) *MockLocator {
		return &MockLocator{
			countFunc:    func() (int, error) { return 1, nil },
			evaluateFunc: func(string) (interface{}, error) { return "input|checkbox", nil },
		}
	}
	yes := mk("Yes")
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return yes }}
	if err := applyValidationFix(pageTarget{page: page}, "#agree", "Yes"); err != nil {
		t.Fatalf("applyValidationFix: %v", err)
	}
	if yes.checkCalls != 1 {
		t.Errorf("expected the box to be ticked, got %d Check calls", yes.checkCalls)
	}

	// An explicit negative must not silently tick the box.
	no := mk("No")
	page2 := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return no }}
	if err := applyValidationFix(pageTarget{page: page2}, "#agree", "No"); err != nil {
		t.Fatalf("applyValidationFix: %v", err)
	}
	if no.uncheckCalls != 1 || no.checkCalls != 0 {
		t.Errorf("an explicit \"No\" must uncheck, not check (check=%d uncheck=%d)", no.checkCalls, no.uncheckCalls)
	}
}

// A selector that matches nothing must report an error rather than being
// swallowed — a silent no-op is what made the retry loop unwinnable.
func TestApplyValidationFix_ReportsUnmatchedSelector(t *testing.T) {
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator {
		return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
	}}
	if err := applyValidationFix(pageTarget{page: page}, "#nope", "x"); err == nil {
		t.Error("expected an error when the selector matches nothing, got nil")
	}
}

func TestApplyValidationFix_FillsPlainTextInputs(t *testing.T) {
	var filled string
	loc := &MockLocator{
		countFunc:    func() (int, error) { return 1, nil },
		evaluateFunc: func(string) (interface{}, error) { return "input|text", nil },
		fillFunc:     func(v string) error { filled = v; return nil },
	}
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}
	if err := applyValidationFix(pageTarget{page: page}, "#phone", "586-555-0100"); err != nil {
		t.Fatalf("applyValidationFix: %v", err)
	}
	if filled != "586-555-0100" {
		t.Errorf("expected the text input to be filled, got %q", filled)
	}
}

// bugs.md #66: SolveValidationErrors returns bare id/name values, not CSS
// selectors. Playwright reads a bare word as a tag name, so "country" matched
// nothing and the form could never be corrected.
func TestResolveFieldLocator_RecoversBareIdentifiers(t *testing.T) {
	tried := []string{}
	page := &MockPage{
		locatorFunc: func(sel string, _ ...playwright.PageLocatorOptions) playwright.Locator {
			tried = append(tried, sel)
			// Only the id form exists on this page, mirroring a real
			// Greenhouse custom question.
			if sel == "#question_9558065008" {
				return &MockLocator{countFunc: func() (int, error) { return 1, nil }}
			}
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		},
	}

	if _, err := resolveFieldLocator(pageTarget{page: page}, "question_9558065008"); err != nil {
		t.Fatalf("expected the bare identifier to resolve via its id form, got: %v", err)
	}
	if len(tried) < 2 || tried[0] != "question_9558065008" {
		t.Errorf("the raw string must be tried first so valid selectors are unaffected; tried=%v", tried)
	}
}

func TestResolveFieldLocator_RecoversViaNameAttribute(t *testing.T) {
	page := &MockPage{
		locatorFunc: func(sel string, _ ...playwright.PageLocatorOptions) playwright.Locator {
			if sel == `[name="country"]` {
				return &MockLocator{countFunc: func() (int, error) { return 1, nil }}
			}
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		},
	}
	if _, err := resolveFieldLocator(pageTarget{page: page}, "country"); err != nil {
		t.Fatalf("expected resolution via the name attribute, got: %v", err)
	}
}

// A real CSS selector must be used as-is and never mangled into "##foo".
func TestResolveFieldLocator_LeavesRealSelectorsAlone(t *testing.T) {
	tried := []string{}
	page := &MockPage{
		locatorFunc: func(sel string, _ ...playwright.PageLocatorOptions) playwright.Locator {
			tried = append(tried, sel)
			return &MockLocator{countFunc: func() (int, error) { return 1, nil }}
		},
	}
	if _, err := resolveFieldLocator(pageTarget{page: page}, "#first_name"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tried) != 1 || tried[0] != "#first_name" {
		t.Errorf("a valid CSS selector must be tried once, unchanged; tried=%v", tried)
	}
}

func TestResolveFieldLocator_ErrorsWhenNothingMatches(t *testing.T) {
	page := &MockPage{
		locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator {
			return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
		},
	}
	if _, err := resolveFieldLocator(pageTarget{page: page}, "nope"); err == nil {
		t.Error("expected an error when no form of the selector matches")
	}
}

func TestLooksLikeCSSSelector(t *testing.T) {
	for _, s := range []string{"#id", ".cls", "[name='x']", "input[type=text]", "div > p"} {
		if !looksLikeCSSSelector(s) {
			t.Errorf("%q should be recognised as a CSS selector", s)
		}
	}
	for _, s := range []string{"country", "question_9558065008", "candidate-location"} {
		if looksLikeCSSSelector(s) {
			t.Errorf("%q is a bare identifier, not a CSS selector", s)
		}
	}
}

// bugs.md #71: the submit click must be able to tell "no visible submit
// control" apart from "here is one, click it". Zimperium (Lever, fit score 85)
// failed with a bare `Timeout 30000ms exceeded` waiting on `.first()` --
// firstVisibleLocator's fallback had handed back a known-hidden match
// (Lever's hidden hcaptchaSubmitBtn) and the click hung out the full timeout.
func TestFirstVisibleSubmit_ReportsWhenNoMatchIsVisible(t *testing.T) {
	hiddenA := &MockLocator{isVisibleFunc: func() (bool, error) { return false, nil }}
	hiddenB := &MockLocator{isVisibleFunc: func() (bool, error) { return false, nil }}

	root := &MockLocator{
		nthFunc: func(index int) playwright.Locator {
			if index == 0 {
				return hiddenA
			}
			return hiddenB
		},
	}

	if _, ok := firstVisibleSubmit(root, 2); ok {
		t.Error("expected ok=false when no match reports visible, so the caller can fail fast instead of clicking a hidden element")
	}
}

func TestFirstVisibleSubmit_ReportsTheVisibleMatch(t *testing.T) {
	hidden := &MockLocator{isVisibleFunc: func() (bool, error) { return false, nil }}
	visible := &MockLocator{isVisibleFunc: func() (bool, error) { return true, nil }}

	root := &MockLocator{
		nthFunc: func(index int) playwright.Locator {
			if index == 0 {
				return hidden
			}
			return visible
		},
	}

	got, ok := firstVisibleSubmit(root, 2)
	if !ok {
		t.Fatal("expected ok=true when a visible match exists")
	}
	if got != playwright.Locator(visible) {
		t.Error("expected the visible index-1 match, not the hidden index-0 one")
	}
}

// bugs.md #72: a fix that Playwright accepted is not necessarily a fix that
// landed. On Greenhouse's candidate-location / country autocompletes the
// visible box is backed by a hidden field, so setting it without choosing a
// suggestion leaves the validated value empty -- reported as success, and
// invisible until someone reads the DOM back.
func TestVerifyFixLanded_DetectsAControlLeftEmpty(t *testing.T) {
	loc := &MockLocator{
		countFunc:    func() (int, error) { return 1, nil },
		evaluateFunc: func(string) (interface{}, error) { return "", nil },
	}
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}

	landed, err := verifyFixLanded(pageTarget{page: page}, "#candidate-location", "Macomb, MI")
	if err != nil {
		t.Fatalf("verifyFixLanded: %v", err)
	}
	if landed {
		t.Error("expected landed=false for a control whose value is still empty")
	}
}

func TestVerifyFixLanded_AcceptsAReformattedValue(t *testing.T) {
	// The form rewrote "586-555-0100" to "(586) 555-0100". That is a fix that
	// landed; strict equality would wrongly flag it.
	loc := &MockLocator{
		countFunc:    func() (int, error) { return 1, nil },
		evaluateFunc: func(string) (interface{}, error) { return "(586) 555-0100", nil },
	}
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}

	landed, err := verifyFixLanded(pageTarget{page: page}, "#phone", "555-0100")
	if err != nil {
		t.Fatalf("verifyFixLanded: %v", err)
	}
	if !landed {
		t.Error("expected landed=true — the form reformatting a value does not mean the fix failed")
	}
}

func TestVerifyFixLanded_TreatsAnUncheckedBoxAsNotLanded(t *testing.T) {
	loc := &MockLocator{
		countFunc:    func() (int, error) { return 1, nil },
		evaluateFunc: func(string) (interface{}, error) { return "false", nil },
	}
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}

	landed, err := verifyFixLanded(pageTarget{page: page}, "#consent", "Yes")
	if err != nil {
		t.Fatalf("verifyFixLanded: %v", err)
	}
	if landed {
		t.Error("expected landed=false for a checkbox still reporting checked=false")
	}
}

// bugs.md #107: the mirror image, and the one that cost a real job. When the
// model deliberately declines a checkbox ("No"), UNCHECKED IS the requested
// state -- reporting it as not-landed marks a correct answer uncommittable and
// routes the whole job to manual review. Live on Sporty Group:
//
//	Attempt 3: 1 fix(es) reported success but left the control empty
//	... input[id='question_8242451101[]_54236360101'] (tried "No")
func TestVerifyFixLanded_AcceptsADeliberatelyUncheckedBox(t *testing.T) {
	loc := &MockLocator{
		countFunc: func() (int, error) { return 1, nil },
		evaluateFunc: func(expr string) (interface{}, error) {
			if strings.Contains(expr, "tagName") {
				return "input|checkbox", nil
			}
			return "false", nil
		},
	}
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}

	for _, no := range []string{"No", "no", "false", "0", "unchecked", "  No  "} {
		landed, err := verifyFixLanded(pageTarget{page: page}, "#optin", no)
		if err != nil {
			t.Fatalf("verifyFixLanded(%q): %v", no, err)
		}
		if !landed {
			t.Errorf("a checkbox declined with %q is in the requested state and must count as landed", no)
		}
	}
}

// The negative set must not swallow real answers that merely contain "no".
func TestIsNegativeCheckboxValue_DoesNotOvermatch(t *testing.T) {
	for _, yes := range []string{"No", "false", "0", "unchecked"} {
		if !isNegativeCheckboxValue(yes) {
			t.Errorf("%q should read as negative", yes)
		}
	}
	for _, notNegative := range []string{"Yes", "I agree", "Nope, I have no objection", "none of the above", "November"} {
		if isNegativeCheckboxValue(notNegative) {
			t.Errorf("%q must NOT read as negative -- it would silently untick a real answer", notNegative)
		}
	}
}

// bugs.md #73: "#430" is a CSS syntax error -- an id selector cannot start
// with a digit -- but Greenhouse numbers its custom-question controls exactly
// that way. Confirmed live on Reddit: the model sent bare "430" on one attempt
// (resolved) and "input#430" on the next (matched nothing), so the same field
// alternated between fillable and unfillable across attempts.
func TestResolveFieldLocator_FallsBackToAttributeFormForNumericIDs(t *testing.T) {
	var tried []string
	loc := &MockLocator{countFunc: func() (int, error) { return 0, nil }}
	hit := &MockLocator{countFunc: func() (int, error) { return 1, nil }}

	page := &MockPage{locatorFunc: func(sel string, _ ...playwright.PageLocatorOptions) playwright.Locator {
		tried = append(tried, sel)
		if sel == `input[id="430"]` || sel == `[id="430"]` {
			return hit
		}
		return loc
	}}

	if _, err := resolveFieldLocator(pageTarget{page: page}, "input#430"); err != nil {
		t.Fatalf("resolveFieldLocator: %v", err)
	}
	if len(tried) < 2 || tried[0] != "input#430" {
		t.Fatalf("expected the verbatim selector to be tried first, got %v", tried)
	}
	var sawAttrForm bool
	for _, s := range tried {
		if s == `input[id="430"]` || s == `[id="430"]` {
			sawAttrForm = true
		}
	}
	if !sawAttrForm {
		t.Errorf("expected an [id=\"430\"] attribute-form retry, got %v", tried)
	}
}

func TestSplitTagID(t *testing.T) {
	cases := []struct {
		in      string
		tag, id string
		ok      bool
	}{
		{"input#430", "input", "430", true},
		{"#430", "", "430", true},
		{"#question_67942415", "", "question_67942415", true},
		{"div.wrap #x", "", "", false},     // descendant combinator
		{"input#a#b", "", "", false},       // two hashes
		{"input[name='x']", "", "", false}, // no hash at all
		{"#", "", "", false},               // empty id
	}
	for _, c := range cases {
		tag, id, ok := splitTagID(c.in)
		if ok != c.ok || tag != c.tag || id != c.id {
			t.Errorf("splitTagID(%q) = (%q, %q, %v), want (%q, %q, %v)", c.in, tag, id, ok, c.tag, c.id, c.ok)
		}
	}
}

// bugs.md #79: the safety property of this whole mechanism. Greenhouse's
// geocoder returns "Macomb, Illinois, United States" as the FIRST hit for
// "Macomb", while the configured address is in Michigan. Committing option-0
// would file real job applications with the wrong location, so a near-miss
// must be rejected outright rather than accepted.
func TestPickComboboxOption_RejectsTheWrongStateEvenWhenItIsFirst(t *testing.T) {
	options := []string{
		"opt-0|Macomb, Illinois, United States",
		"opt-1|Macomb Township, Michigan, United States",
	}
	id, idx, ok := pickComboboxOption(options, "Macomb Township, MI", []string{"Macomb", "Michigan"})
	if !ok {
		t.Fatal("expected the Michigan option to be selected")
	}
	if id != "opt-1" {
		t.Errorf("selected %q — option-0 is Illinois and must not be chosen", id)
	}
	if idx != 1 {
		t.Errorf("index = %d, want 1 — the index drives keyboard selection on widgets where clicking loses a blur race (bugs.md #86)", idx)
	}
}

// If nothing genuinely matches, select nothing. Leaving the field to the
// validation-retry loop is strictly better than filling it with a wrong value.
func TestPickComboboxOption_SelectsNothingWhenNoOptionMatches(t *testing.T) {
	options := []string{
		"opt-0|Macomb, Illinois, United States",
		"opt-1|Macon, Georgia, United States",
	}
	if id, _, ok := pickComboboxOption(options, "Macomb Township, MI", []string{"Macomb", "Michigan"}); ok {
		t.Errorf("expected no selection, got %q", id)
	}
}

// Country has no mustContain tokens, so containment either way decides it:
// the list entry "United States" must satisfy a configured
// "United States of America".
func TestPickComboboxOption_MatchesAShorterListLabelAgainstALongerConfiguredValue(t *testing.T) {
	options := []string{"c-0|United Arab Emirates +971", "c-1|United States +1"}
	id, _, ok := pickComboboxOption(options, "United States of America", nil)
	if !ok {
		t.Fatal("expected a match for the configured country")
	}
	if id != "c-1" {
		t.Errorf("selected %q, want c-1 (United States)", id)
	}
}

// bugs.md #79: these widgets filter by substring against their own labels, so
// the configured value often cannot be typed in full. Measured live:
// "United States of America" matches nothing against a list whose entry is
// "United States", and "Macomb Township, MI" matches nothing at all.
func TestSearchPrefixes_ShortensUntilSomethingCanMatch(t *testing.T) {
	got := searchPrefixes("United States of America")
	want := []string{"United States of America", "United States of", "United States", "United"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("prefix %d = %q, want %q", i, got[i], want[i])
		}
	}
	// Commas are separators, not content.
	if p := searchPrefixes("Macomb Township, MI"); p[0] != "Macomb Township MI" || p[len(p)-1] != "Macomb" {
		t.Errorf("comma handling wrong: %v", p)
	}
}

func TestNormalizeOptionText_StripsDialCodeAndPunctuation(t *testing.T) {
	if got := normalizeOptionText("United States +1"); got != "united states" {
		t.Errorf("got %q, want %q", got, "united states")
	}
	if got := normalizeOptionText("Macomb, Illinois, United States"); got != "macomb illinois united states" {
		t.Errorf("got %q", got)
	}
}

// An ordinary text input must never be driven as a combobox — a stray Enter or
// option click can submit the form before the remaining fixes are applied.
func TestCommitComboboxOnLocator_LeavesPlainInputsAlone(t *testing.T) {
	touched := false
	loc := &MockLocator{
		countFunc:    func() (int, error) { return 1, nil },
		evaluateFunc: func(string) (interface{}, error) { return false, nil },
		clickFunc:    func(...playwright.LocatorClickOptions) error { touched = true; return nil },
		pressFunc:    func(string) error { touched = true; return nil },
		typeFunc:     func(string) error { touched = true; return nil },
	}
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}

	ok, err := commitComboboxOnLocator(pageTarget{page: page}, loc, "586-555-0100", nil)
	if err != nil {
		t.Fatalf("commitComboboxOnLocator: %v", err)
	}
	if ok {
		t.Error("expected ok=false for a control this does not handle")
	}
	if touched {
		t.Error("a plain input must not be clicked, typed into, or sent Enter")
	}
}

// bugs.md #81: react-select puts data-value on .select__input-container to
// mirror the TYPED SEARCH TEXT for input sizing, so it is non-empty the moment
// anything is typed — committed or not. Probed live: after a bare Fill() with
// nothing selected, the old read returned "I don't wish to answer". That false
// "landed" suppressed the commit step for every custom question on every
// Greenhouse form, which is why 13/13 fixes "applied" and the invalid-field
// list came back byte-identical.
//
// Only the widget's rendered selection counts.
func TestReadComboboxValue_IgnoresDataValueWhichMirrorsTypedText(t *testing.T) {
	if strings.Contains(readComboboxValueJS, "data-value") {
		t.Error("readComboboxValueJS must not consult data-value — it mirrors uncommitted typed text, not a committed selection")
	}
	if !strings.Contains(readComboboxValueJS, "select__single-value") {
		t.Error("readComboboxValueJS must read the rendered selection")
	}
}

// bugs.md #83: the context ceiling and the time ceiling are different limits,
// and on this hardware the time one binds first. Measured live: a 50,501-char
// payload fit the 80,000-char context window, passed the old check, and then
// burned the full 45-minute Ollama timeout — the single serialised LLM
// resource, spent on a request that could never have finished.
func TestLikelyExceedsModelContext_RejectsATimeDoomedPayloadThatFitsTheContext(t *testing.T) {
	// The exact size observed live.
	dom := strings.Repeat("x", 50501)
	if !likelyExceedsModelContext(dom, "") {
		t.Error("a 50,501-char payload fits the context window but cannot finish inside the 45-minute timeout — it must be rejected")
	}
	if len(dom) > maxPromptCharsForModelContext {
		t.Fatal("test premise broken: this payload should fit the context ceiling")
	}
}

func TestLikelyExceedsModelContext_StillAllowsNormalPayloads(t *testing.T) {
	// A narrowed retry payload is a few thousand chars; those must pass.
	if likelyExceedsModelContext(strings.Repeat("x", 8000), strings.Repeat("y", 4000)) {
		t.Error("an ordinary narrowed payload must not be rejected")
	}
}

// bugs.md #84: #82 added ErrNeedsUnprovidedAttestation and the cmd/agent
// branch meant to route it was never actually applied — the edit silently did
// not match, the build still passed, and ClickHouse was written off as
// FAILED_SUBMIT with its tailored documents stranded instead of being queued
// for manual completion.
//
// This list is what cmd/agent's catch-all consults, so a sentinel added
// without a routing branch still reaches manual review rather than the generic
// failure path.
func TestIsManualReviewError_CoversEveryManualReviewSentinel(t *testing.T) {
	for _, sentinel := range []error{ErrAuthWall, ErrFormTooLargeForModel, ErrNeedsUnprovidedAttestation} {
		if !IsManualReviewError(sentinel) {
			t.Errorf("%v must be treated as needing manual review", sentinel)
		}
		// Wrapped, as the call sites actually return them.
		if !IsManualReviewError(fmt.Errorf("context: %w", sentinel)) {
			t.Errorf("%v must still be recognised when wrapped", sentinel)
		}
	}
}

// An ordinary automation failure must NOT be diverted to manual review —
// that would hide real bugs behind a queue.
func TestIsManualReviewError_IgnoresOrdinaryFailures(t *testing.T) {
	if IsManualReviewError(fmt.Errorf("failed to submit application after 3 validation error attempts")) {
		t.Error("a genuine automation failure must not be routed to manual review")
	}
	if IsManualReviewError(nil) {
		t.Error("nil is not a manual-review outcome")
	}
}

// bugs.md #86: Lever's location typeahead carries none of react-select's
// markers — no role, no aria-*, no select__ classes. It is a plain
// <input name="location"> beside a hidden <input name="selectedLocation">
// holding the committed value, with results in a sibling .dropdown-results.
// Detecting only react-select made it read as an ordinary text input, so it
// was filled with text and never committed — and selectedLocation is what the
// form actually validates.
func TestComboboxJS_DetectsLeverStyleTypeaheads(t *testing.T) {
	if !strings.Contains(isComboboxInputJS, "dropdown-results") ||
		!strings.Contains(isComboboxInputJS, `input[type="hidden"][name^="selected"]`) {
		t.Error("combobox detection must recognise a sibling results dropdown or hidden commit field, not only react-select markup")
	}
	if !strings.Contains(readHiddenCommitValueJS, `name^="selected"`) {
		t.Error("the hidden commit field is where a Lever-style typeahead keeps its committed value")
	}
	if strings.Contains(readHiddenCommitValueJS, "el.value") {
		t.Error("must never fall back to el.value — that is uncommitted typed text (bugs.md #81)")
	}
	if !strings.Contains(comboboxOptionsJS, "dropdown-location") {
		t.Error("option enumeration must cover Lever's .dropdown-location results")
	}
}

// bugs.md #87: CSS alternations have no precedence — matches come back in DOM
// order. Putting `button:has-text('Apply')` alongside the real submit controls
// meant every retry "submitted" by clicking the click-to-reveal Apply button.
// Measured on a live Greenhouse form:
//
//	[0] visible BUTTON type=button  "Apply"                      <- clicked
//	[1] visible BUTTON type=button  "Quick Apply with MyGreenhouse"
//	[2] visible BUTTON type=submit  "Submit application"          <- the real one
func TestSubmitControlSelectors_PreferRealSubmitControlsAndNeverApply(t *testing.T) {
	if len(submitControlSelectors) == 0 {
		t.Fatal("no submit selectors configured")
	}
	first := submitControlSelectors[0]
	if !strings.Contains(first, "type='submit'") {
		t.Errorf("the highest-precedence group must be real submit controls, got %q", first)
	}
	for _, sel := range submitControlSelectors {
		if strings.Contains(sel, "'Apply'") || strings.Contains(sel, "\"Apply\"") {
			t.Errorf("Apply reveals a form, it never submits one — must not appear in %q", sel)
		}
	}
}

// The prioritised lookup must skip a group whose matches are all invisible and
// keep going, rather than settling for it.
func TestFindSubmitControl_SkipsAGroupWithNoVisibleMatch(t *testing.T) {
	hidden := &MockLocator{
		countFunc:     func() (int, error) { return 1, nil },
		isVisibleFunc: func() (bool, error) { return false, nil },
	}
	visible := &MockLocator{
		countFunc:     func() (int, error) { return 1, nil },
		isVisibleFunc: func() (bool, error) { return true, nil },
	}
	var asked []string
	page := &MockPage{locatorFunc: func(sel string, _ ...playwright.PageLocatorOptions) playwright.Locator {
		asked = append(asked, sel)
		if len(asked) == 1 {
			return hidden // first group matches but is invisible
		}
		return visible
	}}

	loc, count := findSubmitControl(pageTarget{page: page})
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if _, ok := firstVisibleSubmit(loc, count); !ok {
		t.Error("expected the returned group to have a visible match")
	}
	if len(asked) < 2 {
		t.Errorf("expected the lookup to move past the invisible group, selectors tried: %v", asked)
	}
}

// bugs.md #88: an uncommittable required widget is not an automation failure.
// Measured on Lever, whose geocoder returns zero results for "Macomb",
// "Macomb Township" and "Macomb, MI" while Greenhouse resolves the same
// address. With no option to select, the required hidden selectedLocation can
// never be populated. The job is perfectly applicable by hand, so it must be
// preserved rather than written off as FAILED_SUBMIT.
func TestErrUncommittableField_IsAManualReviewOutcome(t *testing.T) {
	if !IsManualReviewError(ErrUncommittableField) {
		t.Error("an uncommittable required field must route to manual review, not FAILED_SUBMIT")
	}
	wrapped := fmt.Errorf("%w: %s", ErrUncommittableField, "input[data-qa='location-input']")
	if !IsManualReviewError(wrapped) {
		t.Error("must still be recognised when wrapped with the offending selectors")
	}
	// The selectors must survive into the message so the log names the field.
	if !strings.Contains(wrapped.Error(), "location-input") {
		t.Errorf("expected the offending field named in the error, got %q", wrapped.Error())
	}
}

// bugs.md #89: Greenhouse replaces the form in place, so a successful submit
// leaves the URL unchanged and only a confirmation phrase can prove it. If
// that page renders after the 10s networkidle wait, the check right after the
// click sees the old DOM and reports failure — and retrying re-submits an
// application that already went through, filing a duplicate with a real
// employer. The re-check before each retry is the only protection against that.
func TestIsSubmissionConfirmed_ConfirmsOnPhraseWithAnUnchangedURL(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/orkes/jobs/5221481008"
	confirmed, reason := isSubmissionConfirmed(same, same, "<h1>Thank you for applying</h1>")
	if !confirmed {
		t.Fatal("a confirmation phrase must confirm even when the URL never changed — that is the Greenhouse case")
	}
	if reason != reasonConfirmationPhrase {
		t.Errorf("reason = %v, want the phrase reason", reason)
	}
}

// Without a phrase and without a URL change there is no evidence, and it must
// not be treated as success — that was bug #51.
func TestIsSubmissionConfirmed_StillRefusesWithNoEvidence(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/orkes/jobs/5221481008"
	if confirmed, _ := isSubmissionConfirmed(same, same, "<form>still here</form>"); confirmed {
		t.Error("an unchanged URL with no confirmation phrase is not evidence of success")
	}
}

// bugs.md #90: a required control offering exactly one option is unambiguous.
// Measured live on Sporty Group: "GDPR Acknowledgement*" offers only
// "Acknowledge/Confirm", the model proposed a differently-worded affirmative,
// nothing matched, and the job went to manual review one click from
// completion — with 10 of its 11 other fields already satisfied.
func TestPickComboboxOption_TakesTheSoleOptionWhenThereIsOnlyOne(t *testing.T) {
	options := []string{"gdpr-0|Acknowledge/Confirm"}
	id, idx, ok := pickComboboxOption(options, "Yes, I acknowledge", nil)
	if !ok {
		t.Fatal("a lone option on a required control is unambiguous and must be selected")
	}
	if id != "gdpr-0" || idx != 0 {
		t.Errorf("got id=%q idx=%d, want gdpr-0/0", id, idx)
	}
}

// But NOT when mustContain is set: those tokens exist because the identity of
// the option matters (#79), so a lone option failing them is a wrong answer,
// not an obvious one. Selecting it would file the wrong location.
func TestPickComboboxOption_StillRefusesALoneOptionThatFailsMustContain(t *testing.T) {
	options := []string{"loc-0|Detroit, ME, USA"}
	if id, _, ok := pickComboboxOption(options, "Macomb Township, MI", []string{"Macomb", "Michigan"}); ok {
		t.Errorf("selected %q — a lone wrong-state option must still be refused", id)
	}
}

// Two options with no match remains a refusal: there is a real choice to get
// wrong.
func TestPickComboboxOption_StillRefusesWhenSeveralOptionsAndNoneMatch(t *testing.T) {
	options := []string{"o-0|Alpha", "o-1|Beta"}
	if id, _, ok := pickComboboxOption(options, "Gamma", nil); ok {
		t.Errorf("selected %q — with several options and no match, nothing should be chosen", id)
	}
}

// bugs.md #92: Greenhouse names checkbox-group controls
// id="question_8242451101[]_54236360101". "#question_...[]_..." is not a valid
// CSS id selector — the brackets read as attribute syntax — so the verbatim
// match fails and the attribute form is the only thing that resolves it.
// Measured live: `selector matched no element (tried 1 form(s))`.
func TestSplitTagID_AllowsBracketsSoCheckboxGroupIDsResolve(t *testing.T) {
	tag, id, ok := splitTagID("input#question_8242451101[]_54236360101")
	if !ok {
		t.Fatal("a bracketed id must still yield an attribute-form retry")
	}
	if tag != "input" || id != "question_8242451101[]_54236360101" {
		t.Errorf("got tag=%q id=%q", tag, id)
	}
}

// Combinators and separators must still disqualify: those mean a compound
// selector, where rewriting the tail as one id would change the meaning.
func TestSplitTagID_StillRefusesCompoundSelectors(t *testing.T) {
	for _, sel := range []string{"div.wrap #x", "input#a, input#b", "div#a > input", "input#a.cls", "input#a:checked"} {
		if _, _, ok := splitTagID(sel); ok {
			t.Errorf("splitTagID(%q) must refuse — rewriting it as one id changes the meaning", sel)
		}
	}
}

// improvements.md #32: the poll must only ever accept a code that arrived
// AFTER the submit which triggered it, so a stale code from an earlier
// application can never be typed into a new one.
func TestWaitForSecurityCode_ReturnsTheCodeAndPassesTheCutoff(t *testing.T) {
	orig := SecurityCodeFetcher
	defer func() { SecurityCodeFetcher = orig }()

	cutoff := time.Date(2026, 7, 25, 16, 58, 0, 0, time.UTC)
	var gotCutoff time.Time
	SecurityCodeFetcher = func(notBefore time.Time) (string, error) {
		gotCutoff = notBefore
		return "uOSBQvRu", nil
	}

	code, err := waitForSecurityCode(cutoff)
	if err != nil {
		t.Fatalf("waitForSecurityCode: %v", err)
	}
	if code != "uOSBQvRu" {
		t.Errorf("code = %q, want uOSBQvRu", code)
	}
	if !gotCutoff.Equal(cutoff) {
		t.Errorf("cutoff = %v, want %v — a stale code must not be reusable", gotCutoff, cutoff)
	}
}

// With no fetcher configured the submitter must behave exactly as bugs.md #93
// described, so the nil case stays a supported configuration.
func TestSecurityCodeFetcher_NilByDefaultIsSupported(t *testing.T) {
	orig := SecurityCodeFetcher
	defer func() { SecurityCodeFetcher = orig }()
	SecurityCodeFetcher = nil
	if SecurityCodeFetcher != nil {
		t.Error("a nil fetcher must be a valid configuration")
	}
}

func TestFillSecurityCode_ReportsWhenNoFieldIsPresent(t *testing.T) {
	loc := &MockLocator{countFunc: func() (int, error) { return 0, nil }}
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}
	if err := fillSecurityCode(pageTarget{page: page}, "uOSBQvRu"); err == nil {
		t.Error("expected an error when there is no security-code field to fill")
	}
}

func TestFillSecurityCode_FillsAVisibleField(t *testing.T) {
	var filled string
	loc := &MockLocator{
		countFunc:     func() (int, error) { return 1, nil },
		isVisibleFunc: func() (bool, error) { return true, nil },
		fillFunc:      func(v string) error { filled = v; return nil },
	}
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}
	if err := fillSecurityCode(pageTarget{page: page}, "uOSBQvRu"); err != nil {
		t.Fatalf("fillSecurityCode: %v", err)
	}
	if filled != "uOSBQvRu" {
		t.Errorf("filled %q, want uOSBQvRu", filled)
	}
}

// bugs.md #95: the submit verdict used to come from one page read taken the
// instant WaitForLoadState(networkidle) returned -- which is frequently at
// once, because Click returns on event dispatch and the app has not issued its
// request yet. These pin the timing rules that replaced it.

func TestDecideSubmissionOutcome_UnchangedPageBeforeFloorIsNotYetRejection(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/acme/jobs/1"
	// The exact live signature: every field committed, the DOM still showing
	// the form, a few milliseconds after the click. Judging here is what
	// produced "applied N/N ... Submission failed validation" in one second.
	v := decideSubmissionOutcome(same, same, "<form>still here</form>", 5*time.Millisecond, 3, false)
	if v.Done {
		t.Fatalf("verdict at 5ms must be inconclusive, got %+v", v)
	}
}

func TestDecideSubmissionOutcome_FlaggedFieldsAfterFloorAreRejection(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/acme/jobs/1"
	v := decideSubmissionOutcome(same, same, "<form>still here</form>", submitOutcomeSettleFloor, 3, false)
	if !v.Done || v.Confirmed {
		t.Fatalf("flagged fields past the floor must be a settled rejection, got %+v", v)
	}
	if v.Reason != reasonFieldsFlagged {
		t.Errorf("reason = %q, want %q", v.Reason, reasonFieldsFlagged)
	}
}

func TestDecideSubmissionOutcome_ConfirmationWinsImmediatelyEvenBeforeFloor(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/acme/jobs/1"
	// A thank-you view that renders fast is no less real than a slow one.
	v := decideSubmissionOutcome(same, same, "<h1>Thank you for applying</h1>", time.Millisecond, 0, false)
	if !v.Done || !v.Confirmed {
		t.Fatalf("early confirmation must be accepted, got %+v", v)
	}
}

// The case the fix exists for: nothing has changed yet because the submission
// is still in flight. Before #95 this read as failure and cost a ~12-minute
// model call plus a duplicate submit click (#89).
func TestDecideSubmissionOutcome_LateConfirmationIsCaught(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/acme/jobs/1"
	if v := decideSubmissionOutcome(same, same, "<form>still here</form>", time.Second, 0, false); v.Done {
		t.Fatalf("must still be waiting at 1s with no evidence either way, got %+v", v)
	}
	v := decideSubmissionOutcome(same, same, "<h1>Application submitted</h1>", 6*time.Second, 0, false)
	if !v.Done || !v.Confirmed {
		t.Fatalf("a confirmation arriving at 6s must be caught, got %+v", v)
	}
}

func TestDecideSubmissionOutcome_GivesUpAtBudget(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/acme/jobs/1"
	v := decideSubmissionOutcome(same, same, "<div>nothing conclusive</div>", submitOutcomeBudget, 0, false)
	if !v.Done || v.Confirmed {
		t.Fatalf("budget exhaustion must settle as unconfirmed, got %+v", v)
	}
	if v.Reason != reasonNoOutcomeInBudget {
		t.Errorf("reason = %q, want %q", v.Reason, reasonNoOutcomeInBudget)
	}
}

// bugs.md #95: validation-error *wording* is rejection evidence even when the
// theme sets no aria-invalid anywhere -- the gap #83 ran into. Without this the
// poll burned its full budget on a form that had already visibly come back.
func TestDecideSubmissionOutcome_ErrorWordingIsRejectionWithoutAriaInvalid(t *testing.T) {
	const before = "https://job-boards.greenhouse.io/acme/jobs/1"
	const after = "https://job-boards.greenhouse.io/acme/jobs/1?err=1"
	content := "<form>Please correct the errors below</form>"
	if v := decideSubmissionOutcome(before, after, content, time.Millisecond, 0, false); v.Done {
		t.Fatalf("error wording before the floor is still too early, got %+v", v)
	}
	v := decideSubmissionOutcome(before, after, content, submitOutcomeSettleFloor, 0, false)
	if !v.Done || v.Confirmed {
		t.Fatalf("error wording past the floor must settle as rejection, got %+v", v)
	}
	if v.Reason != reasonErrorPhrase {
		t.Errorf("reason = %q, want %q", v.Reason, reasonErrorPhrase)
	}
}

// bugs.md #97: an uncommittable field must name the value that was attempted,
// not just the control. Reddit's veteran-status question (#434) refused two
// consecutive attempts and the log said only that #434 was left empty --
// giving no way to tell a broken commit mechanism from a value the widget
// does not offer. Those need opposite fixes, so the log has to distinguish
// them. The option list is real: typing "I don't wish to answer" filters it
// to exactly that entry, so the control is genuinely selectable.
func TestUncommittableFieldNamesTheAttemptedValue(t *testing.T) {
	notLanded := []string{
		fmt.Sprintf("%s (tried %q)", "#434", "I am not a protected veteran"),
	}
	err := fmt.Errorf("%w: %s", ErrUncommittableField, strings.Join(notLanded, ", "))

	if !IsManualReviewError(err) {
		t.Fatal("an uncommittable field must still route to manual review")
	}
	msg := err.Error()
	if !strings.Contains(msg, "#434") {
		t.Errorf("message must name the control, got %q", msg)
	}
	if !strings.Contains(msg, "I am not a protected veteran") {
		t.Errorf("message must name the attempted value, got %q", msg)
	}
}

// bugs.md #98: the model cannot see a react-select's permitted values -- they
// exist only once the widget opens, and these forms carry no native <select>.
// Confirmed live on Reddit, where it proposed "I am not a protected veteran"
// on two consecutive attempts against a widget offering "No military service"
// and "I don't wish to answer".

// newComboboxProbeLocator builds a locator that answers the combobox probe and
// the option read, and records whether it was clicked.
func newComboboxProbeLocator(isCombo bool, options []string) *MockLocator {
	loc := &MockLocator{
		countFunc:     func() (int, error) { return 1, nil },
		isVisibleFunc: func() (bool, error) { return true, nil },
		pressFunc:     func(string) error { return nil },
	}
	loc.clickFunc = func(...playwright.LocatorClickOptions) error { return nil }
	loc.evaluateFunc = func(expr string) (interface{}, error) {
		switch {
		case strings.Contains(expr, "role") && strings.Contains(expr, "combobox"):
			return isCombo, nil
		// aria-controls FIRST: comboboxOptionsJS mentions aria-activedescendant
		// too, as a fallback, so probing for that first would swallow the
		// option read entirely.
		case strings.Contains(expr, "aria-controls"):
			out := make([]interface{}, 0, len(options))
			for _, o := range options {
				out = append(out, o)
			}
			return out, nil
		case strings.Contains(expr, "aria-activedescendant"):
			// Answer the readiness probe immediately; otherwise
			// waitForComboboxReady polls its full 5s budget in tests.
			if isCombo && len(options) > 0 {
				return "opt-0", nil
			}
			return "", nil
		}
		return nil, nil
	}
	return loc
}

func TestEnumerateComboboxOptions_NamesTheExactPermittedValues(t *testing.T) {
	loc := newComboboxProbeLocator(true, []string{"No military service", "I don't wish to answer"})
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}

	got := enumerateComboboxOptions(pageTarget{page: page}, []string{"434"})
	if got == "" {
		t.Fatal("expected an option block for a combobox with options")
	}
	for _, want := range []string{`"No military service"`, `"I don't wish to answer"`, "434"} {
		if !strings.Contains(got, want) {
			t.Errorf("option block missing %s; got:\n%s", want, got)
		}
	}
	// The wording the model invented must NOT appear -- the block is the
	// closed set of real values, not a suggestion.
	if strings.Contains(got, "I am not a protected veteran") {
		t.Error("option block must contain only values the widget actually offers")
	}
}

// The safety-critical one. The invalid-field list routinely includes
// checkboxes (Greenhouse's GDPR consent among them), and clicking one toggles
// it -- silently changing the very answer this function exists to get right.
func TestEnumerateComboboxOptions_NeverClicksANonCombobox(t *testing.T) {
	checkbox := newComboboxProbeLocator(false, nil)
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return checkbox }}

	got := enumerateComboboxOptions(pageTarget{page: page}, []string{"gdpr_processing_consent_given_1"})
	if got != "" {
		t.Errorf("a non-combobox must contribute nothing, got:\n%s", got)
	}
	if checkbox.clickCalls != 0 {
		t.Fatalf("a non-combobox must NEVER be clicked (would toggle it); clicked %d time(s)", checkbox.clickCalls)
	}
}

func TestEnumerateComboboxOptions_EmptyWhenNothingToReport(t *testing.T) {
	if got := enumerateComboboxOptions(pageTarget{page: &MockPage{}}, nil); got != "" {
		t.Errorf("no selectors must yield no block, got %q", got)
	}
	// A combobox that reports no options adds nothing rather than an empty row.
	loc := newComboboxProbeLocator(true, nil)
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}
	if got := enumerateComboboxOptions(pageTarget{page: page}, []string{"434"}); got != "" {
		t.Errorf("a combobox with no readable options must be skipped, got:\n%s", got)
	}
}

// bugs.md #99: Reddit's form reached invalid fields: 0 -- fully satisfied for
// the first time -- and the submit still produced no confirmation and no
// rejection. No Greenhouse security-code email arrived either, while
// ClickHouse's accepted submit produced one within the same second, so
// Reddit's request never reached the server. A read-only probe found the
// submit control clean (single match, visible, enabled, unobstructed) and
// reCAPTCHA Enterprise embedded in the page.
func TestBotProtectionSrcPattern_MatchesKnownProvidersOnly(t *testing.T) {
	re, err := regexp.Compile("(?i)" + botProtectionSrcPattern)
	if err != nil {
		t.Fatalf("botProtectionSrcPattern does not compile: %v", err)
	}
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"reddit's actual recaptcha enterprise anchor",
			"https://www.recaptcha.net/recaptcha/enterprise/anchor?ar=1&k=6LfmcbcpAAAA", true},
		{"google-hosted recaptcha", "https://www.google.com/recaptcha/api2/anchor?k=x", true},
		{"hcaptcha", "https://newassets.hcaptcha.com/captcha/v1/x/static/hcaptcha.html", true},
		{"cloudflare turnstile", "https://challenges.cloudflare.com/cdn-cgi/challenge-platform/x", true},
		// The false-positive guard, and the reason this pattern matches iframe
		// src rather than page wording. bugs.md #45/#46 were CAPTCHA
		// mis-detections from phrase matching that between them killed the
		// large majority of Greenhouse/Lever/Ashby/Workable jobs before they
		// ever reached fit-scoring. A false positive here costs a real
		// application, so unrelated frames must never match.
		{"an ordinary tag-manager frame", "https://www.googletagmanager.com/ns.html?id=GTM-X", false},
		{"a youtube embed", "https://www.youtube.com/embed/abc123", false},
		{"greenhouse's own frame", "https://job-boards.greenhouse.io/embed/job_app?token=1", false},
		{"a google domain that is not recaptcha", "https://www.google.com/maps/embed?pb=x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := re.MatchString(tc.src); got != tc.want {
				t.Errorf("match(%s) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

// bugs.md #100: Akuity applied 7/7 fixes with no not-landed line -- every
// control reported as successfully set -- and the identical 7 fields came back
// flagged. #97 names values only for fields that FAIL to land, so this case
// had no diagnostic at all and the loop could only re-guess.
func TestRejectedDespiteLanding_PairsFieldsWithWhatWasWritten(t *testing.T) {
	applied := map[string]string{
		"input#question_6039579009":    "https://linkedin.com/in/example",
		"textarea#question_6051659009": "Ran production Kubernetes clusters for four years.",
		"input#question_9999999999":    "not rejected, must not appear",
	}
	got := rejectedDespiteLanding([]string{"question_6039579009", "question_6051659009"}, applied)

	if !strings.Contains(got, "question_6039579009 = \"https://linkedin.com/in/example\"") {
		t.Errorf("missing the rejected input's value; got %q", got)
	}
	if !strings.Contains(got, "Ran production Kubernetes clusters") {
		t.Errorf("missing the rejected textarea's value; got %q", got)
	}
	if strings.Contains(got, "must not appear") {
		t.Errorf("a field that was not rejected must not be reported; got %q", got)
	}
}

// The identifiers come from different places: the model writes a selector,
// parser.InvalidFieldIdentifiers reports the bare id. bugs.md #73/#92 are the
// precedent for how many shapes those selectors take.
func TestSelectorTargetsID_AcceptsTheSelectorShapesTheModelEmits(t *testing.T) {
	const id = "question_6039579009"
	for _, sel := range []string{
		id,
		"#" + id,
		"input#" + id,
		"textarea#" + id,
		"input[id='" + id + "']",
		`[id="` + id + `"]`,
	} {
		if !selectorTargetsID(sel, id) {
			t.Errorf("selector %q should target %q", sel, id)
		}
	}
	// Must not match a different control that merely shares a prefix.
	for _, sel := range []string{"input#question_60395790091", "#question_6039579", "input#other"} {
		if selectorTargetsID(sel, id) {
			t.Errorf("selector %q must NOT target %q", sel, id)
		}
	}
}

func TestTruncateForLog_KeepsLongAnswersReadable(t *testing.T) {
	long := strings.Repeat("word ", 60)
	got := truncateForLog(long, 40)
	if len(got) > 43 {
		t.Errorf("expected a truncated value, got %d chars", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncated value should be marked with an ellipsis, got %q", got)
	}
	// Newlines collapse so one answer stays on one log line.
	if got := truncateForLog("line one\nline two", 100); got != "line one line two" {
		t.Errorf("newlines must collapse, got %q", got)
	}
}

// bugs.md #101: three jobs ended 2026-07-25 with a bare
// `playwright: timeout: Timeout 30000ms exceeded` from the submit click
// (Akuity, Nova, Zimperium) and no indication of why the control was
// unactionable. A timeout says the click never landed; it says nothing about
// what stopped it.
func TestDescribeSubmitObstruction_ReportsWhatCoversTheControl(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]interface{}
		want    []string
	}{
		{
			name:    "no submit control at all",
			payload: map[string]interface{}{"found": false},
			want:    []string{"no submit control"},
		},
		{
			name: "clear button, nothing covering it (Reddit's shape in #99)",
			payload: map[string]interface{}{
				"found": true, "disabled": false, "inViewport": true, "coveredBy": "",
			},
			want: []string{"nothing covering it", "disabled=false"},
		},
		{
			name: "an overlay intercepting the click (#34's shape)",
			payload: map[string]interface{}{
				"found": true, "disabled": false, "inViewport": true,
				"coveredBy": "DIV#onetrust-consent-sdk.banner",
			},
			want: []string{"covered by DIV#onetrust-consent-sdk.banner"},
		},
		{
			name: "a captcha frame sitting over the button",
			payload: map[string]interface{}{
				"found": true, "disabled": false, "inViewport": true,
				"coveredBy": "IFRAME.challenge src=https://challenges.cloudflare.com/x",
			},
			want: []string{"covered by IFRAME.challenge", "challenges.cloudflare.com"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := &MockPage{evaluateFunc: func(string) (interface{}, error) {
				return tc.payload, nil
			}}
			got := describeSubmitObstruction(page)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("describeSubmitObstruction = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

// Best-effort: a page that cannot be evaluated must not break the failure path
// it is only trying to explain.
func TestDescribeSubmitObstruction_IsBestEffort(t *testing.T) {
	page := &MockPage{evaluateFunc: func(string) (interface{}, error) {
		return nil, fmt.Errorf("execution context destroyed")
	}}
	if got := describeSubmitObstruction(page); got != "" {
		t.Errorf("an unevaluable page must yield no description, got %q", got)
	}
}

// bugs.md #102: Greenhouse ACCEPTS a submission and issues an emailed
// security-code challenge, then re-renders while the previous attempt's
// aria-invalid markers are still on the page. Both signals are therefore true
// at once, and #95's flagged-field branch was reading the stale one and
// calling an accepted submission a validation failure.
//
// Measured: the Greenhouse code email for Akuity is timestamped 23:40:07,
// between its submit click (~23:40:06) and its verdict (23:40:08); ClickHouse's
// is timestamped 00:05:34, the same second as its submit.
func TestDecideSubmissionOutcome_SecurityCodeGateBeatsStaleInvalidFlags(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/acme/jobs/1"
	content := `<form><input id="security_code" name="security_code">` +
		`Copy and paste this code into the security code field on your application</form>`

	// The exact live shape: fields still flagged AND a code gate present.
	v := decideSubmissionOutcome(same, same, content, submitOutcomeSettleFloor, 7, true)
	if !v.Done {
		t.Fatalf("a security-code gate is a settled outcome, got %+v", v)
	}
	if v.Reason != reasonSecurityCodeGate {
		t.Errorf("reason = %q, want %q -- stale invalid flags must not win", v.Reason, reasonSecurityCodeGate)
	}
	if v.Confirmed {
		t.Error("a code gate is not yet a confirmed application; it still needs the code entered")
	}
}

// The gate wins even before the settle floor: acceptance is acceptance.
func TestDecideSubmissionOutcome_SecurityCodeGateWinsEarly(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/acme/jobs/1"
	v := decideSubmissionOutcome(same, same, "<form>whatever</form>", time.Millisecond, 3, true)
	if !v.Done || v.Reason != reasonSecurityCodeGate {
		t.Fatalf("gate must settle immediately, got %+v", v)
	}
}

// Without a gate the old behaviour must be intact, or #99 would start
// attributing ordinary validation failures to bot protection.
func TestDecideSubmissionOutcome_NoGateKeepsRejectionSignal(t *testing.T) {
	const same = "https://job-boards.greenhouse.io/acme/jobs/1"
	v := decideSubmissionOutcome(same, same, "<form>still here</form>", submitOutcomeSettleFloor, 7, false)
	if !v.Done || v.Confirmed || v.Reason != reasonFieldsFlagged {
		t.Fatalf("flagged fields with no gate must still settle as rejection, got %+v", v)
	}
}

// The floor must be long enough for a challenge that renders after the
// verdict used to be taken. Akuity's verdict landed at 2.2s and missed it.
func TestSubmitOutcomeSettleFloor_LeavesRoomForALateChallenge(t *testing.T) {
	if submitOutcomeSettleFloor < 5*time.Second {
		t.Errorf("settle floor %v is too short for a late-rendering security-code challenge (bugs.md #102)", submitOutcomeSettleFloor)
	}
	if submitOutcomeSettleFloor >= submitOutcomeBudget {
		t.Errorf("settle floor %v must stay below the budget %v", submitOutcomeSettleFloor, submitOutcomeBudget)
	}
}

// bugs.md #103: #100's diagnostic caught this within one cycle of shipping.
// readComboboxOptions returns "id|label" so pickComboboxOption can click by
// id, and #98 put those raw strings in front of the model, which faithfully
// answered "react-select-question_67179376-option-0|Yes" -- an internal DOM id
// no widget offers. Live evidence:
//
//	Rejected despite being set last attempt: question_67179376 =
//	"react-select-question_67179376-option-0|Yes"
func TestOptionLabel_StripsTheInternalOptionID(t *testing.T) {
	cases := map[string]string{
		"react-select-question_67179376-option-0|Yes":     "Yes",
		"react-select-question_67179377-option-1|No":      "No",
		"react-select-question_67179378-option-0|I agree": "I agree",
		// A label containing a pipe keeps everything after the FIRST separator.
		"opt-3|Yes | No | Maybe": "Yes | No | Maybe",
		// Already-bare labels pass through (Lever's shape).
		"No military service": "No military service",
		"":                    "",
	}
	for entry, want := range cases {
		if got := optionLabel(entry); got != want {
			t.Errorf("optionLabel(%q) = %q, want %q", entry, got, want)
		}
	}
}

// The end-to-end guarantee: no internal identifier may reach the prompt.
func TestEnumerateComboboxOptions_NeverLeaksInternalIDsToTheModel(t *testing.T) {
	loc := newComboboxProbeLocator(true, []string{
		"react-select-question_67179376-option-0|Yes",
		"react-select-question_67179376-option-1|No",
	})
	page := &MockPage{locatorFunc: func(string, ...playwright.PageLocatorOptions) playwright.Locator { return loc }}

	got := enumerateComboboxOptions(pageTarget{page: page}, []string{"question_67179376"})
	if strings.Contains(got, "react-select-") || strings.Contains(got, "option-0") {
		t.Errorf("option block leaked an internal id to the model:\n%s", got)
	}
	for _, want := range []string{`"Yes"`, `"No"`} {
		if !strings.Contains(got, want) {
			t.Errorf("option block missing the human-readable label %s; got:\n%s", want, got)
		}
	}
}

// bugs.md #104: Reddit's job 7956443 filled all five custom questions with
// sensible values -- "company website", "Stellantis Financial Services",
// "Yes", "No", "I agree" -- committed all three comboboxes, and the identical
// five came back flagged with the page byte-for-byte unchanged (140544 chars
// twice). Nothing was left for the model to fix; the submission was never
// reaching the server past the page's reCAPTCHA.
func TestAllFieldsWereSet_RequiresEveryRejectedFieldToHaveBeenWritten(t *testing.T) {
	applied := map[string]string{
		"#question_67179374": "company website",
		"#question_67179375": "Stellantis Financial Services",
		"#question_67179376": "Yes",
	}
	all := []string{"question_67179374", "question_67179375", "question_67179376"}
	if !allFieldsWereSet(all, applied) {
		t.Error("every rejected field was written; expected true")
	}

	// One field the previous attempt never set means there IS something left
	// to fix -- an ordinary validation failure that keeps its remaining retry.
	withUnset := append(append([]string{}, all...), "question_67179999")
	if allFieldsWereSet(withUnset, applied) {
		t.Error("a rejected field that was never written must prevent the captcha verdict")
	}
}

func TestAllFieldsWereSet_EmptyInputsAreNeverAMatch(t *testing.T) {
	applied := map[string]string{"#question_1": "x"}
	if allFieldsWereSet(nil, applied) {
		t.Error("no rejected fields must not report a captcha block")
	}
	if allFieldsWereSet([]string{"question_1"}, map[string]string{}) {
		t.Error("nothing applied must not report a captcha block")
	}
}

// bugs.md #104 follow-up. Measured across three Greenhouse boards the same
// night:
//
//	reddit      recaptcha present  -> submit blocked
//	clickhouse  recaptcha absent   -> submit accepted
//	akuity      recaptcha PRESENT  -> submit ACCEPTED (code email 23:40:07)
//
// So a bot-protection frame says nothing on its own, and an accepted
// submission awaiting a code must never be reported as captcha-blocked --
// #102's rule, which #104 could have reintroduced a violation of because its
// check sits above #93's gate handling.
func TestSecurityCodeGateOutranksTheCaptchaVerdict(t *testing.T) {
	gateHTML := `<form><input id="security_code" name="security_code">` +
		`Copy and paste this code into the security code field on your application</form>`
	if !parser.DetectSecurityCodeChallenge(gateHTML) {
		t.Fatal("precondition: the gate must be detectable in this markup")
	}

	applied := map[string]string{"#question_1": "Yes", "#question_2": "No"}
	ids := []string{"question_1", "question_2"}

	// Everything the captcha verdict keys on is true here...
	if !allFieldsWereSet(ids, applied) {
		t.Fatal("precondition: every rejected field was already set")
	}
	// ...so only the gate check can prevent the mislabel. This mirrors the
	// guard's exact composition at the call site.
	if allFieldsWereSet(ids, applied) && !parser.DetectSecurityCodeChallenge(gateHTML) {
		t.Error("an accepted submission awaiting a security code must not be reported as captcha-blocked")
	}

	// Without a gate, the captcha verdict must still be reachable.
	noGate := "<form>nothing here</form>"
	if !(allFieldsWereSet(ids, applied) && !parser.DetectSecurityCodeChallenge(noGate)) {
		t.Error("with no gate present the captcha verdict must remain reachable")
	}
}

// bugs.md #105: #83's ceiling models input size only. Measured on this
// hardware, field count is what actually separates the runs that finish from
// the one that burned the whole 45-minute Ollama timeout:
//
//	ClickHouse   11,140 chars,  3 fields -> ~7 min   ok
//	Reddit       18,639 chars, 13 fields -> ~15 min  ok
//	Remote       30,477 chars, 34 fields -> 45 min   TIMED OUT
func TestExceedsRetryTimeBudget_CountsAnswersNotJustBytes(t *testing.T) {
	// Remote's real shape: comfortably inside the old 40,000-char ceiling,
	// and it still timed out. Field count has to be what catches it.
	dom := strings.Repeat("x", 19481)
	profile := strings.Repeat("y", 11000) // ~30,481 total, as sent
	if !exceedsRetryTimeBudget(dom, profile, 34) {
		t.Error("a 34-field retry must be refused; it consumed the full timeout live")
	}

	// Reddit's shape must still be allowed -- it completed in ~15 minutes.
	okDom := strings.Repeat("x", 7212)
	okProfile := strings.Repeat("y", 11400) // ~18,612 total
	if exceedsRetryTimeBudget(okDom, okProfile, 13) {
		t.Error("a 13-field retry of Reddit's size completed live and must not be refused")
	}
}

func TestExceedsRetryTimeBudget_FieldCountAloneCanRefuse(t *testing.T) {
	tiny := strings.Repeat("x", 500)
	if exceedsRetryTimeBudget(tiny, "", maxInvalidFieldsForTimeBudget) {
		t.Error("exactly at the field limit must be allowed")
	}
	if !exceedsRetryTimeBudget(tiny, "", maxInvalidFieldsForTimeBudget+1) {
		t.Error("one field over the limit must be refused even on a tiny payload")
	}
}

// The character ceiling must sit below the size that was observed to fail.
func TestMaxPromptCharsForTimeBudget_SitsBelowTheObservedFailure(t *testing.T) {
	const observedTimeoutChars = 30477
	if maxPromptCharsForTimeBudget >= observedTimeoutChars {
		t.Errorf("ceiling %d must be below the %d-char payload that timed out live (bugs.md #105)",
			maxPromptCharsForTimeBudget, observedTimeoutChars)
	}
}

// bugs.md #106: Greenhouse names checkbox-group controls
// "question_8242451101[]_54236360101". The brackets alone make
// looksLikeCSSSelector true, but there is no tag#id to split, so the selector
// was used verbatim with NO fallbacks -- "tried 1 form(s)" -- and matched
// nothing, because it is not valid CSS for an id either. Third shape of the
// same defect: #73 fixed "input#430", #92 fixed "#question_...[]_...", this is
// the bare form with no prefix.
func TestResolveFieldLocator_BareBracketedIDGetsAttributeFallbacks(t *testing.T) {
	const bracketed = "question_8242451101[]_54236360101"

	var tried []string
	page := &MockPage{locatorFunc: func(sel string, _ ...playwright.PageLocatorOptions) playwright.Locator {
		tried = append(tried, sel)
		// Only the attribute form resolves, as on the real page.
		if sel == fmt.Sprintf("[id=%q]", bracketed) {
			return &MockLocator{
				countFunc:     func() (int, error) { return 1, nil },
				isVisibleFunc: func() (bool, error) { return true, nil },
			}
		}
		return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
	}}

	loc, err := resolveFieldLocator(pageTarget{page: page}, bracketed)
	if err != nil {
		t.Fatalf("bracketed id must resolve via its attribute form, got: %v", err)
	}
	if loc == nil {
		t.Fatal("expected a locator")
	}
	if len(tried) < 2 {
		t.Errorf("expected attribute fallbacks to be tried, only tried: %v", tried)
	}
	found := false
	for _, s := range tried {
		if s == fmt.Sprintf("[id=%q]", bracketed) {
			found = true
		}
	}
	if !found {
		t.Errorf("the [id=...] form must be among the candidates; tried: %v", tried)
	}
}

// A genuine CSS selector must keep working: the extra candidates are appended
// after the verbatim one and simply match nothing.
func TestResolveFieldLocator_RealCSSSelectorStillWinsFirst(t *testing.T) {
	const css = "input[type='email']"
	var tried []string
	page := &MockPage{locatorFunc: func(sel string, _ ...playwright.PageLocatorOptions) playwright.Locator {
		tried = append(tried, sel)
		if sel == css {
			return &MockLocator{
				countFunc:     func() (int, error) { return 1, nil },
				isVisibleFunc: func() (bool, error) { return true, nil },
			}
		}
		return &MockLocator{countFunc: func() (int, error) { return 0, nil }}
	}}
	if _, err := resolveFieldLocator(pageTarget{page: page}, css); err != nil {
		t.Fatalf("a genuine CSS selector must still resolve: %v", err)
	}
	if len(tried) != 1 || tried[0] != css {
		t.Errorf("the verbatim selector must be tried first and win; tried: %v", tried)
	}
}

// bugs.md #108: Ethos reached `invalid fields: 0`, exhausted the settle budget
// with NO bot-protection frame to explain it, and was then reported as
// "form content exceeds the local model's context window" -- because narrowing
// found nothing to narrow, fell back to the whole document, and the size check
// caught it incidentally. Right outcome, wrong cause. A misleading reason has
// real cost: it is how #83 misdiagnosed the case #93 later reframed.
func TestErrSubmitProducedNoOutcome_IsAManualReviewOutcome(t *testing.T) {
	if !IsManualReviewError(ErrSubmitProducedNoOutcome) {
		t.Fatal("a fully-filled form whose submit went nowhere must be preserved for a human, not written off")
	}
	wrapped := fmt.Errorf("%w: %s", ErrSubmitProducedNoOutcome, "job-boards.greenhouse.io/ethos")
	if !IsManualReviewError(wrapped) {
		t.Error("the wrapped form must still route to manual review")
	}
	// It must be distinguishable from the size sentinel it used to be
	// misreported as -- that distinction is the entire point.
	if errors.Is(wrapped, ErrFormTooLargeForModel) {
		t.Error("must NOT be conflated with ErrFormTooLargeForModel; naming the wrong cause is the bug")
	}
	if !strings.Contains(wrapped.Error(), "no confirmation and no rejection") {
		t.Errorf("the message must state what actually happened, got %q", wrapped.Error())
	}
}

// bugs.md #109: Greenhouse renders some single-choice questions as a set of
// checkboxes sharing one name. Sporty Group asks exactly that way, probed live:
//
//	question_8242451101[]_54236359101  label "Yes"
//	question_8242451101[]_54236360101  label "No"
//	question_8242451101[]_54236362101  label "Prefer not to say"
//
// A model value of "No" means TICK THE BOX LABELLED NO, not "leave this box
// unticked". The two readings produce opposite results, and #107 made the
// wrong one report as landed.
func TestCheckboxGroup_PicksTheOptionNamedByTheValue(t *testing.T) {
	group := []string{
		"question_8242451101[]_54236359101|Yes",
		"question_8242451101[]_54236360101|No",
		"question_8242451101[]_54236362101|Prefer not to say",
	}

	optID, _, ok := pickComboboxOption(group, "No", nil)
	if !ok {
		t.Fatal(`"No" must select an option in the group`)
	}
	if optID != "question_8242451101[]_54236360101" {
		t.Errorf(`"No" selected %q, want the box labelled "No"`, optID)
	}

	if id, _, ok := pickComboboxOption(group, "Yes", nil); !ok || id != "question_8242451101[]_54236359101" {
		t.Errorf(`"Yes" selected %q (ok=%v), want the Yes box`, id, ok)
	}
	if id, _, ok := pickComboboxOption(group, "Prefer not to say", nil); !ok || id != "question_8242451101[]_54236362101" {
		t.Errorf(`"Prefer not to say" selected %q (ok=%v), want that box`, id, ok)
	}
}

// #79's rule carries over: with no matching option, tick nothing rather than
// guess. A wrong tick here is a wrong answer on a real application.
func TestCheckboxGroup_RefusesWhenNoOptionMatches(t *testing.T) {
	group := []string{
		"opt_a|Yes",
		"opt_b|No",
	}
	if id, _, ok := pickComboboxOption(group, "Somewhat", nil); ok {
		t.Errorf("an unmatched value must select nothing, got %q", id)
	}
}

// A standalone checkbox must keep #107's behaviour: no group, so a negative
// means leave it unticked and that counts as the requested state.
func TestCheckboxGroupOptions_EmptyForAStandaloneBox(t *testing.T) {
	loc := &MockLocator{
		countFunc: func() (int, error) { return 1, nil },
		evaluateFunc: func(expr string) (interface{}, error) {
			if strings.Contains(expr, "querySelectorAll") {
				return []interface{}{}, nil // fewer than 2 members
			}
			return "input|checkbox", nil
		},
	}
	if got := checkboxGroupOptions(loc); len(got) != 0 {
		t.Errorf("a standalone checkbox must report no group, got %v", got)
	}
}

// bugs.md #110: the option matcher used raw bidirectional substring, and a
// short label hides inside longer prose -- "No" sits inside "prefer NOt to
// say". Asking for "Prefer not to say" therefore selected the box labelled
// "No", turning a declined EEO answer into a substantive one on a real
// application. Exactly the failure #79 exists to prevent.
func TestOptionTextMatches_ShortLabelCannotHijackLongerAnswer(t *testing.T) {
	if optionTextMatches("no", "prefer not to say") {
		t.Error(`"No" must NOT match "Prefer not to say" -- "no" is only inside the word "not"`)
	}
	if optionTextMatches("yes", "yesterday") {
		t.Error(`"Yes" must NOT match "yesterday"`)
	}
	if optionTextMatches("male", "female") {
		t.Error(`"Male" must NOT match "Female" -- this one would misstate a real answer`)
	}
}

// Every match the old rule was written for must survive.
func TestOptionTextMatches_KeepsTheMatchesItWasWrittenFor(t *testing.T) {
	// bugs.md #79: a shorter list label against a longer configured value.
	if !optionTextMatches("united states", "united states of america") {
		t.Error(`option "United States" must match "United States of America"`)
	}
	// improvements.md #33: a longer geocoder result against a shorter value.
	if !optionTextMatches("macomb mi usa", "macomb mi") {
		t.Error(`option "Macomb, MI, USA" must match "Macomb, MI"`)
	}
	if !optionTextMatches("no military service", "no military service") {
		t.Error("an exact label must match itself")
	}
	if !optionTextMatches("i don t wish to answer", "i don t wish to answer") {
		t.Error("the decline option must match itself")
	}
}

// The words must be contiguous and in order, not merely present.
func TestOptionTextMatches_RequiresAContiguousWordRun(t *testing.T) {
	if optionTextMatches("states united", "united states of america") {
		t.Error("out-of-order words must not match")
	}
	if optionTextMatches("united america", "united states of america") {
		t.Error("non-contiguous words must not match")
	}
}

// bugs.md #111: Greenhouse accepts a submission and emails the security code
// within ~1s, but the code input does not reach the DOM for far longer.
// Measured on Akuity: email timestamped 05:59:19, and the verdict at 05:59:27
// still saw no gate -- so #104's captcha verdict fired on an application that
// had actually gone through. The mailbox is the ground truth; the DOM is not.
func TestPendingSecurityCodeAfter_UsesTheMailboxNotTheDOM(t *testing.T) {
	origFetcher := SecurityCodeFetcher
	defer func() { SecurityCodeFetcher = origFetcher }()

	clickedAt := time.Now()

	// A code issued after the click means the submit was accepted.
	var askedSince time.Time
	SecurityCodeFetcher = func(since time.Time) (string, error) {
		askedSince = since
		return "82taTsxA", nil
	}
	if got := pendingSecurityCodeAfter(clickedAt); got != "82taTsxA" {
		t.Errorf("pendingSecurityCodeAfter = %q, want the emailed code", got)
	}
	if !askedSince.Equal(clickedAt) {
		t.Error("must ask only for codes issued after the submit click, or a stale code could be reused")
	}

	// No code: silent, so the caller's captcha reasoning proceeds.
	SecurityCodeFetcher = func(time.Time) (string, error) { return "", nil }
	if got := pendingSecurityCodeAfter(clickedAt); got != "" {
		t.Errorf("no code must yield empty, got %q", got)
	}

	// A fetch error must not be mistaken for acceptance.
	SecurityCodeFetcher = func(time.Time) (string, error) { return "", fmt.Errorf("imap down") }
	if got := pendingSecurityCodeAfter(clickedAt); got != "" {
		t.Errorf("a fetch error must not read as acceptance, got %q", got)
	}
}

// With no fetcher wired, or no click recorded, this must stay silent rather
// than block or panic -- it runs on the hot retry path.
func TestPendingSecurityCodeAfter_SafeWithoutAFetcherOrClick(t *testing.T) {
	origFetcher := SecurityCodeFetcher
	defer func() { SecurityCodeFetcher = origFetcher }()

	SecurityCodeFetcher = nil
	if got := pendingSecurityCodeAfter(time.Now()); got != "" {
		t.Errorf("no fetcher must yield empty, got %q", got)
	}

	called := false
	SecurityCodeFetcher = func(time.Time) (string, error) { called = true; return "x", nil }
	if got := pendingSecurityCodeAfter(time.Time{}); got != "" {
		t.Errorf("a zero click time must yield empty, got %q", got)
	}
	if called {
		t.Error("must not query the mailbox when no submit click has been recorded")
	}
}
