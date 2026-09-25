package desired

// Desired-state builder and assembler of `services.ipfix` (F-ipfix-sflow, WBS D7.6) on DF-8's
// descriptors (D-104: used as they are):
//
//	services.ipfix.exporters.<n>  first enabled exporter with an IPv4 collector (by name)
//	                                 → ipfix.default-exporter/global (exporter 0, the only one flowprobe uses)
//	                              every other enabled exporter → ipfix.exporter/<collector>
//	services.ipfix.flowprobe      with ≥1 interface → flowprobe.params/global
//	  .interfaces[i]              → flowprobe.interface/<interface> {which: the one of l2|ip4|ip6, direction}
//	services.ipfix.sflow          enabled → sflow.global/global {samplingN, pollingIntervalSec, headerBytes, rx}
//	  .interfaces[i]              → sflow.interface/<interface>
//
// Interface references are logical names (D-065/D-069): the descriptors depend on interface/<name>.
// Exporter 0, flowprobe.params and sflow.global are VPP globals (D-071): the globals owner sets them,
// every other agent only requires them (the descriptors are registered without the role), so their
// leaves cannot be retrieved there and are reported as agent.unsupported-field (not compared by the
// API's drift). Leaves that are not VPP state (exporter description, disabled exporters, sFlow
// collectors / agent address / VRF — hsflowd's, not shipped) are reported the same way. The other
// `services` sub-trees are not implemented by this build: agent.unsupported-field at /services/<key>.

import (
	"net/netip"
	"sort"
	"strconv"
	"sync"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/flowprobe"
	"ngfw/agent/internal/descriptors/ipfix"
	"ngfw/agent/internal/descriptors/sflow"
	"ngfw/agent/internal/scheduler"
)

// Schema defaults of services.ipfix (packages/schema/src/domains/services.ts), applied when a leaf
// is unset in the message (the API sends parsed documents, so this is belt and braces).
const (
	defPathMTU          = 512
	defTemplateInterval = 20
	defActiveTimer      = 15
	defPassiveTimer     = 120
	defSflowSampling    = 10000
	defSflowPolling     = 20
	defSflowHeader      = 128
	defSflowDirection   = "rx" // the schema has no direction: VPP's default
)

// RuleUnsupported is the rule of leaves this agent does not apply or cannot retrieve (the API's
// drift skips them, apps/api state.controller driftOf).
const RuleUnsupported = "agent.unsupported-field"

var ipfixState = struct {
	sync.Mutex
	globalsOwner bool
	names        map[string]string // collector address → exporter name of the stored (applied) document
}{names: map[string]string{}}

// SetIpfixGlobalsOwner records the D-071 role of this agent for the projection (subsystems sets it
// when it registers the ipfix/flowprobe/sflow descriptors).
func SetIpfixGlobalsOwner(owner bool) {
	ipfixState.Lock()
	defer ipfixState.Unlock()
	ipfixState.globalsOwner = owner
}

// IpfixGlobalsOwner reports the role SetIpfixGlobalsOwner recorded.
func IpfixGlobalsOwner() bool {
	ipfixState.Lock()
	defer ipfixState.Unlock()
	return ipfixState.globalsOwner
}

// IpfixExporterName is the configuration name of the exporter with this collector address in the
// stored desired state ("" when unknown). VPP keeps no names; Retrieve and IpfixState use it.
func IpfixExporterName(collector string) string {
	ipfixState.Lock()
	defer ipfixState.Unlock()
	return ipfixState.names[collector]
}

func or32(p *uint32, d uint32) uint32 {
	if p == nil {
		return d
	}
	return *p
}

func orBool(p *bool, d bool) bool {
	if p == nil {
		return d
	}
	return *p
}

func orStr(p *string, d string) string {
	if p == nil {
		return d
	}
	return *p
}

// SetIpfixExporterNames records the exporter names of the agent's STORED desired state (called by
// the service when it refreshes its snapshot after a successful apply/revert and at start) — never
// from a projection, so DryRun, failed or rolled-back transactions do not change them.
func SetIpfixExporterNames(svc *vrxv1.ServicesConfig) {
	names := map[string]string{}
	for name, e := range svc.GetIpfix().GetExporters() {
		if !orBool(e.Enabled, true) {
			continue
		}
		if c, err := netip.ParseAddr(e.GetCollector().GetAddress()); err == nil {
			if prev, dup := names[c.Unmap().String()]; !dup || name < prev {
				names[c.Unmap().String()] = name
			}
		}
	}
	ipfixState.Lock()
	ipfixState.names = names
	ipfixState.Unlock()
}

