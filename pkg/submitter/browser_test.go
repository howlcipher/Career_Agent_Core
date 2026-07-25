package submitter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	err := confirmOrError(page, "Acme", urlBeforeClick, true)
	if err == nil {
		t.Fatal("expected an error when the URL never changed from the pre-click baseline, got nil")
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

	landed, err := verifyFixLanded(pageTarget{page: page}, "#candidate-location")
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

	landed, err := verifyFixLanded(pageTarget{page: page}, "#phone")
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

	landed, err := verifyFixLanded(pageTarget{page: page}, "#consent")
	if err != nil {
		t.Fatalf("verifyFixLanded: %v", err)
	}
	if landed {
		t.Error("expected landed=false for a checkbox still reporting checked=false")
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
