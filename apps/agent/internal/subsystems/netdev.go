package subsystems

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"syscall"

	"google.golang.org/protobuf/proto"

	afpacket "ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

// NetdevKind is desired.NetdevKind: the rtnetlink link kind of a Linux netdev and whether it exists.
type NetdevKind = desired.NetdevKind

// ErrNotVeth is the agent-side guard of D-105 (review F4): af_packet attaches only to a Linux netdev
// of kind veth — the lab data path of D-010 — so the configuration can never take the management NIC
// or any other host interface away from Linux. Product NICs go through DPDK.
var ErrNotVeth = errors.New("af_packet attaches only to a Linux veth (lab data path, D-010/D-105)")

// ifla_info_kind inside IFLA_LINKINFO (linux/if_link.h); the stdlib syscall package does not define it.
const iflaInfoKind = 1

// LinuxNetdevKind is NetdevKind on the agent's own network namespace: one RTM_GETLINK dump.
func LinuxNetdevKind(name string) (string, bool, error) {
	rib, err := syscall.NetlinkRIB(syscall.RTM_GETLINK, syscall.AF_UNSPEC)
	if err != nil {
		return "", false, fmt.Errorf("netlink RTM_GETLINK: %w", err)
	}
	msgs, err := syscall.ParseNetlinkMessage(rib)
	if err != nil {
		return "", false, fmt.Errorf("netlink RTM_GETLINK: %w", err)
	}
	for i := range msgs {
		if msgs[i].Header.Type != syscall.RTM_NEWLINK {
			continue
		}
		attrs, err := syscall.ParseNetlinkRouteAttr(&msgs[i])
		if err != nil {
			return "", false, fmt.Errorf("netlink RTM_NEWLINK attributes: %w", err)
		}
		ifname, kind := "", ""
		for _, a := range attrs {
			switch a.Attr.Type {
			case syscall.IFLA_IFNAME:
				ifname = strings.TrimRight(string(a.Value), "\x00")
			case syscall.IFLA_LINKINFO:
				kind = linkInfoKind(a.Value)
			}
		}
		if ifname == name {
			return kind, true, nil
		}
	}
	return "", false, nil
}

// linkInfoKind returns IFLA_INFO_KIND from the nested attributes of IFLA_LINKINFO.
func linkInfoKind(b []byte) string {
	for len(b) >= syscall.SizeofRtAttr {
		l := int(binary.NativeEndian.Uint16(b[0:2]))
		typ := binary.NativeEndian.Uint16(b[2:4])
		if l < syscall.SizeofRtAttr || l > len(b) {
			return ""
		}
		if typ&0x3fff == iflaInfoKind { // without NLA_F_NESTED / NLA_F_NET_BYTEORDER
			return strings.TrimRight(string(b[syscall.SizeofRtAttr:l]), "\x00")
		}
		next := (l + syscall.RTA_ALIGNTO - 1) &^ (syscall.RTA_ALIGNTO - 1)
		if next > len(b) {
			return ""
		}
		b = b[next:]
	}
	return ""
}

// CheckVeth returns nil when netdev exists and is a veth, else an error wrapping ErrNotVeth (or the
// lookup's error).
func CheckVeth(kind NetdevKind, netdev string) error {
	k, ok, err := kind(netdev)
	switch {
	case err != nil:
		return fmt.Errorf("%w: kind of %q unknown: %w", ErrNotVeth, netdev, err)
	case !ok:
		return fmt.Errorf("%w: no Linux netdev %q", ErrNotVeth, netdev)
	case k != "veth":
		what := "a physical (kind-less)"
		if k != "" {
			what = "a " + k
		}
		return fmt.Errorf("%w: %q is %s netdev", ErrNotVeth, netdev, what)
	}
	return nil
}

// vethOnly wraps the af_packet host-interface descriptor with the Create-time veth check (defence in
// depth: the projection already refuses an existing non-veth netdev with a pointer, this catches a
// netdev that changed after validation or a resync of an old document). The af_packet descriptor has
// no optional scheduler interface (Normalizer, AbsenceDeleter, KeyProvider, Reapplier) to forward.
type vethOnly struct {
	scheduler.Descriptor
	kind NetdevKind
}

// Create implements scheduler.Descriptor.
func (v *vethOnly) Create(ctx context.Context, obj proto.Message) (any, error) {
	if o, ok := obj.(*afpacket.HostInterface); ok {
		if err := CheckVeth(v.kind, o.GetHostIfName()); err != nil {
			return nil, err
		}
	}
	return v.Descriptor.Create(ctx, obj)
}
