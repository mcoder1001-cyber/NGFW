package desired

// F-unbound-chrony-syslog: services.ntp (D-050: NTP lives only here) → chrony.config/vrx, Value =
// chrony.Input(services.ntp), while services.ntp is enabled. Disabled: no object — the reconciler deletes a previous
// one (chrony gets the disabled rendering) — and a note so /state/drift does not compare the disabled defaults.
// Symmetric keys (servers[].keyRef) need the API→agent secret channel, which does not exist yet
// (PENDING-secret-channel): refused here with a DryRun error. NTS server certificates are refused (F-ntp).

import (
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/chrony"
	"ngfw/agent/internal/scheduler"
)

func init() {
	ServicesImplemented["ntp"] = true
}

// RuleSecretChannel is the DryRun rule of a secret reference the agent cannot resolve yet.
const RuleSecretChannel = "agent.secret-channel-pending"

// NTP projects services.ntp (see the file comment).
func NTP(s Sink, ds *vrxv1.DesiredState) {
	ntp := ds.GetServices().GetNtp()
	if ntp == nil {
		return
	}
	in := chrony.Input(ntp)
	if in == nil {
		if proto.Size(ntp) == 0 {
			return
		}
		s.Warnf(Ptr("services", "ntp"), "agent.unsupported-field", "services.ntp is disabled: chrony gets no sources and no server (the disabled rendering); nothing else to compare")
		return
	}
	bad := false
	for i, srv := range ntp.GetServers() {
		if srv.KeyRef != nil {
			s.Errorf(Ptr("services", "ntp", "servers", strconv.Itoa(i), "keyRef"), RuleSecretChannel,
				"symmetric NTP keys need the API→agent secret channel, which this agent build does not have yet (PENDING-secret-channel); use NTS or no key")
			bad = true
		}
	}
	if ntp.GetNtsServer() != nil {
		s.Errorf(Ptr("services", "ntp", "ntsServer"), "agent.unsupported-field", "serving NTS (NTS-KE certificates) is not supported by this agent build (F-ntp)")
		bad = true
	}
	if !bad {
		s.Add(chrony.Key, in, Ptr("services", "ntp"))
	}
}

// AssembleNTP adds the NTP service of a retrieved chrony object to ds.
func AssembleNTP(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if in, ok := kv.Value.(*vrxv1.NtpService); ok && kv.Key == chrony.Key {
			servicesOf(ds).Ntp = in
		}
	}
}
