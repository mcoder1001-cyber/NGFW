package ravpn

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// Received rights must be closed even when a bounded packet is rejected. The
// sender retains its descriptors; these tests never enter a mount namespace.
func TestNamespaceBrokerRejectsMalformedRightsWithoutLeaks(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root peer authentication fixture")
	}
	cases := []struct {
		name    string
		payload []byte
		count   int
	}{
		{"missing", []byte("{}"), 0},
		{"one", []byte("{}"), 1},
		{"three-invalid-message", []byte("{}"), 3},
		{"truncated-rights", []byte("{}"), 4},
		{"oversized-packet", make([]byte, 16386), 3},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				for _, fd := range pair {
					if err := unix.Close(fd); err != nil {
						t.Error(err)
					}
				}
			}()
			file, err := os.Open("/proc/self/ns/net")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					t.Error(err)
				}
			}()
			before, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			var rights []byte
			if test.count != 0 {
				descriptors := make([]int, test.count)
				for i := range descriptors {
					descriptors[i] = int(file.Fd())
				}
				rights = unix.UnixRights(descriptors...)
			}
			if err := unix.Sendmsg(pair[0], test.payload, rights, nil, 0); err != nil {
				t.Fatal(err)
			}
			if err := RunManagedNamespaceBroker(pair[1]); err != ErrBoundary {
				t.Fatalf("malformed handoff accepted: %v", err)
			}
			after, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			if len(after) != len(before) {
				t.Fatalf("received descriptors leaked: before=%d after=%d", len(before), len(after))
			}
		})
	}
}
