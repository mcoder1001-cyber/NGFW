package ravpn

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"path/filepath"
	"strings"
)

// TAPReceipt is root-owned agent metadata outside the writable daemon child.
// Pending is saved before mutation so a crash cannot turn a discovered owner
// tag into permission to adopt an unverified runtime index.
type TAPReceipt struct {
	Instance                           string
	NamespaceInode, HostNamespaceInode uint64
	Boot                               bootid.Identity
	Index                              uint32
	Pending                            bool
	Endpoint                           *tapv2.Tap
}

// TAPReceiptStore persists owner-bound pending and completed transit TAP receipts.
type TAPReceiptStore interface {
	Load(string) (TAPReceipt, error)
	Save(string, TAPReceipt) error
	Remove(string) error
}

// GuardedTAP wraps the existing VPP TAP descriptor without relaxing its owner
// tag checks. Every destructive call additionally proves the complete current
// boot identity, held kernel namespace pair and a fresh full endpoint dump.
type GuardedTAP struct {
	Tap       scheduler.Descriptor
	Store     TAPReceiptStore
	Boot      func() bootid.Identity
	Plan      func(string) (*NetworkPlan, error)
	AllowedID func(uint32) bool
	Guard     MutationGuard
	// Restart recovery requires an all-owner dump, not filtered Retrieve.
	Absent  func(context.Context, *tapv2.Tap, uint32) error
	Retired func(bootid.Identity) bool
}

// CheckPersistent requires durable ownership receipts before descriptor use.
func (d *GuardedTAP) CheckPersistent() error {
	return persist.Require("ra transit TAP claims", d.Store)
}

// Name identifies the underlying TAP scheduler descriptor.
func (*GuardedTAP) Name() string { return tapv2.TapName }

// KeyOf delegates the endpoint key to the underlying TAP descriptor.
func (d *GuardedTAP) KeyOf(value proto.Message) scheduler.Key { return d.Tap.KeyOf(value) }

// Dependencies requires the exact validated namespace alias before a transit TAP.
func (d *GuardedTAP) Dependencies(value proto.Message) []scheduler.Dependency {
	endpoint, ok := value.(*tapv2.Tap)
	if !ok {
		return []scheduler.Dependency{{Key: scheduler.Join(NamespaceName, "invalid")}}
	}
	alias := filepath.Base(endpoint.HostNamespace)
	return []scheduler.Dependency{{Key: scheduler.Join(NamespaceName, alias)}}
}
func (d *GuardedTAP) input(value proto.Message) (*tapv2.Tap, *NetworkPlan, bootid.Identity, error) {
	endpoint, ok := value.(*tapv2.Tap)
	if !ok || endpoint == nil || d.Tap == nil || d.Store == nil || d.Boot == nil || d.Plan == nil || d.AllowedID == nil || endpoint.Id > 8191 || !d.AllowedID(endpoint.Id) {
		return nil, nil, bootid.Identity{}, ErrBoundary
	}
	if filepath.Dir(endpoint.HostNamespace) != filepath.Join(InstanceRoot, "n") || !namespaceAliasName.MatchString(filepath.Base(endpoint.HostNamespace)) {
		return nil, nil, bootid.Identity{}, ErrBoundary
	}
	plan, err := d.Plan(endpoint.HostNamespace)
	boot := d.Boot()
	if err != nil || plan.Validate() != nil || NamespacePath(plan.Instance) != endpoint.HostNamespace || plan.NamespaceInode == 0 || plan.HostNamespaceInode == 0 || plan.NamespaceInode == plan.HostNamespaceInode || !boot.Complete() || boot.PID <= 0 {
		return nil, nil, bootid.Identity{}, ErrBoundary
	}
	outer, inner, err := TransitTAPs(plan, 0, 1)
	if err != nil {
		return nil, nil, bootid.Identity{}, ErrBoundary
	}
	expected := inner
	if endpoint.Name == LinkName(plan.Instance, true) {
		expected = outer
	}
	// Shape construction uses inert IDs; only the reserved actual ID is applied.
	expected.Id = endpoint.Id
	if VerifyTransitTAP(expected, endpoint) != nil {
		return nil, nil, bootid.Identity{}, ErrBoundary
	}
	return endpoint, plan, boot, nil
}
func receiptMatches(receipt TAPReceipt, endpoint *tapv2.Tap, plan *NetworkPlan, boot bootid.Identity) bool {
	return receipt.Instance == plan.Instance && receipt.NamespaceInode == plan.NamespaceInode && receipt.HostNamespaceInode == plan.HostNamespaceInode && receipt.Boot.Equal(boot) && receipt.Boot.Complete() && receipt.Boot.PID > 0 && proto.Equal(receipt.Endpoint, endpoint)
}
func (d *GuardedTAP) observed(ctx context.Context, endpoint *tapv2.Tap) (iface.Meta, error) {
	rows, err := d.Tap.Retrieve(ctx)
	if err != nil {
		return iface.Meta{}, err
	}
	var found *iface.Meta
	for _, row := range rows {
		if row.Key != d.Tap.KeyOf(endpoint) {
			continue
		}
		actual, ok := row.Value.(*tapv2.Tap)
		meta, err := iface.MetaOf(row.Meta)
		if !ok || err != nil || VerifyTransitTAP(endpoint, actual) != nil || found != nil {
			return iface.Meta{}, ErrBoundary
		}
		found = &meta
	}
	if found == nil {
		return iface.Meta{}, ErrBoundary
	}
	return *found, nil
}

