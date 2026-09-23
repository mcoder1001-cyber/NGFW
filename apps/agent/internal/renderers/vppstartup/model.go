// Package vppstartup renders VPP's start-up configuration (/etc/vpp/startup.conf) from the
// `dataplane` domain of the configuration document (WBS D0.6, task F-startup-gen).
//
// The generator is a pure function: BuildModel validates the domain against the host facts it
// is given and returns a Model; Render turns the Model into the file. Nothing here writes the
// live file or restarts VPP — applying a rendering is a manager step (docs/agent/renderers/
// vppstartup.md), so Renderer.Apply always refuses.
package vppstartup

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// ErrInput is wrapped by every error about the desired state or the host facts it is checked
// against (as opposed to I/O errors), so callers can report it as a validation issue. Messages
// name the JSON path of the offending field.
var ErrInput = errors.New("vppstartup: invalid dataplane configuration")

// DefaultBuffersPerNuma is VPP's default `buffers { buffers-per-numa }` (used for the hugepage
// budget when the document does not set it).
const DefaultBuffersPerNuma = 16384

// BufferFootprint is the conservative per-buffer hugepage cost used for the budget: 2048 bytes
// of default data size + vlib_buffer_t metadata + headroom + mempool overhead, rounded up.
const BufferFootprint = 2560

// DPDKPlugin is the plugin file that parses the `dpdk { }` section.
const DPDKPlugin = "dpdk_plugin.so"

const gib = uint64(1) << 30

// Extensions carries dataplane fields the desired-state proto does not have yet (D-055
// stand-in, docs/status/tasks/F-startup-gen-questions.md Q1). They are read from a
// *structpb.Struct document at the JSON paths the schema is expected to use:
//
//	dataplane.managementPci   []string                     NICs never handed to DPDK; always blacklisted
//	dataplane.devices         {<pci>: {name, rxQueues, txQueues, rxDesc, txDesc}}   DPDK devices with logical names (D-069)
//	dataplane.buffersPerNuma  uint32                       buffers { buffers-per-numa }
//	dataplane.plugins         {<file>.so: bool}            plugins { plugin <file> { enable|disable } } (D-060)
type Extensions struct {
	ManagementPCI  []string
	Devices        map[string]DeviceSpec
	BuffersPerNuma uint32
	Plugins        map[string]bool
}

// DeviceSpec is one `dataplane.devices.<pci>` entry. Zero numbers mean "not set".
type DeviceSpec struct {
	Name     string
	RxQueues uint32
	TxQueues uint32
	RxDesc   uint32
	TxDesc   uint32
}

// Host holds the facts of the machine the file is rendered for. Zero values mean "unknown" and
// switch the corresponding check off — except Plugins: a document that names plugins cannot be
// validated without the on-disk list and is rejected.
type Host struct {
	// CPUs is the number of logical CPUs (ids 0..CPUs-1).
	CPUs int
	// IsolCPUs is the kernel's isolated CPU set (isolcpus=, /sys/devices/system/cpu/isolated).
	IsolCPUs []uint32
	// NUMANodes is the number of NUMA nodes (buffers are allocated per node).
	NUMANodes int
	// HugepageBytes is the hugepage memory the host reserves (HugePages_Total × Hugepagesize).
	HugepageBytes uint64
	// Plugins are the plugin file names on disk (/usr/lib/x86_64-linux-gnu/vpp_plugins/*.so).
	Plugins []string
}

