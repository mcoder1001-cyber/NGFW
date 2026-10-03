package subsystems

// F-system-identity: the `system` domain as one singleton descriptor (system.identity/ngfw) around the sysident
// renderer. Only the globals owner (D-071: the product agent on a real box) renders into /etc and sets the kernel
// host name — the box's identity is a singleton like VPP's globals. Every other agent (a test slot, the dev host's
// main stack) renders into <slot dir>/sysident/etc/… and never touches the host's name, zone, banners or resolver
// (docs/lab/shared-host-rules.md).

import (
	"fmt"
	"path/filepath"
	"sync"

	"ngfw/agent/internal/renderers/sysident"
	"ngfw/agent/internal/scheduler"
)

// System is the `system` configuration domain.
const System = "system"

// systemDescriptors are the descriptors of the `system` domain (Domains).
var systemDescriptors = []string{sysident.Name}

// systemIdentityPaths are the renderer paths of env (product or slot, see the file comment).
func systemIdentityPaths(env Env) sysident.Paths {
	if env.GlobalsOwner {
		return sysident.ProductPaths()
	}
	return sysident.PathsUnder(filepath.Join(slotRunDir(env.Owner), "sysident"))
}

// registerSystemIdentity registers the system.identity descriptor. It touches no file: a slot's directory is
// prepared before the first write.
func registerSystemIdentity(r scheduler.Registry, env Env) {
	p := systemIdentityPaths(env)
	d := sysident.New(p, env.Log.With("feature", "system-identity"))
	if !env.GlobalsOwner {
		base := filepath.Join(slotRunDir(env.Owner), "sysident")
		d.WithPrepare(func() error {
			if err := mkdirShared(base); err != nil {
				return fmt.Errorf("sysident slot dir: %w", err)
			}
			return nil
		})
	}
	r.Register(d)
	systemIdentityMu.Lock()
	systemIdentities[env.Owner] = d
	systemIdentityMu.Unlock()
	env.Log.Info("system identity wired", "product_paths", env.GlobalsOwner, "hostname_file", p.Hostname)
}

var systemIdentityMu sync.RWMutex
var systemIdentities = map[string]*sysident.Descriptor{}

// SystemIdentityOf returns the descriptor whose paths were scoped during wiring.
func SystemIdentityOf(owner string) *sysident.Descriptor {
	systemIdentityMu.RLock()
	defer systemIdentityMu.RUnlock()
	return systemIdentities[owner]
}