// Create persists a pending owner claim before mutation and verifies fresh endpoint readback.
func (d *GuardedTAP) Create(ctx context.Context, value proto.Message) (any, error) {
	endpoint, plan, boot, err := d.input(value)
	if err != nil {
		return nil, err
	}
	if d.Guard != nil {
		if err := d.Guard(ctx, plan); err != nil {
			return nil, err
		}
	}
	previous, existing := d.Store.Load(endpoint.Name)
	if !errors.Is(existing, os.ErrNotExist) {
		if existing != nil || previous.Pending || previous.Boot.Equal(boot) || !previous.Boot.Complete() || previous.Boot.PID <= 0 || previous.Instance != plan.Instance || previous.NamespaceInode != plan.NamespaceInode || previous.HostNamespaceInode != plan.HostNamespaceInode || !proto.Equal(previous.Endpoint, endpoint) || d.Absent == nil || d.Retired == nil || !d.Retired(previous.Boot) || d.Absent(ctx, endpoint, previous.Index) != nil || !boot.Equal(d.Boot()) {
			return nil, ErrBoundary
		}
		_, currentPlan, currentBoot, e := d.input(value)
		if e != nil || !currentBoot.Equal(boot) || currentPlan.NamespaceInode != previous.NamespaceInode || currentPlan.HostNamespaceInode != previous.HostNamespaceInode || d.Store.Remove(endpoint.Name) != nil {
			return nil, ErrBoundary
		}
	}
	receipt := TAPReceipt{Instance: plan.Instance, NamespaceInode: plan.NamespaceInode, HostNamespaceInode: plan.HostNamespaceInode, Boot: boot, Pending: true, Endpoint: proto.Clone(endpoint).(*tapv2.Tap)}
	if d.Store.Save(endpoint.Name, receipt) != nil {
		return nil, ErrBoundary
	}
	meta, err := d.Tap.Create(ctx, value)
	if err != nil {
		return receipt, scheduler.PartialCreate(errors.Join(err, scheduler.ErrUncertainOutcome))
	}
	index, err := iface.MetaOf(meta)
	if err != nil {
		return meta, scheduler.PartialCreate(errors.Join(ErrBoundary, scheduler.ErrUncertainOutcome))
	}
	receipt.Index = uint32(index.SwIfIndex)
	if !boot.Equal(d.Boot()) {
		return receipt, scheduler.PartialCreate(errors.Join(ErrBoundary, scheduler.ErrUncertainOutcome))
	}
	actual, err := d.observed(ctx, endpoint)
	if err != nil || actual != index {
		return receipt, scheduler.PartialCreate(errors.Join(ErrBoundary, scheduler.ErrUncertainOutcome))
	}
	receipt.Pending = false
	if d.Store.Save(endpoint.Name, receipt) != nil {
		return receipt, scheduler.PartialCreate(errors.Join(ErrBoundary, scheduler.ErrUncertainOutcome))
	}
	return receipt, nil
}

// Update requests recreation so ownership and readback guards run for changed endpoints.
func (*GuardedTAP) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete removes only a fresh owner-bound endpoint matching its completed receipt.
func (d *GuardedTAP) Delete(ctx context.Context, value proto.Message, meta any) error {
	endpoint, plan, boot, err := d.input(value)
	if err != nil {
		return err
	}
	if d.Guard != nil {
		if err := d.Guard(ctx, plan); err != nil {
			return err
		}
	}
	receipt, ok := meta.(TAPReceipt)
	if !ok || !receiptMatches(receipt, endpoint, plan, boot) {
		return ErrBoundary
	}
	actual, err := d.observed(ctx, endpoint)
	if err != nil || uint32(actual.SwIfIndex) != receipt.Index || !boot.Equal(d.Boot()) {
		return ErrBoundary
	}
	if err = d.Tap.Delete(ctx, endpoint, actual); err != nil {
		return err
	}
	return d.Store.Remove(endpoint.Name)
}

// Retrieve refuses uncertain or unowned transit endpoints instead of adopting them.
func (d *GuardedTAP) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	rows, err := d.Tap.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	var result []scheduler.KV
	for _, row := range rows {
		endpoint, ok := row.Value.(*tapv2.Tap)
		if !ok || !strings.HasPrefix(endpoint.Name, "ra_") {
			continue
		}
		_, plan, boot, err := d.input(endpoint)
		if err != nil {
			return nil, err
		}
		receipt, err := d.Store.Load(endpoint.Name)
		index, indexErr := iface.MetaOf(row.Meta)
		if err != nil || indexErr != nil || receipt.Pending || !receiptMatches(receipt, endpoint, plan, boot) || uint32(index.SwIfIndex) != receipt.Index || !boot.Equal(d.Boot()) {
			return nil, ErrBoundary
		}
		row.Meta = receipt
		result = append(result, row)
	}
	return result, nil
}
