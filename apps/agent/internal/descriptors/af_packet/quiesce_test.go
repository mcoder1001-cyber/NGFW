package afpacket_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"golang.org/x/sys/unix"

	afpapi "ngfw/agent/binapi/af_packet"
	interfaces "ngfw/agent/binapi/interface"
	afpacket "ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp/fake"
)

// fakeLinks is the Linux side of the quiesce (afpacket.Links) for unit tests: netdevs by name, and
// every link-down and settle recorded with the number of VPP requests sent before it, so the order
// against the fake VPP's call log can be asserted.
type fakeLinks struct {
	mu     sync.Mutex
	vpp    *fake.Client
	next   int32
	devs   map[string]*fakeDev
	events []linkEvent
	// injected failures
	lookupErr error // every Lookup
	downErr   error // every SetDown
	upErr     error // every SetUp
	stuckUp   bool  // SetDown answers ok, the link stays up
	vanish    bool  // the netdev disappears between Lookup and SetDown (ENODEV)
}

type fakeDev struct {
	index int32
	up    bool
	kind  string // "veth" unless a test changes it
}

type linkEvent struct {
	at   int // len(vpp.Calls()) when it happened
	what string
}

func newFakeLinks(vpp *fake.Client) *fakeLinks {
	return &fakeLinks{vpp: vpp, next: 10, devs: map[string]*fakeDev{}}
}

func (l *fakeLinks) options() []afpacket.Option {
	return []afpacket.Option{afpacket.WithLinks(l), afpacket.WithSettle(func(context.Context) error { l.record("settle"); return nil })}
}

func (l *fakeLinks) record(what string) {
	at := len(l.vpp.Calls())
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, linkEvent{at, what})
}

// setUp makes name exist and be up.
func (l *fakeLinks) setUp(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	d, ok := l.devs[name]
	if !ok {
		l.next++
		d = &fakeDev{index: l.next, kind: afpacket.VethKind}
		l.devs[name] = d
	}
	d.up = true
}

func (l *fakeLinks) Lookup(name string) (afpacket.Netdev, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lookupErr != nil {
		return afpacket.Netdev{}, l.lookupErr
	}
	d, ok := l.devs[name]
	if !ok {
		return afpacket.Netdev{}, fmt.Errorf("RTM_GETLINK %s: %w", name, unix.ENODEV)
	}
	return afpacket.Netdev{Index: d.index, Up: d.up, Kind: d.kind}, nil
}

func (l *fakeLinks) name(index int32) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for n, d := range l.devs {
		if d.index == index {
			return n
		}
	}
	return ""
}

// SetUp (review L1: only after a failed delete) is recorded as link-up(<dev>).
func (l *fakeLinks) SetUp(index int32) error {
	name := l.name(index)
	if l.upErr != nil {
		return l.upErr
	}
	if name == "" {
		return unix.ENODEV
	}
	l.record("link-up(" + name + ")")
	l.mu.Lock()
	defer l.mu.Unlock()
	l.devs[name].up = true
	return nil
}

func (l *fakeLinks) SetDown(index int32) error {
	name := l.name(index)
	if l.downErr != nil {
		return l.downErr
	}
	if l.vanish {
		l.mu.Lock()
		delete(l.devs, name)
		l.mu.Unlock()
		return fmt.Errorf("RTM_NEWLINK ifindex %d: %w", index, unix.ENODEV)
	}
	if name == "" {
		return unix.ENODEV
	}
	l.record("link-down(" + name + ")")
	l.mu.Lock()
	defer l.mu.Unlock()
	l.devs[name].up = l.stuckUp
	return nil
}

func (l *fakeLinks) up(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	d, ok := l.devs[name]
	return ok && d.up
}

