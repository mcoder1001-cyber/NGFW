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
	"ngfw/agent/internal/vpp/bootid"
)

// KeyStatsEnable is the key of the singleton: "acl.stats-enable/global".
var KeyStatsEnable = scheduler.Join(NameStatsEnable, StatsEnableID)

// StatsEnableDescriptor manages the acl.stats-enable singleton (acl_stats_intf_counters_enable).
//
// The flag is process-global in VPP and has no read API (only the VPP CLI `show acl-plugin tables` prints
// it), so Retrieve reports the last value this descriptor applied *to the running VPP process*:
// the value is stored together with the D-080 VPP boot identity (bootid.Current: kernel boot_id,
// VPP PID, VPP start time; read before the request is sent) and Retrieve reports nothing once the
// identity differs — after `restart-vpp` / `kill -9 vpp` the scheduler sees the object missing and enables
// the counters again. On a fresh agent Retrieve also reports nothing and the desired value is
// re-applied once (enabling twice is harmless). Reset forgets the applied value explicitly (for a
// reconnect hook).
//
// The descriptor never disables the counters: on a shared VPP another owner may depend on
// them, and Delete of the singleton is therefore a no-op. Enabled = false means "not managed by
// this agent", not "off": it is recorded but not sent, and Retrieve then reports false whatever
// the flag is on VPP.
type StatsEnableDescriptor struct {
	client  vpp.Client
	mu      sync.Mutex
	applied *StatsEnable
	vppID   bootid.Identity // D-080 VPP boot identity the applied value belongs to
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
	// identity first: if VPP restarts after this read, Retrieve sees a new identity and the
	// counters are enabled again; the reverse order could record a stale "enabled".
	id, err := bootid.Current(ctx, d.client)
	if err != nil {
		return fmt.Errorf("acl.stats-enable: %w", err)
	}
	if s.Enabled {
		if err := EnableCounters(ctx, d.client); err != nil {
			return err
		}
	}
	d.mu.Lock()
	d.applied = &s
	d.vppID = id
	d.mu.Unlock()
	return nil
}

// Reset forgets the applied value, so the next Retrieve reports nothing and the scheduler
// re-applies the desired value. Retrieve already does this on its own when VPP's identity
// changes; P05 may also call it on every binary-API reconnect.
func (d *StatsEnableDescriptor) Reset() {
	d.mu.Lock()
	d.applied = nil
	d.mu.Unlock()
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

// Retrieve implements scheduler.Descriptor: the last value this process applied to the running
// VPP, or nothing (never applied, Reset, or VPP restarted since — see the type doc).
func (d *StatsEnableDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	d.mu.Lock()
	applied, appliedID := d.applied, d.vppID
	d.mu.Unlock()
	if applied == nil {
		return nil, nil
	}
	id, err := bootid.Current(ctx, d.client)
	if err != nil {
		return nil, fmt.Errorf("acl.stats-enable: %w", err)
	}
	if !id.Equal(appliedID) {
		d.mu.Lock()
		if d.applied == applied { // not re-applied concurrently
			d.applied = nil
		}
		d.mu.Unlock()
		return nil, nil
	}
	return []scheduler.KV{{Key: KeyStatsEnable, Value: applied.Proto()}}, nil
}
