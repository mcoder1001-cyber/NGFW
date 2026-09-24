// Package vppstartup renders VPP's start-up configuration (/etc/vpp/startup.conf) from the
// `dataplane` domain of the configuration document (WBS D0.6, task F-startup-gen).
//
// The generator is a pure function: BuildModel validates the domain against the facts of the host
// the file is for (Host — required, never defaulted) and returns a Model; RenderModel turns the
// Model into the file. Nothing here writes the live file or restarts VPP — applying a rendering
// is a manual manager step (docs/agent/renderers/vppstartup.md), so Renderer.Apply always refuses.
package vppstartup

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// ErrInput is wrapped by every error about the desired state (as opposed to host facts or I/O),
// so callers can report it as a validation issue. Messages name the JSON path of the field and
// are always one line.
var ErrInput = errors.New("vppstartup: invalid dataplane configuration")

// ErrHost is wrapped when the host facts are missing or inconsistent: the generator never renders
// without knowing the host (management NIC, CPUs, hugepages, plugins, current plugin switches).
var ErrHost = errors.New("vppstartup: host facts")

// Schema bounds (packages/schema/src/domains/dataplane.ts), re-checked here because typed input
// and hand-made documents never pass the Zod schema (D-049).
const (
	MaxWorkers        = 255
	MaxCorelist       = 256
	MaxCPUID          = 1023
	MaxQueues         = 256
	MaxHugepagesGB    = 1024
	MinBuffersPerNuma = 1024
	MaxBuffersPerNuma = 4194304
	MaxPCIWhitelist   = 64
	MaxManagementPCI  = 4
	MaxDevices        = 64
	MaxPlugins        = 128
	MinDesc           = 64
	MaxDesc           = 16384
)

// DefaultBuffersPerNuma is VPP's default `buffers { buffers-per-numa }` (used for the hugepage
// budget when the document does not set it).
const DefaultBuffersPerNuma = 16384

// BufferFootprint is the conservative per-buffer hugepage cost used for the budget: 2048 bytes
// of default data size + vlib_buffer_t metadata + headroom + mempool overhead, rounded up.
const BufferFootprint = 2560

// D060Plugins are the plugins D-060 enabled on vrx-a (P12/FRR, NPTv6); an authoritative
// `dataplane.plugins` that does not list them gets a warning.
var D060Plugins = []string{"linux_cp_plugin.so", "linux_nl_plugin.so", "npt66_plugin.so"}

// DPDKPlugin is the plugin file that parses the `dpdk { }` section.
const DPDKPlugin = "dpdk_plugin.so"

const gib = uint64(1) << 30

// Host holds the facts of the machine the file is rendered for. All of them are required
// (Check); ReadHost collects them from a running system.
type Host struct {
	// ManagementPCI are the PCI addresses of the NIC(s) the host is managed through (default route
	// / the address the agent is reached on). They are always blacklisted and can never be a DPDK
	// device, whatever the document says.
	ManagementPCI []string
	// ManagementNotes explain ManagementPCI (which interface, why) and list skipped VPP-owned taps;
	// informational, printed by the CLI.
	ManagementNotes []string
	// OnlineCPUs is the set of online logical CPUs (/sys/devices/system/cpu/online).
	OnlineCPUs []uint32
	// IsolCPUs is the kernel's isolated CPU set (/sys/devices/system/cpu/isolated); empty = none.
	IsolCPUs []uint32
	// NUMANodes is the number of NUMA nodes (buffers are allocated per node).
	NUMANodes int
	// HugepageBytes is the hugepage memory the host reserves (HugePages_Total × Hugepagesize).
	HugepageBytes uint64
	// Plugins are the plugin file names on disk (/usr/lib/x86_64-linux-gnu/vpp_plugins/*.so).
	Plugins []string
	// CurrentPlugins are the `plugins { plugin X { enable|disable } }` switches of the current
	// start-up file; non-nil (empty map = none). Switches the document does not mention are kept.
	CurrentPlugins map[string]bool
}

