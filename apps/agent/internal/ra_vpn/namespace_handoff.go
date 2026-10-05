package ravpn

import (
	"context"
	"ngfw/agent/internal/vpp/bootid"
)

// MountTarget binds a held mount namespace to a complete process identity.
// Parent code must pin its namespace FD before privileged broker execution.
type MountTarget struct {
	Boot       bootid.Identity
	MountInode uint64
}

// NamespaceHandoff publishes the already owned NSFS handles at fixed paths in
// both the VPP and manager mount namespaces. It never accepts arbitrary paths.
// Implementations persist complete target/binding identity and verify exports
// before any TAP/unit handoff. Old missing export receipts are fail-closed.
type NamespaceHandoff interface {
	Preflight(context.Context) error
	Export(context.Context, *NetworkPlan) error
	Verify(context.Context, *NetworkPlan) error
	Remove(context.Context, *NetworkPlan) error
}

// MutationGuard quiesces exactly this instance before any transport change.
// Failure retains every owned transport object needed by an uncertain daemon.
type MutationGuard func(context.Context, *NetworkPlan) error
