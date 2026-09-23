package scheduler_test

// This file is the worked example the descriptor factories copy: a loopback descriptor
// written against scheduler.Descriptor and vpp.Client, unit-tested with internal/vpp/fake.
// The VPP messages are stand-ins declared below because the generated bindings
// (apps/agent/binapi, task P04) are not merged yet; a real descriptor imports them from
// binapi and uses the generated NewServiceClient(client) instead of raw Invoke/NewStream.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/fake"
)

// ---- stand-ins for binapi messages (names as in interface.api / memclnt.api) -------------

type createLoopbackInstance struct {
	UserInstance uint32
}
type createLoopbackInstanceReply struct {
	Retval    int32
	SwIfIndex uint32
}
type deleteLoopback struct{ SwIfIndex uint32 }
type deleteLoopbackReply struct{ Retval int32 }
type swInterfaceSetMtu struct {
	SwIfIndex uint32
	Mtu       uint32
}
type swInterfaceSetMtuReply struct{ Retval int32 }
type swInterfaceTagAddDel struct {
	IsAdd     bool
	SwIfIndex uint32
	Tag       string
}
type swInterfaceTagAddDelReply struct{ Retval int32 }
type swInterfaceDump struct{}
type swInterfaceDetails struct {
	SwIfIndex     uint32
	InterfaceName string
	Tag           string
	Mtu           uint32
}
type controlPing struct{}
type controlPingReply struct{}

func (*createLoopbackInstance) GetMessageName() string { return "create_loopback_instance" }
func (*createLoopbackInstanceReply) GetMessageName() string {
	return "create_loopback_instance_reply"
}
func (*deleteLoopback) GetMessageName() string            { return "delete_loopback" }
func (*deleteLoopbackReply) GetMessageName() string       { return "delete_loopback_reply" }
func (*swInterfaceSetMtu) GetMessageName() string         { return "sw_interface_set_mtu" }
func (*swInterfaceSetMtuReply) GetMessageName() string    { return "sw_interface_set_mtu_reply" }
func (*swInterfaceTagAddDel) GetMessageName() string      { return "sw_interface_tag_add_del" }
func (*swInterfaceTagAddDelReply) GetMessageName() string { return "sw_interface_tag_add_del_reply" }
func (*swInterfaceDump) GetMessageName() string           { return "sw_interface_dump" }
func (*swInterfaceDetails) GetMessageName() string        { return "sw_interface_details" }
func (*controlPing) GetMessageName() string               { return "control_ping" }
func (*controlPingReply) GetMessageName() string          { return "control_ping_reply" }

func (*createLoopbackInstance) GetCrcString() string      { return "0" }
func (*createLoopbackInstanceReply) GetCrcString() string { return "0" }
func (*deleteLoopback) GetCrcString() string              { return "0" }
func (*deleteLoopbackReply) GetCrcString() string         { return "0" }
func (*swInterfaceSetMtu) GetCrcString() string           { return "0" }
func (*swInterfaceSetMtuReply) GetCrcString() string      { return "0" }
func (*swInterfaceTagAddDel) GetCrcString() string        { return "0" }
func (*swInterfaceTagAddDelReply) GetCrcString() string   { return "0" }
func (*swInterfaceDump) GetCrcString() string             { return "0" }
func (*swInterfaceDetails) GetCrcString() string          { return "0" }
func (*controlPing) GetCrcString() string                 { return "0" }
func (*controlPingReply) GetCrcString() string            { return "0" }

func (*createLoopbackInstance) GetMessageType() api.MessageType      { return api.RequestMessage }
func (*createLoopbackInstanceReply) GetMessageType() api.MessageType { return api.ReplyMessage }
func (*deleteLoopback) GetMessageType() api.MessageType              { return api.RequestMessage }
func (*deleteLoopbackReply) GetMessageType() api.MessageType         { return api.ReplyMessage }
func (*swInterfaceSetMtu) GetMessageType() api.MessageType           { return api.RequestMessage }
func (*swInterfaceSetMtuReply) GetMessageType() api.MessageType      { return api.ReplyMessage }
func (*swInterfaceTagAddDel) GetMessageType() api.MessageType        { return api.RequestMessage }
func (*swInterfaceTagAddDelReply) GetMessageType() api.MessageType   { return api.ReplyMessage }
func (*swInterfaceDump) GetMessageType() api.MessageType             { return api.RequestMessage }
func (*swInterfaceDetails) GetMessageType() api.MessageType          { return api.ReplyMessage }
func (*controlPing) GetMessageType() api.MessageType                 { return api.RequestMessage }
func (*controlPingReply) GetMessageType() api.MessageType            { return api.ReplyMessage }

// ---- the descriptor ------------------------------------------------------------------------

