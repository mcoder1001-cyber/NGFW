package dfkit

import (
	"context"
	"fmt"

	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// BootIdentity is the D-080 VPP boot identity (bootid.Current) as the DF-8 descriptors require
// it: complete. Every DF-8 record that refers to VPP indexes or objects (claims on untagged
// interfaces, D-076 applied-once records, the sflow hw→sw map) is bound to it and treated as
// expired when it changes. Unlike the tolerant bootid.Current it fails when the kernel boot id or
// VPP's /proc/<pid>/stat cannot be read (unchanged DF-8 behaviour).
func BootIdentity(ctx context.Context, c vpp.Client) (bootid.Identity, error) {
	id, err := bootid.Current(ctx, c)
	if err != nil {
		return bootid.Identity{}, err
	}
	if !id.Complete() {
		return bootid.Identity{}, fmt.Errorf("boot identity %s: kernel boot_id or /proc/%d/stat unreadable", id, id.PID)
	}
	return id, nil
}

// IdentitySource is the identity function the DF-8 descriptors use; dfkittest.NewFake replaces
// it because the fake VPP's PID is not a real process.
var IdentitySource = BootIdentity
