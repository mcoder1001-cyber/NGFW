// Package bootid is the one implementation of the VPP boot identity (D-080): the triple
// (kernel boot_id, VPP main PID, VPP process start time) that every persisted or in-memory record
// referring to VPP indexes or objects is bound to — D-076 applied-once records, ClaimStore claims
// on untagged objects, the classify table store, the acl stats flag, per-process "what I set" maps.
// The PID alone repeats across host reboots and wraps; the triple does not. When the identity
// changes, every record bound to the old one is expired.
//
// Sources:
//   - PID: control_ping_reply.vpe_pid (VPP's getpid(); the same value show_threads reports for
//     thread 0).
//   - BootID: /proc/sys/kernel/random/boot_id.
//   - StartTime: field 22 (starttime, clock ticks since boot) of /proc/<pid>/stat.
//
// A /proc part that cannot be read (VPP in another PID namespace, a unit test with a fake PID) is
// left zero and encoded "?"; the other parts still change on every VPP restart. Callers that
// must not run on a partial identity check Complete.
//
// Tests point the /proc reads at a fake tree with SetProcRoot (or use a Reader directly).
package bootid

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/vpp"
)

// Identity is the D-080 VPP boot identity. The zero value of a part means "unknown".
type Identity struct {
	BootID    string // kernel boot_id ("" = unreadable)
	PID       int    // VPP main PID (control_ping vpe_pid)
	StartTime uint64 // /proc/<pid>/stat field 22 (0 = unreadable)
}

const unknownPart = "?"

// String is the stable encoding used in persisted records: "<boot_id>/<pid>/<start time>", an
// unknown part as "?". It is the format the DF-6/DF-8/DF-3 factories already wrote, so records
// written before TD-1 keep matching.
func (i Identity) String() string {
	boot := i.BootID
	if boot == "" {
		boot = unknownPart
	}
	start := unknownPart
	if i.StartTime != 0 {
		start = strconv.FormatUint(i.StartTime, 10)
	}
	return boot + "/" + strconv.Itoa(i.PID) + "/" + start
}

// Equal reports whether i and o identify the same VPP instance (all three parts equal; an unknown
// part equals only an unknown part).
func (i Identity) Equal(o Identity) bool { return i == o }

// IsZero reports whether i is the zero Identity (never read).
func (i Identity) IsZero() bool { return i == Identity{} }

// Complete reports whether every part was read (boot_id and start time known).
func (i Identity) Complete() bool { return i.BootID != "" && i.StartTime != 0 }

// ErrLegacy is returned by Parse for a record written in the pre-D-080 PID-only format.
var ErrLegacy = errors.New("bootid: legacy pid-only identity")

// ErrFormat is returned by Parse for anything that is not an encoded Identity.
var ErrFormat = errors.New("bootid: malformed identity")

// Parse decodes String's encoding. A bare PID (the pre-D-080 format) returns ErrLegacy: such a
// record never matches a current identity, so it is treated as belonging to an earlier VPP
// instance (re-add once / claim expired).
func Parse(s string) (Identity, error) {
	if _, err := strconv.ParseUint(s, 10, 32); err == nil {
		return Identity{}, fmt.Errorf("%w: %q", ErrLegacy, s)
	}
	// the boot_id is a UUID (never contains '/')
	parts := strings.Split(s, "/")
	if len(parts) != 3 || parts[0] == "" {
		return Identity{}, fmt.Errorf("%w: %q", ErrFormat, s)
	}
	var id Identity
	if parts[0] != unknownPart {
		id.BootID = parts[0]
	}
	pid, err := strconv.Atoi(parts[1])
	if err != nil || pid < 0 {
		return Identity{}, fmt.Errorf("%w: %q", ErrFormat, s)
	}
	id.PID = pid
	if parts[2] != unknownPart {
		st, err := strconv.ParseUint(parts[2], 10, 64)
		if err != nil || st == 0 {
			return Identity{}, fmt.Errorf("%w: %q", ErrFormat, s)
		}
		id.StartTime = st
	}
	return id, nil
}

// Matches reports whether the recorded encoding belongs to cur. A legacy (PID-only) or malformed
// record never matches.
func Matches(recorded string, cur Identity) bool {
	id, err := Parse(recorded)
	return err == nil && id.Equal(cur)
}

