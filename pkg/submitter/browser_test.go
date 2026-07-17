package submitter

import (
	"fmt"
	"testing"
	"github.com/playwright-community/playwright-go"
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

func TestSafeFill_Empty(t *testing.T) {
	mockPage := &MockPage{}
	err := safeFill(mockPage, "", "text")
	if err != ErrEmptySelector {
		t.Errorf("safeFill with empty selector should return ErrEmptySelector, got %v", err)
	}
	
	err = safeFill(mockPage, "selector", "")
	if err != nil {
		t.Errorf("safeFill with empty text should return nil, got %v", err)
	}
}
