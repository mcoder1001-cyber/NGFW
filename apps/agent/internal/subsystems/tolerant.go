package subsystems

import (
	"context"
	"errors"
	"io"
	"sync"

	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// defaultTolerant wraps a DF-1 attribute descriptor whose object is "present ⇔ differs from the
// interface's creation default" (interface.mtu, interface.rx-mode, D-075). DF-1 rejects a desired
// value equal to that default (ErrMtuDefault / ErrRxModeDefault) because Retrieve could never report
// it. The configuration, however, names values, not deltas: an operator who types MTU 1500 on a NIC
// whose link MTU is 1500 (or rx-mode interrupt on an af_packet interface) asks for a state VPP already
// has. The wrapper turns that into a successful no-op and lets Retrieve report such an object exactly
// while VPP still has that value in effect, so plan, verification and resync converge. Nothing is sent
// to VPP for it; the memory is per process (an agent restart re-learns it on the first resync with the
// same no-op).
type defaultTolerant struct {
	scheduler.Descriptor
	isDefault error
	// inEffect reports whether obj's value is VPP's current (default) value and returns its Meta.
	inEffect func(ctx context.Context, obj proto.Message) (any, bool, error)

	mu  sync.Mutex
	mem map[scheduler.Key]proto.Message
}

func newDefaultTolerant(d scheduler.Descriptor, isDefault error, inEffect func(context.Context, proto.Message) (any, bool, error)) *defaultTolerant {
	return &defaultTolerant{Descriptor: d, isDefault: isDefault, inEffect: inEffect, mem: map[scheduler.Key]proto.Message{}}
}

func (t *defaultTolerant) remember(k scheduler.Key, obj proto.Message) {
	t.mu.Lock()
	t.mem[k] = proto.Clone(obj)
	t.mu.Unlock()
}

func (t *defaultTolerant) forget(k scheduler.Key) {
	t.mu.Lock()
	delete(t.mem, k)
	t.mu.Unlock()
}

// Create implements scheduler.Descriptor.
func (t *defaultTolerant) Create(ctx context.Context, obj proto.Message) (any, error) {
	meta, err := t.Descriptor.Create(ctx, obj)
	if !errors.Is(err, t.isDefault) {
		if err == nil {
			t.forget(t.KeyOf(obj))
		}
		return meta, err
	}
	meta, ok, err := t.inEffect(ctx, obj)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, t.isDefault // a value equal to the default that VPP does not have: let DF-1's error stand
	}
	t.remember(t.KeyOf(obj), obj)
	return meta, nil
}

// Update implements scheduler.Descriptor: a change TO the default value restores the default.
func (t *defaultTolerant) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := t.Descriptor.Update(ctx, oldObj, newObj, meta)
	if !errors.Is(err, t.isDefault) {
		if err == nil {
			t.forget(t.KeyOf(newObj))
		}
		return m, err
	}
	if err := t.Descriptor.Delete(ctx, oldObj, meta); err != nil {
		return nil, err
	}
	m, ok, err := t.inEffect(ctx, newObj)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, t.isDefault
	}
	t.remember(t.KeyOf(newObj), newObj)
	return m, nil
}

// Delete implements scheduler.Descriptor.
func (t *defaultTolerant) Delete(ctx context.Context, obj proto.Message, meta any) error {
	t.mu.Lock()
	_, remembered := t.mem[t.KeyOf(obj)]
	t.mu.Unlock()
	if remembered {
		// nothing was ever set: the interface already has its default
		t.forget(t.KeyOf(obj))
		return nil
	}
	return t.Descriptor.Delete(ctx, obj, meta)
}

// Retrieve implements scheduler.Descriptor: DF-1's objects plus remembered default-valued ones that
// are still in effect.
func (t *defaultTolerant) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	kvs, err := t.Descriptor.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	have := map[scheduler.Key]bool{}
	for _, kv := range kvs {
		have[kv.Key] = true
	}
	t.mu.Lock()
	mem := make(map[scheduler.Key]proto.Message, len(t.mem))
	for k, v := range t.mem {
		mem[k] = v
	}
	t.mu.Unlock()
	for k, obj := range mem {
		if have[k] {
			continue
		}
		meta, ok, err := t.inEffect(ctx, obj)
		if err != nil {
			return nil, err
		}
		if ok {
			kvs = append(kvs, scheduler.KV{Key: k, Value: proto.Clone(obj), Meta: meta})
		}
	}
	return kvs, nil
}

