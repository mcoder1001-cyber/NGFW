package natcommon

import (
	"errors"
	"strings"

	"go.fd.io/govpp/api"
)

// ErrPluginNotLoaded is returned by Retrieve when the plugin's messages are unknown to this
// VPP (docs/lab/host-vrx-a.md: npt66 is not loaded). Integration tests t.Skip on it.
var ErrPluginNotLoaded = errors.New("natcommon: plugin not loaded on this VPP")

// ErrRetrieveUnsupported is returned by Retrieve of object types whose VPP state cannot be
// read back (no dump / getter for the object or for fields the diff needs). D-063: the P05
// reconciler treats such descriptors as write-only — it re-applies desired state on every
// resync (Create is idempotent), never deletes on absence and skips them in post-apply
// verification. A descriptor must not fake Retrieve by echoing cached desired state.
// Same text as scheduler.ErrRetrieveUnsupported (task/P05); once P05 is merged this becomes
// an alias of that variable (one line, DF-3-questions.md Q9).
var ErrRetrieveUnsupported = errors.New("vpp has no dump for this object type")

// ErrDuplicateKey is returned by Retrieve when two retrieved objects map to one key — the
// scheduler contract requires unique keys (review finding 2). Descriptors that can meet
// VPP-side duplicates (pnat bindings) report the extras under distinct "<id>#<index>" keys.
var ErrDuplicateKey = errors.New("natcommon: duplicate key")

// ErrForeignInterface is returned (wrapped) by Create when the named interface is tagged by
// another owner (D-071: foreign tag → never touched).
var ErrForeignInterface = errors.New("natcommon: interface belongs to another owner")

// Retval extracts the VPP api error carried by err (the generated clients wrap Retval with
// api.RetvalToVPPApiError).
func Retval(err error) (api.VPPApiError, bool) {
	var e api.VPPApiError
	if errors.As(err, &e) {
		return e, true
	}
	return 0, false
}

// IsAlreadyEnabled reports the "plugin already enabled" outcomes of the NAT plugins:
// nat44-ed/ei return FEATURE_ALREADY_ENABLED; nat64, nat66 and det44 return the bare 1.
func IsAlreadyEnabled(err error) bool {
	rv, ok := Retval(err)
	return ok && (rv == api.FEATURE_ALREADY_ENABLED || rv == 1)
}

// IsAlreadyDisabled reports the "plugin already disabled" outcomes (FEATURE_ALREADY_DISABLED
// for nat44-ed/ei, bare 1 for nat64/nat66/det44).
func IsAlreadyDisabled(err error) bool {
	rv, ok := Retval(err)
	return ok && (rv == api.FEATURE_ALREADY_DISABLED || rv == 1)
}

// IsNoSuchEntry reports a delete of something that is already gone.
func IsNoSuchEntry(err error) bool {
	rv, ok := Retval(err)
	return ok && (rv == api.NO_SUCH_ENTRY || rv == api.NO_SUCH_FIB)
}

// IsValueExist reports a create of something that already exists.
func IsValueExist(err error) bool {
	rv, ok := Retval(err)
	return ok && rv == api.VALUE_EXIST
}

// CompatChecker is the optional client capability the real govpp connection offers
// (api.Channel.CheckCompatiblity): a descriptor whose plugin may be absent asks it before
// failing with an opaque "unknown message". The fake client does not implement it, so unit
// tests behave as if every plugin were loaded.
type CompatChecker interface {
	CheckCompatiblity(msgs ...api.Message) error
}

// PluginLoaded reports whether msgs are known to the VPP behind c; true when c cannot tell.
func PluginLoaded(c any, msgs ...api.Message) bool {
	cc, ok := c.(CompatChecker)
	if !ok {
		return true
	}
	return cc.CheckCompatiblity(msgs...) == nil
}

// IsUnknownMessage reports govpp's "unknown message" failure that a not-loaded plugin
// causes (a *api.CompatibilityError from the compatibility check, or the message-id lookup
// error of a request), and ErrPluginNotLoaded itself.
func IsUnknownMessage(err error) bool {
	if err == nil {
		return false
	}
	var ce *api.CompatibilityError
	if errors.As(err, &ce) || errors.Is(err, ErrPluginNotLoaded) {
		return true
	}
	return strings.Contains(err.Error(), "unknown message")
}
