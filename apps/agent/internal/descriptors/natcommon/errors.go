package natcommon

import (
	"errors"
	"strings"
	"sync"

	"go.fd.io/govpp/api"
)

// ErrPluginNotLoaded is returned by Retrieve when the plugin's messages are unknown to this
// VPP (docs/lab/host-vrx-a.md: npt66 is not loaded). Integration tests t.Skip on it.
var ErrPluginNotLoaded = errors.New("natcommon: plugin not loaded on this VPP")

// ErrNoDump is returned by Retrieve of object types whose plugin offers no dump message;
// such descriptors return what this process created (restart-unsafe, documented per
// plugin) and this error only when nothing is known.
var ErrNoDump = errors.New("natcommon: VPP offers no dump for this object type")

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

// EnableState remembers whether this process enabled a plugin whose VPP API has no
// "is enabled" getter (nat64, nat66, det44). Retrieve reports the singleton as present when
// the cache says so or when the heuristic (any dependent object exists) says so; a plugin
// that is enabled but empty is therefore re-enabled idempotently once after a restart.
type EnableState struct {
	mu      sync.Mutex
	known   bool
	enabled bool
}

// Set records the outcome of an enable/disable.
func (s *EnableState) Set(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.known, s.enabled = true, enabled
}

// Get returns (enabled, known).
func (s *EnableState) Get() (enabled, known bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled, s.known
}
