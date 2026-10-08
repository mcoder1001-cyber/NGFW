package desired

// F-unbound-chrony-syslog: services.ntp (D-050: NTP lives only here) → chrony.config/ngfw, Value =
// chrony.Input(services.ntp), while services.ntp is enabled. Disabled: no object — the reconciler deletes a previous
// one (chrony gets the disabled rendering) — and a note so /state/drift does not compare the disabled defaults.
// Symmetric keys require selected sealed generations. NTS server certificates remain unsupported (F-ntp).

import (
	"google.golang.org/protobuf/proto"
	"strconv"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/chrony"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/secretvalue"
)

func init() {
	ServicesImplemented["ntp"] = true
}

// RuleSecretChannel is the stable DryRun rule for unavailable selected secret generations.
const RuleSecretChannel = "agent.secret-channel-pending"

// NTP projects services.ntp (see the file comment).
func NTP(s Sink, ds *ngfwv1.DesiredState, options ...HostSecretOptions) {
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
	if len(options) == 0 || options[0].Ref == nil {
		for i, srv := range ntp.GetServers() {
			if srv.GetKeyRef() != "" {
				s.Errorf(Ptr("services", "ntp", "servers", strconv.Itoa(i), "keyRef"), RuleSecretChannel, "selected symmetric key generation is unavailable")
				bad = true
			}
		}
	}
	if ntp.GetNtsServer() != nil {
		s.Errorf(Ptr("services", "ntp", "ntsServer"), "agent.unsupported-field", "serving NTS (NTS-KE certificates) is not supported by this agent build (F-ntp)")
		bad = true
	}
	if !bad {
		if value := bindHostSecrets(s, in, Ptr("services", "ntp"), options); value != nil {
			s.Add(chrony.Key, value, Ptr("services", "ntp"))
		}
	}
}

// AssembleNTP adds the NTP service of a retrieved chrony object to ds.
func AssembleNTP(ds *ngfwv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if kv.Key == chrony.Key {
			in := new(ngfwv1.NtpService)
			if _, err := secretvalue.Unwrap(kv.Value, in); err == nil {
				servicesOf(ds).Ntp = in
			}
		}
	}
}