// Check reports missing or malformed host facts (wrapping ErrHost).
func (h Host) Check() error {
	switch {
	case len(h.ManagementPCI) == 0:
		return fmt.Errorf("%w: the management NIC is unknown (detect it from the default route or pass --mgmt-pci)", ErrHost)
	case len(h.OnlineCPUs) == 0:
		return fmt.Errorf("%w: the online CPU set is unknown", ErrHost)
	case h.NUMANodes < 1:
		return fmt.Errorf("%w: the NUMA node count is unknown", ErrHost)
	case h.HugepageBytes == 0:
		return fmt.Errorf("%w: the host reserves no hugepages (HugePages_Total = 0)", ErrHost)
	case len(h.Plugins) == 0:
		return fmt.Errorf("%w: the on-disk plugin list is unknown", ErrHost)
	case h.CurrentPlugins == nil:
		return fmt.Errorf("%w: the plugin switches of the current start-up file are unknown", ErrHost)
	}
	for _, p := range h.ManagementPCI {
		if _, err := PCIAddress(p); err != nil {
			return fmt.Errorf("%w: management NIC: %v", ErrHost, err)
		}
	}
	return nil
}

// Model is the validated, sorted view of the dataplane domain that the template renders.
type Model struct {
	// HugepagesGB is dataplane.hugepagesGb (0 = unset); rendered as a comment only — the
	// hugepages themselves are reserved by the host (vm.nr_hugepages), not by this file.
	HugepagesGB uint32
	// MainCore is always rendered (explicit pinning, never VPP's start-CPU default).
	MainCore uint32
	// Corelist is the corelist-workers value in VPP range syntax ("2-3,6"); "" = no workers.
	// Always explicit: a document `workers: N` is turned into concrete CPUs here.
	Corelist       string
	BuffersPerNuma uint32
	// DPDK is false when dpdk_plugin.so is disabled: the whole dpdk section is omitted.
	DPDK bool
	// RxQueues / TxQueues are the `dev default` queue counts (0 = not set).
	RxQueues uint32
	TxQueues uint32
	// Devices are the DPDK devices sorted by PCI address.
	Devices []Device
	// Blacklist are the host's management PCI addresses, sorted.
	Blacklist []string
	// Plugins are the effective plugin switches (current file overlaid by the document), sorted.
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

// Desired normalises the renderer input to the dataplane message: a *vrxv1.DesiredState or
// *vrxv1.DataplaneConfig is used as is; a *structpb.Struct holding the JSON configuration
// document is decoded strictly — only its `dataplane` member is read (other domains are ignored),
// and unknown keys inside it are an error (the schema is a strictObject); nil is the empty domain.
func Desired(msg proto.Message) (*vrxv1.DataplaneConfig, error) {
	switch m := msg.(type) {
	case nil:
		return &vrxv1.DataplaneConfig{}, nil
	case *vrxv1.DesiredState:
		if m.GetDataplane() == nil {
			return &vrxv1.DataplaneConfig{}, nil
		}
		return m.GetDataplane(), nil
	case *vrxv1.DataplaneConfig:
		if m == nil {
			return &vrxv1.DataplaneConfig{}, nil
		}
		return m, nil
	case *structpb.Struct:
		dpv, ok := m.GetFields()["dataplane"]
		if !ok || dpv == nil {
			return &vrxv1.DataplaneConfig{}, nil
		}
		dp := dpv.GetStructValue()
		if dp == nil {
			return nil, fmt.Errorf("%w: dataplane must be an object", ErrInput)
		}
		raw, err := protojson.Marshal(dp)
		if err != nil {
			return nil, fmt.Errorf("%w: encode dataplane: %s", ErrInput, oneLine(err.Error()))
		}
		cfg := &vrxv1.DataplaneConfig{}
		if err := protojson.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("%w: dataplane: %s", ErrInput, oneLine(err.Error()))
		}
		return cfg, nil
	default:
		return nil, fmt.Errorf("%w: unsupported input type %T (want *vrxv1.DesiredState, *vrxv1.DataplaneConfig or *structpb.Struct)", ErrInput, msg)
	}
}

// oneLine keeps a library error message on one printable line.
func oneLine(s string) string {
	if strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || r == ' ' || r == ' ' }) < 0 {
		return s
	}
	return strconv.QuoteToASCII(s)
}

