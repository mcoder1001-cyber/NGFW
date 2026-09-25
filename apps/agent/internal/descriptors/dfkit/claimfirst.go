package dfkit

// TD-11b (review 3.2 / 3.3): the persistence checks the DF-8 descriptors call from their
// CheckPersistent (dfkit/persist), and the claim-first form of Target.Claim.

import (
	"context"
	"errors"
	"fmt"

	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
)

// Persistent marks FileBootStore as a store that survives an agent restart (dfkit/persist).
func (*FileBootStore) Persistent() bool { return true }

// CheckClaims is the persistence check of a descriptor that claims untagged interfaces through the
// owner's DF-1 claim store (Target.Claim, iface.Claims(owner)); call it from the descriptor's
// CheckPersistent. The product wiring installs subsystems.IfaceClaims with iface.SetClaimStore.
func CheckClaims(name, owner string) error {
	return persist.Require(name+": claims on untagged interfaces of owner "+owner+" (install a persisted store with iface.SetClaimStore)", iface.Claims(owner))
}

// CheckBoot is the persistence check of a descriptor's D-076 applied-once BootStore; call it from
// the descriptor's CheckPersistent. The product wiring passes Wiring.BootStore() (a FileBootStore).
func CheckBoot(name string, s BootStore) error {
	return persist.Require(name+": applied-once records (pass the persisted Wiring.BootStore())", s)
}

// Claim is a claim recorded BEFORE the VPP add of a Create (TD-11b, review 3.3). Claiming after the
// add left an unclaimed object in VPP whenever the claim failed: invisible to Retrieve (untagged
// objects are reported only when claimed), never deleted by a rollback or a resync. With ClaimFirst
// a claim that cannot be recorded fails the Create with nothing written:
//
//	c, err := tg.ClaimFirst(ctx)
//	if err != nil {
//		return nil, err
//	}
//	if err := add(); err != nil {
//		if alreadyExists(err) {
//			return meta, c.Adopt() // ours only if the claim existed before this Create
//		}
//		return nil, c.Undo(err) // releases a claim this Create made
//	}
//	return meta, nil
type Claim struct {
	t   Target
	had bool // the claim existed before this Create (or the interface is tagged: always ours)
}

// ClaimFirst records the claim on an untagged target before the VPP add (a no-op on our tagged
// interfaces). A persisted store's sw_if_index lookup is bounded by ctx (iface.ContextClaimStore).
func (t Target) ClaimFirst(ctx context.Context) (*Claim, error) {
	if !t.Untagged {
		return &Claim{t: t, had: true}, nil
	}
	s, holder := iface.Claims(t.Owner), t.claimHolder()
	var had bool
	var err error
	if cs, ok := s.(iface.ContextClaimStore); ok {
		had = cs.ClaimedContext(ctx, t.Name, holder)
		err = cs.ClaimContext(ctx, t.Name, holder)
	} else {
		had = s.Claimed(t.Name, holder)
		err = s.Claim(t.Name, holder)
	}
	if err != nil {
		return nil, fmt.Errorf("claim %s for %s: %w", t.Name, t.Holder, err)
	}
	return &Claim{t: t, had: had}, nil
}

// Adopt is Target.Adopt for the "already exists" path after ClaimFirst: the existing object is
// ours only when the claim existed before this Create (a claim made by this Create proves
// nothing). Otherwise the fresh claim is released and ErrNotOurs returned.
func (c *Claim) Adopt() error {
	if c.had {
		return nil
	}
	err := fmt.Errorf("%w: %s on untagged interface %q has no claim of %s", ErrNotOurs, c.t.Holder, c.t.Name, c.t.Owner)
	if rerr := c.t.Release(); rerr != nil {
		return errors.Join(err, rerr)
	}
	return err
}

// Undo is the failed-add path: it releases a claim this Create made (a claim that existed before
// stays) and returns err, joined with a release failure.
func (c *Claim) Undo(err error) error {
	if c.had {
		return err
	}
	if rerr := c.t.Release(); rerr != nil {
		return errors.Join(err, rerr)
	}
	return err
}
