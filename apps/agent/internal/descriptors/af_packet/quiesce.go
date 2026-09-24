package afpacket

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sys/unix"

	afpapi "ngfw/agent/binapi/af_packet"
)

// D-101 / VPP V24: af_packet_delete_if (plugins/af_packet/af_packet.c:895-900) closes the
// interface's socket fds before af_packet_rx_queue_free → clib_file_del_by_index, so the epoll
// DEL fails with EBADF and the same fd number is closed a second time. Deleting while the Linux
// netdev is up (frames still arriving on the socket) is the suspected cause of the shared-VPP
// SIGSEGV at 2026-09-24 07:27:32. The agent therefore brings the netdev down (netlink) before
// every af_packet_delete it sends. This lowers the risk; it does not remove the double close,
// which happens on every delete (D-107) and needs the VPP fix tracked as V24.

// Links is the Linux side of the quiesce: the product uses netlink (quiesce_linux.go), unit tests
// inject a fake that records the call order.
type Links interface {
	// Lookup resolves the netdev name to its ifindex, IFF_UP and link kind. A netdev that does
	// not exist is an error wrapping unix.ENODEV.
	Lookup(name string) (Netdev, error)
	// SetDown clears IFF_UP on ifindex (RTM_NEWLINK, ifi_change = IFF_UP, ifi_flags = 0, acked).
	SetDown(index int32) error
	// SetUp sets IFF_UP on ifindex again (ifi_flags = IFF_UP): only to undo this quiesce when the
	// delete did not happen (review L1).
	SetUp(index int32) error
}

// Netdev is what Lookup reads of a Linux netdev.
type Netdev struct {
	Index int32
	Up    bool
	Kind  string // IFLA_INFO_KIND ("veth", "bond", …); "" for a device without rtnl link ops (a physical NIC, lo)
}

// VethKind is the only link kind the quiesce brings down (D-105: af_packet is lab-only, on veths).
const VethKind = "veth"

// DefaultSettle is how long the quiesce waits after the netdev reads down before af_packet_delete,
// the same 200 ms as tools/lab rig_side_down. Chosen, not tuned (no performance work).
const DefaultSettle = 200 * time.Millisecond

// ErrQuiesce means the host netdev could not be brought down, so af_packet_delete was not sent
// (fail closed, D-101 / VPP V24).
var ErrQuiesce = errors.New("af_packet: host netdev not quiesced, af_packet_delete not sent (D-101, VPP V24)")

// ErrNotVeth means the quiesce refused to bring down an up netdev that is not a veth (wrapped in
// ErrQuiesce, so the delete fails closed). af_packet is lab-only on veths (D-105); a Delete of an
// af_packet interface on any other netdev — the management NIC, say — must not take it down
// (TD-5 review L3).
var ErrNotVeth = errors.New("not a veth: af_packet is lab-only on veth netdevs (D-105), the quiesce brings no other netdev down")

// Option configures the descriptor.
type Option func(*HostInterfaceDescriptor)

// WithLinks replaces the netlink link controller (tests).
func WithLinks(l Links) Option { return func(d *HostInterfaceDescriptor) { d.links = l } }

// WithSettle replaces the settle wait after the link-down (tests: no sleep).
func WithSettle(wait func(context.Context) error) Option {
	return func(d *HostInterfaceDescriptor) { d.settle = wait }
}

// sleep waits for dur or until ctx is done.
func sleep(dur time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		t := time.NewTimer(dur)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
}

// kindName names a link kind for logs and errors.
func kindName(kind string) string {
	if kind == "" {
		return "none (a physical NIC or another device without rtnl link ops)"
	}
	return kind
}

