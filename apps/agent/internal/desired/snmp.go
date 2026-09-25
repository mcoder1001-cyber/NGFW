package desired

// F-snmp: `services.snmp` → one singleton scheduler object `snmpd.config/vrx` (D-109 (d)) whose value
// is the SnmpService message itself; its descriptor (internal/subsystems/snmp.go) wraps RF-4's snmpd
// renderer. The structural check of the renderer (secret refs resolved, every field validated, D-125:
// daemon config validated before any VPP write of the same transaction) runs here, in the projection,
// through the hook the descriptor installs, so a bad `services.snmp` is an issue with a JSON pointer and
// the transaction never starts.

import (
	"regexp"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// SnmpDescriptorName is the singleton snmpd descriptor.
const SnmpDescriptorName = "snmpd.config"

// SnmpKey is its only key.
var SnmpKey = scheduler.Join(SnmpDescriptorName, "vrx")

// SnmpCheck validates a services.snmp message (render + daemon parse run); its error text names the
// field by its dotted path ("services.snmp.communities.ro.secretRef …") and never carries a secret.
type SnmpCheck func(*vrxv1.SnmpService) error

var (
	snmpMu     sync.RWMutex
	snmpChecks = map[string]SnmpCheck{} // owner → check (one stage per agent owner)
)

// Lifecycle limitation: the check is removed by SnmpStage.Close, but the agent's shutdown path
// (internal/agent/service.go, agent core, not F-snmp's file) does not call it yet; in the product one
// agent runs per process and the registration ends with the process. Wiring Close into the shutdown
// (with the other Wiring teardown) is left to the agent-core owner (docs/status/tasks/F-snmp-questions.md).

// SetSnmpCheck installs the check of an owner's stage; nil removes it (stage Close).
func SetSnmpCheck(owner string, f SnmpCheck) {
	snmpMu.Lock()
	defer snmpMu.Unlock()
	if f == nil {
		delete(snmpChecks, owner)
		return
	}
	snmpChecks[owner] = f
}

// snmpCheckFor returns the check to run. project() has no owner parameter (agent core), so the lookup
// uses the only registered owner; with several owners in one process it fails closed (ok=false).
func snmpCheckFor() (SnmpCheck, bool) {
	snmpMu.RLock()
	defer snmpMu.RUnlock()
	switch len(snmpChecks) {
	case 0:
		return nil, true
	case 1:
		for _, f := range snmpChecks {
			return f, true
		}
	}
	return nil, false
}

// SnapshotSnmpChecks copies the registered checks (tests restore them with t.Cleanup).
func SnapshotSnmpChecks() map[string]SnmpCheck {
	snmpMu.RLock()
	defer snmpMu.RUnlock()
	out := make(map[string]SnmpCheck, len(snmpChecks))
	for k, v := range snmpChecks {
		out[k] = v
	}
	return out
}

// RestoreSnmpChecks replaces the registered checks (nil = none).
func RestoreSnmpChecks(m map[string]SnmpCheck) {
	snmpMu.Lock()
	defer snmpMu.Unlock()
	snmpChecks = map[string]SnmpCheck{}
	for k, v := range m {
		snmpChecks[k] = v
	}
}

var pathRe = regexp.MustCompile(`services\.snmp(?:\.[A-Za-z0-9_-]+|\[[0-9]+\])*`)

// SnmpPointer turns the first dotted path in msg into an RFC 6901 pointer (default /services/snmp).
func SnmpPointer(msg string) string {
	m := pathRe.FindString(msg)
	if m == "" {
		return Ptr("services", "snmp")
	}
	m = strings.NewReplacer("[", ".", "]", "").Replace(m)
	return Ptr(strings.Split(m, ".")...)
}

// Snmp projects services (the `services` domain is authoritative in this transaction).
func Snmp(p Sink, svc *vrxv1.ServicesConfig) {
	snmp := svc.GetSnmp()
	if !snmp.GetEnabled() {
		return // no object: an applied configuration is replaced by the disabled rendering (Delete)
	}
	check, ok := snmpCheckFor()
	if !ok {
		p.Errorf(Ptr("services", "snmp"), "services.snmp.render", "several agent owners registered an snmpd stage in one process; refusing to guess")
		return
	}
	if check != nil {
		if err := check(snmp); err != nil {
			p.Errorf(SnmpPointer(err.Error()), "services.snmp.render", "%v", err)
			return
		}
	}
	p.Add(SnmpKey, proto.Clone(snmp), Ptr("services", "snmp"))
}

// AssembleSnmp adds the retrieved services.snmp (the applied value, when the live file still matches it).
func AssembleSnmp(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if kv.Key != SnmpKey {
			continue
		}
		if v, ok := kv.Value.(*vrxv1.SnmpService); ok {
			if ds.Services == nil {
				ds.Services = &vrxv1.ServicesConfig{}
			}
			ds.Services.Snmp = proto.Clone(v).(*vrxv1.SnmpService)
		}
	}
}
