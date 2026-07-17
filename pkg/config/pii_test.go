package config

import (
	"os"
	"testing"
)

func TestLoadPII(t *testing.T) {
	yamlData := `
first_name: "John"
last_name: "Doe"
email: "john.doe@example.com"
phone: "+1234567890"
dob: "1990-01-01"
address: "123 Main St"
`
	tmpFile, err := os.CreateTemp("", "pii_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(yamlData)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	pii, err := LoadPII(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadPII failed: %v", err)
	}

	if pii.FirstName != "John" {
		t.Errorf("Expected FirstName 'John', got '%s'", pii.FirstName)
	}
	if pii.LastName != "Doe" {
		t.Errorf("Expected LastName 'Doe', got '%s'", pii.LastName)
	}
	if pii.Email != "john.doe@example.com" {
		t.Errorf("Expected Email 'john.doe@example.com', got '%s'", pii.Email)
	}
	if pii.Phone != "+1234567890" {
		t.Errorf("Expected Phone '+1234567890', got '%s'", pii.Phone)
	}
	if pii.DOB != "1990-01-01" {
		t.Errorf("Expected DOB '1990-01-01', got '%s'", pii.DOB)
	}
	if pii.Address != "123 Main St" {
		t.Errorf("Expected Address '123 Main St', got '%s'", pii.Address)
	}
}

func TestLoadPII_InvalidFile(t *testing.T) {
	_, err := LoadPII("non_existent_file.yaml")
	if err == nil {
		t.Errorf("Expected error for non-existent file, got nil")
	}
}

func TestLoadPII_InvalidYAML(t *testing.T) {
	yamlData := `first_name: "John"
	invalid_yaml_here
	`
	tmpFile, err := os.CreateTemp("", "pii_invalid_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write([]byte(yamlData))
	tmpFile.Close()

	_, err = LoadPII(tmpFile.Name())
	if err == nil {
		t.Errorf("Expected error for invalid yaml, got nil")
	}
}
