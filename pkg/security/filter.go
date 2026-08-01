package security

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/danielthedm/promptsec"
)

var ErrPromptInjectionDetected = errors.New("prompt injection detected")

// PromptInjectionError identifies content rejected by the deterministic
// quarantine boundary while retaining the scanner findings for audit logging.
// Error deliberately omits the matched content so callers cannot accidentally
// place attacker-controlled instructions into another model prompt or log.
type PromptInjectionError struct {
	Threats []promptsec.Threat
}

func (e *PromptInjectionError) Error() string {
	return "untrusted content quarantined: prompt injection detected"
}

func (e *PromptInjectionError) Unwrap() error {
	return ErrPromptInjectionDetected
}

type QuarantineLayer struct {
	Protector *promptsec.Protector
}

func NewQuarantineLayer() *QuarantineLayer {
	return &QuarantineLayer{
		Protector: promptsec.Moderate(),
	}
}

// QuarantinePayload is the single deterministic pre-model boundary for
// untrusted posting text and page DOM. A nil layer remains a no-op for callers
// that explicitly run without a filter, matching the package's existing test
// and dependency-injection behavior.
func (q *QuarantineLayer) QuarantinePayload(text string) error {
	if q == nil || q.Protector == nil {
		return nil
	}
	result := q.Protector.Analyze(text)
	if result.Safe {
		return nil
	}
	if overrideZeroEvidenceDetection(text, result.Threats) {
		return nil
	}
	return &PromptInjectionError{
		Threats: append([]promptsec.Threat(nil), result.Threats...),
	}
}

func (q *QuarantineLayer) CheckPayload(text string) error {
	return q.QuarantinePayload(text)
}

// CheckPayloadDetailed is CheckPayload plus the raw list of detected threats,
// for callers that need to record what was found (e.g. logging a
// prompt-injection attempt to a CSV for later review) rather than just an
// error string.
func (q *QuarantineLayer) CheckPayloadDetailed(text string) (safe bool, threats []promptsec.Threat, err error) {
	err = q.QuarantinePayload(text)
	if err == nil {
		return true, nil, nil
	}
	var detection *PromptInjectionError
	if errors.As(err, &detection) {
		return false, detection.Threats, err
	}
	return false, nil, fmt.Errorf("quarantine payload: %w", err)
}

// ── zero-evidence second pass (bugs.md #489) ────────────────────────────────
//
// Bug #489: `QUARANTINED_PROMPT_INJECTION` was 51.0% of every `job_funnel` row
// ever written — the single largest terminal status, concentrated on
// `jobs.lever.co` and the `greenhouse.io` family, the two ATS platforms with
// working auto-submit handlers. 88.5% of the logged detections carried no
// `matched_text` at all (99.3% of `instruction_override`, 100% of
// `system_prompt_leak`).
//
// Root cause, traced through the vendored `promptsec@v0.1.0` source: the
// heuristic guard's `Execute` runs three separate stages, and only the first
// is governed by the `Preset`/`Threshold` dial bug #394 turned down.
//
//  1. `patterns.go` — compiled regexes, filtered by `Threshold`, and every hit
//     records a real located span via `FindStringIndex` (`Match`/`Start`/`End`).
//  2. `encoding.go` — `detectEncodingAttacks`, unfiltered. Most branches set a
//     located `Match`; the zero-width and homoglyph branches do not, but they
//     report `ThreatEncodingAttack`, not the two categories below.
//  3. `contextual.go` (`detectContextualAttacks`) plus the inline
//     `fuzzyMatchNormalized` branch — both run unconditionally, are NOT
//     filtered by `Preset`/`Threshold` at all, and by construction never set
//     `Match`/`Start`/`End`, because they are keyword co-occurrence tests over
//     the whole normalized input rather than a single located span.
//
// Stage 3 is the entire false-positive population. Its
// `ThreatSystemPromptLeak` "coercive attempt to extract sensitive data" branch
// fires on bare phrases like "personal data", "social security number",
// "login credentials", "credit card information" and "financial records" —
// i.e. on ordinary privacy notices, background-check disclosures and voluntary
// self-identification sections. Its "attempt to reveal or repurpose prompt
// content" branch fires on "instructions provided" / "guidelines provided",
// which is how every ATS phrases its own application steps. Its fuzzy branch
// fires at severity 0.65 whenever two of eleven "critical keywords" match
// within an edit distance — and "assistance" is within edit distance 1 of
// "assistant" while "systems"/"instructions" match outright, so essentially
// every real posting trips it.
//
// `HeuristicOptions` exposes one `Threshold`/`Preset` for the whole guard and
// it does not even reach stages 2 and 3, so there is no upstream per-category
// knob to turn; the correction has to be a local second pass. This is that
// second pass. It only ever fires when EVERY decisive threat is a stage-3
// zero-evidence `instruction_override`/`system_prompt_leak` hit AND the payload
// reads like real ATS boilerplate AND it carries no unambiguous injection
// marker. Anything with a located `Match`, anything from another guard, and
// any other threat category still quarantines exactly as before.