// Loopback desired state. A real descriptor uses the P03 proto type; here a structpb.Struct
// with fields "name" (string, immutable) and "mtu" (number) stands in.
func loopback(name string, mtu uint32) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]any{"name": name, "mtu": float64(mtu)})
	if err != nil {
		panic(err)
	}
	return s
}

func field(obj proto.Message, key string) *structpb.Value {
	return obj.(*structpb.Struct).GetFields()[key]
}

// loopbackDescriptor manages VPP loopback interfaces owned by one agent.
type loopbackDescriptor struct {
	client vpp.Client
	owner  string // VRX_OWNER; tests use their VRX_TEST_PREFIX
}

const loopbackName = "interface.loopback"

func (loopbackDescriptor) Name() string { return loopbackName }

func (loopbackDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(loopbackName, field(obj, "name").GetStringValue())
}

func (loopbackDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// meta is the runtime handle: the sw_if_index VPP assigned.
type loopbackMeta struct{ SwIfIndex uint32 }

func (d loopbackDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	name := field(obj, "name").GetStringValue()
	tag, err := vpp.OwnerTag(d.owner, name)
	if err != nil {
		return nil, err
	}
	var rep createLoopbackInstanceReply
	if err := d.client.Invoke(ctx, &createLoopbackInstance{}, &rep); err != nil {
		return nil, fmt.Errorf("create_loopback_instance: %w", err)
	}
	if err := api.RetvalToVPPApiError(rep.Retval); err != nil {
		return nil, fmt.Errorf("create_loopback_instance: %w", err)
	}
	meta := loopbackMeta{SwIfIndex: rep.SwIfIndex}
	var tagRep swInterfaceTagAddDelReply
	if err := d.client.Invoke(ctx, &swInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag}, &tagRep); err != nil {
		return nil, fmt.Errorf("sw_interface_tag_add_del: %w", err)
	}
	if err := d.setMtu(ctx, rep.SwIfIndex, uint32(field(obj, "mtu").GetNumberValue())); err != nil {
		return nil, err
	}
	return meta, nil
}

func (d loopbackDescriptor) setMtu(ctx context.Context, swIfIndex, mtu uint32) error {
	var rep swInterfaceSetMtuReply
	if err := d.client.Invoke(ctx, &swInterfaceSetMtu{SwIfIndex: swIfIndex, Mtu: mtu}, &rep); err != nil {
		return fmt.Errorf("sw_interface_set_mtu: %w", err)
	}
	return api.RetvalToVPPApiError(rep.Retval)
}

// Update changes the MTU in place; a different name means a different interface, so it
// asks the scheduler to recreate.
func (d loopbackDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if field(oldObj, "name").GetStringValue() != field(newObj, "name").GetStringValue() {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(loopbackMeta)
	if !ok {
		return nil, fmt.Errorf("loopback: unexpected meta %T", meta)
	}
	if err := d.setMtu(ctx, m.SwIfIndex, uint32(field(newObj, "mtu").GetNumberValue())); err != nil {
		return nil, err
	}
	return m, nil
}

func (d loopbackDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(loopbackMeta)
	if !ok {
		return fmt.Errorf("loopback: unexpected meta %T", meta)
	}
	var rep deleteLoopbackReply
	if err := d.client.Invoke(ctx, &deleteLoopback{SwIfIndex: m.SwIfIndex}, &rep); err != nil {
		return fmt.Errorf("delete_loopback: %w", err)
	}
	return api.RetvalToVPPApiError(rep.Retval)
}

// Retrieve dumps all interfaces and keeps the loopbacks tagged by this owner, decoded into the
// desired shape with the sw_if_index in Meta. (The generated SwInterfaceDump client does the
// stream dance below for you.)
func (d loopbackDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	stream, err := d.client.NewStream(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	if err := stream.SendMsg(&swInterfaceDump{}); err != nil {
		return nil, err
	}
	if err := stream.SendMsg(&controlPing{}); err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for {
		msg, err := stream.RecvMsg()
		if err != nil {
			return nil, err
		}
		switch m := msg.(type) {
		case *controlPingReply:
			return out, nil
		case *swInterfaceDetails:
			id, owned := vpp.ParseOwnerTag(m.Tag, d.owner)
			if !owned {
				continue // another owner's interface, or untagged: never ours
			}
			out = append(out, scheduler.KV{
				Key:   scheduler.Join(loopbackName, id),
				Value: loopback(id, m.Mtu),
				Meta:  loopbackMeta{SwIfIndex: m.SwIfIndex},
			})
		default:
			return nil, fmt.Errorf("unexpected message %T", msg)
		}
	}
}

// ---- the fake VPP: a stateful handler set that behaves like the real thing ---------------

type fakeVPP struct {
	*fake.Client
	next   uint32
	ifaces map[uint32]*swInterfaceDetails
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{Client: fake.New(fake.WithControlPingReply(&controlPingReply{})), next: 1, ifaces: map[uint32]*swInterfaceDetails{}}
	v.On("create_loopback_instance", func(api.Message) ([]api.Message, error) {
		idx := v.next
		v.next++
		v.ifaces[idx] = &swInterfaceDetails{SwIfIndex: idx, InterfaceName: fmt.Sprintf("loop%d", idx), Mtu: 9000}
		return []api.Message{&createLoopbackInstanceReply{SwIfIndex: idx}}, nil
	})
	v.On("sw_interface_tag_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*swInterfaceTagAddDel)
		if i, ok := v.ifaces[r.SwIfIndex]; ok {
			i.Tag = r.Tag
		}
		return []api.Message{&swInterfaceTagAddDelReply{}}, nil
	})
	v.On("sw_interface_set_mtu", func(req api.Message) ([]api.Message, error) {
		r := req.(*swInterfaceSetMtu)
		i, ok := v.ifaces[r.SwIfIndex]
		if !ok {
			return []api.Message{&swInterfaceSetMtuReply{Retval: -2}}, nil // VNET_API_ERROR_INVALID_SW_IF_INDEX
		}
		i.Mtu = r.Mtu
		return []api.Message{&swInterfaceSetMtuReply{}}, nil
	})
	v.On("delete_loopback", func(req api.Message) ([]api.Message, error) {
		delete(v.ifaces, req.(*deleteLoopback).SwIfIndex)
		return []api.Message{&deleteLoopbackReply{}}, nil
	})
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(v.ifaces))
		for idx := uint32(0); idx < v.next; idx++ {
			if i, ok := v.ifaces[idx]; ok {
				out = append(out, i)
			}
		}
		return out, nil
	})
	return v
}

