package ifsanitize_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// swPool models VPP's sw_interface pool (LIFO free list, like every vppinfra pool) and the
// loopback instance bitmap (loopback_instance_alloc).
type swPool struct {
	free []uint32
	next uint32
	tags map[uint32]string
	down map[uint32]bool
	// inst maps a loopback instance in use to its sw_if_index; name is the loopback's name by index
	inst map[uint32]uint32
	name map[uint32]string
}

func newPool(next uint32, free ...uint32) *swPool {
	return &swPool{next: next, free: free, tags: map[uint32]string{}, down: map[uint32]bool{}, inst: map[uint32]uint32{}, name: map[uint32]string{}}
}

func (p *swPool) get() uint32 {
	if n := len(p.free); n > 0 {
		i := p.free[n-1]
		p.free = p.free[:n-1]
		return i
	}
	p.next++
	return p.next - 1
}

func (p *swPool) put(i uint32) { p.free = append(p.free, i) }

func (p *swPool) install(f *fake.Client) {
	f.On("create_loopback", func(api.Message) ([]api.Message, error) {
		for i := uint32(0); ; i++ { // VPP: the lowest free instance
			if _, used := p.inst[i]; !used {
				idx := p.get()
				p.inst[i], p.name[idx] = idx, fmt.Sprintf("loop%d", i)
				return []api.Message{&interfaces.CreateLoopbackReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
			}
		}
	})
	f.On("create_loopback_instance", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.CreateLoopbackInstance)
		if _, used := p.inst[r.UserInstance]; used || !r.IsSpecified || r.UserInstance >= 16384 {
			return nil, api.VPPApiError(-31) // INVALID_REGISTRATION, before any allocation
		}
		idx := p.get()
		p.inst[r.UserInstance], p.name[idx] = idx, fmt.Sprintf("loop%d", r.UserInstance)
		return []api.Message{&interfaces.CreateLoopbackInstanceReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	f.On("delete_loopback", func(req api.Message) ([]api.Message, error) {
		idx := uint32(req.(*interfaces.DeleteLoopback).SwIfIndex)
		for i, at := range p.inst {
			if at == idx {
				delete(p.inst, i)
			}
		}
		delete(p.name, idx)
		p.put(idx)
		return []api.Message{&interfaces.DeleteLoopbackReply{}}, nil
	})
	f.On("sw_interface_tag_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceTagAddDel)
		p.tags[uint32(r.SwIfIndex)] = r.Tag
		return []api.Message{&interfaces.SwInterfaceTagAddDelReply{}}, nil
	})
	f.On("sw_interface_set_flags", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceSetFlags)
		p.down[uint32(r.SwIfIndex)] = r.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP == 0
		return []api.Message{&interfaces.SwInterfaceSetFlagsReply{}}, nil
	})
}

