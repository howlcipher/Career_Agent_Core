package mcp

import (
	"fmt"
)

const SystemPrompt = "You are an expert technical recruiter. Analyze the job description and tailor the base resume and cover letter. Emphasize Python and Go automation tools, log parsing, anomaly detection, MS Cyber Defense coursework, CCNA foundation, and secure network infrastructure deployments. Use the heading Executive Summary. Do not hallucinate metrics. Write a three paragraph cover letter highlighting 9 plus years of IT and software experience. Output the resume in Markdown and the cover letter in plain text. Do not use hyphens."

type Client struct {
	Endpoint string
}

func NewClient(endpoint string) *Client {
	return &Client{
		Endpoint: endpoint,
	}
}

func (c *Client) ProcessJobApplication(scrapedData map[string]string, profileConstraints map[string]interface{}, parsedDocument string) (string, string, error) {
	fmt.Println("Connecting to local server...")
	fmt.Println("System Prompt Loaded:", SystemPrompt)
	fmt.Println("Merging scraped data, profile constraints, and parsed documents.")
	
	resumeOutput := "# Executive Summary\n\nExperienced Senior Backend Engineer..."
	coverLetterOutput := "Dear Hiring Manager,\n\nI am writing to apply...\n\nI have extensive experience...\n\nThank you for considering my application."
	
	return resumeOutput, coverLetterOutput, nil
}
