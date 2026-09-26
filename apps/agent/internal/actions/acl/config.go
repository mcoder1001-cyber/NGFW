package acl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// NameConfig is the agent-local descriptor of the APPLIED acl configuration (review H1): one object
// per L3/L4 list, per MACIP list and one for the attachments, whose value is the configuration
// message itself holding exactly that entry. Only a transaction's Create/Update/Delete change it
// (a DryRun never does; a rolled-back Apply is reverted by the scheduler like any object), so it is
// what Retrieve, AclState and the re-projection watcher attribute VPP's content to. No VPP object:
// the store is in memory and rebuilt by the resync after an agent restart (it records no ownership).
const NameConfig = "acl.config"

// Key kinds of acl.config objects.
const (
	configList        = "list"
	configMacip       = "macip"
	configAttachments = "attachments"
)

// KeyConfigList is the key of the applied configuration of L3/L4 list name.
func KeyConfigList(name string) scheduler.Key { return scheduler.Join(NameConfig, configList, name) }

// KeyConfigMacip is the key of the applied configuration of MACIP list name.
func KeyConfigMacip(name string) scheduler.Key { return scheduler.Join(NameConfig, configMacip, name) }

// KeyConfigAttachments is the key of the applied attachments (acl.attachments + acl.macipAttachments).
var KeyConfigAttachments = scheduler.Join(NameConfig, configAttachments)

// ConfigList is the acl.config value of list name.
func ConfigList(name string, l *vrxv1.AclList) *vrxv1.AclConfig {
	return &vrxv1.AclConfig{Lists: map[string]*vrxv1.AclList{name: l}}
}

// ConfigMacip is the acl.config value of MACIP list name.
func ConfigMacip(name string, l *vrxv1.MacipList) *vrxv1.AclConfig {
	return &vrxv1.AclConfig{Macip: map[string]*vrxv1.MacipList{name: l}}
}

// ConfigAttachments is the acl.config value of the attachments.
func ConfigAttachments(a []*vrxv1.AclAttachment, m []*vrxv1.MacipAttachment) *vrxv1.AclConfig {
	return &vrxv1.AclConfig{Attachments: a, MacipAttachments: m}
}

var errConfigValue = errors.New("acl.config: value must hold exactly one list, one MACIP list, or the attachments")

// configKeyOf returns the key of an acl.config value and the pin key of its configuration.
func configKeyOf(obj proto.Message) (scheduler.Key, string, error) {
	c, ok := obj.(*vrxv1.AclConfig)
	if !ok {
		return "", "", fmt.Errorf("%w: %T", errConfigValue, obj)
	}
	n := len(c.GetLists()) + len(c.GetMacip()) + len(c.GetHost()) + len(c.GetHostAttachments())
	switch {
	case len(c.GetLists()) == 1 && n == 1 && len(c.GetAttachments())+len(c.GetMacipAttachments()) == 0:
		for name := range c.GetLists() {
			return KeyConfigList(name), pinACL(name), nil
		}
	case len(c.GetMacip()) == 1 && n == 1 && len(c.GetAttachments())+len(c.GetMacipAttachments()) == 0:
		for name := range c.GetMacip() {
			return KeyConfigMacip(name), pinMacip(name), nil
		}
	case n == 0 && len(c.GetAttachments())+len(c.GetMacipAttachments()) > 0:
		return KeyConfigAttachments, pinAttachments, nil
	}
	return "", "", errConfigValue
}

// configEntry is one applied configuration object.
type configEntry struct {
	value *vrxv1.AclConfig
	hash  string // ConfigHash of the entry itself (the list / MACIP list / attachments config)
}

// appliedStore is the in-memory applied acl configuration of one agent.
type appliedStore struct {
	mu      sync.Mutex
	entries map[scheduler.Key]configEntry
}

func newAppliedStore() *appliedStore { return &appliedStore{entries: map[scheduler.Key]configEntry{}} }

// entryHash hashes the part of an acl.config value its record entries are keyed by.
func entryHash(c *vrxv1.AclConfig) string {
	for _, l := range c.GetLists() {
		return ConfigHash(l)
	}
	for _, l := range c.GetMacip() {
		return ConfigHash(l)
	}
	return ConfigHash(ConfigAttachments(c.GetAttachments(), c.GetMacipAttachments()))
}

// ConfigDescriptor manages acl.config objects (the applied configuration; no VPP call).
type ConfigDescriptor struct {
	store  *appliedStore
	record *Record
}

var _ scheduler.Descriptor = (*ConfigDescriptor)(nil)

// Name implements scheduler.Descriptor.
func (*ConfigDescriptor) Name() string { return NameConfig }

// KeyOf implements scheduler.Descriptor.
func (*ConfigDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	k, _, err := configKeyOf(obj)
	if err != nil {
		return scheduler.Join(NameConfig, "invalid")
	}
	return k
}

// Dependencies implements scheduler.Descriptor: none (ordering does not matter for a record).
func (*ConfigDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// RecordsNoOwnership declares (TD-11b) that the store records no ownership of any VPP object: it is
// the applied configuration, rebuilt by the resync after an agent restart.
func (*ConfigDescriptor) RecordsNoOwnership() {}

func (d *ConfigDescriptor) put(obj proto.Message) error {
	k, pin, err := configKeyOf(obj)
	if err != nil {
		return err
	}
	v := proto.Clone(obj).(*vrxv1.AclConfig)
	h := entryHash(v)
	d.store.mu.Lock()
	d.store.entries[k] = configEntry{value: v, hash: h}
	d.store.mu.Unlock()
	d.record.pin(pin, h)
	return nil
}

// Create implements scheduler.Descriptor.
func (d *ConfigDescriptor) Create(_ context.Context, obj proto.Message) (any, error) {
	return nil, d.put(obj)
}

// Update implements scheduler.Descriptor.
func (d *ConfigDescriptor) Update(_ context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.put(newObj)
}

// Delete implements scheduler.Descriptor.
func (d *ConfigDescriptor) Delete(_ context.Context, obj proto.Message, _ any) error {
	k, pin, err := configKeyOf(obj)
	if err != nil {
		return err
	}
	d.store.mu.Lock()
	delete(d.store.entries, k)
	d.store.mu.Unlock()
	d.record.pin(pin, "")
	return nil
}

// Retrieve implements scheduler.Descriptor: the applied configuration objects.
func (d *ConfigDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	d.store.mu.Lock()
	defer d.store.mu.Unlock()
	keys := make([]scheduler.Key, 0, len(d.store.entries))
	for k := range d.store.entries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]scheduler.KV, 0, len(keys))
	for _, k := range keys {
		out = append(out, scheduler.KV{Key: k, Value: proto.Clone(d.store.entries[k].value)})
	}
	return out, nil
}

// applied returns the applied value and its hash under key.
func (s *appliedStore) applied(k scheduler.Key) (*vrxv1.AclConfig, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[k]
	return e.value, e.hash, ok
}
