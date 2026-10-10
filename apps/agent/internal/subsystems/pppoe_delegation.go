package subsystems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/df2"
	ip6nd "ngfw/agent/internal/descriptors/ip6_nd"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

// pdOwnership partitions existing address/RA operations into static and dynamic
// instances. Claims are persisted before mutation, retained on partial failure,
// and released only after verified deletion. Base descriptors still enforce the
// agent's interface tag/boot/index ownership; this ledger never establishes it.
type pdOwnership struct {
	mu   sync.RWMutex
	path string
	keys map[string]bool
}

func openPDOwnership(path string) (*pdOwnership, error) {
	s := &pdOwnership{path: path, keys: map[string]bool{}}
	b, err := os.ReadFile(path) //nolint:gosec // Fixed per-owner agent state path; no secrets.
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &s.keys); err != nil {
		return nil, err
	}
	if s.keys == nil {
		s.keys = map[string]bool{}
	}
	for key := range s.keys {
		switch scheduler.Key(key).Descriptor() {
		case core.InterfaceAddrName, ip6nd.RaConfigName, ip6nd.RaPrefixName:
		default:
			return nil, fmt.Errorf("invalid PD ownership descriptor")
		}
	}
	return s, nil
}
func (s *pdOwnership) owns(key scheduler.Key) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keys[string(key)]
}
func (s *pdOwnership) set(key scheduler.Key, present bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]bool, len(s.keys)+1)
	for k, v := range s.keys {
		next[k] = v
	}
	if present {
		next[string(key)] = true
	} else {
		delete(next, string(key))
	}
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".pppoe-pd-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp, s.path)
	}
	if err != nil {
		return err
	}
	// Publish memory after rename even if directory fsync fails: disk may already
	// contain this exact ownership claim and must not disagree with live filtering.
	s.keys = next
	directory, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	return errors.Join(syncErr, directory.Close())
}

type pdDescriptor struct {
	scheduler.Descriptor
	name  string
	owned *pdOwnership
}

func (d *pdDescriptor) Name() string { return d.name }
func (d *pdDescriptor) KeyOf(v proto.Message) scheduler.Key {
	named, ok := v.(interface{ GetInterface() string })
	if !ok {
		return scheduler.Join(d.name, "invalid")
	}
	return scheduler.Join(d.name, named.GetInterface())
}
func (d *pdDescriptor) Update(ctx context.Context, old, next proto.Message, meta any) (any, error) {
	if d.Descriptor.KeyOf(old) != d.Descriptor.KeyOf(next) {
		return nil, scheduler.ErrRecreate
	}
	return d.Descriptor.Update(ctx, old, next, meta)
}
func (d *pdDescriptor) CheckPersistent() error {
	if d.owned == nil || d.owned.path == "" {
		return fmt.Errorf("PD ownership persistence unavailable")
	}
	if checked, ok := d.Descriptor.(interface{ CheckPersistent() error }); ok {
		return checked.CheckPersistent()
	}
	return fmt.Errorf("PD base ownership declaration unavailable")
}
func (d *pdDescriptor) Dependencies(v proto.Message) []scheduler.Dependency {
	deps := d.Descriptor.Dependencies(v)
	switch p := v.(type) {
	case *ip6nd.RaPrefix:
		// Explicitly order prefix after the router address. The existing RA descriptor's
		// optional static-address dependency does not name the dynamic instance.
		deps = append(deps, scheduler.Dependency{Key: scheduler.Join(desired.PppoeDelegationAddress, p.GetInterface())})
	case *ip6nd.RaConfig:
		// Prefix dependency is supplied by descriptor graph through an alias below.
		deps = append(deps, scheduler.Dependency{Key: scheduler.Join(desired.PppoeDelegationPrefix, p.GetInterface())})
	}
	return deps
}
func (d *pdDescriptor) Create(ctx context.Context, v proto.Message) (any, error) {
	key := d.Descriptor.KeyOf(v)
	if !d.owned.owns(key) {
		live, err := d.Descriptor.Retrieve(ctx)
		if err != nil {
			return nil, err
		}
		for _, kv := range live {
			if kv.Key == key {
				return nil, fmt.Errorf("PD refuses existing static address or RA state")
			}
		}
		if err = d.owned.set(key, true); err != nil {
			return nil, err
		}
	}
	meta, err := d.Descriptor.Create(ctx, v)
	if err != nil {
		if scheduler.IsPartialCreate(err) {
			return meta, err
		}
		// Distinguish rejected writes from a mutation followed by failed timer
		// capture. Never delete preexisting state on an ordinary rejected create.
		live, readErr := d.Descriptor.Retrieve(ctx)
		if readErr == nil {
			for _, kv := range live {
				if kv.Key == key {
					return kv.Meta, scheduler.PartialCreate(err)
				}
			}
			if releaseErr := d.owned.set(key, false); releaseErr != nil {
				return nil, errors.Join(err, releaseErr)
			}
			return nil, err
		}
		return meta, scheduler.PartialCreate(errors.Join(err, readErr))
	}
	return meta, nil
}
func (d *pdDescriptor) Delete(ctx context.Context, v proto.Message, meta any) error {
	key := d.Descriptor.KeyOf(v)
	if !d.owned.owns(key) {
		return fmt.Errorf("PD refuses deletion without its ownership record")
	}
	if err := d.Descriptor.Delete(ctx, v, meta); err != nil {
		return err
	}
	live, err := d.Descriptor.Retrieve(ctx)
	if err != nil {
		return err
	}
	for _, kv := range live {
		if kv.Key == key {
			return fmt.Errorf("PD deletion not verified")
		}
	}
	return d.owned.set(key, false)
}
func (d *pdDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	live, err := d.Descriptor.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(live))
	for _, kv := range live {
		if d.owned.owns(kv.Key) {
			kv.Key = d.KeyOf(kv.Value)
			out = append(out, kv)
		}
	}
	return out, nil
}

