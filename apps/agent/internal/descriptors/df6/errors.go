package df6

import (
	"errors"
	"fmt"

	"go.fd.io/govpp/adapter"

	iface "ngfw/agent/internal/descriptors/interface"
)

// Typed errors descriptors return and integration tests recognise.
var (
	// ErrPluginNotLoaded: the VPP on this host does not know the plugin's messages.
	// Integration tests t.Skip on it.
	ErrPluginNotLoaded = errors.New("vpp plugin not loaded")
	// ErrRetrieveUnsupported: VPP has no dump for this object type, so Retrieve cannot report
	// actual state (vxlan/gtpu bypass, l2tpv3 enable, pppoe cp, SR encap globals). The
	// descriptor is write-only; the doc table says so.
	ErrRetrieveUnsupported = errors.New("vpp has no dump for this object type")
	// ErrNoDelete: VPP has no delete message for this object type (l2tpv3 tunnels).
	ErrNoDelete = errors.New("vpp has no delete message for this object type")
	// ErrNoSuchInterface: the named interface does not exist in VPP.
	ErrNoSuchInterface = errors.New("no such interface")
	// ErrBadMeta: Update/Delete received a Meta of the wrong type.
	ErrBadMeta = errors.New("unexpected meta type")
	// ErrBadValue: the desired object is not of the descriptor's proto type or is invalid.
	ErrBadValue = errors.New("invalid desired value")
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

// Meta helpers ------------------------------------------------------------------------------

// IfMeta is the runtime handle of every object that is a VPP interface (tunnels, sessions).
type IfMeta struct{ SwIfIndex uint32 }

// IfMetaOf asserts meta is an IfMeta.
func IfMetaOf(name string, meta any) (IfMeta, error) {
	m, ok := meta.(IfMeta)
	if !ok {
		return IfMeta{}, fmt.Errorf("%s: %w %T", name, ErrBadMeta, meta)
	}
	return m, nil
}

// IsNoSuchInterface reports whether err says the interface does not exist (not: foreign).
func IsNoSuchInterface(err error) bool {
	return errors.Is(err, ErrNoSuchInterface) && !errors.Is(err, iface.ErrForeignInterface)
}
