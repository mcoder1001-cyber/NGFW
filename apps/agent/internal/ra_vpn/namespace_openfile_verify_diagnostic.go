package ravpn

import (
	"context"
	"fmt"
)

// Only fixed boundaries and the four installation slots may enter the journal.
// No artifact, path, process identity or underlying error is a diagnostic input.
type numericPublisherProofReason uint8

const (
	numericPublisherProofEntry numericPublisherProofReason = iota + 1
	numericPublisherProofPreProcess
	numericPublisherProofHeldStat
	numericPublisherProofFreshOpen
	numericPublisherProofStamp
	numericPublisherProofClose
	numericPublisherProofPostProcess
	numericPublisherProofExit
	numericPublisherProofArtifactContext
)

func numericPublisherProofFailureLine(reason numericPublisherProofReason, artifact uint8, state error) string {
	if reason < numericPublisherProofEntry || reason > numericPublisherProofArtifactContext || state != nil && state != context.Canceled && state != context.DeadlineExceeded {
		return ""
	}
	switch reason {
	case numericPublisherProofHeldStat, numericPublisherProofFreshOpen, numericPublisherProofStamp, numericPublisherProofClose, numericPublisherProofArtifactContext:
		if artifact < 1 || artifact > 4 {
			return ""
		}
	default:
		if artifact != 0 {
			return ""
		}
	}
	return fmt.Sprintf("remote-access publisher-proof reason=%d artifact_index=%d deadline_exceeded=%t canceled=%t", reason, artifact, state == context.DeadlineExceeded, state == context.Canceled)
}
