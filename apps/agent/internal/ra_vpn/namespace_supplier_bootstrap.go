package ravpn

import (
	"context"
	"fmt"
	"ngfw/agent/internal/vpp/bootid"
	"time"
)

// Initialize explicitly prepares canonical source references and the fixed VPP
// supplier after the caller's global inactive barrier. Acquisition is readonly.
func (p *SystemdNamespaceTargets) Initialize(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	expected, err := p.expected(bounded)
	if err != nil {
		return &SupplierInitializationFailure{Stage: 1}
	}
	if err := initializeSourceForTarget(bounded, expected); err != nil {
		return err
	}
	current, err := p.expected(bounded)
	if err != nil || !current.Equal(expected) {
		return &SupplierInitializationFailure{Stage: 9}
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
	if err != nil {
		return &SupplierInitializationFailure{Stage: 2}
	}
	if verifyOpenFileSystemdABI(ctx) != nil {
		return &SupplierInitializationFailure{Stage: 3}
	}
	if verifyBrokerVPPUnitIdentity(ctx, MountTarget{Boot: target}) != nil {
		return &SupplierInitializationFailure{Stage: 4}
	}
	if readSourceAgentReference(source) != nil {
		return &SupplierInitializationFailure{Stage: 5}
	}
	if NewSystemdNumericOpenFilePublisher().PublishNumericOpenFile(ctx, NumericOpenFileTargets, "", target) != nil {
		return &SupplierInitializationFailure{Stage: 6}
	}
	if verifyBrokerVPPUnitIdentity(ctx, MountTarget{Boot: target}) != nil {
		return &SupplierInitializationFailure{Stage: 7}
	}
	if readSourceAgentReference(source) != nil {
		return &SupplierInitializationFailure{Stage: 8}
	}
	return nil
}

// SupplierInitializationFailure identifies only a fixed public validation stage;
// it carries no unit output, process identity, profile value or secret material.
// Stages: expected VPP(1), canonical source(2), systemd ABI(3), VPP unit(4),
// source reference(5), publisher(6), post VPP unit(7), post reference(8),
// post expected VPP(9).
type SupplierInitializationFailure struct {
	Stage            uint8
	PublisherStage   uint8
	DeadlineExceeded bool
}

// Error reports only the fixed numeric initialization stage.
func (e *SupplierInitializationFailure) Error() string {
	return fmt.Sprintf("remote-access supplier initialization refused at stage %d", e.Stage)
}

// Unwrap retains the existing fail-closed boundary error classification.
func (*SupplierInitializationFailure) Unwrap() error { return ErrBoundary }