// decisiveSeverity mirrors `promptsec.Protector`'s own (unexported) default
// threshold: `buildResult` marks a result unsafe only when some threat has
// `Severity >= 0.5`. Threats below it never caused a quarantine on their own,
// so they are not evidence for or against overriding one. This matters in
// practice because `Moderate()`'s sanitizer emits a 0.3 "zero-width characters
// were stripped" threat with an empty `Match`, and real posting DOM is full of
// zero-width characters; counting that as corroboration would silently disable
// this whole second pass. It does not weaken the sanitizer: a 0.3 sanitizer
// threat never quarantined anything by itself either.
const decisiveSeverity = 0.5

// minBenignSignatures is how many distinct allowlist signatures a payload must
// carry before a zero-evidence detection is overridden. One is too easy for an
// attacker to bolt onto a hostile payload; real Lever/Greenhouse postings carry
// several, because the payload handed to this layer is the whole posting
// (title + description + raw HTML) or the whole pruned career-page DOM, not a
// single sentence.
const minBenignSignatures = 2

// benignATSSignatures are phrases that only appear in genuine applicant
// tracking system output: EEO statements, ADA/accommodation language,
// background-check and work-authorization disclosures, voluntary
// self-identification sections, candidate privacy notices, and standard
// application-process instructions. They are written in the normalized form
// produced by normalizeForSignatures (lower case, every run of non-alphanumeric
// characters collapsed to one space) so that HTML tags, punctuation and
// hyphenation between the words do not defeat the match — "Equal
// <b>Opportunity</b> Employer" and "E-Verify" normalize to "equal opportunity
// employer" and "e verify".
//
// No entry may be a substring of another (enforced by a test): the count these
// produce is compared against minBenignSignatures, so a subsumed pair like
// "protected veteran"/"veteran status" would let a single phrase satisfy a
// threshold that is meant to require two independent ones.
var benignATSSignatures = []string{
	// Equal employment opportunity / non-discrimination.
	"equal opportunity employer",
	"equal employment opportunity",
	"without regard to race",
	"regardless of race",
	"qualified applicants will receive consideration",
	"affirmative action",
	"protected veteran",

	// Accessibility and reasonable accommodation.
	"reasonable accommodation",
	"americans with disabilities act",
	"accommodation to complete",
	"accommodation during the application",
	"need assistance or an accommodation",
	"alternative method for applying",

	// Background check and work authorization.
	"background check",
	"e verify",
	"employment eligibility",
	"form i 9",
	"authorized to work in the united states",
	"work authorization",
	"offer of employment is contingent",
	"drug screen",

	// Voluntary self-identification.
	"voluntary self identification",
	"invitation to self identify",
	"completion of this form is voluntary",
	"will not be used in the hiring decision",
	"gender identity",
	"race ethnicity",
	"disability status",

	// Candidate privacy and data protection notices.
	"privacy policy",
	"privacy notice",
	"data protection",
	"we will never ask you for",
	"applicant tracking system",

	// Application process and posting structure.
	"application process",
	"submit your application",
	"submitting your application",
	"how to apply",
	"attach your resume",
	"upload your resume",
	"cover letter",
	"follow up email",
	"confirmation email",
	"hiring process",
	"hiring team",
	"job description",
	"employment type",
	"salary range",
	"pay range",
	"benefits package",
}

// injectionMarkers are phrasings no legitimate job posting needs and every
// serious prompt-injection payload uses. Their presence vetoes the override
// outright, ahead of any allowlist match, so that appending an EEO statement to
// a hostile payload does not buy it a pass. Most of these are already caught
// with a located `Match` by `patterns.go`; this list is the backstop for the
// variants that only reach the zero-evidence contextual branches. Deliberately
// excluded: "login credentials", "personal data", "credit card information"
// and similar — those are the exact phrases legitimate anti-fraud and privacy
// notices use, and they are what caused this bug in the first place. Also
// deliberately excluded: "language model" and "content policy", which appear in
// a great many genuine 2026 engineering and trust-and-safety postings, and
// "do not follow", because ATS text routinely says "please do not follow up by
// phone" (`patterns.go` still catches "do not follow your instructions" with a
// located match).
var injectionMarkers = []string{
	"ignore all",
	"ignore previous",
	"ignore the above",
	"ignore everything",
	"ignore your",
	"disregard all",
	"disregard previous",
	"disregard the above",
	"forget everything",
	"forget all",
	"forget your",
	"previous instructions",
	"prior instructions",
	"above instructions",
	"earlier instructions",
	"new instructions",
	"your instructions are",
	"system prompt",
	"initial prompt",
	"original prompt",
	"prompt template",
	"end of prompt",
	"new prompt",
	"you are now",
	"now you are",
	"act as if",
	"pretend you",
	"pretend to be",
	"roleplay as",
	"assume the role",
	"developer mode",
	"jailbreak",
	"no restrictions",
	"without restrictions",
	"ai language model",
	"reveal your",
	"provide me with",
	"give me access to",
	"or i will",
	"must obey",
	"execute the following command",
}

