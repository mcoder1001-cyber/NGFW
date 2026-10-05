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
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

const namespaceExportReceipt = "namespace-exports.json"

// FixedNamespaceHandoff invokes only the installed broker with held NSFS FDs.
// Targets are trusted runtime identities, never configuration document fields.
type FixedNamespaceHandoff struct {
	Targets func(context.Context) ([]MountTarget, error)
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
func (h *FixedNamespaceHandoff) capture(ctx context.Context) (*NamespaceTargetSnapshot, error) {
	if h == nil || h.Provider == nil {
		return nil, ErrBoundary
	}
	snapshot, err := h.Provider.Acquire(ctx)
	if err != nil {
		return nil, ErrBoundary
	}
	if snapshot == nil || snapshot.Validate() != nil {
		if snapshot != nil {
			_ = snapshot.Close()
		}
		return nil, ErrBoundary
	}
	return snapshot, nil
}

func (h *FixedNamespaceHandoff) targets(ctx context.Context) ([]MountTarget, error) {
	snapshot, err := h.capture(ctx)
	if err != nil {
		return nil, ErrBoundary
	}
	targets := append([]MountTarget(nil), snapshot.Targets[:]...)
	if snapshot.Close() != nil {
		return nil, ErrBoundary
	}
	return targets, nil
}

func (h *FixedNamespaceHandoff) Preflight(ctx context.Context) error {
	if h == nil || h.Provider == nil || h.Provider.Preflight(ctx) != nil {
		return ErrBoundary
	}
	executable := h.executable()
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable || validateNamespaceBrokerExecutable(executable) != nil {
		return ErrBoundary
	}
	dispatcher := h.Dispatcher
	if dispatcher == nil {
		dispatcher = &SystemdNamespaceBroker{Executable: executable}
	}
	attested, ok := dispatcher.(NamespaceBrokerAttestedFDDispatch)
	if !ok || attested.Preflight(ctx) != nil {
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
	if plan == nil || plan.Validate() != nil {
		return ErrBoundary
	}
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	before, err := h.capture(bounded)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = before.Close() }()
	role := -1
	for index, observed := range before.Targets {
		if observed.Boot.Equal(target.Boot) && observed.MountInode == target.MountInode {
			role = index
		}
	}
	source := (bootid.Reader{}).ForPID(os.Getpid())
	if role < 0 || !source.Equal(before.Source) {
		return ErrBoundary
	}
	self, err := unix.Open("/proc/self/ns/mnt", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(self) }()
	var selfStat unix.Stat_t
	if unix.Fstat(self, &selfStat) != nil || brokerNamespaceFD(self, unix.CLONE_NEWNS, selfStat.Ino) != nil {
		return ErrBoundary
	}
	host, err := unix.Open(filepath.Join(InstanceRoot, plan.Instance, "hostnetns"), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(host) }()
	private, err := unix.Open(filepath.Join(InstanceRoot, plan.Instance, "netns"), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(private) }()
	request := NamespaceBrokerMessage{Operation: operation, Instance: plan.Instance, Source: MountTarget{source, selfStat.Ino}, Targets: append([]MountTarget(nil), before.Targets[:]...), Target: target, HostNamespace: plan.HostNamespaceInode, Namespace: plan.NamespaceInode}
	fds := [4]int{int(before.Files[role].Fd()), host, private, self}
	if validateAttestedBrokerFDs(request, fds) != nil {
		return ErrBoundary
	}
	dispatcher := h.Dispatcher
	if dispatcher == nil {
		dispatcher = &SystemdNamespaceBroker{Executable: h.executable()}
	}
	attested, ok := dispatcher.(NamespaceBrokerAttestedFDDispatch)
	if !ok {
		return ErrBoundary
	}
	operationError := attested.RunAttestedFDs(bounded, request, fds)
	after, afterError := h.capture(bounded)
	if after != nil {
		defer func() { _ = after.Close() }()
	}
	unchanged := afterError == nil && before.SameTargets(after) && (bootid.Reader{}).ForPID(source.PID).Equal(source)
	if operationError != nil || !unchanged {
		// Only undo this operation's exact owned exports while the original namespace
		// objects are still held. Never adopt a newly observed target or delete its
		// foreign bindings. Failed compensation leaves the pending durable receipt.
		if operation == "export" {
			request.Operation = "remove"
			_ = attested.RunAttestedFDs(bounded, request, fds)
		}
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
