package ravpn

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const canonicalSourceCapabilities = uint64(1<<unix.CAP_NET_ADMIN | 1<<unix.CAP_SYS_ADMIN | 1<<unix.CAP_IPC_LOCK | 1<<unix.CAP_CHOWN | 1<<unix.CAP_DAC_OVERRIDE)
const canonicalBrokerCapabilities = uint64(1<<unix.CAP_SYS_ADMIN | 1<<unix.CAP_SYS_CHROOT)

func sourceCapabilityStatus(data []byte, normalized bool) error {
	return processCapabilityStatus(data, canonicalSourceCapabilities, normalized)
}

func processCapabilityStatus(data []byte, allowed uint64, normalized bool) error {
	if len(data) > 16384 {
		return ErrBoundary
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if _, duplicate := fields[key]; duplicate {
			return ErrBoundary
		}
		fields[key] = strings.TrimSpace(value)
	}
	if strings.Join(strings.Fields(fields["Uid"]), ",") != "0,0,0,0" || fields["NoNewPrivs"] != "1" {
		return ErrBoundary
	}
	for _, key := range []string{"CapEff", "CapPrm", "CapBnd", "CapInh", "CapAmb"} {
		value, err := strconv.ParseUint(fields[key], 16, 64)
		if err != nil {
			return ErrBoundary
		}
		expected := allowed
		if key == "CapAmb" || key == "CapInh" {
			expected = 0
		}
		if key == "CapInh" && !normalized && allowed&uint64(1<<unix.CAP_SYS_ADMIN) != 0 && value == uint64(1<<unix.CAP_SYS_ADMIN) {
			continue
		}
		if value != expected {
			return ErrBoundary
		}
	}
	return nil
}

func verifySourceThreadCapabilities(normalized bool) error {
	return verifyOwnThreadCapabilities(canonicalSourceCapabilities, normalized)
}

func verifyOwnThreadCapabilities(allowed uint64, normalized bool) error {
	tasks, err := os.ReadDir("/proc/self/task")
	if err != nil || len(tasks) == 0 || len(tasks) > 4096 {
		return ErrBoundary
	}
	for _, task := range tasks {
		tid, err := strconv.Atoi(task.Name())
		if err != nil || tid <= 0 || strconv.Itoa(tid) != task.Name() {
			return ErrBoundary
		}
		data, err := os.ReadFile("/proc/self/task/" + strconv.Itoa(tid) + "/status")
		// A reaped thread is harmless; every surviving/new runtime thread inherits
		// the parent state, and AllThreadsSyscall synchronizes the complete runtime.
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || processCapabilityStatus(data, allowed, normalized) != nil {
			return ErrBoundary
		}
	}
	return nil
}

// normalizeCanonicalSourceCapabilities is invoked only after canonical unit,
// full own boot and internally opened actual own ELF proof. It lowers only
// inheritable/ambient state, preserving the exact permitted/effective/bounding.
func normalizeCanonicalSourceCapabilities() error {
	return normalizeOwnCapabilities(canonicalSourceCapabilities)
}

func normalizeOwnCapabilities(allowed uint64) error {
	if allowed != canonicalSourceCapabilities && allowed != canonicalBrokerCapabilities {
		return ErrBoundary
	}
	if verifyOwnThreadCapabilities(allowed, false) != nil {
		return ErrBoundary
	}
	header := new(unix.CapUserHeader)
	header.Version = unix.LINUX_CAPABILITY_VERSION_3
	data := new([2]unix.CapUserData)
	mask := uint32(canonicalSourceCapabilities)
	if allowed == canonicalBrokerCapabilities {
		mask = uint32(canonicalBrokerCapabilities)
	}
	data[0].Effective = mask
	data[0].Permitted = mask
	// Both canonical boundaries contain only low-word capabilities. The
	// prevalidation refuses high bits; the second word stays explicitly zero.
	// Pin kernel argument buffers across the runtime's synchronized OS-thread
	// calls. PID0 selects each calling thread; no foreign process is addressable.
	var pinned runtime.Pinner
	pinned.Pin(header)
	pinned.Pin(data)
	defer pinned.Unpin()
	// #nosec G103 -- PID0 pinned two-word buffers are required by kernel Capset; canonical unit masks are verified before synchronized monotonic calls.
	_, _, err := syscall.AllThreadsSyscall(syscall.SYS_CAPSET, uintptr(unsafe.Pointer(header)), uintptr(unsafe.Pointer(&data[0])), 0)
	runtime.KeepAlive(header)
	runtime.KeepAlive(data)
	if err != 0 {
		return ErrBoundary
	} // CGO builds return ENOTSUP, never fallback.
	_, _, err = syscall.AllThreadsSyscall6(syscall.SYS_PRCTL, unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0, 0)
	if err != 0 || verifyOwnThreadCapabilities(allowed, true) != nil {
		return ErrBoundary
	}
	return nil
}
