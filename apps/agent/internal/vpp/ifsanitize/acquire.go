package ifsanitize

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/internal/vpp"
)

// QuarantineTagPrefix starts the tag of a quarantine holder: "quarantine:<owner>". It is never
// an owner tag of any agent, so no Retrieve ever reports the holder.
const QuarantineTagPrefix = "quarantine:"

// MaxAcquireAttempts bounds how many fresh indices Acquire tries.
var MaxAcquireAttempts = 4

// ErrNoCleanIndex is returned when no clean sw_if_index could be obtained.
var ErrNoCleanIndex = errors.New("no clean sw_if_index obtained (VPP V19 quarantine)")

// Acquire creates an interface with create, sanitizes the sw_if_index VPP returned and reports
// it only when it is clean (TD-3 review H1, D-095 a). When a binding to a deleted table cannot be
// removed (ErrUnclearable) the index is quarantined: the interface is deleted with del, a
// quarantine holder — an admin-down loopback tagged "quarantine:<owner>" — takes the freed
// index (the sw_interface pool reuses the last freed index first), so the index is never handed
// out again while the holder exists, and create is called again for a fresh index. Any other
// sanitize failure deletes the interface and fails. name is for logs.
func Acquire(ctx context.Context, c vpp.Client, owner, name string, create func() (uint32, error), del func(idx uint32) error) (uint32, error) {
	for attempt := 0; attempt < MaxAcquireAttempts; attempt++ {
		idx, err := create()
		if err != nil {
			return 0, err
		}
		_, serr := Sanitize(ctx, c, idx, name)
		if serr == nil {
			return idx, nil
		}
		if derr := del(idx); derr != nil {
			return 0, fmt.Errorf("%w (and removing %s at %d failed: %v)", serr, name, idx, derr)
		}
		if !errors.Is(serr, ErrUnclearable) {
			return 0, serr
		}
		if err := Quarantine(ctx, c, owner, idx); err != nil {
			return 0, fmt.Errorf("%w; quarantine of sw_if_index %d failed: %v", serr, idx, err)
		}
	}
	return 0, fmt.Errorf("%s: %w after %d attempts", name, ErrNoCleanIndex, MaxAcquireAttempts)
}

// Quarantine makes sure the freed sw_if_index idx is held by a quarantine holder: an anonymous
// loopback (create_loopback) that must land on idx, tagged "quarantine:<owner>", admin-down. If the
// holder gets another index the dirty index went to another creator; the holder is removed and
// an error returned.
func Quarantine(ctx context.Context, c vpp.Client, owner string, idx uint32) error {
	svc := interfaces.NewServiceClient(c)
	rep, err := svc.CreateLoopback(ctx, &interfaces.CreateLoopback{})
	if err != nil {
		return fmt.Errorf("create_loopback (quarantine holder): %w", err)
	}
	if uint32(rep.SwIfIndex) != idx {
		_, _ = svc.DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: rep.SwIfIndex})
		return fmt.Errorf("the dirty sw_if_index %d was taken by another creator (holder got %d)", idx, rep.SwIfIndex)
	}
	tag := QuarantineTagPrefix + owner
	if _, err := svc.SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag}); err != nil {
		return fmt.Errorf("tag quarantine holder %d: %w", idx, err)
	}
	if _, err := svc.SwInterfaceSetFlags(ctx, &interfaces.SwInterfaceSetFlags{SwIfIndex: rep.SwIfIndex, Flags: 0}); err != nil {
		return fmt.Errorf("admin-down quarantine holder %d: %w", idx, err)
	}
	recordQuarantine(1)
	slog.Default().Error("sw_if_index quarantined: it carries a binding to a deleted classify table that cannot be removed (VPP V19); held by an admin-down loopback so it is never reused",
		"sw_if_index", idx, "tag", tag)
	return nil
}

// IsQuarantineTag reports whether tag marks a quarantine holder.
func IsQuarantineTag(tag string) bool {
	return len(tag) > len(QuarantineTagPrefix) && tag[:len(QuarantineTagPrefix)] == QuarantineTagPrefix
}