// Model is the validated, sorted view of the dataplane domain that the template renders.
type Model struct {
	// HugepagesGB is dataplane.hugepagesGb (0 = unset); rendered as a comment only — the
	// hugepages themselves are reserved by the host (vm.nr_hugepages), not by this file.
	HugepagesGB uint32
	MainCore    *uint32
	// Corelist is the corelist-workers value in VPP range syntax ("2-3,6"); "" = not set.
	Corelist string
	// Workers is `cpu { workers N }` when no corelist is given (0 = not rendered).
	Workers        uint32
	BuffersPerNuma uint32
	// DPDK is false when dpdk_plugin.so is disabled: the whole dpdk section is omitted.
	DPDK bool
	// RxQueues / TxQueues are the `dev default` queue counts (0 = not set).
	RxQueues uint32
	TxQueues uint32
	// Devices are the DPDK devices sorted by PCI address.
	Devices []Device
	// Blacklist are the management (and other excluded) PCI addresses, sorted.
	Blacklist []string
	// Plugins are the explicit plugin switches sorted by file name.
	Plugins []Plugin
	// Warnings are non-fatal findings (printed by the CLI, reported as info by DryRun).
	Warnings []string
}

// HasDefaultQueues reports whether a `dev default { }` block is rendered.
func (m *Model) HasDefaultQueues() bool { return m.RxQueues > 0 || m.TxQueues > 0 }

// Device is one `dev <pci> { … }` entry.
type Device struct {
	PCI      string
	Name     string
	RxQueues uint32
	TxQueues uint32
	RxDesc   uint32
	TxDesc   uint32
}

// HasBlock reports whether the device needs a `{ }` block (any per-device option).
func (d Device) HasBlock() bool {
	return d.Name != "" || d.RxQueues > 0 || d.TxQueues > 0 || d.RxDesc > 0 || d.TxDesc > 0
}

// Plugin is one `plugin <file> { enable|disable }` line.
type Plugin struct {
	File   string
	Enable bool
}

// Desired normalises the renderer input to the dataplane message plus the stand-in extensions:
// a *vrxv1.DesiredState or *vrxv1.DataplaneConfig is used as is (no extensions); a
// *structpb.Struct holding the JSON configuration document is decoded — only its `dataplane`
// member is read, other domains are ignored; nil is the empty domain.
func Desired(msg proto.Message) (*vrxv1.DataplaneConfig, *Extensions, error) {
	ext := &Extensions{}
	switch m := msg.(type) {
	case nil:
		return &vrxv1.DataplaneConfig{}, ext, nil
	case *vrxv1.DesiredState:
		if m.GetDataplane() == nil {
			return &vrxv1.DataplaneConfig{}, ext, nil
		}
		return m.GetDataplane(), ext, nil
	case *vrxv1.DataplaneConfig:
		if m == nil {
			return &vrxv1.DataplaneConfig{}, ext, nil
		}
		return m, ext, nil
	case *structpb.Struct:
		dpv, ok := m.GetFields()["dataplane"]
		if !ok || dpv == nil {
			return &vrxv1.DataplaneConfig{}, ext, nil
		}
		dp := dpv.GetStructValue()
		if dp == nil {
			return nil, nil, fmt.Errorf("%w: dataplane must be an object", ErrInput)
		}
		raw, err := protojson.Marshal(dp)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: encode dataplane: %v", ErrInput, err)
		}
		cfg := &vrxv1.DataplaneConfig{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, cfg); err != nil {
			return nil, nil, fmt.Errorf("%w: decode dataplane: %v", ErrInput, err)
		}
		if err := readExtensions(dp, ext); err != nil {
			return nil, nil, err
		}
		return cfg, ext, nil
	default:
		return nil, nil, fmt.Errorf("%w: unsupported input type %T (want *vrxv1.DesiredState, *vrxv1.DataplaneConfig or *structpb.Struct)", ErrInput, msg)
	}
}

