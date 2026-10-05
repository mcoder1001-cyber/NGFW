package ravpn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

const namespaceExportReceipt = "namespace-exports.json"

// FixedNamespaceHandoff invokes only the installed broker with held NSFS FDs.
// Targets are trusted runtime identities, never configuration document fields.
type FixedNamespaceHandoff struct {
	Targets    func(context.Context) ([]MountTarget, error)
	// Provider supplies manager-opened role descriptors. Consumers must retain
	// these descriptors through dispatch and compare a fresh observation after
	// dispatch; a process identity alone cannot detect a mount namespace change.
	Provider   NamespaceTargetProvider
	Executable string
	Dispatcher NamespaceBrokerFDDispatch
}

type namespaceExportRecord struct {
	Instance      string
	Namespace     uint64
	HostNamespace uint64
	Targets       []MountTarget
	Pending       bool
}

func (h *FixedNamespaceHandoff) executable() string {
	if h.Executable != "" {
		return h.Executable
	}
	return "/usr/lib/ngfw/ngfw-ra-namespace-broker"
}
func (h *FixedNamespaceHandoff) targets(ctx context.Context) ([]MountTarget, error) {
	if h == nil || h.Targets == nil {
		return nil, ErrBoundary
	}
	targets, e := h.Targets(ctx)
	if e != nil || len(targets) != 2 {
		return nil, ErrBoundary
	}
	// Both roles are required even when they currently share a mount namespace.
	for _, target := range targets {
		if !target.Boot.Complete() || target.MountInode == 0 || !(bootid.Reader{}).ForPID(target.Boot.PID).Equal(target.Boot) {
			return nil, ErrBoundary
		}
	}
	return targets, nil
}
func (h *FixedNamespaceHandoff) Preflight(ctx context.Context) error {
	executable := h.executable()
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable || validateNamespaceBrokerExecutable(executable) != nil {
		return ErrBoundary
	}
	targets, e := h.targets(ctx)
	if e != nil {
		return e
	}
	for _, target := range targets {
		file, e := os.Open("/proc/" + strconv.Itoa(target.Boot.PID) + "/ns/mnt")
		if e != nil {
			return ErrBoundary
		}
		err := brokerNamespaceFD(int(file.Fd()), unix.CLONE_NEWNS, target.MountInode)
		_ = file.Close()
		if err != nil || !(bootid.Reader{}).ForPID(target.Boot.PID).Equal(target.Boot) {
			return ErrBoundary
		}
	}
	dispatcher := h.Dispatcher
	if dispatcher == nil {
		dispatcher = &SystemdNamespaceBroker{Executable: executable}
	}
	if dispatcher.Preflight(ctx) != nil {
		return ErrBoundary
	}
	return nil
}
func uniqueMountTargets(targets []MountTarget) []MountTarget {
	var out []MountTarget
	seen := map[uint64]bool{}
	for _, target := range targets {
		if !seen[target.MountInode] {
			seen[target.MountInode] = true
			out = append(out, target)
		}
	}
	return out
}
func (h *FixedNamespaceHandoff) Export(ctx context.Context, plan *NetworkPlan) error {
	if plan == nil || plan.Validate() != nil || plan.NamespaceInode == 0 || plan.HostNamespaceInode == 0 {
		return ErrBoundary
	}
	targets, e := h.targets(ctx)
	if e != nil {
		return e
	}
	record := namespaceExportRecord{plan.Instance, plan.NamespaceInode, plan.HostNamespaceInode, targets, true}
	if e = writeNamespaceExport(record, false); e != nil {
		return e
	}
	for _, target := range uniqueMountTargets(targets) {
		if e = h.call(ctx, "export", plan, target); e != nil {
			return e
		}
		if e = h.call(ctx, "verify", plan, target); e != nil {
			return e
		}
	}
	record.Pending = false
	return writeNamespaceExport(record, true)
}
func (h *FixedNamespaceHandoff) Verify(ctx context.Context, plan *NetworkPlan) error {
	record, e := readNamespaceExport(plan)
	if e != nil || record.Pending {
		return ErrBoundary
	}
	current, e := h.targets(ctx)
	if e != nil || !sameMountTargets(current, record.Targets) {
		return ErrBoundary
	}
	for _, target := range uniqueMountTargets(current) {
		if h.call(ctx, "verify", plan, target) != nil {
			return ErrBoundary
		}
	}
	return nil
}
func (h *FixedNamespaceHandoff) Remove(ctx context.Context, plan *NetworkPlan) error {
	record, e := readNamespaceExport(plan)
	if e != nil {
		return e
	}
	current, e := h.targets(ctx)
	if e != nil || !sameMountTargets(current, record.Targets) {
		return ErrBoundary
	}
	for _, target := range uniqueMountTargets(current) {
		if h.call(ctx, "remove", plan, target) != nil {
			return ErrBoundary
		}
	}
	return os.Remove(filepath.Join(InstanceRoot, plan.Instance, namespaceExportReceipt))
}
func sameMountTargets(a, b []MountTarget) bool {
	if len(a) != 2 || len(b) != 2 {
		return false
	}
	for i := range a {
		if a[i].MountInode != b[i].MountInode || !a[i].Boot.Equal(b[i].Boot) {
			return false
		}
	}
	return true
}
func (h *FixedNamespaceHandoff) call(ctx context.Context, operation string, plan *NetworkPlan, target MountTarget) error {
	// Proc PID/start is checked before AND after FD acquisition. The actual
	// namespace object stays pinned across helper execution even if PID dies.
	if !(bootid.Reader{}).ForPID(target.Boot.PID).Equal(target.Boot) {
		return ErrBoundary
	}
	path := "/proc/" + strconv.Itoa(target.Boot.PID) + "/ns/mnt"
	mount, err := os.Open(path)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = mount.Close() }()
	if brokerNamespaceFD(int(mount.Fd()), unix.CLONE_NEWNS, target.MountInode) != nil || !(bootid.Reader{}).ForPID(target.Boot.PID).Equal(target.Boot) {
		return ErrBoundary
	}

	if plan == nil {
		return ErrBoundary
	}
	source := (bootid.Reader{}).ForPID(os.Getpid())
	var sourceMount unix.Stat_t
	if unix.Stat("/proc/self/ns/mnt", &sourceMount) != nil || !source.Complete() {
		return ErrBoundary
	}
	host, e := unix.Open(filepath.Join(InstanceRoot, plan.Instance, "hostnetns"), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(host) }()
	private, e := unix.Open(filepath.Join(InstanceRoot, plan.Instance, "netns"), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(private) }()
	targets, e := h.targets(ctx)
	if e != nil {
		return e
	}
	request := NamespaceBrokerMessage{Operation: operation, Instance: plan.Instance, Source: MountTarget{source, sourceMount.Ino}, Targets: targets, Target: target, HostNamespace: plan.HostNamespaceInode, Namespace: plan.NamespaceInode}
	dispatcher := h.Dispatcher
	if dispatcher == nil {
		dispatcher = &SystemdNamespaceBroker{Executable: h.executable()}
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if dispatcher.RunFDs(bounded, request, [3]int{int(mount.Fd()), host, private}) != nil || !(bootid.Reader{}).ForPID(target.Boot.PID).Equal(target.Boot) {
		return ErrBoundary
	}
	var current unix.Stat_t
	if unix.Stat(path, &current) != nil || current.Ino != target.MountInode {
		return ErrBoundary
	}
	return nil
}
func readNamespaceExport(plan *NetworkPlan) (namespaceExportRecord, error) {
	var record namespaceExportRecord
	if plan == nil || !ValidInstance(plan.Instance) {
		return record, ErrBoundary
	}
	path := filepath.Join(InstanceRoot, plan.Instance, namespaceExportReceipt)
	if ValidatePrivateFile(path, 16384) != nil {
		return record, ErrBoundary
	}
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return record, ErrBoundary
	}
	file := os.NewFile(uintptr(fd), "namespace export record")
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, 16385))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || decoder.Decode(new(any)) != io.EOF || record.Instance != plan.Instance || record.Namespace != plan.NamespaceInode || record.HostNamespace != plan.HostNamespaceInode || len(record.Targets) != 2 {
		return record, ErrBoundary
	}
	for _, target := range record.Targets {
		if !target.Boot.Complete() || target.MountInode == 0 {
			return record, ErrBoundary
		}
	}
	return record, nil
}
func writeNamespaceExport(record namespaceExportRecord, replace bool) error {
	root := filepath.Join(InstanceRoot, record.Instance)
	if brokerProtectedParent(root) != nil {
		return ErrBoundary
	}
	path := filepath.Join(root, namespaceExportReceipt)
	if replace {
		if ValidatePrivateFile(path, 16384) != nil {
			return ErrBoundary
		}
	} else {
		if _, e := os.Lstat(path); !os.IsNotExist(e) {
			return ErrBoundary
		}
	}
	data, e := json.Marshal(record)
	if e != nil {
		return ErrBoundary
	}
	temporary := path + ".new"
	fd, e := unix.Open(temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrBoundary
	}
	file := os.NewFile(uintptr(fd), "namespace export record")
	_, e = file.Write(data)
	if e == nil {
		e = file.Sync()
	}
	ce := file.Close()
	if e != nil || ce != nil {
		_ = os.Remove(temporary)
		return ErrBoundary
	}
	if os.Rename(temporary, path) != nil {
		_ = os.Remove(temporary)
		return ErrBoundary
	}
	directory, e := os.Open(root)
	if e != nil {
		return ErrBoundary
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}

func validateNamespaceBrokerExecutable(path string) error {
	data, e := trustedInstallationFile(path, 32<<20, true)
	if e != nil || len(data) < 4 || !bytes.Equal(data[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		return ErrBoundary
	}
	receipt, e := trustedInstallationFile(path+".sha256", 128, false)
	digest := sha256.Sum256(data)
	if e != nil || strings.TrimSpace(string(receipt)) != hex.EncodeToString(digest[:]) {
		return ErrBoundary
	}
	return nil
}

// NewFixedNamespaceHandoff constructs a lazy broker with no filesystem effects.
func NewFixedNamespaceHandoff(targets func(context.Context) ([]MountTarget, error)) (*FixedNamespaceHandoff, error) {
	if targets == nil {
		return nil, ErrBoundary
	}
	return &FixedNamespaceHandoff{Targets: targets}, nil
}

// NewFixedNamespaceHandoffForProvider is a pure constructor. Installation and
// namespace readiness remain read-only preflight responsibilities; construction
// never creates paths or starts a supplier service.
func NewFixedNamespaceHandoffForProvider(provider NamespaceTargetProvider) (*FixedNamespaceHandoff, error) {
	if provider == nil {
		return nil, ErrBoundary
	}
	return &FixedNamespaceHandoff{Provider: provider}, nil
}
