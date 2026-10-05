package ravpn

import (
	"context"
	"ngfw/agent/internal/vpp/bootid"
)

// SystemdNumericOpenFilePublisher is the canonical-agent client of the fixed
// manager-owned supplier provisioning service. No caller path/content is accepted.
type SystemdNumericOpenFilePublisher struct{}

// NewSystemdNumericOpenFilePublisher constructs a pure lazy provisioning client.
// The concrete authenticated transport is implemented in the next checkpoint.
func NewSystemdNumericOpenFilePublisher() NumericOpenFilePublisher {
	return &SystemdNumericOpenFilePublisher{}
}

// PublishNumericOpenFile is explicitly fail-closed until manager transport and
// held executable authentication are complete; this is not runtime readiness.
func (*SystemdNumericOpenFilePublisher) PublishNumericOpenFile(context.Context, NumericOpenFileKind, string, bootid.Identity) error {
	return ErrBoundary
}

// NamespaceHandoffInitialization is an explicit startup operation after the
// global verified inactive barrier. Verify, Preflight and Acquire remain readonly.
type NamespaceHandoffInitialization interface {
	Initialize(context.Context) error
}

// NamespaceTargetInitialization initializes canonical source references and
// the fixed VPP supplier through authenticated manager provisioning.
type NamespaceTargetInitialization interface {
	Initialize(context.Context) error
}