// TestAcquireQuarantinesAnUnclearableIndex is TD-3 review H1 (c): an index whose stale binding
// cannot be removed is never reported created — it is held by an admin-down "quarantine:<owner>"
// loopback and the interface is created again on a fresh index.
func TestAcquireQuarantinesAnUnclearableIndex(t *testing.T) {
	f, m := setup()
	p := newPool(5, 4)
	p.install(f)
	m.Tables[3] = true
	m.If(4).InACL = [3]uint32{9, none, none}
	m.DeleteTable(9)
	defer ifsanitize.DisableResurrect()() // resurrection impossible, not capped

	before := ifsanitize.Snapshot()
	var created, deleted []uint32
	idx, err := ifsanitize.Acquire(context.Background(), f, "w2", "tap7",
		func() (uint32, error) { i := p.get(); created = append(created, i); return i, nil },
		func(i uint32) error { deleted = append(deleted, i); p.put(i); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if idx != 5 || len(created) != 2 || created[0] != 4 || len(deleted) != 1 || deleted[0] != 4 {
		t.Fatalf("idx %d created %v deleted %v", idx, created, deleted)
	}
	if p.tags[4] != "quarantine:w2" || !p.down[4] || !ifsanitize.IsQuarantineTag(p.tags[4]) {
		t.Fatalf("holder of 4: tag %q down %v", p.tags[4], p.down[4])
	}
	// TD-3 re-review M2: the holder is the top of the reserved range, never loop0
	if p.name[4] != "loop16383" || len(f.CallsNamed("create_loopback")) != 0 {
		t.Fatalf("holder of 4 is %q (create_loopback sent %d times)", p.name[4], len(f.CallsNamed("create_loopback")))
	}
	if d := m.Dirty(5); d != "" {
		t.Fatalf("fresh index dirty: %s", d)
	}
	after := ifsanitize.Snapshot()
	if after.Quarantined != before.Quarantined+1 || after.QuarantineTotal != before.QuarantineTotal+1 {
		t.Fatalf("quarantine counters %+v → %+v", before, after)
	}
}

// TestAcquireFailsWithoutACleanIndex: every index dirty → Create fails, nothing is reported.
func TestAcquireFailsWithoutACleanIndex(t *testing.T) {
	f, m := setup()
	p := newPool(100)
	p.install(f)
	m.DeleteTable(9)
	for i := uint32(100); i < 120; i++ {
		m.If(i).InACL = [3]uint32{9, none, none}
	}
	defer ifsanitize.DisableResurrect()()
	_, err := ifsanitize.Acquire(context.Background(), f, "w2", "tap8",
		func() (uint32, error) { return p.get(), nil },
		func(i uint32) error { p.put(i); return nil })
	if !errors.Is(err, ifsanitize.ErrNoCleanIndex) {
		t.Fatalf("err = %v", err)
	}
}

// TestAcquireOtherErrorsDeleteAndFail: a sanitize API failure removes the interface and fails
// without a quarantine.
func TestAcquireOtherErrorsDeleteAndFail(t *testing.T) {
	f, _ := setup()
	p := newPool(3)
	p.install(f)
	f.Fail("classify_table_ids", errors.New("socket closed"))
	var deleted []uint32
	_, err := ifsanitize.Acquire(context.Background(), f, "w2", "tap9",
		func() (uint32, error) { return p.get(), nil },
		func(i uint32) error { deleted = append(deleted, i); return nil })
	if err == nil || errors.Is(err, ifsanitize.ErrUnclearable) || len(deleted) != 1 || len(f.CallsNamed("create_loopback")) != 0 {
		t.Fatalf("err %v deleted %v", err, deleted)
	}
}

// TestQuarantineHolderInstance is TD-3 re-review M2: the holder takes the highest free instance of
// the reserved range loop16000–loop16383 — a user's loop16383 (or an earlier holder) is skipped,
// the user's loop0/loop1 are never touched — and still lands on the dirty sw_if_index.
func TestQuarantineHolderInstance(t *testing.T) {
	f, _ := setup()
	p := newPool(10)
	p.install(f)
	svc := interfaces.NewServiceClient(f)
	for _, inst := range []uint32{0, 1, 16383} { // the user's loopbacks
		if _, err := svc.CreateLoopbackInstance(context.Background(), &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst}); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range []string{"loop16382", "loop16381"} {
		dirty := p.get() // an interface on a fresh index, deleted because it is dirty
		p.put(dirty)
		if err := ifsanitize.Quarantine(context.Background(), f, "w2", dirty); err != nil {
			t.Fatalf("quarantine %d: %v", i, err)
		}
		if p.name[dirty] != want || p.tags[dirty] != "quarantine:w2" || !p.down[dirty] {
			t.Fatalf("holder of %d: %q tag %q down %v, want %s", dirty, p.name[dirty], p.tags[dirty], p.down[dirty], want)
		}
	}
	// a user's loop0 can be deleted and created again: no holder is in its way
	if _, used := p.inst[0]; !used {
		t.Fatal("loop0 lost")
	}
	if ifsanitize.QuarantineInstanceMin != 16000 || ifsanitize.QuarantineInstanceMax != 16383 {
		t.Fatalf("reserved range %d–%d", ifsanitize.QuarantineInstanceMin, ifsanitize.QuarantineInstanceMax)
	}
}

// TestQuarantineReservedRangeFull: when every reserved instance is taken the quarantine fails
// (the create fails) instead of falling back to VPP's lowest free instance.
func TestQuarantineReservedRangeFull(t *testing.T) {
	f, _ := setup()
	p := newPool(0)
	p.install(f)
	for i := ifsanitize.QuarantineInstanceMin; i <= ifsanitize.QuarantineInstanceMax; i++ {
		p.inst[i] = 1 << 20
	}
	err := ifsanitize.Quarantine(context.Background(), f, "w2", 0)
	if err == nil || !strings.Contains(err.Error(), "every reserved instance") || len(f.CallsNamed("create_loopback")) != 0 {
		t.Fatalf("err %v", err)
	}
}

// TestAcquireCappedFailsClosed is TD-3 re-review M1: a classify pool whose free list needs more
// placeholders than the cap (80 freed indices: more than MaxPlaceholders, 64) fails the create closed (ErrCapped, which is ErrNoCleanIndex):
// the interface is deleted, nothing is reported created, there is no retry, the cap hit is
// counted, and no quarantine holder is made for an index that is not known to be dirty.
func TestAcquireCappedFailsClosed(t *testing.T) {
	f, m := setup()
	p := newPool(20)
	p.install(f)
	for id := uint32(0); id < 80; id++ {
		m.Tables[id] = true
	}
	for id := uint32(0); id < 80; id++ { // deleted in creation order: pops 79, 78, … (never ascending)
		m.DeleteTable(id)
	}
	before := ifsanitize.Snapshot()
	var created, deleted []uint32
	_, err := ifsanitize.Acquire(context.Background(), f, "w2", "tap10",
		func() (uint32, error) { i := p.get(); created = append(created, i); return i, nil },
		func(i uint32) error { deleted = append(deleted, i); p.put(i); return nil })
	if !errors.Is(err, ifsanitize.ErrCapped) || !errors.Is(err, ifsanitize.ErrNoCleanIndex) || errors.Is(err, ifsanitize.ErrUnclearable) {
		t.Fatalf("err = %v", err)
	}
	if len(created) != 1 || len(deleted) != 1 || len(p.tags) != 0 {
		t.Fatalf("created %v deleted %v holders %v", created, deleted, p.tags)
	}
	after := ifsanitize.Snapshot()
	if after.Capped["create"] != before.Capped["create"]+1 || after.Errors["create"] != before.Errors["create"]+1 || after.QuarantineTotal != before.QuarantineTotal {
		t.Fatalf("counters %+v → %+v", before, after)
	}
	if n := m.Created; n != ifsanitize.MaxPlaceholders || len(m.Tables) != 0 {
		t.Fatalf("placeholders created %d (cap %d), tables left %v", n, ifsanitize.MaxPlaceholders, m.Tables)
	}
	var b bytes.Buffer
	ifsanitize.WriteMetrics(&b)
	if !strings.Contains(b.String(), `vrx_agent_iface_sanitize_capped_total{phase="create"}`) {
		t.Fatalf("metrics lack the capped counter:\n%s", b.String())
	}
}

// TestAcquireCappedAndUnclearable: a capped run that also proves a binding unclearable (an input
// ACL naming a freed table that was not reached) quarantines that index once and fails without a
// retry — the next index would be capped as well, so retrying would only make more holders.
func TestAcquireCappedAndUnclearable(t *testing.T) {
	f, m := setup()
	p := newPool(8, 7)
	p.install(f)
	for id := uint32(0); id < 80; id++ {
		m.Tables[id] = true
	}
	for id := uint32(0); id < 80; id++ {
		m.DeleteTable(id)
	}
	m.If(7).InACL = [3]uint32{0, none, none} // table 0 is at the bottom of the free list: never reached
	_, err := ifsanitize.Acquire(context.Background(), f, "w2", "tap11",
		func() (uint32, error) { return p.get(), nil },
		func(i uint32) error { p.put(i); return nil })
	if !errors.Is(err, ifsanitize.ErrCapped) || !errors.Is(err, ifsanitize.ErrUnclearable) {
		t.Fatalf("err = %v", err)
	}
	if len(p.tags) != 1 || p.tags[7] != "quarantine:w2" || p.name[7] != "loop16383" {
		t.Fatalf("holders %v names %v", p.tags, p.name)
	}
}
