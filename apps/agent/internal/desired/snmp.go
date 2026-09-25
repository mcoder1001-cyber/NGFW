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
	snmpMu    sync.RWMutex
	snmpCheck SnmpCheck
)

// SetSnmpCheck installs the check (subsystems.registerSnmp); nil removes it.
func SetSnmpCheck(f SnmpCheck) {
	snmpMu.Lock()
	defer snmpMu.Unlock()
	snmpCheck = f
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
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"dhcp", proto.Size(svc.GetDhcp()) > 0}, {"dns", proto.Size(svc.GetDns()) > 0},
		{"lldp", proto.Size(svc.GetLldp()) > 0}, {"ipfix", proto.Size(svc.GetIpfix()) > 0},
		{"ntp", proto.Size(svc.GetNtp()) > 0}, {"qos", proto.Size(svc.GetQos()) > 0},
	} {
		if f.set {
			p.Warnf(Ptr("services", f.name), "agent.unsupported-field", "services.%s is not implemented by this agent build and is not applied", f.name)
		}
	}
	snmp := svc.GetSnmp()
	if !snmp.GetEnabled() {
		return // no object: an applied configuration is replaced by the disabled rendering (Delete)
	}
	snmpMu.RLock()
	check := snmpCheck
	snmpMu.RUnlock()
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
