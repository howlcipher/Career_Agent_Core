package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
)

const SystemPrompt = "You are an expert technical recruiter. Analyze the job description and tailor the base resume and cover letter. Emphasize Python and Go automation tools, log parsing, anomaly detection, MS Cyber Defense coursework, CCNA foundation, and secure network infrastructure deployments. Use the heading Executive Summary. Do not hallucinate metrics. Write a three paragraph cover letter highlighting 9 plus years of IT and software experience. Output the resume in Markdown and the cover letter in plain text. Do not use hyphens."

var apiCallCount uint64

func incrementAndLogAPICall(callType string, payloadLen int) error {
	count := atomic.AddUint64(&apiCallCount, 1)
	log.Printf("[API Metrics] %s API Call #%d executed. Payload length: %d characters.", callType, count, payloadLen)

	if payloadLen > 50000 {
		return fmt.Errorf("CIRCUIT BREAKER TRIGGERED: Payload size %d exceeds safety limit (50k chars). Aborting to prevent runaway LLM costs.", payloadLen)
	}
	return nil
}

// Client routes all LLM calls through a configurable backend.
// The backend is selected via the LLM_PROVIDER environment variable:
// "ollama" (default, local), "claude", or "gemini".
type Client struct {
	// APIKey is the Gemini API key, kept for backward compatibility.
	// Only used when LLM_PROVIDER=gemini.
	APIKey   string
	provider provider
}

func NewClient(apiKey string) *Client {
	p := newProviderFromEnv(apiKey)
	log.Printf("[LLM] Using provider: %s", p.Name())
	return &Client{
		APIKey:   apiKey,
		provider: p,
	}
}

// generate runs a single generation request against the configured provider
// with the provider's own timeout.
func (c *Client) generate(req genRequest) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.provider.Timeout())
	defer cancel()
	return c.provider.Generate(ctx, req)
}

func (c *Client) ScoreJob(scrapedData map[string]string, profileConstraints map[string]interface{}, parsedDocument string) (int, error) {
	prompt := fmt.Sprintf(`Analyze the following job description against my background and constraints.
Return ONLY a single integer from 0 to 100 representing how good of a fit I am. Do not include any other text.

SCORING RUBRIC:
1. Start at a baseline of 80.
2. If "Remote Only" is true and the job requires on-site/hybrid, deduct 80 points.
3. If the job explicitly states a salary below my salary floor, deduct 30 points.
4. Be tech-stack agnostic. Do NOT deduct points if I am missing a specific language/framework (e.g. JS, AWS) but have strong experience in adjacent technologies (e.g. Python/Go, GCP). Assume a senior engineer can easily learn equivalent tools.
5. Deduct 15 points ONLY if I am entirely missing a core domain (e.g., job requires deep Machine Learning or Mobile App Dev, and I have zero background in that domain).
6. Add 10-20 points if my background perfectly aligns with the core domain.

MY CONSTRAINTS:
- Remote Only: %v
- Salary Floor: %v

Job Title: %s
Job Description: %s

My Background:
%s`,
		profileConstraints["remote_only"], profileConstraints["salary_floor"], scrapedData["title"], scrapedData["desc"], parsedDocument)

	if err := incrementAndLogAPICall("ScoreJob", len(prompt)); err != nil {
		return 0, err
	}

	// Lower the temperature so the model is strictly analytical rather than creative when scoring
	raw, err := c.generate(genRequest{prompt: prompt, temperature: 0.1})
	if err != nil {
		return 0, fmt.Errorf("failed to generate content: %w", err)
	}

	scoreStr := strings.Trim(strings.TrimSpace(raw), " \n\r\t\"'")
	score, err := strconv.Atoi(scoreStr)
	if err != nil {
		// Smaller local models sometimes wrap the number in prose despite
		// instructions; salvage the first integer in the response.
		if n, ok := firstInt(scoreStr); ok {
			return n, nil
		}
		return 0, fmt.Errorf("failed to parse score %q: %w", scoreStr, err)
	}

	return score, nil
}

// firstInt returns the first run of digits in s as an integer.
func firstInt(s string) (int, bool) {
	start := -1
	for i, r := range s {
		if unicode.IsDigit(r) {
			if start == -1 {
				start = i
			}
		} else if start != -1 {
			n, err := strconv.Atoi(s[start:i])
			return n, err == nil
		}
	}
	if start != -1 {
		n, err := strconv.Atoi(s[start:])
		return n, err == nil
	}
	return 0, false
}

