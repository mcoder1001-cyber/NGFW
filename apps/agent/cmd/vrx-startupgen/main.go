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
//	--plugin-dir, --cpus, --isolcpus, --numa-nodes, --hugepages-mb, --no-host
//	                     host facts the document is validated against (default: read from this host)
//
// Exit status: 0 = ok / no difference, 1 = --diff found differences, 2 = invalid input or error.
// It never restarts VPP and never touches the running data plane; applying the file is the
// manager procedure in docs/agent/renderers/vppstartup.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	out, diff                    string
	semantic, check, noHost      bool
	pluginDir, isolcpus          string
	cpus, numaNodes, hugepagesMB int
	input                        string
	sysRoot                      string // "" = "/", tests point it at a fake tree
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var o options
	fs := flag.NewFlagSet("vrx-startupgen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.out, "o", "", "write the rendering to `path` (atomic) instead of stdout")
	fs.StringVar(&o.diff, "diff", "", "compare with the `existing` file; exit 1 when different")
	fs.BoolVar(&o.semantic, "semantic", false, "with --diff: compare sections/entries, ignore comments/order/indentation")
	fs.BoolVar(&o.check, "check", false, "validate only")
	fs.BoolVar(&o.noHost, "no-host", false, "do not read host facts; only the flags below are used")
	fs.StringVar(&o.pluginDir, "plugin-dir", "/usr/lib/x86_64-linux-gnu/vpp_plugins", "VPP plugin `dir` (names are validated against its *.so files)")
	fs.IntVar(&o.cpus, "cpus", -1, "number of host CPUs (default: /sys/devices/system/cpu/online)")
	fs.StringVar(&o.isolcpus, "isolcpus", "", "isolated CPU `list` (default: /sys/devices/system/cpu/isolated)")
	fs.IntVar(&o.numaNodes, "numa-nodes", -1, "number of NUMA nodes (default: /sys/devices/system/node)")
	fs.IntVar(&o.hugepagesMB, "hugepages-mb", -1, "hugepage memory reserved by the host in MiB (default: /proc/meminfo)")
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
		_, err := stdout.Write(out)
		if err != nil {
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

// hostFacts reads the facts of this host (unless --no-host); explicit flags always win.
func hostFacts(o options, visited map[string]bool) (vppstartup.Host, error) {
	var h vppstartup.Host
	root := o.sysRoot
	if root == "" {
		root = "/"
	}
	sys := func(p string) string { return filepath.Join(root, p) }

	if !o.noHost {
		if b, err := os.ReadFile(sys("sys/devices/system/cpu/online")); err == nil { //nolint:gosec // fixed sysfs path
			if cpus, err := vppstartup.ParseCPUList(string(b)); err == nil && len(cpus) > 0 {
				h.CPUs = int(cpus[len(cpus)-1]) + 1
			}
		}
		if b, err := os.ReadFile(sys("sys/devices/system/cpu/isolated")); err == nil { //nolint:gosec // fixed sysfs path
			if isol, err := vppstartup.ParseCPUList(string(b)); err == nil {
				h.IsolCPUs = isol
			}
		}
		if nodes, _ := filepath.Glob(sys("sys/devices/system/node/node[0-9]*")); len(nodes) > 0 {
			h.NUMANodes = len(nodes)
		}
		if b, err := os.ReadFile(sys("proc/meminfo")); err == nil { //nolint:gosec // fixed procfs path
			h.HugepageBytes = hugepagesFromMeminfo(string(b))
		}
	}
	if !o.noHost || visited["plugin-dir"] {
		plugins, err := filepath.Glob(filepath.Join(o.pluginDir, "*.so"))
		if err != nil {
			return h, err
		}
		if len(plugins) == 0 {
			return h, fmt.Errorf("no plugins found in %s (use --plugin-dir)", o.pluginDir)
		}
		h.Plugins = make([]string, 0, len(plugins))
		for _, p := range plugins {
			h.Plugins = append(h.Plugins, filepath.Base(p))
		}
	}
	if visited["cpus"] {
		if o.cpus < 0 {
			return h, fmt.Errorf("--cpus must be ≥ 0")
		}
		h.CPUs = o.cpus
	}
	if visited["isolcpus"] {
		isol, err := vppstartup.ParseCPUList(o.isolcpus)
		if err != nil {
			return h, fmt.Errorf("--isolcpus: %w", err)
		}
		h.IsolCPUs = isol
	}
	if visited["numa-nodes"] {
		if o.numaNodes < 0 {
			return h, fmt.Errorf("--numa-nodes must be ≥ 0")
		}
		h.NUMANodes = o.numaNodes
	}
	if visited["hugepages-mb"] {
		if o.hugepagesMB < 0 {
			return h, fmt.Errorf("--hugepages-mb must be ≥ 0")
		}
		h.HugepageBytes = uint64(o.hugepagesMB) << 20
	}
	return h, nil
}

// hugepagesFromMeminfo returns HugePages_Total × Hugepagesize (bytes), 0 if unknown.
func hugepagesFromMeminfo(s string) uint64 {
	var total, sizeKB uint64
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "HugePages_Total:":
			_, _ = fmt.Sscanf(f[1], "%d", &total)
		case "Hugepagesize:":
			_, _ = fmt.Sscanf(f[1], "%d", &sizeKB)
		}
	}
	return total * sizeKB << 10
}
