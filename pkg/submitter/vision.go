package submitter

import (
	"fmt"
	"log"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/mxschmitt/playwright-go"
)

func attemptQuarantinedVisionSubmit(
	page playwright.Page,
	target fillTarget,
	filter *security.QuarantineLayer,
	companyName string,
	applyURL string,
	resumePath string,
	coverPath string,
	pii *config.PII,
	mapper FormMapper,
	copilotMode bool,
	autoSubmitClick bool,
) error {
	if err := quarantineFillTargetDOM(
		filter,
		applyURL,
		companyName,
		target,
	); err != nil {
		return fmt.Errorf("form rejected before vision model use: %w", err)
	}
	return attemptVisionSubmit(
		page,
		target,
		companyName,
		applyURL,
		resumePath,
		coverPath,
		pii,
		mapper,
		copilotMode,
		autoSubmitClick,
	)
}

// visionProviderName is implemented by any FormMapper that can name the
// backend actually doing the vision call. Optional rather than part of
// FormMapper itself, so the interface stays minimal and a mapper that
// cannot name itself (e.g. a test double) just falls back to a generic
// label instead of failing to compile.
type visionProviderName interface {
	ProviderName() string
}

// visionModelLabel names the configured vision backend for logging, instead
// of a hardcoded "Gemini" that is wrong whenever LLM_PROVIDER selects
// anything else -- this vision path runs on whatever provider's
// ExtractFormMappingVision is wired in, which on this repo's default
// configuration is a local Ollama vision model, not Gemini (bugs.md #444).
func visionModelLabel(mapper FormMapper) string {
	if named, ok := mapper.(visionProviderName); ok {
		return named.ProviderName()
	}
	return "the configured vision model"
}

// attemptVisionSubmit is the V3 mechanism that uses the configured vision
// model to literally "look" at the screen and map coordinates/selectors if
// standard HTML DOM pruning fails or is heavily obfuscated.
func attemptVisionSubmit(page playwright.Page, target fillTarget, companyName, applyURL, resumePath, coverPath string, pii *config.PII, mapper FormMapper, copilotMode, autoSubmitClick bool) error {
	log.Printf("[Vision-Submit] Taking a full-page screenshot of %s for Visual Reasoning...", applyURL)

	// Take full page screenshot
	screenshotBytes, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
		Type:     playwright.ScreenshotTypePng,
	})
	if err != nil {
		return fmt.Errorf("failed to take screenshot: %w", err)
	}

	providerLabel := visionModelLabel(mapper)
	log.Printf("[Vision-Submit] Transmitting screenshot to %s for visual mapping...", providerLabel)

	// Pass image byte array to the configured vision model
	mappingJSON, err := mapper.ExtractFormMappingVision(screenshotBytes)
	if err != nil {
		return fmt.Errorf("vision model failed to map visual layout: %w", err)
	}

	log.Printf("[Vision-Submit] %s successfully mapped the visual DOM structure!", providerLabel)

	domain := ExtractDomain(applyURL)
	// Save it to SQLite so we don't have to re-run vision mapping for this specific ATS again!
	if err := storage.SaveFormMapping(domain, mappingJSON); err != nil {
		log.Printf("[Vision-Submit] Warning: Could not cache vision mapping: %v", err)
	}

	// Now execute the standard dynamic handler using the newly generated visual mapping
	urlBeforeClick := page.URL()
	if err := handleDynamic(target, resumePath, coverPath, pii, mappingJSON, copilotMode, autoSubmitClick); err != nil {
		return err
	}

	// Bug #52 follow-up: this path used to return success straight from
	// handleDynamic's bare error value, with no confirmation evidence at
	// all -- confirm it the same way every other ATS path now does.
	return confirmOrError(page, companyName, urlBeforeClick, copilotMode, autoSubmitClick)
}
