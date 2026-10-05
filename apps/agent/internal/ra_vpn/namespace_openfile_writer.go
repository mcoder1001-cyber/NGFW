package ravpn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// FixedNumericOpenFilePublisher writes only deterministic fixed-family drop-ins.
// Construction is pure; callers must explicitly perform trusted bootstrap.
type FixedNumericOpenFilePublisher struct{}

// PublishNumericOpenFile refuses an unverified process or any unknown existing
// configuration. Read-only supplier observation never invokes this operation.
func (FixedNumericOpenFilePublisher) PublishNumericOpenFile(ctx context.Context, kind NumericOpenFileKind, instance string, target bootid.Identity) error {
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if !validNumericOpenFileTarget(target) || !(bootid.Reader{}).ForPID(target.PID).Equal(target) {
		return ErrBoundary
	}
	if verifyNumericOpenFileTarget(bounded, kind, instance, target) != nil {
		return ErrBoundary
	}
	var data []byte
	var path string
	var err error
	switch kind {
	case NumericOpenFileTargets:
		data, err = RenderTargetsOpenFile(target)
		if err == nil {
			path, err = TargetsOpenFilePath(target.PID)
		}
	case NumericOpenFileObserver:
		data, err = RenderObserverOpenFile(instance, target)
		if err == nil {
			path, err = ObserverOpenFilePath(target.PID)
		}
	default:
		return ErrBoundary
	}
	if err != nil || brokerProtectedParent(filepath.Dir(filepath.Dir(path))) != nil {
		return ErrBoundary
	}
	if publishOrRefreshNumericOpenFile(bounded, kind, instance, target, path, data) != nil || verifyNumericOpenFileTarget(bounded, kind, instance, target) != nil {
		return ErrBoundary
	}
	return nil
}

// publishNumericOpenFileAt is the filesystem-only seam for private fixtures.
// Production derives path and data from the closed-kind canonical renderer.
func publishNumericOpenFileAt(path string, data []byte) error {
	if len(data) == 0 || len(data) > 4096 || brokerProtectedParent(filepath.Dir(filepath.Dir(path))) != nil {
		return ErrBoundary
	}
	parent := filepath.Dir(path)
	if err := os.Mkdir(parent, 0700); err != nil && !os.IsExist(err) {
		return ErrBoundary
	}
	var directory unix.Stat_t
	if unix.Lstat(parent, &directory) != nil || directory.Uid != 0 || directory.Mode != unix.S_IFDIR|0700 {
		return ErrBoundary
	}
	existing, err := readSourceGenerationRecord(path)
	if err == nil {
		// Matching reuse never truncates or replaces the protected file.
		if !bytes.Equal(existing, data) {
			return ErrBoundary
		}
		return nil
	}
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err == nil || err != unix.ENOENT {
		return ErrBoundary
	}
	temporary, err := os.CreateTemp(parent, ".ngfw-openfile-")
	if err != nil {
		return ErrBoundary
	}
	tempPath := temporary.Name()
	cleanup := func() error {
		if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
			return ErrBoundary
		}
		return nil
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		_ = cleanup()
		return ErrBoundary
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		_ = cleanup()
		return ErrBoundary
	}
	if err := temporary.Close(); err != nil {
		_ = cleanup()
		return ErrBoundary
	}
	// Link installs only into an absent destination; a concurrent foreign node
	// cannot be overwritten by a rename between the absence check and commit.
	if err := os.Link(tempPath, path); err != nil {
		_ = cleanup()
		return ErrBoundary
	}
	if cleanup() != nil {
		return ErrBoundary
	}
	// #nosec G304 -- parent is the validated closed-kind numeric drop-in directory, never a caller-selected path; opened only for synchronization.
	held, err := os.Open(parent)
	if err != nil {
		return ErrBoundary
	}
	syncErr := held.Sync()
	closeErr := held.Close()
	if syncErr != nil || closeErr != nil {
		return ErrBoundary
	}
	observed, err := readSourceGenerationRecord(path)
	if err != nil || !bytes.Equal(observed, data) {
		return ErrBoundary
	}
	return nil
}

func verifyNumericOpenFileTarget(ctx context.Context, kind NumericOpenFileKind, instance string, target bootid.Identity) error {
	if !validNumericOpenFileTarget(target) || !(bootid.Reader{}).ForPID(target.PID).Equal(target) {
		return ErrBoundary
	}
	if kind == NumericOpenFileTargets {
		if instance != "" {
			return ErrBoundary
		}
		return verifyBrokerVPPUnitIdentity(ctx, MountTarget{Boot: target})
	}
	if kind != NumericOpenFileObserver {
		return ErrBoundary
	}
	if _, err := RenderObserverOpenFile(instance, target); err != nil {
		return ErrBoundary
	}
	fragment := "/usr/lib/systemd/system/ngfw-ra@.service"
	content, err := trustedInstallationFile(fragment, 16384, false)
	if err != nil {
		return ErrBoundary
	}
	sum := sha256.Sum256(content)
	if hex.EncodeToString(sum[:]) != "bf5e89b553235d455db222039d5e8b2cdbe639a1d36054b5dab6cca3ea266d9a" {
		return ErrBoundary
	}
	// #nosec G204 -- RenderObserverOpenFile above permits only a complete 64-lowerhex instance in the fixed owned unit family.
	command := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "--property=MainPID,FragmentPath,DropInPaths,ExecStart", "ngfw-ra@"+instance+".service")
	output, err := command.Output()
	if err != nil || len(output) > 16384 {
		return ErrBoundary
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			fields[key] = value
		}
	}
	pid, err := strconv.Atoi(fields["MainPID"])
	if err != nil || pid != target.PID || fields["FragmentPath"] != fragment || fields["DropInPaths"] != "" || !strings.Contains(fields["ExecStart"], "path=/usr/lib/ngfw/ngfw-ra-daemon ; argv[]=/usr/lib/ngfw/ngfw-ra-daemon "+instance+" ;") || !(bootid.Reader{}).ForPID(target.PID).Equal(target) {
		return ErrBoundary
	}
	return nil
}

