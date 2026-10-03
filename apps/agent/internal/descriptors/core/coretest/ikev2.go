package coretest

import "go.fd.io/govpp/api"
import ikeapi "ngfw/agent/binapi/ikev2"

// Empty native state is a coherent baseline for tests of unrelated domains.
// Native mutation tests use the stateful descriptor fixture or disposable VPP;
// unmodelled setters still fail explicitly rather than pretending to apply.
func init() {
	RegisterExtension("ikev2-native-empty", func(v *VPP) {
		v.On("ikev2_plugin_get_version", func(api.Message) ([]api.Message, error) {
			return []api.Message{&ikeapi.Ikev2PluginGetVersionReply{Major: 1, Minor: 0x56525801}}, nil
		})
		v.On("ikev2_get_sleep_interval", func(api.Message) ([]api.Message, error) {
			return []api.Message{&ikeapi.Ikev2GetSleepIntervalReply{SleepInterval: 1}}, nil
		})
		for _, name := range []string{"ikev2_profile_dump", "ikev2_sa_v3_dump", "ikev2_child_sa_v2_dump"} {
			v.On(name, func(api.Message) ([]api.Message, error) { return nil, nil })
		}
	})
}
