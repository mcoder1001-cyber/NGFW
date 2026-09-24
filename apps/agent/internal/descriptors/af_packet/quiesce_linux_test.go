package afpacket

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

// nlmsg encodes one netlink message (header + payload, padded).
func nlmsg(typ uint16, seq uint32, payload []byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.NativeEndian, unix.NlMsghdr{Len: uint32(unix.SizeofNlMsghdr + len(payload)), Type: typ, Seq: seq}) //nolint:gosec // test sizes
	b.Write(payload)
	for b.Len()%unix.NLMSG_ALIGNTO != 0 {
		b.WriteByte(0)
	}
	return b.Bytes()
}

func errPayload(code int32) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.NativeEndian, unix.NlMsgerr{Error: code})
	return b.Bytes()
}

// linkPayload is an RTM_NEWLINK answer: ifinfomsg, IFLA_IFNAME, and — kind != "" — IFLA_LINKINFO
// (flagged NLA_F_NESTED, as the kernel sends it) holding IFLA_INFO_KIND after an IFLA_MTU.
func linkPayload(index int32, flags uint32, kind string) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.NativeEndian, unix.IfInfomsg{Index: index, Flags: flags})
	b.Write(rtattr(unix.IFLA_IFNAME, []byte("w2-l0\x00")))
	b.Write(rtattr(unix.IFLA_MTU, []byte{0xdc, 0x05, 0, 0}))
	if kind != "" {
		b.Write(rtattr(unix.IFLA_LINKINFO|unix.NLA_F_NESTED, append(rtattr(unix.IFLA_INFO_KIND, append([]byte(kind), 0)), rtattr(unix.IFLA_INFO_DATA, []byte{0, 0, 0, 0})...)))
	}
	return b.Bytes()
}

// TestParseAnswer: the kernel's answers to RTM_GETLINK and to an acked RTM_NEWLINK.
func TestParseAnswer(t *testing.T) {
	var got *linkAnswer
	// NLMSG_ERROR with -EPERM (no CAP_NET_ADMIN), after a stale answer of another sequence number
	done, err := parseAnswer(append(nlmsg(unix.NLMSG_ERROR, 6, errPayload(0)), nlmsg(unix.NLMSG_ERROR, 7, errPayload(-int32(unix.EPERM)))...), 7, true, &got)
	if !done || !errors.Is(err, unix.EPERM) {
		t.Fatalf("EPERM: done %v err %v", done, err)
	}
	// ENODEV for a missing netdev
	if done, err := parseAnswer(nlmsg(unix.NLMSG_ERROR, 8, errPayload(-int32(unix.ENODEV))), 8, false, &got); !done || !errors.Is(err, unix.ENODEV) {
		t.Fatalf("ENODEV: done %v err %v", done, err)
	}
	// ACK
	if done, err := parseAnswer(nlmsg(unix.NLMSG_ERROR, 9, errPayload(0)), 9, true, &got); !done || err != nil {
		t.Fatalf("ACK: done %v err %v", done, err)
	}
	// RTM_NEWLINK answer to RTM_GETLINK (no ACK asked)
	got = nil
	if done, err := parseAnswer(nlmsg(unix.RTM_NEWLINK, 10, linkPayload(42, unix.IFF_UP|unix.IFF_BROADCAST, "veth")), 10, false, &got); !done || err != nil || got == nil ||
		got.info.Index != 42 || got.info.Flags&unix.IFF_UP == 0 || got.kind != "veth" {
		t.Fatalf("NEWLINK: done %v err %v got %+v", done, err, got)
	}
	// a physical NIC has no IFLA_LINKINFO: kind "" (TD-5 review L3: the quiesce refuses it)
	got = nil
	if done, err := parseAnswer(nlmsg(unix.RTM_NEWLINK, 13, linkPayload(2, unix.IFF_UP, "")), 13, false, &got); !done || err != nil || got == nil || got.kind != "" {
		t.Fatalf("NEWLINK without linkinfo: done %v err %v got %+v", done, err, got)
	}
	// a truncated attribute list reads as no kind (fail closed), never as veth
	trunc := linkPayload(3, unix.IFF_UP, "veth")
	if k := linkKind(trunc[unix.SizeofIfInfomsg : len(trunc)-12]); k != "" {
		t.Fatalf("truncated linkinfo: kind %q", k)
	}
	// a message of another sequence number only: not done
	if done, err := parseAnswer(nlmsg(unix.NLMSG_ERROR, 3, errPayload(0)), 11, true, &got); done || err != nil {
		t.Fatalf("foreign seq: done %v err %v", done, err)
	}
	// truncated
	if _, err := parseAnswer(nlmsg(unix.RTM_NEWLINK, 12, []byte{1, 2}), 12, false, &got); err == nil {
		t.Fatal("short ifinfomsg accepted")
	}
}

// TestNetlinkLookup reads (never changes) links of this network namespace: lo is up with
// ifindex 1 and no link kind, a missing name answers ENODEV, an over-long name is refused before
// any request. (The veth kind is read from the real kernel by the host test: its Delete would
// fail closed with ErrNotVeth otherwise.)
func TestNetlinkLookup(t *testing.T) {
	l := defaultLinks()
	nd, err := l.Lookup("lo")
	if err != nil || nd.Index != 1 || !nd.Up || nd.Kind != "" {
		t.Fatalf("lo: %+v err %v", nd, err)
	}
	if _, err := l.Lookup("w2-nosuchdev"); !errors.Is(err, unix.ENODEV) {
		t.Fatalf("missing netdev: %v", err)
	}
	if _, err := l.Lookup("0123456789abcdef"); !errors.Is(err, unix.EINVAL) {
		t.Fatalf("16-byte name: %v", err)
	}
}
