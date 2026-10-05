package ravpn

import (
	"context"
	"ngfw/agent/internal/vpp/bootid"
)

// SystemdNumericOpenFilePublisher is the canonical-agent client of the fixed
// manager-owned supplier provisioning service. No caller path/content is accepted.
type SystemdNumericOpenFilePublisher struct{}

// NewSystemdNumericOpenFilePublisher constructs a pure lazy provisioning client.
// No provisioning or host command runs during construction.
func NewSystemdNumericOpenFilePublisher() NumericOpenFilePublisher {
	return &SystemdNumericOpenFilePublisher{}
}

// PublishNumericOpenFile performs bounded canonical-agent manager provisioning.
// Missing authenticated installation or source identity fails closed.
func (*SystemdNumericOpenFilePublisher) PublishNumericOpenFile(ctx context.Context, kind NumericOpenFileKind, instance string, target bootid.Identity) error {
	return publishNumericThroughManager(ctx, kind, instance, target)
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

// NamespaceHandoffSourceInitialization prepares only canonical source identity
// and monotonic self capability normalization before live-unit observations.
// It never provisions a supplier, mutates VPP, or changes transport objects.
type NamespaceHandoffSourceInitialization interface {
	InitializeSource(context.Context) error
}

// NamespaceTargetSourceInitialization prepares immutable current source
// references; manager provisioning remains in the separate Initialize phase.
type NamespaceTargetSourceInitialization interface {
	InitializeSource(context.Context) error
}
