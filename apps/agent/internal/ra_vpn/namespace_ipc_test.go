package ravpn

import (
	"math"
	"strings"
	"testing"
)

func TestNamespaceIPCRejectsInvalidKernelPID(t *testing.T) {
	for _, pid := range []int{-1, 0, 1, int(math.MaxInt32) + 1} {
		if path, e := NamespaceTargetSocket(pid); e == nil || path != "" {
			t.Fatal("invalid supplier PID accepted")
		}
		if path, e := NamespaceObserverSocket(pid); e == nil || path != "" {
			t.Fatal("invalid observer PID accepted")
		}
	}
	for _, pid := range []int{2, math.MaxInt32} {
		target, e := NamespaceTargetSocket(pid)
		observer, oe := NamespaceObserverSocket(pid)
		if e != nil || oe != nil || len(target) >= 108 || len(observer) >= 108 || !strings.HasPrefix(target, NamespaceIPCRoot+"/") || !strings.HasPrefix(observer, NamespaceIPCRoot+"/") {
			t.Fatal("canonical bounded IPC derivation failed")
		}
	}
	if strings.HasPrefix(NamespaceIPCRoot+"/", "/run/ngfw/") {
		t.Fatal("manager sockets remain under agent runtime ownership sweep")
	}
}