// jsonKey quotes a record key for an error path when it is not a plain token, so hostile keys
// (newlines, braces) cannot forge the error text.
func jsonKey(k string) string {
	for i := 0; i < len(k); i++ {
		c := k[i]
		plain := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("_.:-", c) >= 0
		if !plain {
			return strconv.QuoteToASCII(k)
		}
	}
	if k == "" {
		return `""`
	}
	return k
}

func inputErr(path string, format string, args ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrInput, path, oneLine(fmt.Sprintf(format, args...)))
}

// BuildModel validates the dataplane domain against host and returns the model to render. Every
// error wraps ErrInput (document) or ErrHost (host facts).
func BuildModel(dp *vrxv1.DataplaneConfig, host Host) (*Model, error) {
	if err := host.Check(); err != nil {
		return nil, err
	}
	if dp == nil {
		dp = &vrxv1.DataplaneConfig{}
	}
	if err := checkBounds(dp); err != nil {
		return nil, err
	}
	m := &Model{HugepagesGB: dp.GetHugepagesGb(), BuffersPerNuma: dp.GetBuffersPerNuma(), DPDK: true}
	if err := buildPlugins(dp, host, m); err != nil {
		return nil, err
	}
	if err := buildCPU(dp, host, m); err != nil {
		return nil, err
	}
	if err := buildDevices(dp, host, m); err != nil {
		return nil, err
	}
	if !m.DPDK && len(m.Devices) > 0 {
		return nil, inputErr("dataplane.plugins.switches."+DPDKPlugin, "dpdk_plugin.so cannot be disabled while DPDK devices are listed (%d)", len(m.Devices))
	}

	// RSS: queues vs the worker threads that poll them
	maxQ := max(uint32(len(corelistOf(m))), 1) //nolint:gosec // ≤ 256
	if q := dp.GetRxQueues(); q > maxQ {
		return nil, inputErr("dataplane.rxQueues", "%d RX queues exceed the %d worker thread(s) that can poll them", q, maxQ)
	}
	m.RxQueues, m.TxQueues = dp.GetRxQueues(), dp.GetTxQueues()
	for _, d := range m.Devices {
		if d.RxQueues > maxQ {
			return nil, inputErr("dataplane.devices."+d.PCI+".rxQueues", "%d RX queues exceed the %d worker thread(s) that can poll them", d.RxQueues, maxQ)
		}
	}

	if err := checkHugepages(dp, host, m); err != nil {
		return nil, err
	}
	return m, nil
}

func corelistOf(m *Model) []uint32 {
	l, _ := ParseCPUList(m.Corelist)
	return l
}

// checkBounds re-applies the schema's numeric and size bounds (and so rejects VPP's ~0 sentinel).
func checkBounds(dp *vrxv1.DataplaneConfig) error {
	rng := func(path string, v, lo, hi uint32) error {
		if v < lo || v > hi {
			return inputErr(path, "%d not in %d..%d", v, lo, hi)
		}
		return nil
	}
	type chk struct {
		set       bool
		path      string
		v, lo, hi uint32
	}
	for _, c := range []chk{
		{dp.Workers != nil, "dataplane.workers", dp.GetWorkers(), 0, MaxWorkers},
		{dp.MainCore != nil, "dataplane.mainCore", dp.GetMainCore(), 0, MaxCPUID},
		{dp.RxQueues != nil, "dataplane.rxQueues", dp.GetRxQueues(), 1, MaxQueues},
		{dp.TxQueues != nil, "dataplane.txQueues", dp.GetTxQueues(), 1, MaxQueues},
		{dp.HugepagesGb != nil, "dataplane.hugepagesGb", dp.GetHugepagesGb(), 1, MaxHugepagesGB},
		{dp.BuffersPerNuma != nil, "dataplane.buffersPerNuma", dp.GetBuffersPerNuma(), MinBuffersPerNuma, MaxBuffersPerNuma},
	} {
		if c.set {
			if err := rng(c.path, c.v, c.lo, c.hi); err != nil {
				return err
			}
		}
	}
	for i, c := range dp.GetCorelist() {
		if err := rng(fmt.Sprintf("dataplane.corelist[%d]", i), c, 0, MaxCPUID); err != nil {
			return err
		}
	}
	for _, c := range []struct {
		path   string
		n, max int
	}{
		{"dataplane.corelist", len(dp.GetCorelist()), MaxCorelist},
		{"dataplane.pciWhitelist", len(dp.GetPciWhitelist()), MaxPCIWhitelist},
		{"dataplane.managementPci", len(dp.GetManagementPci()), MaxManagementPCI},
		{"dataplane.devices", len(dp.GetDevices()), MaxDevices},
		{"dataplane.plugins.switches", len(dp.GetPlugins().GetSwitches()), MaxPlugins},
	} {
		if c.n > c.max {
			return inputErr(c.path, "%d entries, at most %d", c.n, c.max)
		}
	}
	return nil
}