func readExtensions(dp *structpb.Struct, ext *Extensions) error {
	f := dp.GetFields()
	if v, ok := f["managementPci"]; ok {
		list := v.GetListValue()
		if list == nil {
			return fmt.Errorf("%w: dataplane.managementPci must be an array of PCI addresses", ErrInput)
		}
		for i, e := range list.GetValues() {
			s, isStr := e.GetKind().(*structpb.Value_StringValue)
			if !isStr {
				return fmt.Errorf("%w: dataplane.managementPci[%d] must be a string", ErrInput, i)
			}
			ext.ManagementPCI = append(ext.ManagementPCI, s.StringValue)
		}
	}
	if v, ok := f["buffersPerNuma"]; ok {
		n, err := uintValue(v, "dataplane.buffersPerNuma")
		if err != nil {
			return err
		}
		ext.BuffersPerNuma = n
	}
	if v, ok := f["devices"]; ok {
		devs := v.GetStructValue()
		if devs == nil {
			return fmt.Errorf("%w: dataplane.devices must be an object keyed by PCI address", ErrInput)
		}
		ext.Devices = map[string]DeviceSpec{}
		for pci, dv := range devs.GetFields() {
			path := "dataplane.devices." + jsonKey(pci)
			d := dv.GetStructValue()
			if d == nil {
				return fmt.Errorf("%w: %s must be an object", ErrInput, path)
			}
			var spec DeviceSpec
			for k, fv := range d.GetFields() {
				var err error
				switch k {
				case "name":
					s, isStr := fv.GetKind().(*structpb.Value_StringValue)
					if !isStr {
						return fmt.Errorf("%w: %s.name must be a string", ErrInput, path)
					}
					if s.StringValue == "" {
						return fmt.Errorf("%w: %s.name must not be empty (omit it for a VPP-named device)", ErrInput, path)
					}
					spec.Name = s.StringValue
				case "rxQueues":
					spec.RxQueues, err = uintValue(fv, path+".rxQueues")
				case "txQueues":
					spec.TxQueues, err = uintValue(fv, path+".txQueues")
				case "rxDesc":
					spec.RxDesc, err = uintValue(fv, path+".rxDesc")
				case "txDesc":
					spec.TxDesc, err = uintValue(fv, path+".txDesc")
				default:
					return fmt.Errorf("%w: %s: unknown field %s", ErrInput, path, jsonKey(k))
				}
				if err != nil {
					return err
				}
			}
			ext.Devices[pci] = spec
		}
	}
	if v, ok := f["plugins"]; ok {
		pl := v.GetStructValue()
		if pl == nil {
			return fmt.Errorf("%w: dataplane.plugins must be an object {\"<file>.so\": true|false}", ErrInput)
		}
		ext.Plugins = map[string]bool{}
		for name, pv := range pl.GetFields() {
			b, isBool := pv.GetKind().(*structpb.Value_BoolValue)
			if !isBool {
				return fmt.Errorf("%w: dataplane.plugins.%s must be a boolean (true = enable)", ErrInput, jsonKey(name))
			}
			ext.Plugins[name] = b.BoolValue
		}
	}
	return nil
}

func uintValue(v *structpb.Value, path string) (uint32, error) {
	n, ok := v.GetKind().(*structpb.Value_NumberValue)
	if !ok || n.NumberValue < 0 || n.NumberValue > math.MaxUint32 || n.NumberValue != math.Trunc(n.NumberValue) {
		return 0, fmt.Errorf("%w: %s must be an integer 0..4294967295", ErrInput, path)
	}
	return uint32(n.NumberValue), nil
}

// jsonKey quotes a record key for an error path when it is not a plain token, so hostile keys
// (newlines, braces) cannot forge the error text.
func jsonKey(k string) string {
	for i := 0; i < len(k); i++ {
		c := k[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("_.:-", c) >= 0) {
			return fmt.Sprintf("%q", k)
		}
	}
	if k == "" {
		return `""`
	}
	return k
}

func inputErr(path string, format string, args ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrInput, path, fmt.Sprintf(format, args...))
}