// order merges the fake VPP's call log with the link events: VPP requests by message name,
// af_packet_create_v3 / af_packet_delete with their host_if_name.
func (f *fakeAfp) order() []string {
	f.links.mu.Lock()
	ev := slices.Clone(f.links.events)
	f.links.mu.Unlock()
	var out []string
	emit := func(i int) {
		for len(ev) > 0 && ev[0].at <= i {
			out = append(out, ev[0].what)
			ev = ev[1:]
		}
	}
	for i, c := range f.Calls() {
		emit(i)
		switch r := c.(type) {
		case *afpapi.AfPacketDelete:
			out = append(out, "af_packet_delete("+r.HostIfName+")")
		case *afpapi.AfPacketCreateV3:
			out = append(out, "af_packet_create_v3("+r.HostIfName+")")
		default:
			out = append(out, c.GetMessageName())
		}
	}
	emit(len(f.Calls()) + 1)
	return out
}

// pos returns the index of the first entry equal to want at or after from (-1: absent).
func pos(seq []string, want string, from int) int {
	for i := max(from, 0); i < len(seq); i++ {
		if seq[i] == want {
			return i
		}
	}
	return -1
}

// ascending reports whether every position is present (≥ 0) and each comes after the one before.
func ascending(p ...int) bool {
	prev := -1
	for _, x := range p {
		if x <= prev {
			return false
		}
		prev = x
	}
	return true
}

// TestDeleteQuiescesFirst: Delete sends link-down(<dev>), waits the settle, then clears the
// bindings (ifsanitize.BeforeDelete, first message sw_interface_set_l2_bridge) and only then
// af_packet_delete (D-101 / VPP V24).
func TestDeleteQuiescesFirst(t *testing.T) {
	f := newFake()
	d := f.desc()
	o := &afpacket.HostInterface{Name: "w2-l0", HostIfName: "w2-l0"}
	meta, err := d.Create(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	if !f.links.up("w2-l0") {
		t.Fatal("fake: the netdev is not up after the create")
	}
	start := len(f.order())
	if err := d.Delete(ctx, o, meta); err != nil {
		t.Fatal(err)
	}
	seq := f.order()
	down, settle := pos(seq, "link-down(w2-l0)", start), pos(seq, "settle", start)
	sanitize, del := pos(seq, "sw_interface_set_l2_bridge", start), pos(seq, "af_packet_delete(w2-l0)", start)
	if !ascending(down, settle, sanitize, del) {
		t.Fatalf("order after Delete %v: link-down %d, settle %d, BeforeDelete %d, af_packet_delete %d", seq[start:], down, settle, sanitize, del)
	}
	if f.links.up("w2-l0") {
		t.Fatal("the netdev is still up after Delete")
	}
	t.Logf("Delete: %s", strings.Join(seq[start:down+2], " → ")+" → … → "+seq[del])
}

// TestCreateRollbackQuiesces (TD-3 re-review L7): the rollback delete of Create — the tag failing,
// and the sanitize failing — brings the netdev down before af_packet_delete as well.
func TestCreateRollbackQuiesces(t *testing.T) {
	for _, tc := range []struct {
		name   string
		inject func(f *fakeAfp)
		want   error
	}{
		{"tag fails", func(f *fakeAfp) { f.FailTag = 1 }, nil},
		{"sanitize fails", func(f *fakeAfp) { f.Fail("classify_set_interface_ip_table", errRefused) }, errRefused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake()
			tc.inject(f)
			_, err := f.desc().Create(ctx, &afpacket.HostInterface{Name: "w2-w0", HostIfName: "w2-w0"})
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("Create err = %v, want %v", err, tc.want)
			}
			seq := f.order()
			create, down := pos(seq, "af_packet_create_v3(w2-w0)", 0), pos(seq, "link-down(w2-w0)", 0)
			settle, del := pos(seq, "settle", 0), pos(seq, "af_packet_delete(w2-w0)", 0)
			if !ascending(create, down, settle, del) {
				t.Fatalf("order %v: create %d, link-down %d, settle %d, rollback af_packet_delete %d", seq, create, down, settle, del)
			}
			if _, left := f.hosts["w2-w0"]; left || f.links.up("w2-w0") {
				t.Fatalf("after the rollback: in VPP %v, netdev up %v", left, f.links.up("w2-w0"))
			}
			t.Logf("%s: %s → … → %s → %s → %s (err: %v)", tc.name, seq[create], seq[down], seq[settle], seq[del], err)
		})
	}
}

