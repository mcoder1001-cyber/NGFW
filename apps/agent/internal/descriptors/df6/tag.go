package df6

import (
	"context"
	"fmt"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TagInterface stamps sw_if_index idx with the owner tag of key (vpp.OwnerTag(owner,
// key.ID())). Every tunnel/session interface a DF-6 descriptor creates is tagged this way and
// Retrieve keeps only tagged interfaces.
func TagInterface(ctx context.Context, c vpp.Client, owner string, key scheduler.Key, idx interface_types.InterfaceIndex) error {
	tag, err := vpp.OwnerTag(owner, key.ID())
	if err != nil {
		return err
	}
	if _, err := interfaces.NewServiceClient(c).SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: idx, Tag: tag}); err != nil {
		return fmt.Errorf("sw_interface_tag_add_del: %w", err)
	}
	return nil
}

// TagOrRollback tags idx and, when tagging fails, calls rollback (the descriptor's delete of
// the just-created object) so no untagged object leaks on the shared VPP.
func TagOrRollback(ctx context.Context, c vpp.Client, owner string, key scheduler.Key, idx interface_types.InterfaceIndex, rollback func() error) error {
	if err := TagInterface(ctx, c, owner, key, idx); err != nil {
		if rerr := rollback(); rerr != nil {
			return fmt.Errorf("%w (rollback failed: %v)", err, rerr)
		}
		return err
	}
	return nil
}