// registerPppoeDelegation uses a current-generation verified snapshot supplied by
// carrier reconciliation. It never treats a raw hook state as forwarding ready.
func (w *Wiring) registerPppoeDelegation(reg scheduler.Registry, snapshot func() []desired.PppoeDelegationLease) error {
	if snapshot == nil {
		return fmt.Errorf("PD verified snapshot is required")
	}
	owned, err := openPDOwnership(filepath.Join(w.env.StateDir, "pppoe-pd-"+w.env.Owner+".json"))
	if err != nil {
		return err
	}
	claims, err := w.KeyedClaims("acl")
	if err != nil {
		return err
	}
	life, err := ip6nd.OpenLifetimeStore(filepath.Join(w.env.StateDir, "pppoe-pd-lifetimes-"+w.env.Owner+".json"))
	if err != nil {
		return err
	}
	bases := []scheduler.Descriptor{
		&core.InterfaceAddrDescriptor{Env: core.Env{Client: w.env.Client, Owner: w.env.Owner, Owned: w.env.Owned, IfRef: core.AliasInterfaceRef, Claims: w.ifaceClaim}},
		ip6nd.NewRaPrefix(w.env.Client, w.env.Owner, df2.WithClaims(claims)).WithLifetimeStore(life),
		ip6nd.NewRaConfig(w.env.Client, w.env.Owner, df2.WithClaims(claims)),
	}
	names := []string{desired.PppoeDelegationAddress, desired.PppoeDelegationPrefix, desired.PppoeDelegationRA}
	lookup, ok := reg.(interface {
		ForKey(scheduler.Key) (scheduler.Descriptor, bool)
	})
	if !ok {
		return fmt.Errorf("PD requires descriptor lookup")
	}
	for i, base := range bases {
		static, exists := lookup.ForKey(scheduler.Join(base.Name(), "pd-probe"))
		if !exists {
			return fmt.Errorf("PD static descriptor unavailable")
		}
		exclude, ok := static.(interface {
			SetDelegationExclusion(func(scheduler.Key) bool)
		})
		if !ok {
			return fmt.Errorf("PD static descriptor lacks ownership separation")
		}
		exclude.SetDelegationExclusion(owned.owns)
		reg.Register(&pdDescriptor{Descriptor: base, name: names[i], owned: owned})
	}
	var cacheMu sync.RWMutex
	var cached []desired.PppoeDelegationLease
	return w.AddDynamicSource(DynamicSource{Name: "pppoe-delegation", Descriptors: names,
		Desired: func(doc *ngfwv1.DesiredState) []scheduler.KV {
			cacheMu.RLock()
			defer cacheMu.RUnlock()
			return desired.PppoeDelegation(doc, cached, time.Now())
		},
		Run: func(ctx context.Context, apply SyncFunc) {
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			runPppoeDelegation(ctx, tick.C, snapshot, func(leases []desired.PppoeDelegationLease) {
				cacheMu.Lock()
				cached = append([]desired.PppoeDelegationLease(nil), leases...)
				cacheMu.Unlock()
			}, apply, func(err error) {
				w.env.Log.Warn("PPPoE delegation reconciliation failed", "error", err)
			})
		},
	})
}

// runPppoeDelegation keeps active lease lifetimes and failure retries on every
// tick. An empty snapshot is reconciled once successfully (including withdrawal),
// then waits for a lease transition: repeated empty syncs needlessly retrieve
// unrelated dependencies such as FRR and can prevent startup readiness.
func runPppoeDelegation(ctx context.Context, ticks <-chan time.Time, snapshot func() []desired.PppoeDelegationLease, publish func([]desired.PppoeDelegationLease), apply SyncFunc, failed func(error)) {
	emptyApplied := false
	for {
		leases := snapshot()
		sort.Slice(leases, func(i, j int) bool { return leases[i].Logical < leases[j].Logical })
		publish(leases)
		if len(leases) != 0 || !emptyApplied {
			err := apply(ctx)
			emptyApplied = len(leases) == 0 && err == nil
			if err != nil {
				failed(err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

// Preserve PD ownership separation through the remote-access shared-family wrapper.
func (d *raScopedDescriptor) SetDelegationExclusion(exclude func(scheduler.Key) bool) {
	if hook, ok := d.inner.(interface {
		SetDelegationExclusion(func(scheduler.Key) bool)
	}); ok {
		hook.SetDelegationExclusion(exclude)
	}
}