// TestQuiesceFailsClosed: a link-down that fails for any reason but "no such device" (EPERM,
// a timeout) or does not take effect sends no af_packet_delete — nor the BeforeDelete that would
// follow it — and the error names the netdev and D-101 / V24.
func TestQuiesceFailsClosed(t *testing.T) {
	timeout := fmt.Errorf("netlink: no answer within 2s (timeout): %w", unix.EAGAIN)
	for _, tc := range []struct {
		name   string
		inject func(l *fakeLinks)
		want   error
	}{
		{"EPERM", func(l *fakeLinks) { l.downErr = unix.EPERM }, unix.EPERM},
		{"timeout", func(l *fakeLinks) { l.downErr = timeout }, unix.EAGAIN},
		{"lookup EPERM", func(l *fakeLinks) { l.lookupErr = unix.EPERM }, unix.EPERM},
		{"still up", func(l *fakeLinks) { l.stuckUp = true }, afpacket.ErrQuiesce},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake()
			d := f.desc()
			o := &afpacket.HostInterface{Name: "w2-l0", HostIfName: "w2-l0"}
			meta, err := d.Create(ctx, o)
			if err != nil {
				t.Fatal(err)
			}
			tc.inject(f.links)
			n := len(f.Calls())
			err = d.Delete(ctx, o, meta)
			if !errors.Is(err, afpacket.ErrQuiesce) || !errors.Is(err, tc.want) || !strings.Contains(err.Error(), "D-101") ||
				!strings.Contains(err.Error(), "V24") || !strings.Contains(err.Error(), "netdev w2-l0") {
				t.Fatalf("Delete err = %v", err)
			}
			if sent := f.Calls()[n:]; len(sent) != 0 {
				t.Fatalf("VPP requests after a failed quiesce: %v", f.order()[len(f.order())-len(sent):])
			}
			if _, ok := f.hosts["w2-l0"]; !ok {
				t.Fatal("host-w2-l0 was deleted")
			}
			t.Logf("Delete refused: %v", err)
		})
	}
}

// TestRollbackQuiesceFailsClosed: when the rollback's quiesce fails, the untagged interface is
// left in VPP (not deleted with the netdev up) and the Create error names that orphan. (iface.AcquireAndTag
// and ifsanitize.Acquire add the rollback error with %v, so it is matched by text.)
func TestRollbackQuiesceFailsClosed(t *testing.T) {
	f := newFake()
	f.FailTag = 1
	f.links.downErr = unix.EPERM
	_, err := f.desc().Create(ctx, &afpacket.HostInterface{Name: "w2-w0", HostIfName: "w2-w0"})
	if err == nil {
		t.Fatal("Create succeeded")
	}
	for _, want := range []string{"untagged orphan host-w2-w0 (sw_if_index 2) left in VPP", "D-101", "V24", "netdev w2-w0", unix.EPERM.Error()} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Create err = %v, lacks %q", err, want)
		}
	}
	if n := len(f.CallsNamed("af_packet_delete")); n != 0 {
		t.Fatalf("af_packet_delete sent %d times with the netdev up", n)
	}
	idx, ok := f.hosts["w2-w0"]
	if row, _ := f.Get(idx); !ok || row.Tag != "" {
		t.Fatalf("orphan: in VPP %v, tag %q", ok, row.Tag)
	}
	t.Logf("Create: %v", err)
}

// TestQuiesceNothingToDo: a netdev that is gone (ENODEV) or already down needs no link-down and no
// settle; the delete goes ahead.
func TestQuiesceNothingToDo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(l *fakeLinks)
	}{
		{"ENODEV", func(l *fakeLinks) { delete(l.devs, "w2-l0") }},
		{"already down", func(l *fakeLinks) { l.devs["w2-l0"].up = false }},
		{"already down, not a veth", func(l *fakeLinks) { l.devs["w2-l0"].up, l.devs["w2-l0"].kind = false, "" }},
		{"ENODEV at the link-down", func(l *fakeLinks) { l.vanish = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake()
			d := f.desc()
			o := &afpacket.HostInterface{Name: "w2-l0", HostIfName: "w2-l0"}
			meta, err := d.Create(ctx, o)
			if err != nil {
				t.Fatal(err)
			}
			tc.setup(f.links)
			if err := d.Delete(ctx, o, meta); err != nil {
				t.Fatal(err)
			}
			seq := f.order()
			if pos(seq, "link-down(w2-l0)", 0) >= 0 || pos(seq, "settle", 0) >= 0 || pos(seq, "af_packet_delete(w2-l0)", 0) < 0 {
				t.Fatalf("order %v", seq)
			}
			if kvs, _ := d.Retrieve(ctx); len(kvs) != 0 {
				t.Fatalf("Retrieve after Delete: %+v", kvs)
			}
		})
	}
}

