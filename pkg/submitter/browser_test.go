package submitter

import (
	"fmt"
	"testing"
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
	
	err := AttemptSubmit(mockBrowser, nil, nil, "TestCompany", "https://example.com/apply", nil, nil, "", false, false)
	
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
	
	err := AttemptSubmit(mockBrowser, nil, nil, "TestCompany", "https://example.com/apply", nil, nil, "", false, false)
	
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
}

func (m *MockPage) Locator(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
	if m.locatorFunc != nil {
		return m.locatorFunc(selector, options...)
	}
	return nil
}

func (m *MockPage) WaitForTimeout(timeout float64) {}

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
	countFunc  func() (int, error)
	clickFunc  func(options ...playwright.LocatorClickOptions) error
	clickCalls int
	fillFunc   func(value string) error
	fillCalls  int
}

func (m *MockLocator) First() playwright.Locator { return m }

func (m *MockLocator) Count() (int, error) {
	if m.countFunc != nil {
		return m.countFunc()
	}
	return 0, nil
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
		{"", false},
	}

	for _, tt := range tests {
		got := isDeadJobPage(tt.content)
		if got != tt.want {
			t.Errorf("isDeadJobPage(%q) = %v, want %v", tt.content, got, tt.want)
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
	selLocator := &MockLocator{fillFunc: func(value string) error { return nil }}
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