// isZeroEvidenceThreat reports whether a threat came from `promptsec`'s
// unlocated heuristic co-occurrence paths (contextual.go / the inline fuzzy
// branch) in one of the two categories bug #489's audit implicated. A threat
// from any other guard, or any heuristic threat that recorded a real located
// span, is corroborated evidence and is never eligible for override.
func isZeroEvidenceThreat(t promptsec.Threat) bool {
	if t.Guard != "heuristic" || t.Match != "" {
		return false
	}
	return t.Type == promptsec.ThreatInstructionOverride ||
		t.Type == promptsec.ThreatSystemPromptLeak
}

// overrideZeroEvidenceDetection decides whether an unsafe `promptsec` result is
// a bug #489 false positive that should be released. It returns true only when
// all three of the following hold, and errs toward staying quarantined
// otherwise:
//
//  1. every decisive threat (severity >= decisiveSeverity) is zero-evidence —
//     one located match, one non-heuristic guard hit, or one threat of any
//     other category (role_manipulation, delimiter_injection, encoding_attack)
//     is enough to keep the payload quarantined;
//  2. the payload carries no injectionMarkers phrase;
//  3. the payload carries at least minBenignSignatures distinct
//     benignATSSignatures phrases.
func overrideZeroEvidenceDetection(text string, threats []promptsec.Threat) bool {
	decisive := 0
	types := make([]string, 0, len(threats))
	for _, t := range threats {
		if t.Severity < decisiveSeverity {
			continue
		}
		if !isZeroEvidenceThreat(t) {
			return false
		}
		decisive++
		types = append(types, string(t.Type))
	}
	// Defensive: an unsafe result always has at least one decisive threat, but
	// never release a payload on an empty set.
	if decisive == 0 {
		return false
	}

	normalized := normalizeForSignatures(text)
	for _, marker := range injectionMarkers {
		if strings.Contains(normalized, marker) {
			return false
		}
	}

	matched := countBenignSignatures(normalized)
	if matched < minBenignSignatures {
		return false
	}

	// Content-free visibility. Overridden payloads never reach
	// LogPromptInjectionDetections (that CSV is written by the callers only on
	// the unsafe path), so without this line the override rate would be
	// unmeasurable and #489's acceptance criteria could not be re-verified
	// live. Nothing here echoes the payload: the threat types are fixed
	// enumeration values and, by definition of the zero-evidence path, there is
	// no matched text to leak.
	sort.Strings(types)
	log.Printf(
		"[Quarantine] Released zero-evidence detection: %d threat(s) [%s], "+
			"%d benign ATS signature(s) matched (bugs.md #489)",
		decisive, strings.Join(types, ","), matched,
	)
	return true
}

// countBenignSignatures counts how many distinct allowlist phrases the
// normalized payload contains.
func countBenignSignatures(normalized string) int {
	count := 0
	for _, sig := range benignATSSignatures {
		if strings.Contains(normalized, sig) {
			count++
		}
	}
	return count
}

// htmlNoise matches HTML tags and character entities. Both are dropped before
// signature matching because the payload this layer receives in production is
// posting title + description + raw HTML (cmd/agent/main.go), so a phrase like
// "Equal <b>Opportunity</b> Employer" or "Equal&nbsp;Opportunity" would
// otherwise normalize with "b" and "nbsp" wedged between the words. Dropping
// them helps injectionMarkers as much as it helps the allowlist: "ig<i>nore
// all" collapses back to "ignore all".
var htmlNoise = regexp.MustCompile(`(?s)<[^<>]*>|&[a-zA-Z][a-zA-Z0-9]{1,9};|&#[0-9]{1,7};`)

// normalizeForSignatures strips HTML noise, lower-cases the payload and
// collapses every run of non-alphanumeric characters into a single space, so
// signature and marker phrases match across markup, punctuation, hyphenation
// and arbitrary whitespace. The result is padded with a leading and trailing
// space so callers could anchor on word boundaries if that ever becomes
// necessary.
func normalizeForSignatures(text string) string {
	text = htmlNoise.ReplaceAllString(text, " ")

	var b strings.Builder
	b.Grow(len(text) + 2)
	b.WriteByte(' ')
	pendingSpace := false
	wroteWord := false
	for _, r := range text {
		lr := unicode.ToLower(r)
		if unicode.IsLetter(lr) || unicode.IsDigit(lr) {
			if pendingSpace && wroteWord {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(lr)
			wroteWord = true
			continue
		}
		pendingSpace = true
	}
	b.WriteByte(' ')
	return b.String()
}
