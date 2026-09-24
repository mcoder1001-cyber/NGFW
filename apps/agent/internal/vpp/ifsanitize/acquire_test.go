package ifsanitize_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// swPool models VPP's sw_interface pool (LIFO free list, like every vppinfra pool).
type swPool struct {
	free []uint32
	next uint32
	tags map[uint32]string
	down map[uint32]bool
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
		return []api.Message{&interfaces.CreateLoopbackReply{SwIfIndex: interface_types.InterfaceIndex(p.get())}}, nil
	})
	f.On("delete_loopback", func(req api.Message) ([]api.Message, error) {
		p.put(uint32(req.(*interfaces.DeleteLoopback).SwIfIndex))
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
	p := &swPool{next: 5, free: []uint32{4}, tags: map[uint32]string{}, down: map[uint32]bool{}}
	p.install(f)
	m.Tables[3] = true
	m.If(4).InACL = [3]uint32{9, none, none}
	m.DeleteTable(9)
	defer func(n int) { ifsanitize.MaxPlaceholders = n }(ifsanitize.MaxPlaceholders)
	ifsanitize.MaxPlaceholders = 0 // resurrection impossible

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
	p := &swPool{next: 100, tags: map[uint32]string{}, down: map[uint32]bool{}}
	p.install(f)
	m.DeleteTable(9)
	for i := uint32(100); i < 120; i++ {
		m.If(i).InACL = [3]uint32{9, none, none}
	}
	defer func(n int) { ifsanitize.MaxPlaceholders = n }(ifsanitize.MaxPlaceholders)
	ifsanitize.MaxPlaceholders = 0
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
	p := &swPool{next: 3, tags: map[uint32]string{}, down: map[uint32]bool{}}
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