func (c *Client) ProcessJobApplication(scrapedData map[string]string, profileConstraints map[string]interface{}, parsedDocument string) (string, string, string, error) {
	toneContext := ""
	if tone, ok := profileConstraints["cover_letter_tone"].(string); ok && tone != "" {
		toneContext = fmt.Sprintf("\n\nCRITICAL DIRECTIVE: You must strictly adhere to this exact tone for the cover letter: %s", tone)
	}

	compContext := ""
	if comp, ok := profileConstraints["target_compensation"].(int); ok && comp > 0 {
		compContext = fmt.Sprintf("\n\nNOTE: If a desired salary or target compensation is requested, explicitly state it as $%d.", comp)
	}

	prompt := fmt.Sprintf("Job Title: %s\n\nJob Description: %s\n\nMy Background:\n%s%s%s\n\nPlease output the Markdown resume followed by exactly this separator on its own line: ===COVERLETTER===\nThen output the plain text cover letter below it, followed by exactly this separator on its own line: ===INTERVIEWPREP===\nThen output a cheat sheet of likely interview questions and talking points based on my profile.",
		scrapedData["title"], scrapedData["desc"], parsedDocument, toneContext, compContext)

	fmt.Printf("Sending application context to %s...\n", c.provider.Name())
	if err := incrementAndLogAPICall("ProcessJobApplication", len(prompt)); err != nil {
		return "", "", "", err
	}

	fullText, err := c.generate(genRequest{system: SystemPrompt, prompt: prompt, temperature: -1})
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate content: %w", err)
	}

	// Parse the output using the separators
	resumeOut := ""
	coverOut := ""
	prepOut := "Interview prep failed to generate properly."

	parts1 := strings.Split(fullText, "===COVERLETTER===")
	if len(parts1) > 0 {
		resumeOut = strings.TrimSpace(parts1[0])
	}
	if len(parts1) > 1 {
		parts2 := strings.Split(parts1[1], "===INTERVIEWPREP===")
		if len(parts2) > 0 {
			coverOut = strings.TrimSpace(parts2[0])
		}
		if len(parts2) > 1 {
			prepOut = strings.TrimSpace(parts2[1])
		}
	}

	return resumeOut, coverOut, prepOut, nil
}

// ExtractFormMapping parses an unknown ATS DOM and generates a JSON mapping for Playwright
func (c *Client) ExtractFormMapping(domHTML string) (string, error) {
	systemDirective := `You are an expert web scraper and DOM analyst. You will be provided with the HTML source of a job application form.
Your task is to identify the precise CSS selectors needed by Playwright to fill out this form.
Map the following logical fields to their corresponding CSS selectors (prefer id, name, or specific data-qa attributes):
- first_name
- last_name
- email
- phone
- resume
- cover_letter
- submit_button

Also identify each field's visible accessible label text where one exists - the text of an associated <label> element, or an aria-label/aria-labelledby value (e.g. "First Name", "Email Address"). This is used as a fallback if the CSS selector guess turns out to be wrong, so include it whenever the form has one, even if you are confident in the selector.

Return a JSON object in this exact format:
{
  "fields": {
    "first_name": "selector",
    "last_name": "selector",
    ...
  },
  "labels": {
    "first_name": "visible label text, or omit if none exists",
    "last_name": "visible label text, or omit if none exists",
    ...
  }
}`

	prompt := fmt.Sprintf("Analyze this DOM and extract the input selectors:\n\n%s", domHTML)

	if err := incrementAndLogAPICall("ExtractFormMapping", len(prompt)); err != nil {
		return "", err
	}

	raw, err := c.generate(genRequest{system: systemDirective, prompt: prompt, json: true, temperature: -1})
	if err != nil {
		return "", fmt.Errorf("failed to generate form mapping: %w", err)
	}

	return stripJSONFences(raw), nil
}