// TestSettleCancelled: a context cancelled during the settle fails the delete closed.
func TestSettleCancelled(t *testing.T) {
	f := newFake()
	d := afpacket.New(f, owner, afpacket.WithLinks(f.links), afpacket.WithSettle(func(context.Context) error { return context.Canceled }))
	o := &afpacket.HostInterface{Name: "w2-l0", HostIfName: "w2-l0"}
	meta, err := d.Create(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, o, meta.(iface.Meta)); !errors.Is(err, afpacket.ErrQuiesce) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.CallsNamed("af_packet_delete")); n != 0 {
		t.Fatalf("af_packet_delete sent %d times", n)
	}
	// review L1: the interface stays in VPP, so the netdev the quiesce took down comes up again
	if !f.links.up("w2-l0") || pos(f.order(), "link-up(w2-l0)", 0) < 0 {
		t.Fatalf("netdev up %v, order %v", f.links.up("w2-l0"), f.order())
	}
}

// TestQuiesceRefusesNonVeth (TD-5 review L3, D-105): an up netdev that is not a veth — a physical
// NIC such as the management NIC (no link kind), a bond, a vlan — is never brought down. Delete fails
// closed (ErrQuiesce wrapping ErrNotVeth, naming D-105), sends nothing to VPP and leaves the netdev up;
// Create's rollback on such a netdev leaves the untagged orphan and names it.
func TestQuiesceRefusesNonVeth(t *testing.T) {
	for _, kind := range []string{"", "bond", "vlan", "tun"} {
		t.Run("kind="+kind, func(t *testing.T) {
			f := newFake()
			d := f.desc()
			o := &afpacket.HostInterface{Name: "w2-l0", HostIfName: "w2-l0"}
			meta, err := d.Create(ctx, o)
			if err != nil {
				t.Fatal(err)
			}
			f.links.devs["w2-l0"].kind = kind
			n := len(f.Calls())
			err = d.Delete(ctx, o, meta)
			if !errors.Is(err, afpacket.ErrQuiesce) || !errors.Is(err, afpacket.ErrNotVeth) || !strings.Contains(err.Error(), "D-105") ||
				!strings.Contains(err.Error(), "netdev w2-l0") {
				t.Fatalf("Delete err = %v", err)
			}
			if sent := f.Calls()[n:]; len(sent) != 0 {
				t.Fatalf("VPP requests after the refusal: %d", len(sent))
			}
			if seq := f.order(); !f.links.up("w2-l0") || pos(seq, "link-down(w2-l0)", 0) >= 0 {
				t.Fatalf("netdev up %v, order %v", f.links.up("w2-l0"), seq)
			}
			t.Logf("Delete refused: %v", err)
		})
	}
	t.Run("rollback", func(t *testing.T) {
		f := newFake()
		f.FailTag = 1
		f.links.setUp("w2-w0")
		f.links.devs["w2-w0"].kind = ""
		_, err := f.desc().Create(ctx, &afpacket.HostInterface{Name: "w2-w0", HostIfName: "w2-w0"})
		if err == nil || !strings.Contains(err.Error(), "untagged orphan host-w2-w0") || !strings.Contains(err.Error(), "D-105") {
			t.Fatalf("Create err = %v", err)
		}
		if n := len(f.CallsNamed("af_packet_delete")); n != 0 || !f.links.up("w2-w0") {
			t.Fatalf("af_packet_delete sent %d times, netdev up %v", n, f.links.up("w2-w0"))
		}
	})
}

