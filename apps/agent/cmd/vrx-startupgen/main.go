// Command vrx-startupgen renders VPP's startup.conf from the `dataplane` domain of a VRX
// configuration document (JSON) — WBS D0.6, task F-startup-gen.
//
//	vrx-startupgen [flags] [document.json|-]
//
//	-o <path>            write the rendering to <path> (atomic rename, mode 0644) instead of stdout
//	--diff <existing>    print a unified diff existing → rendering; exit 1 when they differ
//	--semantic           with --diff: compare sections/entries instead of text (comments, order and
//	                     indentation ignored)
//	--check              validate only; print warnings; exit 0/2
//	--current <file>     current start-up file whose plugin switches are kept ("none" = no file)
//	--mgmt-if, --mgmt-pci  extra management NICs besides the detected ones (default routes and the
//	                     NICs established control connections arrive on, --control-ports, default 22)
//	stderr always shows the management NICs found and the sha256 of the rendering
//	--plugin-dir, --online-cpus, --isolcpus, --numa-nodes, --hugepages-mb
//	                     host fact overrides; --no-host reads nothing from /sys and /proc, so every
//	                     fact must then come from a flag (the management NIC via --mgmt-pci)
//
// Exit status: 0 = ok / no difference, 1 = --diff found differences, 2 = invalid input or error.
// It never restarts VPP and never touches the running data plane; applying the file is the
// manual manager procedure in docs/agent/renderers/vppstartup.md.
package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers/vppstartup"
)

const maxInput = 16 << 20

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type options struct {
	out, diff               string
	semantic, check, noHost bool
	pluginDir, current      string
	mgmtIfs, mgmtPCIs       string
	controlPorts            string
	onlineCPUs, isolcpus    string
	numaNodes, hugepagesMB  int
	input                   string
	sysRoot                 string // "" = "/", tests point it at a fake tree
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var o options
	fs := flag.NewFlagSet("vrx-startupgen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.out, "o", "", "write the rendering to `path` (atomic) instead of stdout")
	fs.StringVar(&o.diff, "diff", "", "compare with the `existing` file; exit 1 when different")
	fs.BoolVar(&o.semantic, "semantic", false, "with --diff: compare sections/entries, ignore comments/order/indentation")
	fs.BoolVar(&o.check, "check", false, "validate only")
	fs.BoolVar(&o.noHost, "no-host", false, "read nothing from /sys and /proc; every host fact comes from the flags")
	fs.StringVar(&o.pluginDir, "plugin-dir", vppstartup.DefaultPluginDir, "VPP plugin `dir` (names are validated against its *.so files)")
	fs.StringVar(&o.current, "current", vppstartup.DefaultConfPath, "current start-up `file` whose plugin switches are kept; \"none\" = there is none")
	fs.StringVar(&o.mgmtIfs, "mgmt-if", "", "extra management `interfaces` (comma separated) besides the default-route interface(s)")
	fs.StringVar(&o.mgmtPCIs, "mgmt-pci", "", "extra management NIC PCI `addresses` (comma separated); required with --no-host")
	fs.StringVar(&o.controlPorts, "control-ports", "22", "local TCP `ports` of control connections (sshd, agent API) whose established peers mark their NIC as management")
	fs.StringVar(&o.onlineCPUs, "online-cpus", "", "online CPU `list` (default: /sys/devices/system/cpu/online)")
	fs.StringVar(&o.isolcpus, "isolcpus", "", "isolated CPU `list` (default: /sys/devices/system/cpu/isolated)")
	fs.IntVar(&o.numaNodes, "numa-nodes", 0, "number of NUMA nodes (default: /sys/devices/system/node)")
	fs.IntVar(&o.hugepagesMB, "hugepages-mb", 0, "hugepage memory reserved by the host in MiB (default: /proc/meminfo)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: vrx-startupgen [flags] [document.json|-]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	switch fs.NArg() {
	case 0:
		o.input = "-"
	case 1:
		o.input = fs.Arg(0)
	default:
		_, _ = fmt.Fprintln(stderr, "vrx-startupgen: at most one document")
		return 2
	}
	if o.semantic && o.diff == "" {
		_, _ = fmt.Fprintln(stderr, "vrx-startupgen: --semantic needs --diff")
		return 2
	}
	if o.check && (o.out != "" || o.diff != "") {
		_, _ = fmt.Fprintln(stderr, "vrx-startupgen: --check cannot be combined with -o or --diff")
		return 2
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })

	code, err := generate(o, visited, stdin, stdout, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vrx-startupgen: %v\n", err)
	}
	return code
}

