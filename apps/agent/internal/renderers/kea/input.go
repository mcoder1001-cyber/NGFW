package kea

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// The render input of one family (F-kea-dhcp-relay). The agent drives each Kea daemon through one singleton
// scheduler descriptor (descriptor.go) whose Value is exactly this input; the renderer embeds the input in the
// rendered configuration (top-level user-context.vrx.input), so Retrieve can read it back from the daemon
// (config-get) or from the file the daemon loads at start, re-render it and prove with ConfigDrift that the daemon
// runs exactly that — the Value is derived from the daemon's state, never echoed from memory (D-063).

// FamilyName is the configuration spelling of family 4 / 6.
func FamilyName(family int) string {
	if family == 6 {
		return "ipv6"
	}
	return "ipv4"
}

// FamilyOf returns 4 or 6 for a server ("" = ipv4) and false for an unknown family.
func FamilyOf(s *vrxv1.DhcpServer) (int, bool) {
	switch s.GetFamily() {
	case "", "ipv4":
		return 4, true
	case "ipv6":
		return 6, true
	}
	return 0, false
}

// Input returns the render input of one family: every DHCP server of that family (enabled or not, exactly as the
// document has it) and, for DHCPv4, the IPv4 addresses of the interfaces those servers name (the "<if>/<addr>"
// bindings). nil when the document has no server of the family.
func Input(ds *vrxv1.DesiredState, family int) *vrxv1.DesiredState {
	servers := map[string]*vrxv1.DhcpServer{}
	for name, s := range ds.GetServices().GetDhcp().GetServers() {
		if f, ok := FamilyOf(s); ok && f == family {
			servers[name] = proto.Clone(s).(*vrxv1.DhcpServer)
		}
	}
	if len(servers) == 0 {
		return nil
	}
	in := &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{Dhcp: &vrxv1.DhcpService{Servers: servers}}}
	if family != 4 {
		return in
	}
	for _, name := range sortedKeys(servers) {
		for _, ifn := range servers[name].GetInterfaces() {
			itf, ok := ds.GetInterfaces()[ifn]
			if !ok || len(itf.GetIpv4()) == 0 {
				continue
			}
			if in.Interfaces == nil {
				in.Interfaces = map[string]*vrxv1.Interface{}
			}
			in.Interfaces[ifn] = &vrxv1.Interface{Ipv4: append([]string(nil), itf.GetIpv4()...)}
		}
	}
	return in
}

// inputOf is Input for what extract returned (the renderer also accepts a ServicesConfig or DhcpService).
func inputOf(in input, family int) *vrxv1.DesiredState {
	return Input(&vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{Dhcp: in.dhcp}, Interfaces: in.interfaces}, family)
}

// topContext is the top-level user-context of a rendered Dhcp4/Dhcp6 configuration with servers.
type topContext struct {
	VRX topVRX `json:"vrx"`
}

type topVRX struct {
	// Input is the base64 of the deterministic protobuf encoding of Input(document, family).
	Input string `json:"input"`
	// Servers names the servers of the input (readability of config-get; not read back).
	Servers []string `json:"servers,omitempty"`
}

func encodeInput(in *vrxv1.DesiredState) (*topContext, error) {
	if in == nil {
		return nil, nil
	}
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("kea: encode input: %w", err)
	}
	names := sortedKeys(in.GetServices().GetDhcp().GetServers())
	return &topContext{VRX: topVRX{Input: base64.StdEncoding.EncodeToString(b), Servers: names}}, nil
}

// EmbeddedInput reads the render input back from a Dhcp4/Dhcp6 configuration (config-get arguments or a rendered
// file). ok is false for a configuration without one: an idle configuration, or one this renderer did not write —
// including a file that is not plain JSON (Kea accepts comments; the packaged /etc/kea files have them, rendered
// files never do). An error means a configuration that carries a render input which cannot be decoded.
func EmbeddedInput(config []byte) (*vrxv1.DesiredState, bool, error) {
	var root map[string]struct {
		UserContext *topContext `json:"user-context"`
	}
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, false, nil // not ours
	}
	keys := make([]string, 0, len(root))
	for k := range root {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		uc := root[k].UserContext
		if (k != "Dhcp4" && k != "Dhcp6") || uc == nil || uc.VRX.Input == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(uc.VRX.Input)
		if err != nil {
			return nil, false, fmt.Errorf("kea: %s user-context.vrx.input: %w", k, err)
		}
		in := &vrxv1.DesiredState{}
		if err := proto.Unmarshal(raw, in); err != nil {
			return nil, false, fmt.Errorf("kea: %s user-context.vrx.input: %w", k, err)
		}
		return in, true, nil
	}
	return nil, false, nil
}
