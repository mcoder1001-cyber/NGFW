package acl

import (
	"context"
	"fmt"
	"sync"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/acl"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// KeyStatsEnable is the key of the singleton: "acl.stats-enable/global".
var KeyStatsEnable = scheduler.Join(NameStatsEnable, StatsEnableID)

// StatsEnableDescriptor manages the acl.stats-enable singleton (acl_stats_intf_counters_enable).
//
// The flag is process-global in VPP and has no read API (only `show acl-plugin tables` prints
// it), so Retrieve reports the last value this descriptor applied: on a fresh agent it reports
// nothing and the scheduler re-applies the desired value once (enabling twice is harmless).
// The descriptor never disables the counters: on a shared VPP another owner may depend on
// them, and Delete of the singleton is therefore a no-op. Enabled = false is recorded but not
// sent.
type StatsEnableDescriptor struct {
	client  vpp.Client
	mu      sync.Mutex
	applied *StatsEnable
}

var _ scheduler.Descriptor = (*StatsEnableDescriptor)(nil)

// NewStatsEnable returns the acl.stats-enable descriptor.
func NewStatsEnable(client vpp.Client) *StatsEnableDescriptor {
	return &StatsEnableDescriptor{client: client}
}

// Name implements scheduler.Descriptor.
func (*StatsEnableDescriptor) Name() string { return NameStatsEnable }

// KeyOf implements scheduler.Descriptor: always the singleton key.
func (*StatsEnableDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyStatsEnable }

// Dependencies implements scheduler.Descriptor: none.
func (*StatsEnableDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// EnableCounters sends acl_stats_intf_counters_enable(enable=true). It is exported so tests and
// tools can switch the counters on without the scheduler. VPP 26.06 answers this request with
// the acl_del_reply message id (acl.c, REPLY_MACRO (VL_API_ACL_DEL_REPLY) in the handler), which
// the generated Invoke would reject, so the reply is read on a raw stream and both reply types
// are accepted.
func EnableCounters(ctx context.Context, client vpp.Client) error {
	return setCounters(ctx, client, true)
}

// setCounters sends acl_stats_intf_counters_enable(enable) on a raw stream (see EnableCounters).
// Production code only ever passes true; the integration test uses false to restore the host.
func setCounters(ctx context.Context, client vpp.Client, enable bool) error {
	stream, err := client.NewStream(ctx)
	if err != nil {
		return fmt.Errorf("acl_stats_intf_counters_enable: %w", err)
	}
	defer func() { _ = stream.Close() }()
	if err := stream.SendMsg(&acl.ACLStatsIntfCountersEnable{Enable: enable}); err != nil {
		return fmt.Errorf("acl_stats_intf_counters_enable: %w", err)
	}
	msg, err := stream.RecvMsg()
	if err != nil {
		return fmt.Errorf("acl_stats_intf_counters_enable: %w", err)
	}
	var retval int32
	switch m := msg.(type) {
	case *acl.ACLStatsIntfCountersEnableReply:
		retval = m.Retval
	case *acl.ACLDelReply: // VPP 26.06 replies with this id (acl.c handler uses VL_API_ACL_DEL_REPLY)
		retval = m.Retval
	default:
		return fmt.Errorf("acl_stats_intf_counters_enable: unexpected reply %T", msg)
	}
	if err := api.RetvalToVPPApiError(retval); err != nil {
		return fmt.Errorf("acl_stats_intf_counters_enable: %w", err)
	}
	return nil
}

func (d *StatsEnableDescriptor) apply(ctx context.Context, obj proto.Message) error {
	s, err := StatsEnableFromProto(obj)
	if err != nil {
		return err
	}
	if s.Enabled {
		if err := EnableCounters(ctx, d.client); err != nil {
			return err
		}
	}
	d.mu.Lock()
	d.applied = &s
	d.mu.Unlock()
	return nil
}

// Create implements scheduler.Descriptor. Meta is nil (there is no handle).
func (d *StatsEnableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor.
func (d *StatsEnableDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor: forgets the applied value; the counters stay as they
// are on VPP (never disabled, see the type doc).
func (d *StatsEnableDescriptor) Delete(context.Context, proto.Message, any) error {
	d.mu.Lock()
	d.applied = nil
	d.mu.Unlock()
	return nil
}

// Retrieve implements scheduler.Descriptor: the last applied value of this process, or nothing.
func (d *StatsEnableDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.applied == nil {
		return nil, nil
	}
	return []scheduler.KV{{Key: KeyStatsEnable, Value: d.applied.Proto()}}, nil
}
