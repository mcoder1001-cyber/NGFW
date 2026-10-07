package ravpn

import (
	"context"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"path/filepath"
	"strconv"
)

// NamespaceHandoffStoppedRepair repairs an owned export generation only after
// the shared runtime mutation guard has positively verified quiescence. The
// implementation must retain ambiguous or inaccessible old exports for recovery;
// it cannot adopt a current target by replacing an old ownership receipt.
type NamespaceHandoffStoppedRepair interface {
	ExportExistingRepair(context.Context, *NetworkPlan) error
}

// ExportExistingRepair resumes an owned partial export or authenticates a
// stopped VPP generation change in the same, independently proven manager mount
// namespace. Inaccessible old mount namespaces remain recorded and fail closed.
func (h *FixedNamespaceHandoff) ExportExistingRepair(ctx context.Context, plan *NetworkPlan) error {
	if h == nil || h.Guard == nil || plan == nil || plan.Validate() != nil || h.Guard(ctx, plan) != nil {
		return ErrBoundary
	}
	// A completed source namespace may precede the first export receipt when
	// the agent exits between creation and export. Reconstruct only that exact
	// protected namespace and its three authenticated placeholder birth roles.
	path := filepath.Join(InstanceRoot, plan.Instance, namespaceExportReceipt)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		actual, readErr := ReadAgentPlan(plan.Instance)
		birth, birthErr := readNamespaceBirth(plan.Instance)
		if readErr != nil || birthErr != nil || len(birth.Bindings) != 3 || actual.Owner != plan.Owner || actual.Profile != plan.Profile || actual.NamespaceInode != plan.NamespaceInode || actual.HostNamespaceInode != plan.HostNamespaceInode {
			return ErrBoundary
		}
		return h.Export(ctx, actual)
	}
	record, err := readNamespaceExport(plan)
	if err != nil {
		return ErrBoundary
	}
	current, err := h.targets(ctx)
	if err != nil {
		return ErrBoundary
	}
	if sameMountTargets(current, record.Targets) {
		for _, target := range uniqueMountTargets(current) {
			if record.Pending && h.call(ctx, "export", plan, target) != nil {
				return ErrBoundary
			}
			if h.call(ctx, "verify", plan, target) != nil {
				return ErrBoundary
			}
		}
		record.Pending = false
		return writeNamespaceExport(record, true)
	}
	stable, err := repairStableMountTargets(record.Targets, current, func(identity bootid.Identity) bool {
		current := (bootid.Reader{}).ForPID(identity.PID)
		if current.Complete() {
			return !current.Equal(identity)
		}
		// An unreadable process identity is ambiguous, not proof of death.
		_, err := os.Stat("/proc/" + strconv.Itoa(identity.PID))
		return os.IsNotExist(err)
	})
	if err != nil {
		return ErrBoundary
	}
	// Verify existing owned bindings through unchanged manager roles before
	// replacing metadata. Never dispatch against an old PID or newly changed MNT.
	for _, target := range stable {
		if h.call(ctx, "verify", plan, target) != nil {
			return ErrBoundary
		}
	}
	record.Targets = current
	record.Pending = false
	if writeNamespaceExport(record, true) != nil {
		return ErrBoundary
	}
	if h.Verify(ctx, plan) != nil {
		record.Pending = true
		if writeNamespaceExport(record, true) != nil {
			return ErrBoundary
		}
		return ErrBoundary
	}
	return nil
}

func repairStableMountTargets(old, current []MountTarget, gone func(bootid.Identity) bool) ([]MountTarget, error) {
	if len(old) != 2 || len(current) != 2 || gone == nil {
		return nil, ErrBoundary
	}
	var stable []MountTarget
	for index, prior := range old {
		next := current[index]
		if !prior.Boot.Complete() || !next.Boot.Complete() || prior.MountInode == 0 || prior.MountInode != next.MountInode {
			return nil, ErrBoundary
		}
		if prior.Boot.Equal(next.Boot) {
			stable = append(stable, next)
		} else if !gone(prior.Boot) {
			return nil, ErrBoundary
		}
	}
	for _, prior := range uniqueMountTargets(old) {
		found := false
		for _, target := range stable {
			if target.MountInode == prior.MountInode {
				found = true
			}
		}
		if !found {
			return nil, ErrBoundary
		}
	}
	return uniqueMountTargets(stable), nil
}
