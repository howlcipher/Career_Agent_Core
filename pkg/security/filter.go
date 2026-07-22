package security

import (
	"fmt"

	"github.com/danielthedm/promptsec"
)

type QuarantineLayer struct {
	Protector *promptsec.Protector
}

func NewQuarantineLayer() *QuarantineLayer {
	return &QuarantineLayer{
		Protector: promptsec.Strict(),
	}
}

func (q *QuarantineLayer) CheckPayload(text string) error {
	result := q.Protector.Analyze(text)
	if !result.Safe {
		return fmt.Errorf("indirect prompt injection attack detected: %v", result.Threats)
	}
	return nil
}

// CheckPayloadDetailed is CheckPayload plus the raw list of detected threats,
// for callers that need to record what was found (e.g. logging a
// prompt-injection attempt to a CSV for later review) rather than just an
// error string.
func (q *QuarantineLayer) CheckPayloadDetailed(text string) (safe bool, threats []promptsec.Threat, err error) {
	result := q.Protector.Analyze(text)
	if !result.Safe {
		return false, result.Threats, fmt.Errorf("indirect prompt injection attack detected: %v", result.Threats)
	}
	return true, nil, nil
}
