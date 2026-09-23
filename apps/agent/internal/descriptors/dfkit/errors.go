package dfkit

import (
	"errors"
	"fmt"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"
)

var (
	// ErrRetrieveUnsupported is returned (wrapped) by Retrieve when VPP has no dump or getter for
	// the object type (D-063): the descriptor is write-only for the reconciler, which re-applies
	// the desired objects on every resync (Create is idempotent) and never deletes them because
	// of absence. The message text equals scheduler.ErrRetrieveUnsupported's (P05), which
	// recognises it by text until the sentinel is on main; DF-8 aliases it then.
	ErrRetrieveUnsupported = errors.New("vpp has no dump for this object type")
	// ErrPluginNotLoaded means the VPP on this host does not know the plugin's messages.
	// Integration tests t.Skip on it.
	ErrPluginNotLoaded = errors.New("vpp plugin not loaded")
	// ErrBadMeta means Update/Delete received a Meta of the wrong type.
	ErrBadMeta = errors.New("unexpected meta type")
	// ErrNotSupported means the requested change cannot be made through the VPP API at all (for
	// example disabling the http_static server); the error names the manual step.
	ErrNotSupported = errors.New("not supported by the VPP API")
)

// RetrieveUnsupported returns the wrapped ErrRetrieveUnsupported for descriptor name.
func RetrieveUnsupported(name string) error {
	return fmt.Errorf("%s: %w", name, ErrRetrieveUnsupported)
}

// PluginError maps govpp's unknown-message error (the plugin defining the message is not loaded)
// to ErrPluginNotLoaded; other errors pass through.
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

// IsVPPError reports whether err carries one of the given VPP API return values.
func IsVPPError(err error, codes ...api.VPPApiError) bool {
	var apiErr api.VPPApiError
	if !errors.As(err, &apiErr) {
		return false
	}
	for _, c := range codes {
		if apiErr == c {
			return true
		}
	}
	return false
}