// BuildModel validates the dataplane domain (+ stand-in extensions) against host and returns
// the model to render. Every error wraps ErrInput and names the JSON path.
func BuildModel(dp *vrxv1.DataplaneConfig, ext *Extensions, host Host) (*Model, error) {
	if dp == nil {
		dp = &vrxv1.DataplaneConfig{}
	}
	if ext == nil {
		ext = &Extensions{}
	}
	m := &Model{HugepagesGB: dp.GetHugepagesGb(), BuffersPerNuma: ext.BuffersPerNuma, DPDK: true}

	// ---- plugins (D-060): names from the on-disk list only
	if len(ext.Plugins) > 0 && host.Plugins == nil {
		return nil, inputErr("dataplane.plugins", "the on-disk plugin list is unknown; cannot validate plugin names")
	}
	onDisk := map[string]bool{}
	for _, p := range host.Plugins {
		onDisk[p] = true
	}
	for name, enable := range ext.Plugins {
		path := "dataplane.plugins." + jsonKey(name)
		if err := PluginName(name); err != nil {
			return nil, inputErr(path, "%v", err)
		}
		if !onDisk[name] {
			return nil, inputErr(path, "plugin %s is not installed (not in the on-disk plugin directory)", name)
		}
		m.Plugins = append(m.Plugins, Plugin{File: name, Enable: enable})
		if name == DPDKPlugin && !enable {
			m.DPDK = false
		}
	}
	slices.SortFunc(m.Plugins, func(a, b Plugin) int { return cmp.Compare(a.File, b.File) })

	// ---- CPU placement
	if err := buildCPU(dp, host, m); err != nil {
		return nil, err
	}

	// ---- DPDK devices, management NIC
	if err := buildDevices(dp, ext, m); err != nil {
		return nil, err
	}
	if !m.DPDK && len(m.Devices) > 0 {
		return nil, inputErr("dataplane.plugins."+DPDKPlugin, "dpdk_plugin.so cannot be disabled while DPDK devices are listed (%d)", len(m.Devices))
	}

	// ---- RSS: queues vs workers
	workers := effectiveWorkers(dp)
	maxQ := max(workers, 1)
	if q := dp.GetRxQueues(); q > 0 {
		if q > maxQ {
			return nil, inputErr("dataplane.rxQueues", "%d RX queues exceed the %d worker thread(s) that can poll them", q, maxQ)
		}
		m.RxQueues = q
	}
	if q := dp.GetTxQueues(); q > 0 {
		m.TxQueues = q
	}
	for _, d := range m.Devices {
		if d.RxQueues > maxQ {
			return nil, inputErr("dataplane.devices."+d.PCI+".rxQueues", "%d RX queues exceed the %d worker thread(s) that can poll them", d.RxQueues, maxQ)
		}
	}

	// ---- hugepage budget
	if err := checkHugepages(dp, ext, host, m); err != nil {
		return nil, err
	}
	return m, nil
}

// effectiveWorkers is the number of worker threads VPP will start.
func effectiveWorkers(dp *vrxv1.DataplaneConfig) uint32 {
	if n := len(dp.GetCorelist()); n > 0 {
		return uint32(n) //nolint:gosec // bounded by the schema (≤ 256)
	}
	return dp.GetWorkers()
}

