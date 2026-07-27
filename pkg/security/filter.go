package security

import (
	"errors"
	"fmt"

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
		Protector: promptsec.Strict(),
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
	if !result.Safe {
		return &PromptInjectionError{
			Threats: append([]promptsec.Threat(nil), result.Threats...),
		}
	}
	return nil
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
