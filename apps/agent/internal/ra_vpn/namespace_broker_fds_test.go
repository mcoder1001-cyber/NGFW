package ravpn

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

func TestAttestedBrokerRefusesOrdinarySourceDescriptor(t *testing.T) {
	mount, err := os.Open("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mount.Close() })
	var stat unix.Stat_t
	if err := unix.Fstat(int(mount.Fd()), &stat); err != nil {
		t.Fatal(err)
	}
	identity := (bootid.Reader{}).ForPID(os.Getpid())
	request := NamespaceBrokerMessage{
		Source:        MountTarget{Boot: identity, MountInode: stat.Ino},
		Target:        MountTarget{Boot: identity, MountInode: stat.Ino},
		HostNamespace: 1, Namespace: 2,
	}
	ordinary, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ordinary.Close() })
	// A source MNT role cannot be represented by a regular/device descriptor,
	// even when the claimed process identity and target MNT are complete.
	if validateAttestedBrokerFDs(request, [4]int{int(mount.Fd()), int(ordinary.Fd()), int(ordinary.Fd()), int(ordinary.Fd())}) == nil {
		t.Fatal("accepted ordinary or repeated namespace descriptors")
	}
}