// quiesce brings the Linux netdev dev down before its af_packet interface is deleted: a netdev
// that is gone (ENODEV) or already down needs nothing; an up veth is set down, must then read
// down, and the settle wait follows. An up netdev that is not a veth is refused (ErrNotVeth,
// D-105). Any failure but ENODEV is ErrQuiesce naming dev (fail closed). downed is the ifindex
// this call brought down (0: none), so a delete that does not happen can bring it up again.
func (d *HostInterfaceDescriptor) quiesce(ctx context.Context, dev string) (downed int32, err error) {
	log := slog.Default().With("netdev", dev, "descriptor", HostInterfaceName)
	fail := func(step string, err error) (int32, error) {
		log.Error("af_packet quiesce failed: af_packet_delete not sent (D-101, VPP V24)", "step", step, "err", err)
		return downed, fmt.Errorf("%w: netdev %s: %s: %w", ErrQuiesce, dev, step, err)
	}
	gone := func() (int32, error) {
		log.Info("af_packet quiesce: netdev does not exist, nothing to quiesce (D-101, VPP V24)")
		return 0, nil
	}
	nd, err := d.links.Lookup(dev)
	switch {
	case errors.Is(err, unix.ENODEV):
		return gone()
	case err != nil:
		return fail("lookup", err)
	case !nd.Up:
		log.Info("af_packet quiesce: netdev already down (D-101, VPP V24)", "ifindex", nd.Index, "kind", kindName(nd.Kind))
		return 0, nil
	case nd.Kind != VethKind:
		return fail("kind", fmt.Errorf("%w; link kind %s", ErrNotVeth, kindName(nd.Kind)))
	}
	if err := d.links.SetDown(nd.Index); err != nil {
		if errors.Is(err, unix.ENODEV) {
			return gone()
		}
		return fail("link down", err)
	}
	downed = nd.Index
	after, err := d.links.Lookup(dev)
	switch {
	case errors.Is(err, unix.ENODEV):
		return gone()
	case err != nil:
		return fail("confirm down", err)
	case after.Up:
		return fail("confirm down", errors.New("IFF_UP still set after RTM_NEWLINK"))
	}
	if err := d.settle(ctx); err != nil {
		return fail("settle", err)
	}
	log.Info("af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24)", "ifindex", nd.Index, "kind", nd.Kind)
	return downed, nil
}

// restore brings the netdev this quiesce took down (downed) up again after a step that left the
// af_packet interface in VPP failed (review L1: otherwise the live interface's data path stays
// dead with nothing saying why). Best effort: a failure is logged.
func (d *HostInterfaceDescriptor) restore(dev string, downed int32, cause error) {
	log := slog.Default().With("netdev", dev, "descriptor", HostInterfaceName, "ifindex", downed, "cause", cause)
	if err := d.links.SetUp(downed); err != nil {
		log.Warn("af_packet quiesce: netdev left down after a failed delete; bringing it up again failed", "err", err)
		return
	}
	log.Warn("af_packet quiesce: the delete failed before af_packet_delete; netdev brought up again (the interface stays in VPP)")
}

// quiescedDelete is the agent's only af_packet_delete sender (TestEveryAfPacketDeleteIsQuiesced):
// quiesce dev, run between (ifsanitize.BeforeDelete for Delete; nil for Create's rollback) and
// only then delete host-<dev>. A failed quiesce returns ErrQuiesce and sends nothing (a quiesce
// that failed after its link-down, and a failed between, bring the netdev up again: the
// interface is provably still in VPP). A failed af_packet_delete leaves the netdev down — its
// outcome in VPP is unknown — with a WARN.
func (d *HostInterfaceDescriptor) quiescedDelete(ctx context.Context, dev string, between func() error) error {
	downed, err := d.quiesce(ctx, dev)
	if err != nil {
		if downed != 0 {
			d.restore(dev, downed, err)
		}
		return err
	}
	if between != nil {
		if err := between(); err != nil {
			if downed != 0 {
				d.restore(dev, downed, err)
			}
			return err
		}
	}
	if _, err := d.svc().AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: dev}); err != nil {
		if downed != 0 {
			slog.Default().Warn("af_packet quiesce: af_packet_delete failed, netdev left down (the interface may or may not be in VPP)",
				"netdev", dev, "descriptor", HostInterfaceName, "ifindex", downed, "err", err)
		}
		return fmt.Errorf("af_packet_delete %s: %w", dev, err)
	}
	return nil
}
