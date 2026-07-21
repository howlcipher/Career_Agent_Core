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
	locatorFunc func(selector string, options ...playwright.PageLocatorOptions) playwright.Locator
}

func (m *MockPage) Locator(selector string, options ...playwright.PageLocatorOptions) playwright.Locator {
	if m.locatorFunc != nil {
		return m.locatorFunc(selector, options...)
	}
	return nil
}

func (m *MockPage) WaitForTimeout(timeout float64) {}

// pwLocator aliases playwright.Locator so it can be embedded under a field
// name other than "Locator" — the interface has its own Locator(...)
// chaining method, which otherwise collides with the embedded field name.
type pwLocator = playwright.Locator

type MockLocator struct {
	pwLocator
	countFunc  func() (int, error)
	clickFunc  func(options ...playwright.LocatorClickOptions) error
	clickCalls int
}

func (m *MockLocator) First() playwright.Locator { return m }

func (m *MockLocator) Count() (int, error) {
	if m.countFunc != nil {
		return m.countFunc()
	}
	return 0, nil
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
