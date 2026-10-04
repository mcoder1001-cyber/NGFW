package pki

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/scheduler"
)

// The singleton scheduler object of the materialiser (D-109 d: one descriptor per renderer-like stage).
const (
	// Name is the descriptor name (Domains["vpn"]).
	Name = "pki.files"
	// ID is the only object id.
	ID = "ngfw"
)

// Key is the key of the singleton.
var Key = scheduler.Join(Name, ID)

// Descriptor reconciles the operational PKI file set in the agent-owned root.
// Scheduler values contain references and fingerprints only, never certificate/key material.
type Descriptor struct{ m *Materialiser }

var (
	_ scheduler.Descriptor = (*Descriptor)(nil)
	_ scheduler.Stager     = (*Descriptor)(nil)
	_ persist.Checker      = (*Descriptor)(nil)
)

// NewDescriptor wraps a materialiser.
func NewDescriptor(m *Materialiser) *Descriptor { return &Descriptor{m: m} }

// Materialiser returns the wrapped materialiser.
func (d *Descriptor) Materialiser() *Materialiser { return d.m }

// Name implements scheduler.Descriptor.
func (*Descriptor) Name() string { return Name }

// KeyOf implements scheduler.Descriptor.
func (*Descriptor) KeyOf(proto.Message) scheduler.Key { return Key }

// Dependencies implements scheduler.Descriptor: none.
func (*Descriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Stage implements scheduler.Stager: files for a daemon.
func (*Descriptor) Stage() scheduler.Stage { return scheduler.StageDaemon }

// CheckPersistent implements persist.Checker: the descriptor records which files it wrote in the manifest, a file in
// the agent's state dir (TD-11b).
func (d *Descriptor) CheckPersistent() error {
	return persist.Require("pki.files: manifest of written files", d.m.Manifest())
}

func set(obj proto.Message) (*ngfwv1.PkiFileStateSet, error) {
	s, ok := obj.(*ngfwv1.PkiFileStateSet)
	if !ok || s == nil {
		return nil, fmt.Errorf("pki: value of %s is %T, want *ngfw.v1.PkiFileStateSet", Key, obj)
	}
	return s, nil
}

// Create writes the files of obj (and removes files of the manifest it no longer lists).
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := set(obj)
	if err != nil {
		return nil, err
	}
	return nil, d.m.Apply(ctx, s)
}

// Update is Create with the new value.
func (d *Descriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete removes every file this agent wrote.
func (d *Descriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.m.Apply(ctx, &ngfwv1.PkiFileStateSet{})
}

// Retrieve reports the files on disk (none: no object).
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	s, err := d.m.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	if len(s.GetFiles()) == 0 {
		return nil, nil
	}
	return []scheduler.KV{{Key: Key, Value: s}}, nil
}