func buildCPU(dp *vrxv1.DataplaneConfig, host Host, m *Model) error {
	corelist := slices.Clone(dp.GetCorelist())
	if dp.Workers != nil && len(corelist) > 0 && int(dp.GetWorkers()) != len(corelist) {
		return inputErr("dataplane.workers", "workers=%d but corelist names %d cores", dp.GetWorkers(), len(corelist))
	}
	seen := map[uint32]bool{}
	for i, c := range corelist {
		if seen[c] {
			return inputErr(fmt.Sprintf("dataplane.corelist[%d]", i), "core %d listed twice", c)
		}
		seen[c] = true
	}
	if dp.MainCore != nil {
		mc := dp.GetMainCore()
		m.MainCore = &mc
		if seen[mc] {
			return inputErr("dataplane.mainCore", "main core %d is also a worker core", mc)
		}
	}
	if len(corelist) > 0 && dp.MainCore == nil {
		// VPP: "main-core must be specified when using corelist-* or coremask-* attribute"
		return inputErr("dataplane.mainCore", "must be set when corelist is given (VPP requires main-core with corelist-workers)")
	}

	// the cores VPP will actually use: main core (default 1) + workers (corelist, or the next N
	// consecutive cores after the main core when only a count is given)
	mainCore := uint32(1)
	if m.MainCore != nil {
		mainCore = *m.MainCore
	}
	workerCores := slices.Clone(corelist)
	if len(corelist) == 0 && dp.GetWorkers() > 0 {
		for c := mainCore + 1; uint32(len(workerCores)) < dp.GetWorkers(); c++ { //nolint:gosec // ≤ 255
			workerCores = append(workerCores, c)
		}
	}
	if host.CPUs > 0 {
		if dp.MainCore != nil && int(mainCore) >= host.CPUs {
			return inputErr("dataplane.mainCore", "core %d does not exist (host has %d CPUs: 0-%d)", mainCore, host.CPUs, host.CPUs-1)
		}
		if need := len(workerCores) + 1; need > host.CPUs {
			return inputErr("dataplane.workers", "%d worker(s) + main thread need %d CPUs, host has %d", len(workerCores), need, host.CPUs)
		}
		for i, c := range workerCores {
			if int(c) >= host.CPUs {
				path := fmt.Sprintf("dataplane.corelist[%d]", i)
				if len(corelist) == 0 {
					path = "dataplane.workers"
				}
				return inputErr(path, "worker core %d does not exist (host has %d CPUs: 0-%d)", c, host.CPUs, host.CPUs-1)
			}
		}
	}
	if len(host.IsolCPUs) > 0 && len(workerCores) > 0 {
		isol := map[uint32]bool{}
		for _, c := range host.IsolCPUs {
			isol[c] = true
		}
		if isol[mainCore] {
			return inputErr("dataplane.mainCore", "main core %d is an isolated CPU (isolcpus=%s); keep the main thread on a housekeeping CPU", mainCore, FormatCPUList(host.IsolCPUs))
		}
		for i, c := range workerCores {
			if !isol[c] {
				path := fmt.Sprintf("dataplane.corelist[%d]", i)
				if len(corelist) == 0 {
					path = "dataplane.workers"
				}
				return inputErr(path, "worker core %d is not in the isolated CPU set (isolcpus=%s)", c, FormatCPUList(host.IsolCPUs))
			}
		}
	}
	if len(corelist) > 0 {
		m.Corelist = FormatCPUList(corelist)
	} else {
		m.Workers = dp.GetWorkers()
	}
	return nil
}

