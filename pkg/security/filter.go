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
