package dfkit

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// ProcRoot is where BootIdentity reads the kernel boot id and VPP's process start time.
var ProcRoot = "/proc"

// BootIdentity is the D-080 VPP boot identity "<kernel boot_id>/<VPP main PID>/<VPP start time>":
// /proc/sys/kernel/random/boot_id, the main-thread PID from show_threads and field 22 (starttime,
// clock ticks since boot) of /proc/<pid>/stat. The PID alone repeats across reboots and PID
// namespaces; the triple does not. Every DF-8 record that refers to VPP indexes or objects
// (claims on untagged interfaces, D-076 applied-once records, the sflow hw→sw map) is bound to it
// and treated as expired when it changes. P08 moves this helper into the agent's vpp package.
func BootIdentity(ctx context.Context, c vpp.Client) (string, error) {
	pid, err := iface.VPPIdentity(ctx, c)
	if err != nil {
		return "", err
	}
	bootID, err := os.ReadFile(ProcRoot + "/sys/kernel/random/boot_id") //nolint:gosec // fixed kernel path
	if err != nil {
		return "", fmt.Errorf("boot identity: %w", err)
	}
	start, err := procStartTime(pid)
	if err != nil {
		return "", fmt.Errorf("boot identity: %w", err)
	}
	return fmt.Sprintf("%s/%d/%s", strings.TrimSpace(string(bootID)), pid, start), nil
}

// procStartTime returns field 22 of /proc/<pid>/stat (the comm field may contain blanks and
// parentheses, so fields are counted after the last ')').
func procStartTime(pid uint32) (string, error) {
	raw, err := os.ReadFile(ProcRoot + "/" + strconv.FormatUint(uint64(pid), 10) + "/stat") //nolint:gosec // /proc/<pid>/stat
	if err != nil {
		return "", err
	}
	s := string(raw)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return "", fmt.Errorf("/proc/%d/stat: no comm field", pid)
	}
	f := strings.Fields(s[i+1:]) // f[0] is field 3 (state)
	if len(f) < 20 {
		return "", fmt.Errorf("/proc/%d/stat: %d fields", pid, len(f)+2)
	}
	return f[19], nil // field 22
}

// IdentitySource is the identity function the DF-8 descriptors use; tests replace it
// (dfkittest.NewFake) because the fake VPP's PID is not a real process.
var IdentitySource = BootIdentity
