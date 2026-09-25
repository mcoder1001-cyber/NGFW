package subsystems

import (
	"context"
	"errors"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/lisp"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// registerLisp registers DF-6's LISP / LISP-GPE descriptors (F-lisp, domain `tunnels`): keyed claims in
// the persisted df6 pair store (TD-11b: df6 claim ids are not interface names, so not IfaceClaims), the
// VPP-global switches (LISP, LISP-GPE, PITR) as setters only on the globals owner and as require
// variants elsewhere (D-071).
func registerLisp(r scheduler.Registry, w *Wiring) error {
	pairs, err := w.PairClaims("df6")
	if err != nil {
		return err
	}
	lisp.Register(lispRegistry{r, w.env.Client}, w.env.Client, w.env.Owner, df6.WithClaims(pairs), df6.WithGlobalsOwner(w.env.GlobalsOwner))
	return nil
}

// lispRegistry decorates every LISP descriptor before it is registered:
//
//   - Retrieve on a VPP without the lisp plugin reports no objects instead of failing the whole
//     Retrieve / plan of every domain (a VPP that does not know LISP holds no LISP object); creating
//     one still fails with df6.ErrPluginNotLoaded;
//   - the three VPP-global singletons declare, for the persistence guard (dfkit/persist, TD-11b), that
//     they record no ownership: they belong to the globals owner by role (D-071), not by a claim. df6's
//     singleton / require descriptors do not declare it themselves (df6 is read-only here — see
//     F-lisp-questions.md).
type lispRegistry struct {
	scheduler.Registry
	c vpp.Client
}

func (g lispRegistry) Register(d scheduler.Descriptor) {
	t := lispTolerant{Descriptor: d, c: g.c}
	switch d.Name() {
	case lisp.EnableName, lisp.GpeEnableName, lisp.PitrName:
		g.Registry.Register(lispGlobal{t})
	default:
		g.Registry.Register(t)
	}
}

type lispTolerant struct {
	scheduler.Descriptor
	c vpp.Client
}

// Unwrap exposes the decorated descriptor (dfkit/persist walks the chain for CheckPersistent).
func (t lispTolerant) Unwrap() scheduler.Descriptor { return t.Descriptor }

// Retrieve implements scheduler.Descriptor.
func (t lispTolerant) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	kvs, err := t.Descriptor.Retrieve(ctx)
	if err != nil && !scheduler.IsRetrieveUnsupported(err) && lispPluginAbsent(t.c, err) {
		return nil, nil
	}
	return kvs, err
}

// DeleteOnAbsence forwards the decorated descriptor's choice (the globals are kept on absence).
func (t lispTolerant) DeleteOnAbsence() bool {
	if d, ok := t.Descriptor.(interface{ DeleteOnAbsence() bool }); ok {
		return d.DeleteOnAbsence()
	}
	return true
}

type lispGlobal struct{ lispTolerant }

// RecordsNoOwnership implements persist.NoOwnership.
func (lispGlobal) RecordsNoOwnership() {}

// lispPluginAbsent: the VPP does not know the lisp messages — govpp's unknown-message error
// (df6.ErrPluginNotLoaded), or a client that can tell it has no handler for show_lisp_status (the
// unit-test fake VPP, which models no LISP unless a test installs it).
func lispPluginAbsent(c vpp.Client, err error) bool {
	if errors.Is(err, df6.ErrPluginNotLoaded) {
		return true
	}
	h, ok := c.(interface{ Handles(string) bool })
	return ok && !h.Handles("show_lisp_status")
}
