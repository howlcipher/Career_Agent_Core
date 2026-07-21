package submitter

import (
	"fmt"
	"log"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/mxschmitt/playwright-go"
)

// AttemptVisionSubmit is the V3 mechanism that uses Gemini Vision to literally "look" at the screen
// and map coordinates/selectors if standard HTML DOM pruning fails or is heavily obfuscated.
func AttemptVisionSubmit(page playwright.Page, target fillTarget, companyName, applyURL, resumePath string, pii *config.PII, mapper FormMapper, autoSubmitClick bool) error {
	log.Printf("[Vision-Submit] Taking a full-page screenshot of %s for Visual Reasoning...", applyURL)
	
	// Take full page screenshot
	screenshotBytes, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
		Type:     playwright.ScreenshotTypePng,
	})
	if err != nil {
		return fmt.Errorf("failed to take screenshot: %w", err)
	}

	log.Println("[Vision-Submit] Transmitting screenshot to Gemini-1.5-Pro for visual mapping...")
	
	// Pass image byte array to Gemini
	mappingJSON, err := mapper.ExtractFormMappingVision(screenshotBytes)
	if err != nil {
		return fmt.Errorf("gemini vision failed to map visual layout: %w", err)
	}

	log.Println("[Vision-Submit] Gemini successfully mapped the visual DOM structure!")
	
	domain := ExtractDomain(applyURL)
	// Save it to SQLite so we don't have to use API credits for this specific ATS again!
	if err := storage.SaveFormMapping(domain, mappingJSON); err != nil {
		log.Printf("[Vision-Submit] Warning: Could not cache vision mapping: %v", err)
	}

	// Now execute the standard dynamic handler using the newly generated visual mapping
	return handleDynamic(target, resumePath, pii, mappingJSON, autoSubmitClick)
}
