package ravpn

import (
	"context"
	"fmt"
	"time"
)

// numericPublisherCheckpointLine accepts only closed numeric progress and
// context state. It has no error/property/identity input to expose in journals.
func numericPublisherCheckpointLine(ctx context.Context, kind, checkpoint uint8, elapsed time.Duration, failed bool) string {
	if !failed || ctx == nil {
		return ""
	}
	state := numericPublisherDiagnosticState{captured: true, state: ctx.Err()}
	return state.line(kind, checkpoint, elapsed, failed)
}

type numericPublisherDiagnosticState struct {
	captured bool
	state    error
}

func (state *numericPublisherDiagnosticState) captureBeforeCancel(ctx context.Context, failed bool) {
	if failed && !state.captured && ctx != nil {
		state.state = ctx.Err()
		state.captured = true
	}
}

func (state numericPublisherDiagnosticState) line(kind, checkpoint uint8, elapsed time.Duration, failed bool) string {
	if !failed || !state.captured || state.state != nil && state.state != context.DeadlineExceeded && state.state != context.Canceled {
		return ""
	}
	prefix := ""
	switch kind {
	case 1:
		if checkpoint < 1 || checkpoint > 10 {
			return ""
		}
		prefix = "remote-access publisher-manager-query checkpoint="
	case 2:
		if checkpoint < 1 || checkpoint > 18 {
			return ""
		}
		prefix = "remote-access publisher-server checkpoint16="
	default:
		return ""
	}
	milliseconds := elapsed.Milliseconds()
	if milliseconds < 0 {
		milliseconds = 0
	}
	if milliseconds > 40000 {
		milliseconds = 40000
	}
	return fmt.Sprintf("%s%d deadline_exceeded=%t canceled=%t elapsed_ms=%d", prefix, checkpoint, state.state == context.DeadlineExceeded, state.state == context.Canceled, milliseconds)
}
