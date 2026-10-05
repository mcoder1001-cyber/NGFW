package ravpn

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// Initialize explicitly prepares canonical source references and the fixed VPP
// supplier after the caller's global inactive barrier. Acquisition is readonly.
func (p *SystemdNamespaceTargets) Initialize(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	expected, err := p.expected(bounded)
	if err != nil || initializeSourceForTarget(bounded, expected) != nil {
		return ErrBoundary
	}
	current, err := p.expected(bounded)
	if err != nil || !current.Equal(expected) {
		return ErrBoundary
	}
	return nil
}

// Initialize delegates only to an explicitly initializable canonical provider.
// Legacy trusted target callbacks do not implicitly gain a provisioning path.
func (h *FixedNamespaceHandoff) Initialize(ctx context.Context) error {
	if h == nil || h.Provider == nil {
		return ErrBoundary
	}
	initializer, ok := h.Provider.(NamespaceTargetInitialization)
	if !ok {
		return ErrBoundary
	}
	return initializer.Initialize(ctx)
}

// InitializeSource performs only own-source normalization/reference publication.
func (p *SystemdNamespaceTargets) InitializeSource(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	source, err := verifyCanonicalSourceInitializerBeforeNormalization(bounded)
	if err != nil || normalizeCanonicalSourceCapabilities() != nil {
		return ErrBoundary
	}
	if _, err := verifyCanonicalSourceInitializer(bounded); err != nil {
		return ErrBoundary
	}
	if ensureSourceBootstrapRoot() != nil {
		return ErrBoundary
	}
	return publishSourceAgentGeneration(bounded, "/run/ngfw/ra", source)
}

// InitializeSource delegates source-only work without provisioning or transport.
func (h *FixedNamespaceHandoff) InitializeSource(ctx context.Context) error {
	if h == nil || h.Provider == nil {
		return ErrBoundary
	}
	initializer, ok := h.Provider.(NamespaceTargetSourceInitialization)
	if !ok {
		return ErrBoundary
	}
	return initializer.InitializeSource(ctx)
}

func initializeSourceForTarget(ctx context.Context, target bootid.Identity) error {
	source, err := verifyCanonicalSourceInitializer(ctx)
	if err != nil || verifyOpenFileSystemdABI(ctx) != nil || verifyBrokerVPPUnitIdentity(ctx, MountTarget{Boot: target}) != nil {
		return ErrBoundary
	}
	if readSourceAgentReference(source) != nil {
		return ErrBoundary
	}
	if NewSystemdNumericOpenFilePublisher().PublishNumericOpenFile(ctx, NumericOpenFileTargets, "", target) != nil {
		return ErrBoundary
	}
	if verifyBrokerVPPUnitIdentity(ctx, MountTarget{Boot: target}) != nil || readSourceAgentReference(source) != nil {
		return ErrBoundary
	}
	return nil
}

func verifyOpenFileSystemdABI(ctx context.Context) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/systemctl", "--version").Output()
	if err != nil || len(output) > 16384 {
		return ErrBoundary
	}
	fields := strings.Fields(string(output))
	if len(fields) < 2 || fields[0] != "systemd" {
		return ErrBoundary
	}
	version := strings.SplitN(fields[1], ".", 2)[0]
	number, err := strconv.Atoi(version)
	if err != nil || number < 253 || strconv.Itoa(number) != version {
		return ErrBoundary
	}
	return nil
}

func verifyCanonicalSourceInitializerBeforeNormalization(ctx context.Context) (bootid.Identity, error) {
	return verifyOwnCanonicalSourceImage(ctx, false)
}

func verifyCanonicalSourceInitializer(ctx context.Context) (bootid.Identity, error) {
	return verifyOwnCanonicalSourceImage(ctx, true)
}

func verifyOwnCanonicalSourceImage(ctx context.Context, normalized bool) (bootid.Identity, error) {
	source := (bootid.Reader{}).ForPID(os.Getpid())
	gid := os.Getegid()
	if os.Geteuid() != 0 || source.PID <= 1 || uint64(source.PID) > uint64(2147483647) || gid < 0 || uint64(gid) > uint64(4294967295) || verifyFixedAgentIdentity(ctx, &unix.Ucred{Pid: int32(source.PID), Uid: 0, Gid: uint32(gid)}, source) != nil {
		return bootid.Identity{}, ErrBoundary
	}
	if normalized && verifySourceThreadCapabilities(true) != nil {
		return bootid.Identity{}, ErrBoundary
	}
	// This is the canonical process's own EXE FD, not another process's proc
	// namespace observation. Manager provisioning independently re-attests it.
	running, err := os.Open("/proc/self/exe")
	if err != nil {
		return bootid.Identity{}, ErrBoundary
	}
	verifyErr := validateSourceAgentExecutable(running)
	closeErr := running.Close()
	if verifyErr != nil || closeErr != nil || !(bootid.Reader{}).ForPID(source.PID).Equal(source) {
		return bootid.Identity{}, ErrBoundary
	}
	return source, nil
}

func ensureSourceBootstrapRoot() error {
	if brokerProtectedParent("/run") != nil {
		return ErrBoundary
	}
	for _, path := range []string{"/run/ngfw", "/run/ngfw/ra"} {
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return ErrBoundary
		}
		if brokerProtectedParent(path) != nil {
			return ErrBoundary
		}
	}
	return nil
}