func buildDevices(dp *vrxv1.DataplaneConfig, ext *Extensions, m *Model) error {
	mgmt := map[string]bool{}
	for i, raw := range ext.ManagementPCI {
		path := fmt.Sprintf("dataplane.managementPci[%d]", i)
		pci, err := PCIAddress(raw)
		if err != nil {
			return inputErr(path, "%v", err)
		}
		if mgmt[pci] {
			return inputErr(path, "PCI address %s listed twice", pci)
		}
		mgmt[pci] = true
		m.Blacklist = append(m.Blacklist, pci)
	}
	slices.Sort(m.Blacklist)

	devs := map[string]*Device{}
	for i, raw := range dp.GetPciWhitelist() {
		path := fmt.Sprintf("dataplane.pciWhitelist[%d]", i)
		pci, err := PCIAddress(raw)
		if err != nil {
			return inputErr(path, "%v", err)
		}
		if devs[pci] != nil {
			return inputErr(path, "duplicate PCI address %s", pci)
		}
		devs[pci] = &Device{PCI: pci}
	}
	// devices are keyed by PCI; two keys may spell the same address differently (case)
	keys := make([]string, 0, len(ext.Devices))
	for k := range ext.Devices {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	fromDevices := map[string]string{}
	for _, raw := range keys {
		spec := ext.Devices[raw]
		path := "dataplane.devices." + jsonKey(raw)
		pci, err := PCIAddress(raw)
		if err != nil {
			return inputErr(path, "%v", err)
		}
		if prev, dup := fromDevices[pci]; dup {
			return inputErr(path, "duplicate PCI address %s (also %s)", pci, jsonKey(prev))
		}
		fromDevices[pci] = raw
		d := devs[pci]
		if d == nil {
			d = &Device{PCI: pci}
			devs[pci] = d
		}
		if spec.Name != "" {
			if err := LogicalName(spec.Name); err != nil {
				return inputErr(path+".name", "%v", err)
			}
		}
		for _, q := range []struct {
			v    uint32
			name string
			max  uint32
		}{{spec.RxQueues, "rxQueues", 256}, {spec.TxQueues, "txQueues", 256}} {
			if q.v > q.max {
				return inputErr(path+"."+q.name, "%d not in 1..%d", q.v, q.max)
			}
		}
		for _, q := range []struct {
			v    uint32
			name string
		}{{spec.RxDesc, "rxDesc"}, {spec.TxDesc, "txDesc"}} {
			if q.v != 0 && (q.v < 64 || q.v > 16384 || q.v&(q.v-1) != 0) {
				return inputErr(path+"."+q.name, "%d must be a power of two in 64..16384", q.v)
			}
		}
		d.Name, d.RxQueues, d.TxQueues, d.RxDesc, d.TxDesc = spec.Name, spec.RxQueues, spec.TxQueues, spec.RxDesc, spec.TxDesc
	}

	names := map[string]string{}
	for pci, d := range devs {
		if mgmt[pci] {
			return inputErr("dataplane", "management NIC %s must never be a DPDK device (it is blacklisted; remove it from pciWhitelist/devices)", pci)
		}
		if d.Name != "" {
			if prev, dup := names[d.Name]; dup {
				a, b := min(prev, pci), max(prev, pci)
				return inputErr("dataplane.devices", "logical name %q used by both %s and %s", d.Name, a, b)
			}
			names[d.Name] = pci
		}
		m.Devices = append(m.Devices, *d)
	}
	slices.SortFunc(m.Devices, func(a, b Device) int { return cmp.Compare(a.PCI, b.PCI) })
	if len(m.Devices) > 0 && len(m.Blacklist) == 0 {
		return inputErr("dataplane.managementPci", "must name the management NIC(s) when DPDK devices are listed, so they are blacklisted explicitly")
	}
	for _, d := range m.Devices {
		if d.Name == "" {
			m.Warnings = append(m.Warnings, fmt.Sprintf("dataplane: DPDK device %s has no logical name; VPP will name it after its PCI slot (D-069 expects a logical name)", d.PCI))
		}
	}
	return nil
}

func checkHugepages(dp *vrxv1.DataplaneConfig, ext *Extensions, host Host, m *Model) error {
	perNuma := uint64(ext.BuffersPerNuma)
	if perNuma == 0 {
		perNuma = DefaultBuffersPerNuma
	}
	nodes := uint64(max(host.NUMANodes, 1)) //nolint:gosec // positive
	budget := perNuma * nodes * BufferFootprint

	configured := host.HugepageBytes
	source := "host reservation"
	if dp.HugepagesGb != nil {
		configured = uint64(dp.GetHugepagesGb()) * gib
		source = "dataplane.hugepagesGb"
		if host.HugepageBytes > 0 && configured > host.HugepageBytes {
			m.Warnings = append(m.Warnings, fmt.Sprintf("dataplane.hugepagesGb: %d GiB configured but the host reserves only %s; raise vm.nr_hugepages before applying", dp.GetHugepagesGb(), humanBytes(host.HugepageBytes)))
		}
	}
	if configured == 0 {
		return nil // unknown: nothing to check against
	}
	if budget > configured {
		path := "dataplane.hugepagesGb"
		if ext.BuffersPerNuma > 0 {
			path = "dataplane.buffersPerNuma"
		}
		return inputErr(path, "buffer memory %s (%d buffers × %d NUMA node(s) × %d B) exceeds the %s from %s", humanBytes(budget), perNuma, nodes, BufferFootprint, humanBytes(configured), source)
	}
	return nil
}

func humanBytes(b uint64) string {
	switch {
	case b >= gib && b%gib == 0:
		return fmt.Sprintf("%d GiB", b/gib)
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
