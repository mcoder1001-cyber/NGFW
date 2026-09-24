package df7

import (
	"errors"
	"fmt"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"

	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// Typed errors the DF-7 descriptors return and the integration tests recognise.
var (
	// ErrRetrieveUnsupported (D-063): VPP 26.06 has no usable dump for this object type, so
	// Retrieve cannot report actual state. The descriptor is write-only: the reconciler re-applies
	// desired state on every resync (Create is idempotent), never deletes on absence and skips it
	// in post-apply verification. The text equals scheduler.ErrRetrieveUnsupported (P05) and
	// df2.ErrRetrieveUnsupported so errors wrapping either match IsRetrieveUnsupported.
	ErrRetrieveUnsupported = scheduler.ErrRetrieveUnsupported
	// ErrSpec is wrapped by every validation / decoding error of a desired value.
	ErrSpec = errors.New("invalid spec")
	// ErrNoSuchInterface: no interface has this logical name (DF-1's iface.ErrNotFound).
	ErrNoSuchInterface = iface.ErrNotFound
	// ErrForeignInterface: the interface is tagged by another owner; DF-7 objects are never put
	// on, nor reference, another owner's interface (DF-1's iface.ErrForeignInterface, D-069).
	ErrForeignInterface = iface.ErrForeignInterface
	// ErrBadMeta: Update/Delete received a Meta of the wrong type.
	ErrBadMeta = errors.New("unexpected meta type")
	// ErrPluginNotLoaded: the VPP on this host does not know the plugin's messages.
	ErrPluginNotLoaded = errors.New("vpp plugin not loaded")
)

// Specf wraps ErrSpec with a formatted message.
func Specf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrSpec, fmt.Sprintf(format, args...))
}

// BadMeta returns the ErrBadMeta error of descriptor name for meta.
func BadMeta(name string, meta any) error {
	return fmt.Errorf("%s: %w %T", name, ErrBadMeta, meta)
}

// Unsupported returns ErrRetrieveUnsupported wrapped with the descriptor name and the reason.
func Unsupported(name, reason string) error {
	return fmt.Errorf("%s: %w (%s)", name, ErrRetrieveUnsupported, reason)
}

// PluginError maps govpp's unknown-message error (the plugin defining the message is not
// loaded) to ErrPluginNotLoaded; other errors pass through.
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

// IsVPPError reports whether err carries one of the given VPP API return codes.
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