// IpfixSflow emits the objects of services.ipfix. vrfID maps a VRF name to its table id.
func IpfixSflow(s Sink, svc *vrxv1.ServicesConfig, vrfID func(string) (uint32, bool)) {
	if svc == nil {
		return
	}
	unsupportedServices(s, svc)
	ix := svc.GetIpfix()
	if ix == nil {
		return
	}
	owner := IpfixGlobalsOwner()
	defaultName := exporters(s, ix, vrfID, owner, map[string]string{})

	fp := ix.GetFlowprobe()
	if len(fp.GetInterfaces()) == 0 {
		if fp != nil && proto.Size(fp) > 0 {
			s.Warnf(Ptr("services", "ipfix", "flowprobe"), RuleUnsupported, "flowprobe parameters are applied only with at least one monitored interface")
		}
	} else {
		flowprobeObjects(s, fp, owner, defaultName)
	}
	sflowObjects(s, ix.GetSflow(), owner)
}

// unsupportedServices reports the services sub-trees this build does not implement.
func unsupportedServices(s Sink, svc *vrxv1.ServicesConfig) {
	for _, it := range []struct {
		key string
		m   proto.Message
		set bool
	}{
		{"dhcp", svc.GetDhcp(), svc.GetDhcp() != nil},
		{"dns", svc.GetDns(), svc.GetDns() != nil},
		{"snmp", svc.GetSnmp(), svc.GetSnmp() != nil},
		{"lldp", svc.GetLldp(), svc.GetLldp() != nil},
		{"ntp", svc.GetNtp(), svc.GetNtp() != nil},
		{"qos", svc.GetQos(), svc.GetQos() != nil},
	} {
		if it.set && proto.Size(it.m) > 0 {
			s.Warnf(Ptr("services", it.key), RuleUnsupported, "services.%s is not implemented by this agent build and is not applied", it.key)
		}
	}
}

// exporters projects the exporters and returns the name of the one that became exporter 0 ("" when none).
func exporters(s Sink, ix *vrxv1.IpfixService, vrfID func(string) (uint32, bool), owner bool, names map[string]string) string {
	defaultName := ""
	for _, name := range sortedKeys(ix.GetExporters()) {
		e := ix.GetExporters()[name]
		pt := Ptr("services", "ipfix", "exporters", name)
		if !orBool(e.Enabled, true) {
			s.Warnf(pt, RuleUnsupported, "exporter %s is disabled and not applied", name)
			continue
		}
		if e.Description != nil {
			s.Warnf(Ptr("services", "ipfix", "exporters", name, "description"), RuleUnsupported, "the exporter description is not VPP state")
		}
		c, err := netip.ParseAddr(e.GetCollector().GetAddress())
		if err != nil {
			s.Errorf(Ptr("services", "ipfix", "exporters", name, "collector", "address"), "services.ipfix-exporter", "collector address: %v", err)
			continue
		}
		src, err := netip.ParseAddr(e.GetSourceAddress())
		if err != nil {
			s.Errorf(Ptr("services", "ipfix", "exporters", name, "sourceAddress"), "services.ipfix-exporter", "source address: %v", err)
			continue
		}
		vrf := orStr(e.Vrf, "default")
		table, ok := vrfID(vrf)
		if !ok {
			s.Errorf(Ptr("services", "ipfix", "exporters", name, "vrf"), "services.vrf-exists", "VRF %q does not exist", vrf)
			continue
		}
		port := or32(e.GetCollector().Port, ipfix.DefaultCollectorPort)
		if port > 65535 {
			s.Errorf(Ptr("services", "ipfix", "exporters", name, "collector", "port"), "services.ipfix-exporter", "port %d out of range", port)
			continue
		}
		v := ipfix.Exporter{
			Collector: c.Unmap().String(), CollectorPort: uint16(port), Src: src.Unmap().String(), VRF: table,
			PathMTU: or32(e.PathMtu, defPathMTU), TemplateInterval: or32(e.TemplateIntervalSec, defTemplateInterval),
			UDPChecksum: e.GetUdpChecksum(),
		}
		if err := v.Validate(); err != nil {
			s.Errorf(pt, "services.ipfix-exporter", "%v", err)
			continue
		}
		names[v.Collector] = name
		if defaultName == "" && c.Unmap().Is4() {
			defaultName = name
			s.Add(ipfix.KeyDefaultExporter, v.Proto(), pt)
			if !owner {
				s.Warnf(pt, RuleUnsupported, "exporter 0 is a VPP global: this agent is not the globals owner and only requires it (D-071); it is not retrieved")
			}
			continue
		}
		s.Add(scheduler.Join(ipfix.NameExporter, v.Collector), v.Proto(), pt)
	}
	return defaultName
}

