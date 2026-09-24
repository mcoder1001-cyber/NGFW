package ifsanitize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp"
)

// QuarantineTagPrefix starts the tag of a quarantine holder: "quarantine:<owner>". It is never
// an owner tag of any agent, so no Retrieve ever reports the holder.
const QuarantineTagPrefix = "quarantine:"

// MaxAcquireAttempts bounds how many fresh indices Acquire tries.
var MaxAcquireAttempts = 4

// Quarantine holders are loopbacks with an instance from a reserved range at the top of VPP's
// loopback instances (LOOPBACK_MAX_INSTANCE = 16384, vnet/ethernet/interface.c:740), scanned
// downwards, so a holder is never loop0/loop1 and never blocks a user's loopback name (TD-3
// re-review M2). loop16000–loop16383 are reserved for the agent (docs/agent/descriptors/interface.md).
const (
	QuarantineInstanceMin uint32 = 16000
	QuarantineInstanceMax uint32 = 16383
)

// errInstanceInUse is VNET_API_ERROR_INVALID_REGISTRATION (vnet/error.h), the answer of
// create_loopback_instance for an instance that is taken (loopback_instance_alloc).
const errInstanceInUse api.VPPApiError = -31

// ErrNoCleanIndex is returned when no clean sw_if_index could be obtained.
var ErrNoCleanIndex = errors.New("no clean sw_if_index obtained (VPP V19 quarantine)")

// Acquire creates an interface with create, sanitizes the sw_if_index VPP returned and reports
// it only when it is clean (TD-3 review H1, D-095 a). When a binding to a deleted table cannot be
// removed (ErrUnclearable) the index is quarantined: the interface is deleted with del, a
// quarantine holder — an admin-down loopback tagged "quarantine:<owner>" — takes the freed
// index (the sw_interface pool reuses the last freed index first), so the index is never handed
// out again while the holder exists, and create is called again for a fresh index. Any other
// sanitize failure deletes the interface and fails. A run that reaches the placeholder cap
// (ErrCapped) fails closed: the interface is deleted, the index is quarantined only when a binding
// was proven unclearable, and there is no retry — the cap is a property of the classify pool, not
// of the index, so the next index would hit it as well. name is for logs.
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
		if errors.Is(serr, ErrCapped) {
			return 0, serr
		}
	}
	return 0, fmt.Errorf("%s: %w after %d attempts", name, ErrNoCleanIndex, MaxAcquireAttempts)
}

// Quarantine makes sure the freed sw_if_index idx is held by a quarantine holder: a loopback
// that must land on idx, tagged "quarantine:<owner>", admin-down. Its instance is the highest free
// one of the reserved range QuarantineInstanceMin–Max (create_loopback_instance with is_specified;
// the instance only names the loopback, the sw_if_index still comes from the sw_interface pool,
// which hands out the index just freed first). If the holder gets another index the dirty index
// went to another creator; the holder is removed and an error returned.
func Quarantine(ctx context.Context, c vpp.Client, owner string, idx uint32) error {
	svc := interfaces.NewServiceClient(c)
	rep, inst, err := createHolder(ctx, svc)
	if err != nil {
		return err
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
		"sw_if_index", idx, "tag", tag, "holder", fmt.Sprintf("loop%d", inst))
	return nil
}

// createHolder creates the quarantine loopback on the highest free instance of the reserved range,
// scanning down from QuarantineInstanceMax (an instance in use answers INVALID_REGISTRATION before
// VPP allocates anything). It never falls back to VPP's lowest free instance.
func createHolder(ctx context.Context, svc interfaces.RPCService) (*interfaces.CreateLoopbackInstanceReply, uint32, error) {
	for inst := QuarantineInstanceMax; inst >= QuarantineInstanceMin; inst-- {
		rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
		if err == nil {
			return rep, inst, nil
		}
		var apiErr api.VPPApiError
		if !errors.As(err, &apiErr) || apiErr != errInstanceInUse {
			return nil, 0, fmt.Errorf("create_loopback_instance loop%d (quarantine holder): %w", inst, err)
		}
	}
	return nil, 0, fmt.Errorf("create_loopback_instance (quarantine holder): every reserved instance loop%d–loop%d is in use", QuarantineInstanceMin, QuarantineInstanceMax)
}

// IsQuarantineTag reports whether tag marks a quarantine holder.
func IsQuarantineTag(tag string) bool {
	return len(tag) > len(QuarantineTagPrefix) && tag[:len(QuarantineTagPrefix)] == QuarantineTagPrefix
}

// Release retries the sanitize of every quarantine holder of owner (tag "quarantine:<owner>") and
// deletes the holders that are clean now — the index is then free and clean for the next creator.
// A holder that is still unclearable stays. It returns how many holders were released.
func Release(ctx context.Context, c vpp.Client, owner string) (int, error) {
	svc := interfaces.NewServiceClient(c)
	stream, err := svc.SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		return 0, fmt.Errorf("sw_interface_dump: %w", err)
	}
	tag := QuarantineTagPrefix + owner
	var holders []uint32
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("sw_interface_dump: %w", err)
		}
		if strings.TrimRight(d.Tag, "\x00") == tag {
			holders = append(holders, uint32(d.SwIfIndex))
		}
	}
	released := 0
	var errs []error
	for _, idx := range holders {
		name := fmt.Sprintf("quarantine holder %d", idx)
		if _, err := Sanitize(ctx, c, idx, name); err != nil {
			if !errors.Is(err, ErrUnclearable) {
				errs = append(errs, err)
			}
			continue
		}
		if _, err := svc.DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
			errs = append(errs, fmt.Errorf("delete_loopback (%s): %w", name, err))
			continue
		}
		recordQuarantine(-1)
		released++
		slog.Default().Info("quarantined sw_if_index released: its stale bindings are gone (VPP V19)", "sw_if_index", idx, "tag", tag)
	}
	return released, errors.Join(errs...)
}
