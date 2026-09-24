package vpn

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
)

// VPP-global singletons (D-071).
//
// ipsec.backend, ipsec.async-mode, ikev2.local-key, ikev2.sleep-interval, ikev2.liveness and
// wireguard.async-mode change VPP-wide state that every owner on the VPP shares. Only the
// designated globals owner (agent config globalsOwner: true — the product agent on a real box,
// never a test slot on the shared host; each package's WithGlobalsOwner option) registers their
// setters. Every other agent registers the Require variant: Create/Update succeed only when VPP
// already has the desired value (checked through VPP's getter where one exists) and fail with
// ErrNotGlobalsOwner otherwise — always for getter-less globals; Delete is a no-op; Retrieve is
// write-only (ErrRetrieveUnsupported) and absence never plans a delete.

// ErrNotGlobalsOwner is returned when a non-owner would have to set a VPP-global.
var ErrNotGlobalsOwner = errors.New("not the globals owner: VPP-global settings are managed by the globals owner only (D-071)")

// Getter reads VPP's current value of a global in the shape of the desired value (ok=false:
// unset / not reported).
type Getter func(ctx context.Context, desired proto.Message) (have proto.Message, ok bool, err error)

// Global returns setter for the globals owner and the Require variant (checking with get; nil =
// VPP has no getter) for everybody else. Both never delete on absence.
func Global(owner bool, setter scheduler.Descriptor, get Getter) scheduler.Descriptor {
	if owner {
		return ownerGlobal{setter}
	}
	return &Require{inner: setter, get: get}
}

// ownerGlobal is the globals owner's setter: a global cannot be absent, so a retrieved value
// that the desired state does not mention is left alone (P05 AbsenceDeleter).
type ownerGlobal struct{ scheduler.Descriptor }

// DeleteOnAbsence implements scheduler.AbsenceDeleter.
func (ownerGlobal) DeleteOnAbsence() bool { return false }

// Require is the non-owner variant of a VPP-global setter (see the file comment).
type Require struct {
	inner scheduler.Descriptor
	get   Getter
}

// Name implements scheduler.Descriptor.
func (r *Require) Name() string { return r.inner.Name() }

// KeyOf implements scheduler.Descriptor.
func (r *Require) KeyOf(obj proto.Message) scheduler.Key { return r.inner.KeyOf(obj) }

// Dependencies implements scheduler.Descriptor.
func (r *Require) Dependencies(obj proto.Message) []scheduler.Dependency {
	return r.inner.Dependencies(obj)
}

// Create implements scheduler.Descriptor: check, never set.
func (r *Require) Create(ctx context.Context, obj proto.Message) (any, error) {
	if r.get == nil {
		return nil, fmt.Errorf("%s: %w; VPP has no getter, configure it on the globals owner", r.Name(), ErrNotGlobalsOwner)
	}
	have, ok, err := r.get(ctx, obj)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", r.Name(), err)
	}
	if !ok || !proto.Equal(have, obj) {
		return nil, fmt.Errorf("%s: %w; required %v, VPP has %v", r.Name(), ErrNotGlobalsOwner, obj, have)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (r *Require) Update(ctx context.Context, _, newObj proto.Message, meta any) (any, error) {
	if _, err := r.Create(ctx, newObj); err != nil {
		return nil, err
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor: a non-owner never resets a global.
func (*Require) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor: a requirement is write-only for the reconciler.
func (r *Require) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s (not the globals owner, requirement only): %w", r.Name(), ErrRetrieveUnsupported)
}

// DeleteOnAbsence implements scheduler.AbsenceDeleter.
func (*Require) DeleteOnAbsence() bool { return false }