// ParseStat returns field 22 (starttime) of a /proc/<pid>/stat line. Field 2 (comm) is
// parenthesised and may itself contain blanks and ')' characters, so fields are counted after the
// LAST ')'.
func ParseStat(stat []byte) (uint64, error) {
	s := string(stat)
	i := strings.LastIndexByte(s, ')')
	if i < 0 || strings.IndexByte(s, '(') < 0 {
		return 0, errors.New("stat: no comm field")
	}
	f := strings.Fields(s[i+1:]) // f[0] is field 3 (state)
	if len(f) < 20 {
		return 0, fmt.Errorf("stat: %d fields", len(f)+2)
	}
	v, err := strconv.ParseUint(f[19], 10, 64) // field 22
	if err != nil {
		return 0, fmt.Errorf("stat: starttime %q: %w", f[19], err)
	}
	return v, nil
}

// Reader reads the /proc parts of an Identity under ProcRoot ("" = "/proc").
type Reader struct{ ProcRoot string }

func (r Reader) root() string {
	if r.ProcRoot == "" {
		return "/proc"
	}
	return r.ProcRoot
}

// BootID returns the kernel boot id ("" when unreadable).
func (r Reader) BootID() string {
	b, err := os.ReadFile(filepath.Join(r.root(), "sys/kernel/random/boot_id")) //nolint:gosec // fixed kernel path under the proc root
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// StartTime returns field 22 of <root>/<pid>/stat (0 when unreadable or malformed).
func (r Reader) StartTime(pid int) uint64 {
	if pid <= 0 {
		return 0
	}
	b, err := os.ReadFile(filepath.Join(r.root(), strconv.Itoa(pid), "stat")) //nolint:gosec // /proc/<pid>/stat
	if err != nil {
		return 0
	}
	v, err := ParseStat(b)
	if err != nil {
		return 0
	}
	return v
}

// ForPID returns the identity of process pid on this kernel.
func (r Reader) ForPID(pid int) Identity {
	return Identity{BootID: r.BootID(), PID: pid, StartTime: r.StartTime(pid)}
}

// Current returns the identity of the VPP instance c is connected to. It fails only when
// control_ping fails; unreadable /proc parts are left unknown (see Complete).
func (r Reader) Current(ctx context.Context, c vpp.Client) (Identity, error) {
	rep, err := memclnt.NewServiceClient(c).ControlPing(ctx, &memclnt.ControlPing{})
	if err != nil {
		return Identity{}, fmt.Errorf("vpp boot identity: control_ping: %w", err)
	}
	return r.ForPID(int(rep.VpePID)), nil
}

var procRoot atomic.Pointer[string]

// SetProcRoot makes Current read boot_id and /proc/<pid>/stat under root (tests: a fake tree, see
// WriteFakeProc). It returns a function restoring the previous root.
func SetProcRoot(root string) (restore func()) {
	prev := procRoot.Swap(&root)
	return func() { procRoot.Store(prev) }
}

func defaultReader() Reader {
	if p := procRoot.Load(); p != nil {
		return Reader{ProcRoot: *p}
	}
	return Reader{}
}

// Current returns the identity of the running VPP instance (Reader.Current under the proc root set
// by SetProcRoot, default /proc).
func Current(ctx context.Context, c vpp.Client) (Identity, error) {
	return defaultReader().Current(ctx, c)
}

// WriteFakeProc writes a fake proc tree under root for tests: boot_id and, for each pid → start
// time, a <pid>/stat line whose comm contains blanks and parentheses.
func WriteFakeProc(root, bootID string, starts map[int]uint64) error {
	if err := os.MkdirAll(filepath.Join(root, "sys/kernel/random"), 0o755); err != nil {
		return err
	}
	if bootID != "" {
		if err := os.WriteFile(filepath.Join(root, "sys/kernel/random/boot_id"), []byte(bootID+"\n"), 0o600); err != nil {
			return err
		}
	}
	for pid, st := range starts {
		dir := filepath.Join(root, strconv.Itoa(pid))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		line := fmt.Sprintf("%d (vpp main) (x)) S 1 %d %d 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 3 0 %d 1000 10 0\n", pid, pid, pid, st)
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(line), 0o600); err != nil {
			return err
		}
	}
	return nil
}