// buildPlugins (D-084): `dataplane.plugins` present → exactly its switches (warnings for missing
// D-060 plugins and for switches of the current file that disappear); absent → the current file's
// switches are kept. Every rendered name must be a plugin file on disk.
func buildPlugins(dp *vrxv1.DataplaneConfig, host Host, m *Model) error {
	onDisk := map[string]bool{}
	for _, p := range host.Plugins {
		onDisk[p] = true
	}
	check := func(path, name string) error {
		if err := PluginName(name); err != nil {
			return inputErr(path, "%v", err)
		}
		if !onDisk[name] {
			return inputErr(path, "plugin %s is not installed (not in the on-disk plugin directory)", name)
		}
		return nil
	}
	eff := map[string]bool{}
	docSwitches := dp.GetPlugins().GetSwitches()
	if dp.Plugins == nil {
		// absent (D-084): keep the current file's switches
		for name, enable := range host.CurrentPlugins {
			if err := check("current start-up file: plugins."+jsonKey(name), name); err != nil {
				return err
			}
			eff[name] = enable
			m.Warnings = append(m.Warnings, fmt.Sprintf("plugins: %s { %s } kept from the current start-up file (dataplane.plugins absent)", name, enableWord(enable)))
		}
	} else {
		// present (D-084): the document is authoritative
		for _, want := range D060Plugins {
			if _, listed := docSwitches[want]; !listed {
				m.Warnings = append(m.Warnings, fmt.Sprintf("dataplane.plugins.switches: %s (D-060) is not listed and will not be enabled", want))
			}
		}
		for name, enable := range host.CurrentPlugins {
			if _, listed := docSwitches[name]; !listed {
				m.Warnings = append(m.Warnings, fmt.Sprintf("plugins: %s { %s } in the current start-up file is removed (not in dataplane.plugins.switches)", name, enableWord(enable)))
			}
		}
	}
	for name, enable := range docSwitches {
		if err := check("dataplane.plugins.switches."+jsonKey(name), name); err != nil {
			return err
		}
		eff[name] = enable
	}
	for name, enable := range eff {
		m.Plugins = append(m.Plugins, Plugin{File: name, Enable: enable})
		if name == DPDKPlugin && !enable {
			m.DPDK = false
		}
	}
	slices.SortFunc(m.Plugins, func(a, b Plugin) int { return cmp.Compare(a.File, b.File) })
	slices.Sort(m.Warnings)
	return nil
}

func enableWord(b bool) string {
	if b {
		return "enable"
	}
	return "disable"
}

