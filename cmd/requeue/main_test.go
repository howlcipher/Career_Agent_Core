package main

import "testing"

func TestResolvePatterns_NamedSources(t *testing.T) {
	patterns, err := resolvePatterns("lever,greenhouse", "")
	if err != nil {
		t.Fatalf("resolvePatterns failed: %v", err)
	}
	if len(patterns) != 2 || patterns["lever"] != "%lever.co%" || patterns["greenhouse"] != "%greenhouse%" {
		t.Errorf("unexpected patterns: %+v", patterns)
	}
}

func TestResolvePatterns_UnknownSource(t *testing.T) {
	_, err := resolvePatterns("not-a-real-source", "")
	if err == nil {
		t.Error("expected an error for an unknown source name")
	}
}

func TestResolvePatterns_RawPattern(t *testing.T) {
	patterns, err := resolvePatterns("", "%example.com%")
	if err != nil {
		t.Fatalf("resolvePatterns failed: %v", err)
	}
	if len(patterns) != 1 || patterns["custom"] != "%example.com%" {
		t.Errorf("unexpected patterns: %+v", patterns)
	}
}

func TestResolvePatterns_NeitherGiven(t *testing.T) {
	patterns, err := resolvePatterns("", "")
	if err != nil {
		t.Fatalf("resolvePatterns failed: %v", err)
	}
	if len(patterns) != len(sourcePatterns) {
		t.Errorf("expected all %d known sources when neither -source nor -pattern is given, got %d", len(sourcePatterns), len(patterns))
	}
}
