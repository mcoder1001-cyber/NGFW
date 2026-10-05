package ravpn

import (
	"context"
	"ngfw/agent/internal/vpp/bootid"
	"time"
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
