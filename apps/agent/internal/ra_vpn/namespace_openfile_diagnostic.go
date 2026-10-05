package ravpn

import (
	"context"
	"fmt"
)

// NumericPublisherFailure contains only a fixed validation stage and deadline flag.
// Stages 1..24 identify identity, peer, reference, helper, template, socket unit,
// service unit, active process, cgroup, executable, request, socket metadata,
// socket creation, connection, peer credentials, encoding, send, source send,
// receive, response, source executable, acknowledgement, first exit, and postcheck.
type NumericPublisherFailure struct {
	Stage            uint8
	DeadlineExceeded bool
}

// Error exposes no underlying diagnostic or input values.
func (e *NumericPublisherFailure) Error() string {
	return fmt.Sprintf("remote-access publisher refused at stage %d deadline=%t", e.Stage, e.DeadlineExceeded)
}

// Unwrap preserves fail-closed classification.
func (*NumericPublisherFailure) Unwrap() error { return ErrBoundary }

func numericPublisherFailure(ctx context.Context, stage uint8) error {
	return &NumericPublisherFailure{Stage: stage, DeadlineExceeded: ctx != nil && ctx.Err() == context.DeadlineExceeded}
}