// buildCPU turns the document into explicit `main-core` + `corelist-workers`, so VPP's pinning is
// exactly the validated one (VPP would otherwise use the CPU it happened to start on as main core
// and pick worker CPUs itself — vlib/threads.c).
func buildCPU(dp *vrxv1.DataplaneConfig, host Host, m *Model) error {
	online := map[uint32]bool{}
	for _, c := range host.OnlineCPUs {
		online[c] = true
	}
	isol := map[uint32]bool{}
	for _, c := range host.IsolCPUs {
		isol[c] = true
	}
	onlineList := slices.Sorted(maps.Keys(online))
	hostDesc := fmt.Sprintf("online CPUs %s", FormatCPUList(onlineList))

	corelist := slices.Clone(dp.GetCorelist())
	if dp.Workers != nil && len(corelist) > 0 && int(dp.GetWorkers()) != len(corelist) {
		return inputErr("dataplane.workers", "workers=%d but corelist names %d cores", dp.GetWorkers(), len(corelist))
	}
	seen := map[uint32]bool{}
	for i, c := range corelist {
		path := fmt.Sprintf("dataplane.corelist[%d]", i)
		if seen[c] {
			return inputErr(path, "core %d listed twice", c)
		}
		seen[c] = true
		if !online[c] {
			return inputErr(path, "worker core %d is not an online CPU (%s)", c, hostDesc)
		}
	}
	nWorkers := uint32(len(corelist)) //nolint:gosec // ≤ 256
	if len(corelist) == 0 {
		nWorkers = dp.GetWorkers()
	}

	// main core: explicit, or the lowest online non-isolated CPU other than 0 that is no worker
	var main uint32
	if dp.MainCore != nil {
		main = dp.GetMainCore()
		if !online[main] {
			return inputErr("dataplane.mainCore", "core %d is not an online CPU (%s)", main, hostDesc)
		}
		if seen[main] {
			return inputErr("dataplane.mainCore", "main core %d is also a worker core", main)
		}
	} else {
		found := false
		for _, want0 := range []bool{false, true} { // CPU 0 only as a last resort
			for _, c := range onlineList {
				if (c == 0) != want0 || seen[c] || isol[c] {
					continue
				}
				main, found = c, true
				break
			}
			if found {
				break
			}
		}
		if !found {
			return inputErr("dataplane.mainCore", "no online, non-isolated CPU left for the main thread (%s); set mainCore", hostDesc)
		}
		m.Warnings = append(m.Warnings, fmt.Sprintf("dataplane.mainCore not set: main-core %d chosen (lowest online, non-isolated CPU)", main))
	}

	workers := corelist
	if len(corelist) == 0 && nWorkers > 0 {
		// same rule as VPP's automatic placement, made explicit: lowest free CPUs, CPU 0 last; with
		// isolated CPUs only those
		var cand []uint32
		for _, want0 := range []bool{false, true} {
			for _, c := range onlineList {
				if (c == 0) != want0 || c == main || (len(isol) > 0 && !isol[c]) {
					continue
				}
				cand = append(cand, c)
			}
		}
		if uint32(len(cand)) < nWorkers { //nolint:gosec // ≤ 4096
			pool := "online CPUs other than the main core"
			if len(isol) > 0 {
				pool = "isolated CPUs " + FormatCPUList(host.IsolCPUs)
			}
			return inputErr("dataplane.workers", "%d worker(s) need %d CPUs, only %d %s available (%s)", nWorkers, nWorkers, len(cand), pool, hostDesc)
		}
		workers = cand[:nWorkers]
		m.Warnings = append(m.Warnings, fmt.Sprintf("dataplane.workers=%d without corelist: corelist-workers %s chosen", nWorkers, FormatCPUList(workers)))
	}

	if len(isol) > 0 && len(workers) > 0 {
		if isol[main] {
			return inputErr("dataplane.mainCore", "main core %d is an isolated CPU (isolcpus=%s); keep the main thread on a housekeeping CPU", main, FormatCPUList(host.IsolCPUs))
		}
		for i, c := range workers {
			if !isol[c] {
				return inputErr(fmt.Sprintf("dataplane.corelist[%d]", i), "worker core %d is not in the isolated CPU set (isolcpus=%s)", c, FormatCPUList(host.IsolCPUs))
			}
		}
	}
	m.MainCore = main
	if len(workers) > 0 {
		m.Corelist = FormatCPUList(workers)
	}
	return nil
}

