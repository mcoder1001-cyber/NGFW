package desired

// F-unbound-chrony-syslog: the `management` domain (this build implements its syslog leaf only; users, AAA, TLS and
// the later feature keys are agent.unsupported-field until their feature projects them — F-dashboard-prom-alarms
// registers its keys in ManagementImplemented from its own file).
//
//	management.syslog (non-empty) → rsyslog.config/vrx   Value = rsyslog.Input(document) (*vrxv1.ManagementConfig)
//
// Without targets there is no object: the reconciler deletes a previous one (rsyslog gets the empty export). TLS
// targets need the API→agent secret channel for their certificate and key references (PENDING-secret-channel):
// refused here with a DryRun error until it lands (the renderer itself is tested with a fixture resolver, D-086).

import (
	"strconv"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/rsyslog"
	"ngfw/agent/internal/scheduler"
)

// ManagementImplemented lists the `management` sub-keys (JSON names) this build projects.
var ManagementImplemented = map[string]bool{"syslog": true}

// ManagementUnsupported warns agent.unsupported-field for every non-empty `management` sub-key no builder projects.
func ManagementUnsupported(s Sink, m *vrxv1.ManagementConfig) {
	unsupported(s, "management", m, ManagementImplemented)
}

// Syslog projects management.syslog (see the file comment).
func Syslog(s Sink, ds *vrxv1.DesiredState) {
	in := rsyslog.Input(ds)
	if in == nil {
		return
	}
	bad := false
	for i, t := range in.GetSyslog() {
		if t.GetTls() != nil {
			s.Errorf(Ptr("management", "syslog", strconv.Itoa(i), "tls"), RuleSecretChannel,
				"TLS syslog export needs its CA/certificate/key references resolved through the API→agent secret channel, which this agent build does not have yet (PENDING-secret-channel); use tcp or udp")
			bad = true
		}
		if v := t.GetVrf(); v != "" && v != "default" {
			// rsyslog runs in the host's default network namespace: the export goes out through the host's routing,
			// never through a VPP VRF (a per-VRF export is F-logging's). The leaf is noted, not applied.
			s.Warnf(Ptr("management", "syslog", strconv.Itoa(i), "vrf"), "agent.unsupported-field",
				"syslog export uses the host's default routing; VRF %q is not applied by this agent build", v)
			t.Vrf = nil
		}
	}
	if !bad {
		s.Add(rsyslog.Key, in, Ptr("management", "syslog"))
	}
}

// AssembleSyslog adds the targets of a retrieved rsyslog object to ds.
func AssembleSyslog(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		in, ok := kv.Value.(*vrxv1.ManagementConfig)
		if kv.Key != rsyslog.Key || !ok {
			continue
		}
		if ds.Management == nil {
			ds.Management = &vrxv1.ManagementConfig{}
		}
		ds.Management.Syslog = in.GetSyslog()
	}
}

// HostServices is the projection of F-unbound-chrony-syslog, one call in agent.project(): services.dns and
// services.ntp when `services` is authoritative (with the agent.unsupported-field notes for the services sub-keys
// no builder projects), management.syslog when `management` is (notes for its other leaves).
func HostServices(s Sink, ds *vrxv1.DesiredState, services, management bool) {
	if services {
		DNS(s, ds)
		NTP(s, ds)
		ServicesUnsupported(s, ds.GetServices())
	}
	if management {
		Syslog(s, ds)
		ManagementUnsupported(s, ds.GetManagement())
	}
}

// AssembleHostServices is the assembler of F-unbound-chrony-syslog, one call in agent.assemble().
func AssembleHostServices(ds *vrxv1.DesiredState, kvs []scheduler.KV, services, management bool) {
	if services {
		AssembleDNS(ds, kvs)
		AssembleNTP(ds, kvs)
	}
	if management {
		AssembleSyslog(ds, kvs)
	}
}