// ---- the test ------------------------------------------------------------------------------

func TestExampleLoopbackDescriptor(t *testing.T) {
	ctx := context.Background()
	vppFake := newFakeVPP()
	// Another owner's interface lives on the same VPP; it must be invisible to us.
	vppFake.ifaces[0] = &swInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"}
	vppFake.ifaces[42] = &swInterfaceDetails{SwIfIndex: 42, InterfaceName: "loop42", Tag: "w3:loop300", Mtu: 1500}
	vppFake.next = 43

	d := loopbackDescriptor{client: vppFake, owner: "w2"}
	reg := scheduler.NewRegistry()
	reg.Register(d)
	if got, ok := reg.ForKey(d.KeyOf(loopback("loop200", 9000))); !ok || got.Name() != loopbackName {
		t.Fatal("registry does not route the descriptor's own keys back to it")
	}

	// Create
	desired := loopback("loop200", 9000)
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if meta.(loopbackMeta).SwIfIndex != 43 {
		t.Fatalf("meta = %+v", meta)
	}
	tags := vppFake.CallsNamed("sw_interface_tag_add_del")
	if len(tags) != 1 || tags[0].(*swInterfaceTagAddDel).Tag != "w2:loop200" {
		t.Fatalf("owner tag not applied: %+v", tags)
	}

	// Retrieve == desired, other owners filtered, Meta filled like Create
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 1 {
		t.Fatalf("Retrieve returned %d objects, want only ours: %+v", len(actual), actual)
	}
	if actual[0].Key != "interface.loopback/loop200" || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, want key/value/meta equal to what Create produced", actual[0])
	}

	// Update in place (mtu) and update that needs recreate (name)
	newMeta, err := d.Update(ctx, desired, loopback("loop200", 1500), meta)
	if err != nil || newMeta != meta {
		t.Fatalf("Update mtu: %v, meta %+v", err, newMeta)
	}
	if actual, _ = d.Retrieve(ctx); uint32(field(actual[0].Value, "mtu").GetNumberValue()) != 1500 {
		t.Fatalf("mtu not updated: %+v", actual[0].Value)
	}
	if _, err := d.Update(ctx, desired, loopback("loop201", 1500), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("rename: got %v, want ErrRecreate", err)
	}

	// Delete, then Retrieve shows nothing of ours
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("after Delete Retrieve = %+v", actual)
	}
	if _, ok := vppFake.ifaces[42]; !ok {
		t.Fatal("the other owner's loopback was touched")
	}

	// Errors from VPP surface: disconnected client, non-zero retval
	vppFake.SetConnected(false)
	if _, err := d.Create(ctx, desired); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: got %v", err)
	}
	vppFake.SetConnected(true)
	if err := d.setMtu(ctx, 999, 1500); err == nil {
		t.Fatal("non-zero retval must be an error (api.RetvalToVPPApiError)")
	}
}
