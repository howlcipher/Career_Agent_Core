package answers

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// questionStopWords are the words that carry no distinguishing meaning in an
// application question. Dropping them makes "Have you ever been convicted of a
// felony?" and "Have you been convicted of a felony?" the same key, which is
// the point.
//
// The list is deliberately short and closed. Every word removed is a word two
// genuinely different questions can no longer be told apart by, so this errs
// toward keeping words: a key collision between two different questions would
// put the wrong answer on a real application, while a missed collision only
// costs the operator one approval that then becomes an alias forever.
var questionStopWords = map[string]bool{
	"a": true, "an": true, "the": true, "do": true, "does": true, "did": true,
	"you": true, "your": true, "yours": true, "are": true, "is": true,
	"was": true, "were": true, "be": true, "been": true, "will": true,
	"would": true, "please": true, "if": true, "of": true, "for": true,
	"to": true, "in": true, "on": true, "at": true, "we": true, "our": true,
	"this": true, "that": true, "and": true, "or": true,
	"currently": true, "ever": true, "have": true, "has": true, "had": true,
	"any": true, "there": true, "it": true, "as": true, "with": true,
}

// requiredMarkers are the decorations ATS forms bolt onto a label. They are
// presentation, not question text, and leaving them in would make the same
// question on two boards produce two keys.
var requiredMarkers = []string{
	"(required)", "(optional)", "required", "optional", "*",
}

// Normalize reduces an employer's question label to a stable comparison key.
//
// It is intentionally conservative: lowercase, strip markup and the required/
// optional decorations, reduce everything that is not a letter or digit to a
// space, drop the closed stop-word list above, and collapse whitespace. It does
// no stemming and no synonym expansion — those are the point at which "the same
// question" becomes a judgement call, and this project answers judgement calls
// with the curated pattern table (patterns.go) or with the operator, not with a
// heuristic that can silently equate two different attestations.
func Normalize(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = stripMarkup(text)
	for _, marker := range requiredMarkers {
		text = strings.ReplaceAll(text, marker, " ")
	}
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if questionStopWords[field] {
			continue
		}
		out = append(out, field)
	}
	return strings.Join(out, " ")
}

// stripMarkup removes anything between angle brackets. Labels are read from the
// accessible name in the browser and are normally plain text, but a label built
// from innerHTML can carry a stray tag, and a tag is never part of the
// question.
func stripMarkup(text string) string {
	var b strings.Builder
	depth := 0
	for _, r := range text {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
			b.WriteRune(' ')
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Tokens returns the normalized words of a question, for the pattern matcher.
func Tokens(text string) []string {
	normalized := Normalize(text)
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, " ")
}

// QuestionKey is the storage key for a question. It is the normalized text
// when that is short enough to keep readable in the database, and a hash
// otherwise — a long free-text prompt would otherwise make the key column
// unbounded, and the readable prefix is kept so a human reading the table can
// still tell rows apart.
func QuestionKey(prompt string) string {
	normalized := Normalize(prompt)
	if normalized == "" {
		return ""
	}
	const maxReadableKey = 160
	if len(normalized) <= maxReadableKey {
		return normalized
	}
	sum := sha256.Sum256([]byte(normalized))
	return normalized[:maxReadableKey] + "#" + hex.EncodeToString(sum[:8])
}

// normalizeCompany reduces an employer name to a scope-stable form, so
// "Grafana Labs, Inc." and "Grafana Labs" are one company. It mirrors the
// intent of the assisted queue's own duplicate normalization rather than
// sharing code with it: that one compares postings, this one keys answers, and
// tying them together would make a change to either silently move the other.
func normalizeCompany(company string) string {
	company = strings.ToLower(strings.TrimSpace(company))
	fields := strings.FieldsFunc(company, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	suffixes := map[string]bool{
		"inc": true, "llc": true, "ltd": true, "limited": true, "corp": true,
		"corporation": true, "co": true, "gmbh": true, "plc": true, "sa": true,
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if suffixes[field] {
			continue
		}
		out = append(out, field)
	}
	return strings.Join(out, " ")
}
