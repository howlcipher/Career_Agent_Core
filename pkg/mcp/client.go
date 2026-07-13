package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const SystemPrompt = `You are an elite technical recruiter and stealth writer. Your primary directive is to completely bypass AI detection and sound 100% human. 
Analyze the job description and tailor the base resume and cover letter. Emphasize Python and Go automation tools, log parsing, anomaly detection, MS Cyber Defense coursework, CCNA foundation, and secure network infrastructure deployments. 
Use the heading Executive Summary in the resume. Do not hallucinate metrics. 
Write a three-paragraph cover letter highlighting 9+ years of IT and software experience. 
STEALTH WRITING RULES:
- NEVER use typical AI buzzwords (e.g., delve, tapestry, testament, beacon, thrilled, dynamic, seamless, paradigm, leverage, synergy).
- Use varied sentence lengths. Mix short, punchy sentences with longer, complex ones.
- Maintain a conversational, direct, and professional tone. Avoid overly corporate jargon.
- Write in active voice. 
- Output the resume in Markdown and the cover letter in plain text. Do not use hyphens.`

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

	model := client.GenerativeModel("gemini-1.5-pro")

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

	model := client.GenerativeModel("gemini-1.5-pro")

	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(SystemPrompt)},
	}

	toneContext := ""
	if tone, ok := profileConstraints["cover_letter_tone"].(string); ok && tone != "" {
		toneContext = fmt.Sprintf("\n\nCRITICAL DIRECTIVE: You must strictly adhere to this exact tone for the cover letter: %s", tone)
	}

	prompt := fmt.Sprintf("Job Title: %s\n\nJob Description: %s\n\nMy Background:\n%s%s\n\nPlease output the Markdown resume followed by exactly this separator on its own line: ===COVERLETTER===\nThen output the plain text cover letter below it, followed by exactly this separator on its own line: ===INTERVIEWPREP===\nThen output a cheat sheet of likely interview questions and talking points based on my profile.",
		scrapedData["title"], scrapedData["desc"], parsedDocument, toneContext)

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