func flowprobeObjects(s Sink, fp *vrxv1.IpfixService_Flowprobe, owner bool, defaultName string) {
	base := []string{"services", "ipfix", "flowprobe"}
	params := flowprobe.Params{
		RecordL2: orBool(fp.RecordL2, false), RecordL3: orBool(fp.RecordL3, true), RecordL4: orBool(fp.RecordL4, true),
		ActiveTimer: or32(fp.ActiveTimerSec, defActiveTimer), PassiveTimer: or32(fp.PassiveTimerSec, defPassiveTimer),
	}
	if err := params.Validate(); err != nil {
		s.Errorf(Ptr(base...), "services.ipfix-flowprobe", "%v", err)
		return
	}
	if defaultName == "" {
		s.Errorf(Ptr(append(base, "interfaces")...), "services.ipfix-sflow-flowprobe-exporter", "flowprobe records are sent through IPFIX exporter 0 only: enable an exporter with an IPv4 collector")
		return
	}
	s.Add(flowprobe.KeyParams, params.Proto(), Ptr(base...))
	if !owner {
		for _, leaf := range []string{"activeTimerSec", "passiveTimerSec", "recordL2", "recordL3", "recordL4"} {
			s.Warnf(Ptr(append(base, leaf)...), RuleUnsupported, "flowprobe parameters are a VPP global: this agent only requires them (D-071); not retrieved")
		}
	}
	for i, f := range fp.GetInterfaces() {
		pt := Ptr(append(base, "interfaces", strconv.Itoa(i))...)
		// VPP records one variant per interface (DF-8 flowprobe.md): the schema's default ip4+ip6 is
		// realised as ip4 (precedence ip4, ip6, l2); the dropped flags are reported, not applied.
		var which string
		for _, c := range []struct {
			name string
			on   bool
		}{{"ip4", orBool(f.Ip4, true)}, {"ip6", orBool(f.Ip6, true)}, {"l2", orBool(f.L2, false)}} {
			if !c.on {
				continue
			}
			if which == "" {
				which = c.name
				continue
			}
			s.Warnf(Ptr(append(base, "interfaces", strconv.Itoa(i), c.name)...), RuleUnsupported,
				"VPP records one flowprobe variant per interface: %s records %s flows only, %s is not applied", f.GetInterface(), which, c.name)
		}
		if which == "" {
			s.Errorf(pt, "services.ipfix-flowprobe", "enable one of l2, ip4, ip6 on %s", f.GetInterface())
			continue
		}
		v := flowprobe.Interface{Interface: f.GetInterface(), Which: which, Direction: orStr(f.Direction, "both")}
		if err := v.Validate(); err != nil {
			s.Errorf(pt, "services.ipfix-flowprobe", "%v", err)
			continue
		}
		s.Add(scheduler.Join(flowprobe.NameInterface, v.Interface), v.Proto(), pt)
	}
}

func sflowObjects(s Sink, sf *vrxv1.IpfixService_Sflow, owner bool) {
	if sf == nil {
		return
	}
	base := []string{"services", "ipfix", "sflow"}
	if !sf.GetEnabled() {
		s.Warnf(Ptr(base...), RuleUnsupported, "sFlow is disabled: nothing is applied")
		return
	}
	for _, leaf := range []string{"collectors", "agentAddress", "vrf"} {
		s.Warnf(Ptr(append(base, leaf)...), RuleUnsupported, "sFlow export to collectors is hsflowd's job, which this build does not ship: VPP samples, nothing is exported")
	}
	g := sflow.Global{
		SamplingRate: or32(sf.SamplingN, defSflowSampling), PollingInterval: or32(sf.PollingIntervalSec, defSflowPolling),
		HeaderBytes: or32(sf.HeaderBytes, defSflowHeader), Direction: defSflowDirection,
	}
	if err := g.Validate(); err != nil {
		s.Errorf(Ptr(append(base, "headerBytes")...), "services.ipfix-sflow-header-bytes", "%v", err)
		return
	}
	s.Add(sflow.KeyGlobal, g.Proto(), Ptr(base...))
	if !owner {
		for _, leaf := range []string{"samplingN", "pollingIntervalSec", "headerBytes"} {
			s.Warnf(Ptr(append(base, leaf)...), RuleUnsupported, "sFlow parameters are a VPP global: this agent only requires them (D-071); not retrieved")
		}
	}
	for i, name := range sf.GetInterfaces() {
		if name == "" {
			s.Errorf(Ptr(append(base, "interfaces", strconv.Itoa(i))...), "services.ipfix-sflow", "empty interface name")
			continue
		}
		s.Add(scheduler.Join(sflow.NameInterface, name), sflow.Interface{Interface: name}.Proto(), Ptr(append(base, "interfaces", strconv.Itoa(i))...))
	}
}