func publishOrRefreshNumericOpenFile(ctx context.Context, kind NumericOpenFileKind, instance string, target bootid.Identity, path string, data []byte) error {
	existing, err := readSourceGenerationRecord(path)
	if err != nil || bytes.Equal(existing, data) {
		return publishNumericOpenFileAt(path, data)
	}
	var old bootid.Identity
	switch kind {
	case NumericOpenFileTargets:
		if instance != "" {
			return ErrBoundary
		}
		old, err = ParseTargetsOpenFile(existing)
	case NumericOpenFileObserver:
		var previousInstance string
		previousInstance, old, err = ParseObserverOpenFile(existing)
		if previousInstance != instance {
			return ErrBoundary
		}
	default:
		return ErrBoundary
	}
	if err != nil || old.PID != target.PID || old.Equal(target) || (bootid.Reader{}).ForPID(old.PID).Equal(old) {
		return ErrBoundary
	}
	// A reused numeric path is repaired only after the old complete generation
	// is proven gone and the supplier has no main/control or cgroup processes.
	if numericSupplierInactive(ctx, kind, target.PID) != nil {
		return ErrBoundary
	}
	if verifyNumericOpenFileTarget(ctx, kind, instance, target) != nil {
		return ErrBoundary
	}
	return replaceNumericOpenFileAt(path, existing, data)
}

func numericSupplierInactive(ctx context.Context, kind NumericOpenFileKind, pid int) error {
	if pid <= 1 || pid > math.MaxInt32 {
		return ErrBoundary
	}
	prefix := "ngfw-ra-targets@"
	if kind == NumericOpenFileObserver {
		prefix = "ngfw-ra-observer@"
	} else if kind != NumericOpenFileTargets {
		return ErrBoundary
	}
	name := prefix + strconv.Itoa(pid) + ".service"
	// #nosec G204 -- Closed supplier kind and canonical bounded decimal PID derive this fixed unit, never caller text.
	command := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "--property=MainPID,ControlPID,ActiveState,ControlGroup", name)
	output, err := command.Output()
	if err != nil {
		return ErrBoundary
	}
	group, err := parseInactiveUnit(output)
	if err != nil {
		return ErrBoundary
	}
	if group == "" {
		return nil
	}
	if len(group) > 512 || filepath.Clean(group) != group || !strings.HasPrefix(group, "/system.slice/") || !strings.HasSuffix(group, "/"+name) {
		return ErrBoundary
	}
	var fs unix.Statfs_t
	if unix.Statfs("/sys/fs/cgroup", &fs) != nil || fs.Type != unix.CGROUP2_SUPER_MAGIC {
		return ErrBoundary
	}
	fd, err := unix.Open("/sys/fs/cgroup"+group+"/cgroup.events", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil
	}
	if err != nil {
		return ErrBoundary
	}
	file := os.NewFile(uintptr(fd), "owned supplier cgroup events")
	data, readErr := io.ReadAll(io.LimitReader(file, 1025))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) > 1024 {
		return ErrBoundary
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "populated 0" {
			return nil
		}
	}
	return ErrBoundary
}

func replaceNumericOpenFileAt(path string, old, data []byte) error {
	if len(data) == 0 || len(data) > 4096 || brokerProtectedParent(filepath.Dir(path)) != nil {
		return ErrBoundary
	}
	current, err := readSourceGenerationRecord(path)
	if err != nil || !bytes.Equal(current, old) {
		return ErrBoundary
	}
	var before unix.Stat_t
	if unix.Lstat(path, &before) != nil || before.Uid != 0 || before.Mode != unix.S_IFREG|0600 || before.Nlink != 1 {
		return ErrBoundary
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".ngfw-openfile-")
	if err != nil {
		return ErrBoundary
	}
	temporary := file.Name()
	cleanup := func() error {
		if err := os.Remove(temporary); err != nil && !os.IsNotExist(err) {
			return ErrBoundary
		}
		return nil
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = cleanup()
		return ErrBoundary
	}
	var after unix.Stat_t
	if unix.Lstat(path, &after) != nil || before != after {
		_ = cleanup()
		return ErrBoundary
	}
	current, err = readSourceGenerationRecord(path)
	if err != nil || !bytes.Equal(current, old) {
		_ = cleanup()
		return ErrBoundary
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = cleanup()
		return ErrBoundary
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return ErrBoundary
	}
	syncErr = directory.Sync()
	closeErr = directory.Close()
	if syncErr != nil || closeErr != nil {
		return ErrBoundary
	}
	current, err = readSourceGenerationRecord(path)
	if err != nil || !bytes.Equal(current, data) {
		return ErrBoundary
	}
	return nil
}
