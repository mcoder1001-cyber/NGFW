package nftables

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Modes of the renderer (HostAclStateResponse.mode).
const (
	// ModeApply loads the table into the agent's own (root) network namespace: the product.
	ModeApply = "apply"
	// ModeNetns loads the table inside a named network namespace (/run/netns/<Netns>), entered with
	// setns(2) on a locked OS thread for every nft call: test slots on the shared host.
	ModeNetns = "netns"
	// ModeCheck validates every rendering with `nft -c` and never loads it; Retrieve returns the stored
	// value (a slot agent without a namespace, or the product stack on a shared host).
	ModeCheck = "check"
)

// ProductTable is the table of the product agent (owner "vrx").
const ProductTable = "vrx"

// Environment of the host firewall (read once when the agent wires the family).
const (
	// EnvMode forces a mode: "apply" | "netns" | "check".
	EnvMode = "VRX_HOST_ACL_MODE"
	// EnvNetns names the network namespace of mode netns ("ns-<prefix>-…").
	EnvNetns = "VRX_HOST_ACL_NETNS"
)

// Paths is where and how one owner's host firewall lives.
type Paths struct {
	// Table is the nftables table name in family inet ("vrx"; tests "vrx_<prefix>").
	Table string
	// Mode is ModeApply, ModeNetns or ModeCheck.
	Mode string
	// Netns is the network namespace of ModeNetns ("" otherwise).
	Netns string
	// RulesFile is the rendered file `nft -f` loads (kept for inspection).
	RulesFile string
	// StoreFile holds the last applied value (protobuf JSON, 0600).
	StoreFile string
}

var (
	tableRe = regexp.MustCompile(`^vrx(_[A-Za-z0-9_-]{1,24})?$`)
	netnsRe = regexp.MustCompile(`^ns-[A-Za-z0-9]{1,6}-[A-Za-z0-9_-]{1,16}$`)
	ownerRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,24}$`)
)

// TableFor is the table of owner: "vrx" for the product owner "vrx", "vrx_<owner>" for a test slot
// ("vrx_<hash>" when the owner has characters an nft identifier cannot carry).
func TableFor(owner string) string {
	if owner == ProductTable {
		return ProductTable
	}
	if !ownerRe.MatchString(owner) {
		sum := sha256.Sum256([]byte(owner))
		return ProductTable + "_" + hex.EncodeToString(sum[:4])
	}
	return ProductTable + "_" + owner
}

// ProductPaths are the paths of owner's host firewall with its files in stateDir, in mode apply for the
// product owner and mode check for every other owner (a slot agent never touches the root netns).
func ProductPaths(stateDir, owner string) Paths {
	p := Paths{
		Table:     TableFor(owner),
		Mode:      ModeApply,
		RulesFile: filepath.Join(stateDir, "host-acl-"+owner+".nft"),
		StoreFile: filepath.Join(stateDir, "host-acl-"+owner+".json"),
	}
	if owner != ProductTable {
		p.Mode = ModeCheck
	}
	return p
}

// TestPaths are the paths of a test slot (prefix "w9"): table vrx_<prefix> inside the namespace netns,
// files in dir.
func TestPaths(prefix, netns, dir string) Paths {
	return Paths{
		Table:     TableFor(prefix),
		Mode:      ModeNetns,
		Netns:     netns,
		RulesFile: filepath.Join(dir, "host-acl-"+prefix+".nft"),
		StoreFile: filepath.Join(dir, "host-acl-"+prefix+".json"),
	}
}

// PathsFromEnv are ProductPaths adjusted by EnvMode and EnvNetns:
//   - EnvNetns set → mode netns in that namespace (any owner);
//   - EnvMode=check → mode check; EnvMode=apply is refused for any owner but "vrx" (the root netns of a
//     shared host is never a test slot's to change);
//   - nothing set → ProductPaths.
func PathsFromEnv(stateDir, owner string) (Paths, error) {
	p := ProductPaths(stateDir, owner)
	ns := strings.TrimSpace(os.Getenv(EnvNetns))
	mode := strings.TrimSpace(os.Getenv(EnvMode))
	if ns != "" {
		p.Mode, p.Netns = ModeNetns, ns
	}
	switch mode {
	case "":
	case ModeCheck:
		p.Mode, p.Netns = ModeCheck, ""
	case ModeNetns:
		if ns == "" {
			return p, fmt.Errorf("%s=netns needs %s", EnvMode, EnvNetns)
		}
	case ModeApply:
		if owner != ProductTable {
			return p, fmt.Errorf("%s=apply is only for the product owner %q (owner %q): use %s", EnvMode, ProductTable, owner, EnvNetns)
		}
		if ns != "" {
			return p, fmt.Errorf("%s=apply contradicts %s=%s", EnvMode, EnvNetns, ns)
		}
	default:
		return p, fmt.Errorf("%s=%q: want apply, netns or check", EnvMode, mode)
	}
	return p, p.Validate(owner)
}

// Validate checks the paths: table and namespace names, absolute clean files, and that only the product
// owner's table may be loaded into the root network namespace.
func (p Paths) Validate(owner string) error {
	switch {
	case !tableRe.MatchString(p.Table):
		return fmt.Errorf("nftables: table name %q is not vrx or vrx_<prefix>", p.Table)
	case p.Mode == ModeNetns && !netnsRe.MatchString(p.Netns):
		return fmt.Errorf("nftables: namespace %q is not ns-<prefix>-<name>", p.Netns)
	case p.Mode != ModeNetns && p.Netns != "":
		return fmt.Errorf("nftables: namespace %q set in mode %s", p.Netns, p.Mode)
	case p.Mode == ModeApply && (owner != ProductTable || p.Table != ProductTable):
		return fmt.Errorf("nftables: only the product owner loads table %q into the root network namespace (owner %q, table %q)", ProductTable, owner, p.Table)
	case p.Mode != ModeApply && p.Mode != ModeNetns && p.Mode != ModeCheck:
		return fmt.Errorf("nftables: unknown mode %q", p.Mode)
	}
	for _, f := range []string{p.RulesFile, p.StoreFile} {
		if !filepath.IsAbs(f) || filepath.Clean(f) != f {
			return fmt.Errorf("nftables: path %q must be absolute and clean", f)
		}
	}
	return nil
}
