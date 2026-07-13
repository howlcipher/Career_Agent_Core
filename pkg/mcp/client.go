package mcp

import (
	"context"
	"fmt"
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

func (c *Client) ProcessJobApplication(scrapedData map[string]string, profileConstraints map[string]interface{}, parsedDocument string) (string, string, error) {
	if c.APIKey == "" {
		return "", "", fmt.Errorf("GEMINI_API_KEY is not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(c.APIKey))
	if err != nil {
		return "", "", fmt.Errorf("failed to create gemini client: %w", err)
	}
	defer client.Close()

	// Use Gemini Pro
	model := client.GenerativeModel("gemini-1.5-pro")

	// Set system instruction exactly as specified
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(SystemPrompt)},
	}

	// Craft the prompt and instruct how to separate the two files without using hyphens (as per SystemPrompt instructions)
	prompt := fmt.Sprintf("Job Title: %s\n\nJob Description: %s\n\nMy Background:\n%s\n\nPlease output the Markdown resume followed by exactly this separator on its own line: ===COVERLETTER===\nThen output the plain text cover letter below it.",
		scrapedData["title"], scrapedData["desc"], parsedDocument)

	fmt.Println("Sending application context to Gemini Pro...")
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", "", fmt.Errorf("failed to generate content from gemini: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", "", fmt.Errorf("empty response from gemini")
	}

	var fullText string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			fullText += string(text)
		}
	}

	// Parse the output using the separator
	parts := strings.Split(fullText, "===COVERLETTER===")
	if len(parts) < 2 {
		// Fallback if separator wasn't strictly followed
		return fullText, "Cover letter generation failed or merged into resume.", nil
	}

	resumeOutput := strings.TrimSpace(parts[0])
	coverLetterOutput := strings.TrimSpace(parts[1])

	return resumeOutput, coverLetterOutput, nil
}
