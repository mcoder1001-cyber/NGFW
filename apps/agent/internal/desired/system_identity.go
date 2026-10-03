package desired

// F-system-identity: the `system` domain (hostname, time zone, banners, DNS client; DEC-system-identity, D-152) →
// system.identity/ngfw, Value = sysident.Input(system). The domain always has a value (every field has a schema
// default), so the object always exists while `system` is authoritative. Structural checks run here (DryRun
// errors at the leaf); the zone-file check needs the host and runs in the descriptor's Validator.

import (
	"errors"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/sysident"
	"ngfw/agent/internal/scheduler"
)

// RuleSystemIdentity is the DryRun rule of a `system` value the renderer refuses.
const RuleSystemIdentity = "system.identity"

// SystemIdentity projects ds.system (see the file comment).
func SystemIdentity(s Sink, ds *ngfwv1.DesiredState) {
	in := sysident.Input(ds.GetSystem())
	// The zone file is checked by the Validator against the agent's real tzdata; "/" never holds one.
	if err := sysident.CheckStructure(in); err != nil {
		var fe *sysident.FieldError
		if errors.As(err, &fe) {
			s.Errorf(fe.Pointer, RuleSystemIdentity, "%s", fe.Msg)
			return
		}
		s.Errorf(Ptr("system"), RuleSystemIdentity, "%v", err)
		return
	}
	s.Add(sysident.Key, in, Ptr("system"))
}

// AssembleSystemIdentity sets ds.system from a retrieved system.identity object.
func AssembleSystemIdentity(ds *ngfwv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if in, ok := kv.Value.(*ngfwv1.SystemConfig); ok && kv.Key == sysident.Key {
			ds.System = in
		}
	}
}
