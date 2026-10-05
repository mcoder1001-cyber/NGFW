package desired

import (
	"sort"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// RemoteAccess refuses enabled drafts until the approved engine can negotiate
// road-warrior authentication and virtual addresses. A warning would let Apply
// report success without creating the requested VPN listener.
func RemoteAccess(s Sink, ds *ngfwv1.DesiredState, in map[string]bool) {
	if !in["vpn"] {
		return
	}
	profiles := ds.GetVpn().GetRemoteAccess()
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profile := profiles[name]
		// Optional enabled follows the schema default=true, matching other builders.
		if profile.GetEnabled() || profile.Enabled == nil {
			s.Errorf(Ptr("vpn", "remoteAccess", name, "enabled"), "vpn.remote-access-native-capability",
				"remote-access VPN is unavailable on the approved native IKEv2 engine; disable this inactive draft before applying")
		}
	}
}
