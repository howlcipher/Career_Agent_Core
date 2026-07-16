package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type PII struct {
	FirstName string `yaml:"first_name"`
	LastName  string `yaml:"last_name"`
	Email     string `yaml:"email"`
	Phone     string `yaml:"phone"`
	DOB       string `yaml:"dob"`
	Address   string `yaml:"address"`
}

func LoadPII(path string) (*PII, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read PII file: %w", err)
	}

	var p PII
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse PII file: %w", err)
	}

	return &p, nil
}
