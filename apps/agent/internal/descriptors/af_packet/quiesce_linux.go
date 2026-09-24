package afpacket

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// netlinkLinks is the product Links: rtnetlink over golang.org/x/sys/unix (no new module, and no
// exec of ip — host_if_name is config input, 00-CONTEXT rule 9). Setting a link down needs
// CAP_NET_ADMIN (the vrx-agent unit, P10); without it SetDown fails with EPERM and the delete
// fails closed.
type netlinkLinks struct {
	timeout time.Duration // per request (SO_RCVTIMEO / SO_SNDTIMEO)
}

// netlinkTimeout bounds one netlink request; no answer within it is a failure (fail closed).
const netlinkTimeout = 2 * time.Second

var nlSeq atomic.Uint32

// Lookup sends RTM_GETLINK by IFLA_IFNAME and reads ifi_index, IFF_UP and the link kind
// (IFLA_LINKINFO / IFLA_INFO_KIND) from the RTM_NEWLINK answer; a missing netdev answers ENODEV.
func (n netlinkLinks) Lookup(name string) (Netdev, error) {
	if name == "" || len(name) >= unix.IFNAMSIZ {
		return Netdev{}, fmt.Errorf("netdev name %q: %w", name, unix.EINVAL)
	}
	attr := rtattr(unix.IFLA_IFNAME, append([]byte(name), 0))
	link, err := n.request(unix.RTM_GETLINK, 0, unix.IfInfomsg{Family: unix.AF_UNSPEC}, attr)
	if err != nil {
		return Netdev{}, fmt.Errorf("RTM_GETLINK %s: %w", name, err)
	}
	if link == nil {
		return Netdev{}, fmt.Errorf("RTM_GETLINK %s: no link in the answer", name)
	}
	return Netdev{Index: link.info.Index, Up: link.info.Flags&unix.IFF_UP != 0, Kind: link.kind}, nil
}

// SetDown sends RTM_NEWLINK for ifindex with ifi_change = IFF_UP and ifi_flags = 0 and waits for
// the kernel's ACK.
func (n netlinkLinks) SetDown(index int32) error {
	if _, err := n.request(unix.RTM_NEWLINK, unix.NLM_F_ACK, unix.IfInfomsg{Family: unix.AF_UNSPEC, Index: index, Flags: 0, Change: unix.IFF_UP}, nil); err != nil {
		return fmt.Errorf("RTM_NEWLINK ifindex %d (clear IFF_UP): %w", index, err)
	}
	return nil
}

// SetUp sends RTM_NEWLINK for ifindex with ifi_change = IFF_UP and ifi_flags = IFF_UP and waits
// for the kernel's ACK.
func (n netlinkLinks) SetUp(index int32) error {
	if _, err := n.request(unix.RTM_NEWLINK, unix.NLM_F_ACK, unix.IfInfomsg{Family: unix.AF_UNSPEC, Index: index, Flags: unix.IFF_UP, Change: unix.IFF_UP}, nil); err != nil {
		return fmt.Errorf("RTM_NEWLINK ifindex %d (set IFF_UP): %w", index, err)
	}
	return nil
}

// linkAnswer is the part of an RTM_NEWLINK answer the quiesce reads.
type linkAnswer struct {
	info unix.IfInfomsg
	kind string // IFLA_LINKINFO / IFLA_INFO_KIND; "" when absent
}

// findAttr walks the route attributes in b and returns the payload of the first one of type typ
// (the NLA_F_NESTED / NLA_F_NET_BYTEORDER flags masked off). A malformed list ends the walk.
func findAttr(b []byte, typ uint16) ([]byte, bool) {
	for len(b) >= unix.SizeofRtAttr {
		l := int(binary.NativeEndian.Uint16(b[0:2]))
		t := binary.NativeEndian.Uint16(b[2:4]) &^ (unix.NLA_F_NESTED | unix.NLA_F_NET_BYTEORDER)
		if l < unix.SizeofRtAttr || l > len(b) {
			return nil, false
		}
		if t == typ {
			return b[unix.SizeofRtAttr:l], true
		}
		next := (l + unix.RTA_ALIGNTO - 1) & ^(unix.RTA_ALIGNTO - 1)
		if next >= len(b) {
			return nil, false
		}
		b = b[next:]
	}
	return nil, false
}

// linkKind reads IFLA_INFO_KIND from IFLA_LINKINFO in the attributes of an RTM_NEWLINK answer
// ("" when either is missing or malformed: not a veth, so the quiesce fails closed).
func linkKind(attrs []byte) string {
	info, ok := findAttr(attrs, unix.IFLA_LINKINFO)
	if !ok {
		return ""
	}
	kind, ok := findAttr(info, unix.IFLA_INFO_KIND)
	if !ok {
		return ""
	}
	return string(bytes.TrimRight(kind, "\x00"))
}