func generate(o options, visited map[string]bool, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	raw, err := readInput(o.input, stdin)
	if err != nil {
		return 2, err
	}
	doc := &structpb.Struct{}
	if err := protojson.Unmarshal(raw, doc); err != nil {
		return 2, fmt.Errorf("document is not a JSON object: %w", err)
	}
	host, err := hostFacts(o, visited)
	if err != nil {
		return 2, err
	}
	out, model, err := vppstartup.Generate(doc, host, vppstartup.DefaultSettings())
	if err != nil {
		return 2, err
	}
	_, _ = fmt.Fprintf(stderr, "vrx-startupgen: host management NIC(s) %s (always blacklisted)\n", strings.Join(host.ManagementPCI, ","))
	for _, n := range host.ManagementNotes {
		_, _ = fmt.Fprintf(stderr, "vrx-startupgen: management: %s\n", n)
	}
	// N5: the sha256 of exactly this rendering, so a reviewer can pin what gets installed
	_, _ = fmt.Fprintf(stderr, "vrx-startupgen: rendered sha256 %x\n", sha256.Sum256(out))
	for _, w := range model.Warnings {
		_, _ = fmt.Fprintf(stderr, "vrx-startupgen: warning: %s\n", w)
	}
	switch {
	case o.check:
		_, _ = fmt.Fprintf(stderr, "vrx-startupgen: ok (%d DPDK device(s), %d plugin switch(es))\n", len(model.Devices), len(model.Plugins))
		return 0, nil
	case o.diff != "":
		existing, err := os.ReadFile(o.diff) //nolint:gosec // operator-supplied file to compare with, read only
		if err != nil {
			return 2, err
		}
		if o.semantic {
			onlyOld, onlyNew, err := vppstartup.SemanticDiff(existing, out)
			if err != nil {
				return 2, err
			}
			for _, l := range onlyOld {
				_, _ = fmt.Fprintf(stdout, "- %s\n", l)
			}
			for _, l := range onlyNew {
				_, _ = fmt.Fprintf(stdout, "+ %s\n", l)
			}
			if len(onlyOld)+len(onlyNew) > 0 {
				return 1, nil
			}
			return 0, nil
		}
		d, err := vppstartup.UnifiedDiff(o.diff, "rendered", existing, out)
		if err != nil {
			return 2, err
		}
		if _, err := io.WriteString(stdout, d); err != nil {
			return 2, err
		}
		if d != "" {
			return 1, nil
		}
		return 0, nil
	case o.out != "":
		return 0, writeAtomic(o.out, out)
	default:
		if _, err := stdout.Write(out); err != nil {
			return 2, err
		}
		return 0, nil
	}
}

func readInput(name string, stdin io.Reader) ([]byte, error) {
	r := stdin
	if name != "-" {
		f, err := os.Open(name) //nolint:gosec // operator-supplied document, read only
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, maxInput+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxInput {
		return nil, fmt.Errorf("document larger than %d bytes", maxInput)
	}
	return b, nil
}

// writeAtomic writes b next to path and renames it into place (mode 0644).
func writeAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func parsePorts(s string) ([]uint16, error) {
	ports := []uint16{}
	for _, p := range splitList(s) {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("--control-ports: bad port %q", p)
		}
		ports = append(ports, uint16(n))
	}
	return ports, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// hostFacts reads this host's facts (vppstartup.ReadHost) unless --no-host, applies the flag
// overrides and checks that every fact is present (Host.Check).
func hostFacts(o options, visited map[string]bool) (vppstartup.Host, error) {
	current := o.current
	if current == "none" {
		current = ""
	}
	var h vppstartup.Host
	if !o.noHost {
		var err error
		ports, err := parsePorts(o.controlPorts)
		if err != nil {
			return h, err
		}
		h, err = vppstartup.ReadHost(vppstartup.HostSources{
			Root: o.sysRoot, PluginDir: o.pluginDir, CurrentConf: current,
			MgmtIfaces: splitList(o.mgmtIfs), MgmtPCI: splitList(o.mgmtPCIs), ControlPorts: ports,
		})
		if err != nil {
			return h, err
		}
	} else {
		if visited["mgmt-if"] {
			return h, fmt.Errorf("--mgmt-if needs the host's /sys (drop --no-host or use --mgmt-pci)")
		}
		for _, p := range splitList(o.mgmtPCIs) {
			pci, err := vppstartup.PCIAddress(p)
			if err != nil {
				return h, fmt.Errorf("--mgmt-pci: %w", err)
			}
			h.ManagementPCI = append(h.ManagementPCI, pci)
		}
		if visited["plugin-dir"] {
			plugins, _ := filepath.Glob(filepath.Join(o.pluginDir, "*.so"))
			for _, p := range plugins {
				h.Plugins = append(h.Plugins, filepath.Base(p))
			}
			if len(h.Plugins) == 0 {
				return h, fmt.Errorf("no plugins found in %s (use --plugin-dir)", o.pluginDir)
			}
		}
		if visited["current"] {
			h.CurrentPlugins = map[string]bool{}
			if current != "" {
				b, err := os.ReadFile(current) //nolint:gosec // operator-supplied current start-up file, read only
				if err != nil {
					return h, err
				}
				if h.CurrentPlugins, err = vppstartup.PluginSwitches(b); err != nil {
					return h, fmt.Errorf("--current %s: %w", current, err)
				}
			}
		}
	}
	if visited["online-cpus"] {
		cpus, err := vppstartup.ParseCPUList(o.onlineCPUs)
		if err != nil {
			return h, fmt.Errorf("--online-cpus: %w", err)
		}
		h.OnlineCPUs = cpus
	}
	if visited["isolcpus"] {
		isol, err := vppstartup.ParseCPUList(o.isolcpus)
		if err != nil {
			return h, fmt.Errorf("--isolcpus: %w", err)
		}
		h.IsolCPUs = isol
	}
	if visited["numa-nodes"] {
		h.NUMANodes = o.numaNodes
	}
	if visited["hugepages-mb"] {
		if o.hugepagesMB < 0 {
			return h, fmt.Errorf("--hugepages-mb must be ≥ 0")
		}
		h.HugepageBytes = uint64(o.hugepagesMB) << 20
	}
	return h, h.Check()
}
