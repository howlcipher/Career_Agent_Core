package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const SystemPrompt = "You are an expert technical recruiter. Analyze the job description and tailor the base resume and cover letter. Emphasize Python and Go automation tools, log parsing, anomaly detection, MS Cyber Defense coursework, CCNA foundation, and secure network infrastructure deployments. Use the heading Executive Summary. Do not hallucinate metrics. Write a three paragraph cover letter highlighting 9 plus years of IT and software experience. Output the resume in Markdown and the cover letter in plain text. Do not use hyphens."

type Client struct {
	APIKey string
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey: apiKey,
	}
}

func (c *Client) ScoreJob(scrapedData map[string]string, parsedDocument string) (int, error) {
	if c.APIKey == "" {
		return 0, fmt.Errorf("GEMINI_API_KEY is not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(c.APIKey))
	if err != nil {
		return 0, fmt.Errorf("failed to create gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-pro-latest")

	prompt := fmt.Sprintf("Analyze the following job description and my background. Return ONLY a single integer from 0 to 100 representing how good of a fit I am for this role. Do not include any other text.\n\nJob Title: %s\n\nJob Description: %s\n\nMy Background:\n%s",
		scrapedData["title"], scrapedData["desc"], parsedDocument)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return 0, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return 0, fmt.Errorf("empty response")
	}

	scoreStr := ""
	if text, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
		scoreStr = strings.TrimSpace(string(text))
	}

	// Remove any extraneous characters
	scoreStr = strings.Trim(scoreStr, " \n\r\t\"'")
	score, err := strconv.Atoi(scoreStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse score %q: %w", scoreStr, err)
	}

	return score, nil
}

func (c *Client) ProcessJobApplication(scrapedData map[string]string, profileConstraints map[string]interface{}, parsedDocument string) (string, string, string, error) {
	if c.APIKey == "" {
		return "", "", "", fmt.Errorf("GEMINI_API_KEY is not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(c.APIKey))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-pro-latest")

	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(SystemPrompt)},
	}

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

	fmt.Println("Sending application context to Gemini Pro...")
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate content from gemini: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", "", "", fmt.Errorf("empty response from gemini")
	}

	var fullText string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			fullText += string(text)
		}
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

// ExtractFormMapping uses Gemini to parse an unknown ATS DOM and generate a JSON mapping for Playwright
func (c *Client) ExtractFormMapping(domHTML string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY is not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(c.APIKey))
	if err != nil {
		return "", fmt.Errorf("failed to create gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-pro-latest")
	
	// Force JSON output
	model.ResponseMIMEType = "application/json"

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

Return a JSON object in this exact format:
{
  "fields": {
    "first_name": "selector",
    "last_name": "selector",
    ...
  }
}`
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemDirective)},
	}

	prompt := fmt.Sprintf("Analyze this DOM and extract the input selectors:\n\n%s", domHTML)
	
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("failed to generate form mapping: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from gemini for form mapping")
	}

	var jsonOutput string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			jsonOutput += string(text)
		}
	}

	return strings.TrimSpace(jsonOutput), nil
}

// GetEmbedding uses Gemini text-embedding-004 to create a vector for semantic search
func (c *Client) GetEmbedding(text string) ([]float32, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(c.APIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}
	defer client.Close()

	em := client.EmbeddingModel("embedding-001")
	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding: %w", err)
	}

	if res == nil || res.Embedding == nil {
		return nil, fmt.Errorf("empty embedding response")
	}

	return res.Embedding.Values, nil
}