// rtattr encodes one route attribute (padded to 4 bytes).
func rtattr(typ uint16, data []byte) []byte {
	l := unix.SizeofRtAttr + len(data)
	b := make([]byte, (l+unix.RTA_ALIGNTO-1) & ^(unix.RTA_ALIGNTO-1))
	binary.NativeEndian.PutUint16(b[0:2], uint16(l)) //nolint:gosec // an interface name: < 20 bytes
	binary.NativeEndian.PutUint16(b[2:4], typ)
	copy(b[unix.SizeofRtAttr:], data)
	return b
}

// request sends one rtnetlink request and returns the ifinfomsg and link kind of an RTM_NEWLINK
// answer (nil for a bare ACK). An NLMSG_ERROR answer is returned as its errno (ENODEV, EPERM, …).
func (n netlinkLinks) request(typ, flags uint16, info unix.IfInfomsg, attrs []byte) (*linkAnswer, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, fmt.Errorf("netlink socket: %w", err)
	}
	defer func() { _ = unix.Close(fd) }()
	timeout := n.timeout
	if timeout <= 0 {
		timeout = netlinkTimeout
	}
	tv := unix.NsecToTimeval(timeout.Nanoseconds())
	for _, opt := range []int{unix.SO_RCVTIMEO, unix.SO_SNDTIMEO} {
		if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, opt, &tv); err != nil {
			return nil, fmt.Errorf("netlink setsockopt: %w", err)
		}
	}
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("netlink bind: %w", err)
	}
	seq := nlSeq.Add(1)
	var body bytes.Buffer
	_ = binary.Write(&body, binary.NativeEndian, info) // fixed-size struct: cannot fail
	body.Write(attrs)
	var msg bytes.Buffer
	_ = binary.Write(&msg, binary.NativeEndian, unix.NlMsghdr{
		Len:  uint32(unix.SizeofNlMsghdr + body.Len()), //nolint:gosec // < 64 bytes
		Type: typ, Flags: unix.NLM_F_REQUEST | flags, Seq: seq,
	})
	msg.Write(body.Bytes())
	if err := unix.Sendto(fd, msg.Bytes(), 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("netlink send: %w", err)
	}
	buf := make([]byte, 1<<16)
	var got *linkAnswer
	for {
		nr, _, err := unix.Recvfrom(fd, buf, 0)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			return nil, fmt.Errorf("netlink: no answer within %s (timeout): %w", timeout, err)
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("netlink receive: %w", err)
		}
		done, err := parseAnswer(buf[:nr], seq, flags&unix.NLM_F_ACK != 0, &got)
		if err != nil || done {
			return got, err
		}
	}
}

// parseAnswer walks the netlink messages in b that answer seq: an RTM_NEWLINK is stored in got
// (it ends a request without NLM_F_ACK), an NLMSG_ERROR ends the request (error 0 = ACK).
func parseAnswer(b []byte, seq uint32, wantAck bool, got **linkAnswer) (bool, error) {
	for len(b) >= unix.SizeofNlMsghdr {
		var h unix.NlMsghdr
		if err := binary.Read(bytes.NewReader(b[:unix.SizeofNlMsghdr]), binary.NativeEndian, &h); err != nil {
			return true, fmt.Errorf("netlink: bad header: %w", err)
		}
		l := int(h.Len)
		if l < unix.SizeofNlMsghdr || l > len(b) {
			return true, fmt.Errorf("netlink: bad message length %d", l)
		}
		payload := b[unix.SizeofNlMsghdr:l]
		if next := (l + unix.NLMSG_ALIGNTO - 1) & ^(unix.NLMSG_ALIGNTO - 1); next < len(b) {
			b = b[next:]
		} else {
			b = nil
		}
		if h.Seq != seq {
			continue // not ours (a stale answer)
		}
		switch h.Type {
		case unix.NLMSG_ERROR:
			if len(payload) < 4 {
				return true, errors.New("netlink: short NLMSG_ERROR")
			}
			if code := int32(binary.NativeEndian.Uint32(payload[:4])); code != 0 { //nolint:gosec // errno as the kernel sends it
				return true, syscall.Errno(-code) //nolint:gosec // -errno: a small positive number
			}
			return true, nil
		case unix.RTM_NEWLINK:
			if len(payload) < unix.SizeofIfInfomsg {
				return true, errors.New("netlink: short ifinfomsg")
			}
			var info unix.IfInfomsg
			if err := binary.Read(bytes.NewReader(payload[:unix.SizeofIfInfomsg]), binary.NativeEndian, &info); err != nil {
				return true, fmt.Errorf("netlink: bad ifinfomsg: %w", err)
			}
			*got = &linkAnswer{info: info, kind: linkKind(payload[unix.SizeofIfInfomsg:])}
			if !wantAck {
				return true, nil
			}
		case unix.NLMSG_DONE:
			return true, nil
		}
	}
	return false, nil
}

// defaultLinks is the product link controller.
func defaultLinks() Links { return netlinkLinks{timeout: netlinkTimeout} }