// TestFailedDeleteRestoresTheLink (TD-5 review L1): when ifsanitize.BeforeDelete fails after the
// link-down, af_packet_delete is not sent and the interface provably stays in VPP (and in the
// running config: the scheduler does not journal a failed delete), so the netdev is brought up
// again. A failed af_packet_delete — its outcome in VPP unknown — leaves the netdev down.
func TestFailedDeleteRestoresTheLink(t *testing.T) {
	t.Run("BeforeDelete fails", func(t *testing.T) {
		f := newFake()
		d := f.desc()
		o := &afpacket.HostInterface{Name: "w2-l0", HostIfName: "w2-l0"}
		meta, err := d.Create(ctx, o)
		if err != nil {
			t.Fatal(err)
		}
		start := len(f.order())
		f.Fail("classify_set_interface_ip_table", errRefused)
		if err := d.Delete(ctx, o, meta); !errors.Is(err, errRefused) {
			t.Fatalf("Delete err = %v", err)
		}
		seq := f.order()
		if down, up := pos(seq, "link-down(w2-l0)", start), pos(seq, "link-up(w2-l0)", start); !ascending(down, up) ||
			pos(seq, "af_packet_delete(w2-l0)", start) >= 0 {
			t.Fatalf("order %v", seq[start:])
		}
		if _, ok := f.hosts["w2-l0"]; !ok || !f.links.up("w2-l0") {
			t.Fatalf("in VPP %v, netdev up %v", ok, f.links.up("w2-l0"))
		}
	})
	t.Run("af_packet_delete fails", func(t *testing.T) {
		f := newFake()
		d := f.desc()
		o := &afpacket.HostInterface{Name: "w2-l0", HostIfName: "w2-l0"}
		meta, err := d.Create(ctx, o)
		if err != nil {
			t.Fatal(err)
		}
		f.Fail("af_packet_delete", errRefused)
		if err := d.Delete(ctx, o, meta); !errors.Is(err, errRefused) {
			t.Fatalf("Delete err = %v", err)
		}
		if seq := f.order(); f.links.up("w2-l0") || pos(seq, "link-up(w2-l0)", 0) >= 0 {
			t.Fatalf("netdev up %v, order %v", f.links.up("w2-l0"), seq)
		}
	})
}

// TestRollbackOutlivesTheCreateContext (TD-5 review L2): a Create that fails because its context
// ended (the tag request is the last one it gets through) still rolls back — quiesce, settle,
// af_packet_delete — on a detached context bounded by RollbackTimeout, instead of leaving an
// untagged orphan and a netdev down.
func TestRollbackOutlivesTheCreateContext(t *testing.T) {
	f := newFake()
	cctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var settleErr error
	var deadline time.Duration
	d := afpacket.New(f, owner, afpacket.WithLinks(f.links), afpacket.WithSettle(func(sctx context.Context) error {
		f.links.record("settle")
		if dl, ok := sctx.Deadline(); ok {
			deadline = time.Until(dl)
		}
		settleErr = sctx.Err()
		return settleErr
	}))
	f.On("sw_interface_tag_add_del", func(api.Message) ([]api.Message, error) {
		cancel() // the caller's context ends while the tag is in flight
		return []api.Message{&interfaces.SwInterfaceTagAddDelReply{Retval: -9}}, nil
	})
	_, err := d.Create(cctx, &afpacket.HostInterface{Name: "w2-w0", HostIfName: "w2-w0"})
	if err == nil || strings.Contains(err.Error(), "orphan") {
		t.Fatalf("Create err = %v", err)
	}
	seq := f.order()
	if !ascending(pos(seq, "af_packet_create_v3(w2-w0)", 0), pos(seq, "link-down(w2-w0)", 0), pos(seq, "settle", 0), pos(seq, "af_packet_delete(w2-w0)", 0)) {
		t.Fatalf("order %v", seq)
	}
	if _, left := f.hosts["w2-w0"]; left || settleErr != nil || deadline <= 0 || deadline > afpacket.RollbackTimeout {
		t.Fatalf("orphan %v, settle ctx err %v, rollback deadline in %s", left, settleErr, deadline)
	}
	t.Logf("Create: %v; rollback ran on a detached context (deadline in %s)", err, deadline.Round(time.Second))
}