func buildDevices(dp *vrxv1.DataplaneConfig, host Host, m *Model) error {
	hostMgmt := map[string]bool{}
	for _, p := range host.ManagementPCI {
		pci, _ := PCIAddress(p) // checked by Host.Check
		hostMgmt[pci] = true
	}
	m.Blacklist = slices.Sorted(maps.Keys(hostMgmt))

	docMgmt := map[string]bool{}
	for i, raw := range dp.GetManagementPci() {
		path := fmt.Sprintf("dataplane.managementPci[%d]", i)
		pci, err := PCIAddress(raw)
		if err != nil {
			return inputErr(path, "%v", err)
		}
		if docMgmt[pci] {
			return inputErr(path, "PCI address %s listed twice", pci)
		}
		docMgmt[pci] = true
	}
	if len(docMgmt) > 0 {
		same := len(docMgmt) == len(hostMgmt)
		for p := range docMgmt {
			same = same && hostMgmt[p]
		}
		if !same {
			doc := slices.Sorted(maps.Keys(docMgmt))
			return inputErr("dataplane.managementPci", "%s does not match the host's management NIC(s) %s", strings.Join(doc, ","), strings.Join(m.Blacklist, ","))
		}
	}

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
	keys := slices.Sorted(maps.Keys(dp.GetDevices()))
	fromDevices := map[string]string{}
	for _, raw := range keys {
		spec := dp.GetDevices()[raw]
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
		if spec.Name != nil {
			if err := LogicalName(spec.GetName()); err != nil {
				return inputErr(path+".name", "%v", err)
			}
		}
		for _, q := range []struct {
			set  bool
			v    uint32
			name string
		}{{spec.RxQueues != nil, spec.GetRxQueues(), "rxQueues"}, {spec.TxQueues != nil, spec.GetTxQueues(), "txQueues"}} {
			if q.set && (q.v < 1 || q.v > MaxQueues) {
				return inputErr(path+"."+q.name, "%d not in 1..%d", q.v, MaxQueues)
			}
		}
		for _, q := range []struct {
			set  bool
			v    uint32
			name string
		}{{spec.RxDesc != nil, spec.GetRxDesc(), "rxDesc"}, {spec.TxDesc != nil, spec.GetTxDesc(), "txDesc"}} {
			if q.set && (q.v < MinDesc || q.v > MaxDesc || q.v&(q.v-1) != 0) {
				return inputErr(path+"."+q.name, "%d must be a power of two in %d..%d", q.v, MinDesc, MaxDesc)
			}
		}
		d.Name, d.RxQueues, d.TxQueues, d.RxDesc, d.TxDesc = spec.GetName(), spec.GetRxQueues(), spec.GetTxQueues(), spec.GetRxDesc(), spec.GetTxDesc()
	}

	names := map[string]string{}
	for pci, d := range devs {
		if hostMgmt[pci] {
			return inputErr("dataplane", "%s is the host's management NIC and must never be a DPDK device (remove it from pciWhitelist/devices)", pci)
		}
		if d.Name != "" {
			if prev, dup := names[d.Name]; dup {
				return inputErr("dataplane.devices", "logical name %q used by both %s and %s", d.Name, min(prev, pci), max(prev, pci))
			}
			names[d.Name] = pci
		}
		m.Devices = append(m.Devices, *d)
	}
	slices.SortFunc(m.Devices, func(a, b Device) int { return cmp.Compare(a.PCI, b.PCI) })
	for _, d := range m.Devices {
		if d.Name == "" {
			m.Warnings = append(m.Warnings, fmt.Sprintf("dataplane: DPDK device %s has no logical name; VPP will name it after its PCI slot (D-069 expects a logical name)", d.PCI))
		}
	}
	return nil
}

// checkHugepages: the buffer memory must fit in what the host actually reserves, and in
// hugepagesGb when the document sets it (the smaller of the two).
func checkHugepages(dp *vrxv1.DataplaneConfig, host Host, m *Model) error {
	perNuma := uint64(dp.GetBuffersPerNuma())
	if perNuma == 0 {
		perNuma = DefaultBuffersPerNuma
	}
	nodes := uint64(host.NUMANodes) //nolint:gosec // ≥ 1 (Host.Check)
	budget := perNuma * nodes * BufferFootprint

	limit, source := host.HugepageBytes, "the host's hugepage reservation"
	if dp.HugepagesGb != nil {
		configured := uint64(dp.GetHugepagesGb()) * gib
		if configured > host.HugepageBytes {
			m.Warnings = append(m.Warnings, fmt.Sprintf("dataplane.hugepagesGb: %d GiB configured but the host reserves only %s; raise vm.nr_hugepages before applying", dp.GetHugepagesGb(), humanBytes(host.HugepageBytes)))
		} else {
			limit, source = configured, "dataplane.hugepagesGb"
		}
	}
	if budget > limit {
		path := "dataplane.hugepagesGb"
		if dp.BuffersPerNuma != nil {
			path = "dataplane.buffersPerNuma"
		}
		return inputErr(path, "buffer memory %s (%d buffers × %d NUMA node(s) × %d B) exceeds the %s of %s", humanBytes(budget), perNuma, nodes, BufferFootprint, humanBytes(limit), source)
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
