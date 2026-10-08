package desired

// F-unbound-chrony-syslog: the `management` domain (this build implements its syslog leaf only; users, AAA, TLS and
// the later feature keys are agent.unsupported-field until their feature projects them — F-dashboard-prom-alarms
// registers its keys in ManagementImplemented from its own file).
//
//	management.syslog (non-empty) → rsyslog.config/ngfw   Value = rsyslog.Input(document) (*ngfwv1.ManagementConfig)
//
// Without targets there is no object: the reconciler deletes a previous one (rsyslog gets the empty export). TLS
// targets require exact selected sealed certificate/key generations; unavailable references fail closed.

import (
	"strconv"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/rsyslog"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/secretvalue"
)

// ManagementImplemented lists the `management` sub-keys (JSON names) this build projects.
var ManagementImplemented = map[string]bool{"syslog": true}

// ManagementUnsupported warns agent.unsupported-field for every non-empty `management` sub-key no builder projects.
func ManagementUnsupported(s Sink, m *ngfwv1.ManagementConfig) {
	unsupported(s, "management", m, ManagementImplemented)
}

// Syslog projects management.syslog (see the file comment).
func Syslog(s Sink, ds *ngfwv1.DesiredState, options ...HostSecretOptions) {
	in := rsyslog.Input(ds)
	if in == nil {
		return
	}
	bad := false
	for i, t := range in.GetSyslog() {
		if t.GetTls() != nil && (len(options) == 0 || options[0].Ref == nil) {
			s.Errorf(Ptr("management", "syslog", strconv.Itoa(i), "tls"), RuleSecretChannel, "selected TLS credential generations are unavailable")
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
	if bad {
		return
	}
	if value := bindHostSecrets(s, in, Ptr("management", "syslog"), options); value != nil {
		s.Add(rsyslog.Key, value, Ptr("management", "syslog"))
	}
}

// AssembleSyslog adds the targets of a retrieved rsyslog object to ds.
func AssembleSyslog(ds *ngfwv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		in := new(ngfwv1.ManagementConfig)
		_, err := secretvalue.Unwrap(kv.Value, in)
		if kv.Key != rsyslog.Key || err != nil {
			continue
		}
		if ds.Management == nil {
			ds.Management = &ngfwv1.ManagementConfig{}
		}
		ds.Management.Syslog = in.GetSyslog()
	}
}

// HostServices is the projection of F-unbound-chrony-syslog, one call in agent.project(): services.dns and
// services.ntp when `services` is authoritative (the agent.unsupported-field notes for the services sub-keys are
// emitted once by ServicesUnsupported in agent.project), management.syslog when `management` is (notes for its other leaves).
func HostServices(s Sink, ds *ngfwv1.DesiredState, services, management bool, options ...HostSecretOptions) {
	if services {
		DNS(s, ds)
		NTP(s, ds, options...) // services.ServicesUnsupported runs once per projection in agent.project (F-qos-flat block)
	}
	if management {
		Syslog(s, ds, options...)
		ManagementUnsupported(s, ds.GetManagement())
	}
}

// AssembleHostServices is the assembler of F-unbound-chrony-syslog, one call in agent.assemble().
func AssembleHostServices(ds *ngfwv1.DesiredState, kvs []scheduler.KV, services, management bool) {
	if services {
		AssembleDNS(ds, kvs)
		AssembleNTP(ds, kvs)
	}
	if management {
		AssembleSyslog(ds, kvs)
	}
}
