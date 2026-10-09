package pppoe

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
)

const CarrierNamespaceName = "pppoe.carrier.namespace"

// CarrierLease is verified by the helper against the pinned live namespace, not
// a ready flag copied from disk. Configured is kernel preparation only; VPP
// forwarding readiness requires additional policy/route/link verification.
type CarrierLease struct {
	Spec           ren.CarrierSpec `json:"spec"`
	Token          string          `json:"token"`
	Generation     string          `json:"generation"`
	Boot           string          `json:"boot"`
	Namespace      []uint64        `json:"namespace"`
	Configured     bool            `json:"configured"`
	RepairRequired bool            `json:"repair_required"`
}

// CarrierNamespaceHost is implemented by the packaged namespace helper adapter.
// Implementations must validate inode/boot/generation ownership on every call.
type CarrierNamespaceHost interface {
	Provision(context.Context, ren.CarrierSpec) (CarrierLease, error)
	Inventory(context.Context, string) ([]CarrierLease, error)
	Remove(context.Context, CarrierLease) error
}

// NamespaceDescriptor owns only namespace lifetime. Its TAP and daemon consumers
// depend on this key, so scheduler rollback/teardown removes them first.
type NamespaceDescriptor struct {
	owner string
	host  CarrierNamespaceHost
	// Admit checks observed VPP ownership/topology before a namespace is created.
	Admit func(context.Context, ren.CarrierSpec) error
}

func NewCarrierNamespace(owner string, host CarrierNamespaceHost) *NamespaceDescriptor {
	return &NamespaceDescriptor{owner: owner, host: host}
}

// Namespace ownership must survive an agent restart. The packaged host stores
// exact boot/inode/generation receipts on disk; an in-memory host is not valid in
// product wiring. A host reboot destroys the namespace along with its /run ledger.
func (d *NamespaceDescriptor) CheckPersistent() error {
	return persist.Require(CarrierNamespaceName, d.host)
}

func (*NamespaceDescriptor) Name() string { return CarrierNamespaceName }
func (*NamespaceDescriptor) KeyOf(value proto.Message) scheduler.Key {
	var spec ren.CarrierSpec
	if decodeCarrierNamespace(value, &spec) != nil {
		return ""
	}
	return CarrierNamespaceKey(spec.Token())
}
func CarrierNamespaceKey(token string) scheduler.Key {
	return scheduler.Join(CarrierNamespaceName, token)
}
func (*NamespaceDescriptor) Dependencies(value proto.Message) []scheduler.Dependency {
	var spec ren.CarrierSpec
	if decodeCarrierNamespace(value, &spec) != nil {
		return nil
	}
	return []scheduler.Dependency{{Key: iface.AliasKey(spec.Parent)}}
}

func (d *NamespaceDescriptor) spec(value proto.Message) (ren.CarrierSpec, error) {
	var spec ren.CarrierSpec
	if err := decodeCarrierNamespace(value, &spec); err != nil {
		return spec, err
	}
	if err := spec.Validate(); err != nil {
		return spec, err
	}
	if spec.Owner != d.owner {
		return spec, errors.New("PPPoE carrier belongs to another owner")
	}
	if d.host == nil {
		return spec, errors.New("PPPoE carrier namespace helper is unavailable")
	}
	return spec, nil
}

var carrierGeneration = regexp.MustCompile(`^[a-f0-9]{32,64}$`)

func (l CarrierLease) Validate() error {
	if err := l.Spec.Validate(); err != nil {
		return err
	}
	if l.Token != l.Spec.Token() || !carrierGeneration.MatchString(l.Generation) || l.Boot == "" || len(l.Namespace) != 2 || l.Namespace[0] == 0 || l.Namespace[1] == 0 {
		return errors.New("PPPoE carrier namespace identity receipt is incomplete")
	}
	return nil
}

func (d *NamespaceDescriptor) Create(ctx context.Context, value proto.Message) (any, error) {
	spec, err := d.spec(value)
	if err != nil {
		return nil, err
	}
	if d.Admit == nil {
		return nil, errors.New("PPPoE carrier VPP admission is unavailable")
	}
	if err := d.Admit(ctx, spec); err != nil {
		return nil, err
	}
	lease, err := d.host.Provision(ctx, spec)
	if err != nil {
		return nil, err
	}
	if err := lease.Validate(); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(lease.Spec, spec) {
		return nil, errors.New("PPPoE carrier helper returned a different namespace specification")
	}
	return lease, nil
}

func (d *NamespaceDescriptor) Update(_ context.Context, old, next proto.Message, meta any) (any, error) {
	if _, err := d.spec(next); err != nil {
		return nil, err
	}
	if !proto.Equal(old, next) {
		return nil, scheduler.ErrRecreate
	}
	return meta, nil
}

func (d *NamespaceDescriptor) Delete(ctx context.Context, value proto.Message, meta any) error {
	spec, err := d.spec(value)
	if err != nil {
		return err
	}
	lease, ok := meta.(CarrierLease)
	if !ok || lease.Validate() != nil || !reflect.DeepEqual(lease.Spec, spec) {
		return errors.New("PPPoE carrier removal requires the exact observed lease")
	}
	return d.host.Remove(ctx, lease)
}

func (d *NamespaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if d.host == nil {
		return nil, errors.New("PPPoE carrier namespace helper is unavailable")
	}
	leases, err := d.host.Inventory(ctx, d.owner)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]scheduler.KV, 0, len(leases))
	for _, lease := range leases {
		if err := lease.Validate(); err != nil {
			return nil, err
		}
		if lease.Spec.Owner != d.owner || seen[lease.Token] {
			return nil, fmt.Errorf("PPPoE carrier inventory contains a foreign or duplicate namespace")
		}
		seen[lease.Token] = true
		value := dfkit.Encode(lease.Spec)
		if lease.RepairRequired {
			value.Fields["_repair_required"] = structpb.NewBoolValue(true)
		}
		out = append(out, scheduler.KV{Key: CarrierNamespaceKey(lease.Token), Value: value, Meta: lease})
	}
	return out, nil
}

// The retrieved-only repair marker forces a dependency-safe full recreation.
// It is never part of the immutable helper specification or desired projection.
func decodeCarrierNamespace(value proto.Message, spec *ren.CarrierSpec) error {
	if doc, ok := value.(*structpb.Struct); ok && doc.Fields["_repair_required"] != nil {
		if !doc.Fields["_repair_required"].GetBoolValue() {
			return errors.New("invalid carrier repair marker")
		}
		copy := proto.Clone(doc).(*structpb.Struct)
		delete(copy.Fields, "_repair_required")
		return dfkit.Decode(copy, spec)
	}
	return dfkit.Decode(value, spec)
}