// ExtractFormMappingVision visually analyzes a screenshot of an ATS form
// and generates a JSON mapping for Playwright, bypassing HTML DOM obfuscation entirely.
func (c *Client) ExtractFormMappingVision(screenshotBytes []byte) (string, error) {
	systemDirective := `You are an expert autonomous web automation agent. You will be provided with a screenshot of a job application form.
Your task is to identify the precise CSS selectors or coordinates needed by Playwright to fill out this form.
Map the following logical fields to their corresponding CSS selectors (if visible in standard structural layout) or describe the input placeholder text:
- first_name
- last_name
- email
- phone
- resume
- cover_letter
- submit_button

Also read the visible label text printed next to or above each input field (e.g. "First Name", "Email Address") and include it separately. This is used as a fallback if the CSS selector guess turns out to be wrong, so include it whenever you can read one, even if you are confident in the selector.

Return a JSON object in this exact format:
{
  "fields": {
    "first_name": "selector",
    "last_name": "selector",
    ...
  },
  "labels": {
    "first_name": "visible label text, or omit if none is visible",
    "last_name": "visible label text, or omit if none is visible",
    ...
  }
}`

	prompt := "Analyze this screenshot and extract the input selectors based on visual placement and placeholders:"

	if err := incrementAndLogAPICall("ExtractFormMappingVision", len(prompt)); err != nil {
		return "", err
	}

	raw, err := c.generate(genRequest{system: systemDirective, prompt: prompt, json: true, temperature: -1, imagePNG: screenshotBytes})
	if err != nil {
		return "", fmt.Errorf("failed to generate form mapping from vision: %w", err)
	}

	return stripJSONFences(raw), nil
}

// GetEmbedding creates a vector for semantic search using the configured
// embedding backend (Ollama by default; text-embedding-004 on Gemini).
func (c *Client) GetEmbedding(text string) ([]float32, error) {
	if err := incrementAndLogAPICall("GetEmbedding", len(text)); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.provider.Timeout())
	defer cancel()
	return c.provider.Embed(ctx, text)
}

// ExtractRejectionReason reads an HR rejection email and explicitly figures out why the candidate was rejected.
func (c *Client) ExtractRejectionReason(emailText string) (string, error) {
	if err := incrementAndLogAPICall("AnalyzeEmail", len(emailText)); err != nil {
		return "", err
	}

	system := "You are an HR analytics expert. Analyze this rejection email and concisely state WHY the candidate was rejected (e.g., 'Not enough Kubernetes experience', 'Role was canceled', 'Timezone mismatch', or 'Generic templated rejection')."
	raw, err := c.generate(genRequest{system: system, prompt: emailText, temperature: -1})
	if err != nil {
		return "", fmt.Errorf("failed to extract rejection reason: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return "Generic templated rejection (no specific reason provided)", nil
	}

	return raw, nil
}

// SolveValidationErrors analyzes a failed form submission and generates values for missing required fields
func (c *Client) SolveValidationErrors(domHTML string, profileContext string) (map[string]string, error) {
	systemDirective := `You are an expert web scraper and DOM analyst. You are provided with the HTML source of a job application form that just FAILED validation (required fields are missing or invalid).
You are also provided with the applicant's profile context.
Your task is to identify ALL the missing or invalid fields in the form (like custom questions, URLs, visa status, etc.), determine the correct CSS selector for each, and generate the appropriate string value to fill them in based on the applicant's profile.

CRITICAL RULE: Never invent a value for any field asking about race, ethnicity, gender, sex, veteran/military status, disability status, sexual orientation, or any other legally sensitive demographic/EEO category. Only answer such a field using an exact value given in the "EEO / voluntary self-identification answers" section of the profile context. If that section says a category was not provided, you MUST select or type its decline option (e.g. "Decline to answer", "Prefer not to say", "I don't wish to answer") instead of guessing. This rule overrides the general instruction to fill in every field.

Return a JSON object in this exact format mapping the CSS selector to the string value to fill:
{
  "selector_1": "value_1",
  "selector_2": "value_2"
}`

	prompt := fmt.Sprintf("Applicant Profile:\n%s\n\nFailed Form DOM:\n%s", profileContext, domHTML)

	if err := incrementAndLogAPICall("SolveValidationErrors", len(prompt)); err != nil {
		return nil, err
	}

	raw, err := c.generate(genRequest{system: systemDirective, prompt: prompt, json: true, temperature: -1})
	if err != nil {
		return nil, fmt.Errorf("failed to solve validation errors: %w", err)
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(stripJSONFences(raw)), &result); err != nil {
		return nil, fmt.Errorf("failed to parse json response: %w", err)
	}

	return result, nil
}