// Normalize forwards DF-1's Normalizer (scheduler extension).
func (t *defaultTolerant) Normalize(obj proto.Message) proto.Message {
	if n, ok := t.Descriptor.(scheduler.Normalizer); ok {
		return n.Normalize(obj)
	}
	return obj
}

// resolveOwned returns the dump row of the interface ref names when a per-interface object of holder
// on it is ours (our tag, or untagged — the claim is made by DF-1 on its own successful Create; a
// remembered default value needs none because nothing was changed).
func resolveOwned(ctx context.Context, c vpp.Client, owner, ref string) (uint32, *ifapi.SwInterfaceDetails, bool, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return 0, nil, false, err
	}
	idx, err := t.Index(ref)
	if err != nil {
		return 0, nil, false, nil //nolint:nilerr // a vanished interface: not in effect
	}
	det, ok := t.Details(idx)
	return idx, det, ok, nil
}

// mtuInEffect: obj's per-protocol MTU equals the interface's creation default and VPP has exactly it.
func mtuInEffect(c vpp.Client, owner string) func(context.Context, proto.Message) (any, bool, error) {
	return func(ctx context.Context, obj proto.Message) (any, bool, error) {
		o, ok := obj.(*iface.Mtu)
		if !ok {
			return nil, false, nil
		}
		idx, det, ok, err := resolveOwned(ctx, c, owner, o.GetInterface())
		if err != nil || !ok {
			return nil, false, err
		}
		def := [4]uint32{uint32(det.LinkMtu), 0, 0, 0}
		if iface.Kind(det) == iface.SubinterfaceName {
			def = [4]uint32{}
		}
		var cur [4]uint32
		copy(cur[:], det.Mtu)
		want := [4]uint32{o.GetMtu(), o.GetIp4(), o.GetIp6(), o.GetMpls()}
		return iface.Meta{SwIfIndex: idx}, want == def && cur == def, nil
	}
}

// rxModeInEffect: every queue of the interface runs obj's mode, which is the device class default.
func rxModeInEffect(c vpp.Client, owner string) func(context.Context, proto.Message) (any, bool, error) {
	return func(ctx context.Context, obj proto.Message) (any, bool, error) {
		o, ok := obj.(*iface.RxMode)
		if !ok {
			return nil, false, nil
		}
		idx, det, ok, err := resolveOwned(ctx, c, owner, o.GetInterface())
		if err != nil || !ok {
			return nil, false, err
		}
		def := iface.RxModeKind_RX_MODE_KIND_POLLING
		if iface.Kind(det) == iface.HostInterfaceName {
			def = iface.RxModeKind_RX_MODE_KIND_INTERRUPT
		}
		if o.GetMode() != def {
			return nil, false, nil
		}
		stream, err := ifapi.NewServiceClient(c).SwInterfaceRxPlacementDump(ctx, &ifapi.SwInterfaceRxPlacementDump{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if err != nil {
			return nil, false, err
		}
		queues := 0
		for {
			p, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, false, err
			}
			if uint32(p.SwIfIndex) != idx {
				continue
			}
			queues++
			if rxKind(p.Mode) != def {
				return nil, false, nil
			}
		}
		return iface.Meta{SwIfIndex: idx}, queues > 0, nil
	}
}

func rxKind(m interface_types.RxMode) iface.RxModeKind {
	switch m {
	case interface_types.RX_MODE_API_INTERRUPT:
		return iface.RxModeKind_RX_MODE_KIND_INTERRUPT
	case interface_types.RX_MODE_API_ADAPTIVE:
		return iface.RxModeKind_RX_MODE_KIND_ADAPTIVE
	}
	return iface.RxModeKind_RX_MODE_KIND_POLLING
}
