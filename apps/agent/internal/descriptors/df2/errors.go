package df2

import (
	"errors"
	"fmt"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"
)

// Typed errors descriptors return and integration tests recognise.
var (
	// ErrPluginNotLoaded: the VPP on this host does not know the plugin's messages
	// (docs/lab/host-vrx-a.md: ip6_dad_autoremove, linux_cp, …). Integration tests t.Skip on it.
	ErrPluginNotLoaded = errors.New("vpp plugin not loaded")
	// ErrRetrieveUnsupported: VPP has no dump for this object type, so Retrieve cannot report
	// actual state (adl, classify ip/l2 table bindings, output-acl). The descriptor is
	// write-only; the doc table says so.
	ErrRetrieveUnsupported = errors.New("vpp has no dump for this object type")
	// ErrNoSuchInterface: the named interface does not exist in VPP.
	ErrNoSuchInterface = errors.New("no such interface")
	// ErrBadMeta: Update/Delete received a Meta of the wrong type.
	ErrBadMeta = errors.New("unexpected meta type")
)

// PluginError maps govpp's unknown-message error (the message id lookup fails because the
// plugin that defines it is not loaded) to ErrPluginNotLoaded; other errors pass through.
func PluginError(plugin string, err error) error {
	if err == nil {
		return nil
	}
	var unknown *adapter.UnknownMsgError
	if errors.As(err, &unknown) {
		return fmt.Errorf("%w: %s (%s)", ErrPluginNotLoaded, plugin, unknown.MsgName)
	}
	return err
}

// InterfaceVanished reports whether err is VPP's INVALID_SW_IF_INDEX (-2): the interface was
// deleted between the sw_interface_dump snapshot and a per-interface query. Retrieve skips
// such an interface instead of failing (it no longer carries anything to report).
func InterfaceVanished(err error) bool {
	var apiErr api.VPPApiError
	return errors.As(err, &apiErr) && apiErr == api.INVALID_SW_IF_INDEX
}
