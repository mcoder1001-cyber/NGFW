package coretest

// F-ipfix-sflow (wave-A-hotspots A6 pattern: own file + one install line in New): a stateful model
// of VPP's IPFIX exporter pool (index 0 = exporter 0), flowprobe parameters/interfaces and sFlow
// global/interfaces, so the agent's `services` domain runs against the same fake as the core.
// sFlow reports hw_if_index = sw_if_index + HwOffset (VPP V17: the descriptor must learn the map).

import (
	"context"
	"net/netip"
	"os"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/flowprobe"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipfix_export"
	"ngfw/agent/binapi/sflow"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// HwOffset is the fake's hw_if_index − sw_if_index of sFlow interfaces.
const HwOffset = 100

// FlowState is the modelled flow-export state (guarded by VPP.mu).
type FlowState struct {
	Exporters []ipfix_export.IpfixAllExporterDetails // [0] = exporter 0
	Params    flowprobe.FlowprobeGetParamsReply
	Flowprobe map[uint32]flowprobe.FlowprobeInterfaceDetails
	Sflow     map[uint32]bool // sw_if_index → enabled
	SflowG    struct{ Rate, Polling, Header, Dir, Drop uint32 }
	nextStat  uint32
}

func unspec4() ip_types.Address { return ip_types.NewAddress(netip.IPv4Unspecified().AsSlice()) }

// Flow returns a copy of the flow-export state.
func (v *VPP) Flow() FlowState {
	v.mu.Lock()
	defer v.mu.Unlock()
	f := *v.flow
	f.Exporters = append([]ipfix_export.IpfixAllExporterDetails(nil), v.flow.Exporters...)
	f.Flowprobe = map[uint32]flowprobe.FlowprobeInterfaceDetails{}
	for k, d := range v.flow.Flowprobe {
		f.Flowprobe[k] = d
	}
	f.Sflow = map[uint32]bool{}
	for k, d := range v.flow.Sflow {
		f.Sflow[k] = d
	}
	return f
}

func (v *VPP) installIpfixSflow() {
	// The fake VPP is not a process: the DF-8 descriptors (sflow's learned hw→sw map) need a complete
	// D-080 identity, so use a fixed one as dfkittest.NewFake does (never the host's /proc).
	dfkit.IdentitySource = func(ctx context.Context, c vpp.Client) (bootid.Identity, error) {
		id, err := bootid.Reader{ProcRoot: os.DevNull}.Current(ctx, c)
		if err != nil {
			return bootid.Identity{}, err
		}
		id.BootID, id.StartTime = "coretest", 1
		return id, nil
	}
	v.flow = &FlowState{
		Exporters: []ipfix_export.IpfixAllExporterDetails{{PathMtu: 512, TemplateInterval: 20, VrfID: ^uint32(0), CollectorAddress: unspec4(), SrcAddress: unspec4()}},
		Flowprobe: map[uint32]flowprobe.FlowprobeInterfaceDetails{},
		Sflow:     map[uint32]bool{},
	}
	v.flow.Params = flowprobe.FlowprobeGetParamsReply{ActiveTimer: 15, PassiveTimer: 120}
	v.flow.SflowG.Rate, v.flow.SflowG.Polling, v.flow.SflowG.Header, v.flow.SflowG.Dir = 10000, 20, 128, 1
	f := v.flow
	lock := func(fn func() api.Message) func(api.Message) ([]api.Message, error) {
		return func(api.Message) ([]api.Message, error) {
			v.mu.Lock()
			defer v.mu.Unlock()
			return reply(fn())
		}
	}
	lookup := func(a ip_types.Address) int {
		for i := 1; i < len(f.Exporters); i++ {
			if f.Exporters[i].CollectorAddress == a {
				return i
			}
		}
		return -1
	}
	v.On("ipfix_exporter_create_delete", func(m api.Message) ([]api.Message, error) {
		r := m.(*ipfix_export.IpfixExporterCreateDelete)
		v.mu.Lock()
		defer v.mu.Unlock()
		i := lookup(r.CollectorAddress)
		if !r.IsCreate {
			if i < 0 {
				return reply(&ipfix_export.IpfixExporterCreateDeleteReply{Retval: RetvalNoSuchEntry})
			}
			f.Exporters = append(f.Exporters[:i], f.Exporters[i+1:]...)
			return reply(&ipfix_export.IpfixExporterCreateDeleteReply{})
		}
		e := ipfix_export.IpfixAllExporterDetails{CollectorAddress: r.CollectorAddress, CollectorPort: r.CollectorPort, SrcAddress: r.SrcAddress,
			VrfID: r.VrfID, PathMtu: r.PathMtu, TemplateInterval: r.TemplateInterval, UDPChecksum: r.UDPChecksum}
		if i < 0 {
			f.Exporters = append(f.Exporters, e)
		} else {
			f.Exporters[i] = e
		}
		f.nextStat++
		return reply(&ipfix_export.IpfixExporterCreateDeleteReply{StatIndex: 40 + f.nextStat})
	})
	v.On("ipfix_all_exporter_get", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for i := range f.Exporters {
			d := f.Exporters[i]
			out = append(out, &d)
		}
		return append(out, &ipfix_export.IpfixAllExporterGetReply{Cursor: ^uint32(0)}), nil
	})
	v.On("set_ipfix_exporter", func(m api.Message) ([]api.Message, error) {
		r := m.(*ipfix_export.SetIpfixExporter)
		v.mu.Lock()
		defer v.mu.Unlock()
		f.Exporters[0] = ipfix_export.IpfixAllExporterDetails{CollectorAddress: r.CollectorAddress, CollectorPort: r.CollectorPort, SrcAddress: r.SrcAddress,
			VrfID: r.VrfID, PathMtu: r.PathMtu, TemplateInterval: r.TemplateInterval, UDPChecksum: r.UDPChecksum}
		return reply(&ipfix_export.SetIpfixExporterReply{})
	})
	v.On("ipfix_exporter_dump", lock(func() api.Message {
		e := f.Exporters[0]
		return &ipfix_export.IpfixExporterDetails{CollectorAddress: e.CollectorAddress, CollectorPort: e.CollectorPort, SrcAddress: e.SrcAddress,
			VrfID: e.VrfID, PathMtu: e.PathMtu, TemplateInterval: e.TemplateInterval, UDPChecksum: e.UDPChecksum}
	}))
	v.On("flowprobe_set_params", func(m api.Message) ([]api.Message, error) {
		r := m.(*flowprobe.FlowprobeSetParams)
		v.mu.Lock()
		defer v.mu.Unlock()
		if len(f.Flowprobe) > 0 {
			return reply(&flowprobe.FlowprobeSetParamsReply{Retval: int32(api.UNSUPPORTED)})
		}
		f.Params = flowprobe.FlowprobeGetParamsReply{RecordFlags: r.RecordFlags, ActiveTimer: r.ActiveTimer, PassiveTimer: r.PassiveTimer}
		return reply(&flowprobe.FlowprobeSetParamsReply{})
	})
	v.On("flowprobe_get_params", lock(func() api.Message { p := f.Params; return &p }))
	v.On("flowprobe_interface_add_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*flowprobe.FlowprobeInterfaceAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		idx := uint32(r.SwIfIndex)
		cur, exists := f.Flowprobe[idx]
		switch {
		case v.Ifaces[idx] == nil:
			return reply(&flowprobe.FlowprobeInterfaceAddDelReply{Retval: RetvalInvalidSwIfIndex})
		case f.Params.RecordFlags == 0:
			return reply(&flowprobe.FlowprobeInterfaceAddDelReply{Retval: int32(api.CANNOT_ENABLE_DISABLE_FEATURE)})
		case r.IsAdd && exists:
			return reply(&flowprobe.FlowprobeInterfaceAddDelReply{Retval: int32(api.ENTRY_ALREADY_EXISTS)})
		case !r.IsAdd && (!exists || cur.Which != r.Which):
			return reply(&flowprobe.FlowprobeInterfaceAddDelReply{Retval: int32(api.NO_SUCH_ENTRY)})
		case r.IsAdd:
			f.Flowprobe[idx] = flowprobe.FlowprobeInterfaceDetails{SwIfIndex: r.SwIfIndex, Which: r.Which, Direction: r.Direction}
		default:
			delete(f.Flowprobe, idx)
		}
		return reply(&flowprobe.FlowprobeInterfaceAddDelReply{})
	})
	v.On("flowprobe_interface_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, d := range f.Flowprobe {
			d := d
			out = append(out, &d)
		}
		return out, nil
	})
	v.On("sflow_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*sflow.SflowEnableDisable)
		v.mu.Lock()
		defer v.mu.Unlock()
		sw := uint32(r.HwIfIndex) // VPP reads a sw_if_index from the field named hw_if_index (V17)
		if v.Ifaces[sw] == nil {
			return reply(&sflow.SflowEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		if f.Sflow[sw] == r.EnableDisable {
			return reply(&sflow.SflowEnableDisableReply{Retval: int32(api.VALUE_EXIST)})
		}
		if r.EnableDisable {
			f.Sflow[sw] = true
		} else {
			delete(f.Sflow, sw)
		}
		return reply(&sflow.SflowEnableDisableReply{})
	})
	v.On("sflow_interface_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for sw := range f.Sflow {
			out = append(out, &sflow.SflowInterfaceDetails{HwIfIndex: interface_types.InterfaceIndex(sw + HwOffset)})
		}
		return out, nil
	})
	set := func(p *uint32, get func(api.Message) uint32, rep api.Message) func(api.Message) ([]api.Message, error) {
		return func(m api.Message) ([]api.Message, error) {
			v.mu.Lock()
			defer v.mu.Unlock()
			*p = get(m)
			return reply(rep)
		}
	}
	g := &f.SflowG
	v.On("sflow_sampling_rate_set", set(&g.Rate, func(m api.Message) uint32 { return m.(*sflow.SflowSamplingRateSet).SamplingN }, &sflow.SflowSamplingRateSetReply{}))
	v.On("sflow_polling_interval_set", set(&g.Polling, func(m api.Message) uint32 { return m.(*sflow.SflowPollingIntervalSet).PollingS }, &sflow.SflowPollingIntervalSetReply{}))
	v.On("sflow_header_bytes_set", set(&g.Header, func(m api.Message) uint32 { return m.(*sflow.SflowHeaderBytesSet).HeaderB }, &sflow.SflowHeaderBytesSetReply{}))
	v.On("sflow_direction_set", set(&g.Dir, func(m api.Message) uint32 { return m.(*sflow.SflowDirectionSet).SamplingD }, &sflow.SflowDirectionSetReply{}))
	v.On("sflow_drop_monitoring_set", set(&g.Drop, func(m api.Message) uint32 { return m.(*sflow.SflowDropMonitoringSet).DropM }, &sflow.SflowDropMonitoringSetReply{}))
	v.On("sflow_sampling_rate_get", lock(func() api.Message { return &sflow.SflowSamplingRateGetReply{SamplingN: g.Rate} }))
	v.On("sflow_polling_interval_get", lock(func() api.Message { return &sflow.SflowPollingIntervalGetReply{PollingS: g.Polling} }))
	v.On("sflow_header_bytes_get", lock(func() api.Message { return &sflow.SflowHeaderBytesGetReply{HeaderB: g.Header} }))
	v.On("sflow_direction_get", lock(func() api.Message { return &sflow.SflowDirectionGetReply{SamplingD: g.Dir} }))
	v.On("sflow_drop_monitoring_get", lock(func() api.Message { return &sflow.SflowDropMonitoringGetReply{DropM: g.Drop} }))
}
