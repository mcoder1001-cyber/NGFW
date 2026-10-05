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

// NumericPublisherServerStage identifies a fixed server boundary. It carries no
// request, unit, path, credential, or underlying error text.
type NumericPublisherServerStage uint8

const (
	// NumericPublisherServerRoles validates manager-opened descriptor roles.
	NumericPublisherServerRoles NumericPublisherServerStage = iota + 1
	// NumericPublisherServerCaps validates the restricted service process.
	NumericPublisherServerCaps
	// NumericPublisherServerInstallation authenticates the complete installed artifacts.
	NumericPublisherServerInstallation
	// NumericPublisherServerListener validates the fixed listening socket.
	NumericPublisherServerListener
	// NumericPublisherServerAccept accepts the single canonical-agent connection.
	NumericPublisherServerAccept
	// NumericPublisherServerPeer authenticates the canonical agent process.
	NumericPublisherServerPeer
	// NumericPublisherServerReference authenticates its immutable source generation.
	NumericPublisherServerReference
	// NumericPublisherServerSourceImage authenticates the manager-held agent executable.
	NumericPublisherServerSourceImage
	// NumericPublisherServerRequest receives and validates the bounded request.
	NumericPublisherServerRequest
	// NumericPublisherServerPrevious validates the first server's held source image.
	NumericPublisherServerPrevious
	// NumericPublisherServerProof rechecks the held artifact proof before mutation.
	NumericPublisherServerProof
	// NumericPublisherServerPublish writes only the fixed authenticated numeric configuration.
	NumericPublisherServerPublish
	// NumericPublisherServerPostPeer rechecks the canonical source after publication.
	NumericPublisherServerPostPeer
	// NumericPublisherServerTarget verifies the canonical target before activation.
	NumericPublisherServerTarget
	// NumericPublisherServerActivate activates only the fixed numeric supplier.
	NumericPublisherServerActivate
	// NumericPublisherServerPostTarget rechecks the target after activation.
	NumericPublisherServerPostTarget
	// NumericPublisherServerManager verifies the fixed publisher manager state.
	NumericPublisherServerManager
	// NumericPublisherServerReplyPeer rechecks the source before responding.
	NumericPublisherServerReplyPeer
	// NumericPublisherServerSend sends the bounded response and held source image.
	NumericPublisherServerSend
	// NumericPublisherServerAck receives and validates the final acknowledgement.
	NumericPublisherServerAck
)

// Valid reports whether this value belongs to the closed server-stage contract.
func (s NumericPublisherServerStage) Valid() bool {
	return s >= NumericPublisherServerRoles && s <= NumericPublisherServerAck
}