// AssembleIpfixSflow adds services.ipfix to ds from retrieved KVs. tableName maps a table id to its
// VRF name.
func AssembleIpfixSflow(ds *vrxv1.DesiredState, kvs []scheduler.KV, tableName func(uint32) string) {
	ix := &vrxv1.IpfixService{}
	var exps []ipfix.Exporter
	var defaultExp *ipfix.Exporter
	var params *flowprobe.Params
	var fpIfs []flowprobe.Interface
	var global *sflow.Global
	var sfIfs []string
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case ipfix.NameDefaultExporter:
			var e ipfix.Exporter
			if dfkit.Decode(kv.Value, &e) == nil {
				defaultExp = &e
			}
		case ipfix.NameExporter:
			var e ipfix.Exporter
			if dfkit.Decode(kv.Value, &e) == nil {
				exps = append(exps, e)
			}
		case flowprobe.NameParams:
			var p flowprobe.Params
			if dfkit.Decode(kv.Value, &p) == nil {
				params = &p
			}
		case flowprobe.NameInterface:
			var f flowprobe.Interface
			if dfkit.Decode(kv.Value, &f) == nil {
				fpIfs = append(fpIfs, f)
			}
		case sflow.NameGlobal:
			var g sflow.Global
			if dfkit.Decode(kv.Value, &g) == nil {
				global = &g
			}
		case sflow.NameInterface:
			var i sflow.Interface
			if dfkit.Decode(kv.Value, &i) == nil {
				sfIfs = append(sfIfs, i.Interface)
			}
		}
	}
	addExp := func(e ipfix.Exporter, fallback string) {
		name := IpfixExporterName(e.Collector)
		if name == "" {
			name = fallback
		}
		if ix.Exporters == nil {
			ix.Exporters = map[string]*vrxv1.IpfixService_Exporter{}
		}
		vrf := "default"
		if e.VRF != ipfix.NoVRF {
			vrf = tableName(e.VRF)
		}
		ix.Exporters[name] = &vrxv1.IpfixService_Exporter{
			Enabled:             proto.Bool(true),
			Collector:           &vrxv1.SocketAddress{Address: proto.String(e.Collector), Port: proto.Uint32(uint32(e.CollectorPort))},
			SourceAddress:       proto.String(e.Src),
			Vrf:                 proto.String(vrf),
			PathMtu:             proto.Uint32(e.PathMTU),
			TemplateIntervalSec: proto.Uint32(e.TemplateInterval),
			UdpChecksum:         proto.Bool(e.UDPChecksum),
		}
	}
	if defaultExp != nil {
		addExp(*defaultExp, "exporter-0")
	}
	for _, e := range exps {
		addExp(e, "collector-"+sanitizeName(e.Collector))
	}
	if params != nil || len(fpIfs) > 0 {
		fp := &vrxv1.IpfixService_Flowprobe{}
		if params != nil {
			fp.ActiveTimerSec = proto.Uint32(params.ActiveTimer)
			fp.PassiveTimerSec = proto.Uint32(params.PassiveTimer)
			fp.RecordL2 = proto.Bool(params.RecordL2)
			fp.RecordL3 = proto.Bool(params.RecordL3)
			fp.RecordL4 = proto.Bool(params.RecordL4)
		}
		sort.Slice(fpIfs, func(a, b int) bool { return fpIfs[a].Interface < fpIfs[b].Interface })
		for _, f := range fpIfs {
			fp.Interfaces = append(fp.Interfaces, &vrxv1.IpfixService_Flowprobe_Interface{
				Interface: proto.String(f.Interface), Direction: proto.String(f.Direction),
				L2: proto.Bool(f.Which == "l2"), Ip4: proto.Bool(f.Which == "ip4"), Ip6: proto.Bool(f.Which == "ip6"),
			})
		}
		ix.Flowprobe = fp
	}
	if global == nil && len(sfIfs) > 0 && IpfixGlobalsOwner() {
		g := sflow.DefaultGlobal() // Retrieve reports sflow.global only while it differs from VPP's defaults
		global = &g
	}
	if global != nil || len(sfIfs) > 0 {
		sf := &vrxv1.IpfixService_Sflow{Enabled: proto.Bool(true)}
		if global != nil {
			sf.SamplingN = proto.Uint32(global.SamplingRate)
			sf.PollingIntervalSec = proto.Uint32(global.PollingInterval)
			sf.HeaderBytes = proto.Uint32(global.HeaderBytes)
		}
		sort.Strings(sfIfs)
		sf.Interfaces = sfIfs
		ix.Sflow = sf
	}
	if proto.Size(ix) == 0 {
		return
	}
	if ds.Services == nil {
		ds.Services = &vrxv1.ServicesConfig{}
	}
	ds.Services.Ipfix = ix
}

// sanitizeName turns an address into an objectName-safe suffix.
func sanitizeName(a string) string {
	b := []byte(a)
	for i, c := range b {
		if c == '.' || c == ':' {
			b[i] = '-'
		}
	}
	return string(b)
}
