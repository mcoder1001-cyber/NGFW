package lcp

import (
	"encoding/binary"
	"errors"
	"math"
	"net"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNetlinkAckError(t *testing.T) {
	for _, tc := range []struct {
		name string
		code uint32
		want error
	}{
		{"success", 0, nil},
		{"already removed", ^uint32(unix.EADDRNOTAVAIL) + 1, nil},
		{"permission denied", ^uint32(unix.EPERM) + 1, unix.EPERM},
		{"minimum signed code", 1 << 31, unix.Errno(1 << 31)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, 4)
			binary.NativeEndian.PutUint32(data, tc.code)
			if got := netlinkAckError(data); !errors.Is(got, tc.want) {
				t.Fatalf("ack %x: got %v, want %v", tc.code, got, tc.want)
			}
		})
	}
	for _, data := range [][]byte{nil, {0}, {0, 0, 0}} {
		if err := netlinkAckError(data); err == nil {
			t.Fatal("short acknowledgement accepted")
		}
	}
	data := make([]byte, 4)
	binary.NativeEndian.PutUint32(data, 1)
	if err := netlinkAckError(data); err == nil {
		t.Fatal("positive error code accepted")
	}
}

func TestAddrReqValidatesBeforeSocketIO(t *testing.T) {
	ip := net.IPv4(192, 0, 2, 1).To4()
	for _, tc := range []struct {
		name    string
		index   int
		prefix  int
		ip      net.IP
		message string
	}{
		{"negative index", -1, 24, ip, "interface index"},
		{"zero index", 0, 24, ip, "interface index"},
		{"negative prefix", 1, -1, ip, "prefix length"},
		{"large prefix", 1, 33, ip, "prefix length"},
		{"missing address", 1, 24, nil, "four-byte IPv4"},
		{"IPv6 address", 1, 24, net.ParseIP("2001:db8::1"), "four-byte IPv4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := addrReq(-1, unix.RTM_DELADDR, 0, 1, tc.index, tc.ip, tc.prefix)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %q before socket I/O, got %v", tc.message, err)
			}
		})
	}
	// Invalid FD confirms valid boundary inputs reach Sendto without opening a socket.
	for _, prefix := range []int{0, 32} {
		if err := addrReq(-1, unix.RTM_DELADDR, 0, 1, 1, ip, prefix); !errors.Is(err, unix.EBADF) {
			t.Fatalf("valid prefix %d: %v", prefix, err)
		}
	}
	if int64(math.MaxInt) > math.MaxUint32 { // Wider int: exercise the uint32 narrowing boundary.
		if err := addrReq(-1, unix.RTM_DELADDR, 0, 1, math.MaxInt, ip, 24); err == nil || !strings.Contains(err.Error(), "interface index") {
			t.Fatalf("overflow interface index: %v", err)
		}
	}
}
