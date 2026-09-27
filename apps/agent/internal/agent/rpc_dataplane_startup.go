package agent

// F-dataplane-ui: the DataplaneStartupState and DataplaneStartupPreview RPCs — read-only views of the VPP
// start-up configuration (installed file, host facts) and a rendering of a candidate `dataplane` domain with
// the F-startup-gen renderer (internal/renderers/vppstartup). Nothing here writes a file or restarts VPP:
// installing startup.conf is deploy/vpp/apply-startup.sh, a manager step gated by TD-17.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/vppstartup"
)

// startupSources are the paths the two RPCs read. Overridable for tests and lab slots:
// VRX_VPP_STARTUP_CONF (installed file), VRX_VPP_PLUGIN_DIR, VRX_SYS_ROOT (prefix of /sys and /proc).
type startupSources struct {
	conf, pluginDir, sysRoot string
}

func startupSourcesFromEnv() startupSources {
	s := startupSources{conf: vppstartup.DefaultConfPath, pluginDir: vppstartup.DefaultPluginDir, sysRoot: "/"}
	if v := os.Getenv("VRX_VPP_STARTUP_CONF"); v != "" {
		s.conf = v
	}
	if v := os.Getenv("VRX_VPP_PLUGIN_DIR"); v != "" {
		s.pluginDir = v
	}
	if v := os.Getenv("VRX_SYS_ROOT"); v != "" {
		s.sysRoot = v
	}
	return s
}

// maxStartupFile caps what the agent reads of the installed file.
const maxStartupFile = 1 << 20

func readStartup(path string) ([]byte, bool, error) {
	b, err := os.ReadFile(path) //nolint:gosec // fixed product path (or test override), read only
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(b) > maxStartupFile {
		return nil, true, fmt.Errorf("%s is larger than %d bytes", path, maxStartupFile)
	}
	return b, true, nil
}

// DataplaneStartupState implements the DataplaneStartupState RPC.
func (g *server) DataplaneStartupState(_ context.Context, req *vrxv1.DataplaneStartupStateRequest) (*vrxv1.DataplaneStartupStateResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	return startupState(startupSourcesFromEnv()), nil
}

func startupState(src startupSources) *vrxv1.DataplaneStartupStateResponse {
	out := &vrxv1.DataplaneStartupStateResponse{StartupPath: src.conf, Plugins: map[string]bool{}, RetrievedAt: timestamppb.Now()}
	var errs []string
	b, present, err := readStartup(src.conf)
	out.StartupPresent = present
	if err != nil {
		errs = append(errs, err.Error())
	} else if present {
		if err := fillFromStartup(out, b); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src.conf, err))
		}
	}
	at := func(p string) string { return filepath.Join(src.sysRoot, p) }
	if on, err := os.ReadFile(at("sys/devices/system/cpu/online")); err == nil { //nolint:gosec // fixed sysfs path
		out.OnlineCpus = strings.TrimSpace(string(on))
	} else {
		errs = append(errs, "online CPUs: "+err.Error())
	}
	if mi, err := os.ReadFile(at("proc/meminfo")); err == nil { //nolint:gosec // fixed procfs path
		out.HugepagesTotalBytes, out.HugepagesFreeBytes = hugepages(string(mi))
	} else {
		errs = append(errs, "hugepages: "+err.Error())
	}
	out.Error = strings.Join(errs, "; ")
	return out
}

// fillFromStartup copies the cpu and plugins sections of an installed start-up file.
func fillFromStartup(out *vrxv1.DataplaneStartupStateResponse, b []byte) error {
	root, err := vppstartup.Parse(b)
	if err != nil {
		return err
	}
	for _, sec := range root.Sections {
		if sec.Name != "cpu" {
			continue
		}
		for _, e := range sec.Entries {
			f := strings.Fields(e)
			if len(f) != 2 {
				continue
			}
			switch f[0] {
			case "workers":
				if n, err := strconv.ParseUint(f[1], 10, 32); err == nil {
					v := uint32(n)
					out.Workers = &v
				}
			case "main-core":
				if n, err := strconv.ParseUint(f[1], 10, 32); err == nil {
					v := uint32(n)
					out.MainCore = &v
				}
			case "corelist-workers":
				out.CorelistWorkers = f[1]
			}
		}
	}
	sw, err := vppstartup.PluginSwitches(b)
	if err != nil {
		return err
	}
	out.Plugins = sw
	return nil
}

// hugepages returns HugePages_Total and HugePages_Free × Hugepagesize in bytes.
func hugepages(meminfo string) (total, free uint64) {
	var t, f, sizeKB uint64
	for _, line := range strings.Split(meminfo, "\n") {
		fs := strings.Fields(line)
		if len(fs) < 2 {
			continue
		}
		n, _ := strconv.ParseUint(fs[1], 10, 64)
		switch fs[0] {
		case "HugePages_Total:":
			t = n
		case "HugePages_Free:":
			f = n
		case "Hugepagesize:":
			sizeKB = n
		}
	}
	return t * sizeKB << 10, f * sizeKB << 10
}

// DataplaneStartupPreview implements the DataplaneStartupPreview RPC.
func (g *server) DataplaneStartupPreview(_ context.Context, req *vrxv1.DataplaneStartupPreviewRequest) (*vrxv1.DataplaneStartupPreviewResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	return startupPreview(startupSourcesFromEnv(), req.GetDataplane())
}

func startupPreview(src startupSources, dp *vrxv1.DataplaneConfig) (*vrxv1.DataplaneStartupPreviewResponse, error) {
	existing, present, err := readStartup(src.conf)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "installed start-up file: %v", err)
	}
	current := ""
	if present {
		current = src.conf
	}
	host, err := vppstartup.ReadHost(vppstartup.HostSources{Root: src.sysRoot, PluginDir: src.pluginDir, CurrentConf: current})
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "host facts: %v", err)
	}
	if dp == nil {
		dp = &vrxv1.DataplaneConfig{}
	}
	rendered, model, err := vppstartup.Generate(dp, host, vppstartup.DefaultSettings())
	switch {
	case errors.Is(err, vppstartup.ErrInput):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case err != nil:
		return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
	}
	sum := sha256.Sum256(rendered)
	out := &vrxv1.DataplaneStartupPreviewResponse{
		Rendered: string(rendered), StartupPath: src.conf, Warnings: model.Warnings, Sha256: hex.EncodeToString(sum[:]),
	}
	if !present {
		out.Changed = true
		return out, nil
	}
	d, err := vppstartup.UnifiedDiff(src.conf, "rendered", existing, rendered)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "diff: %v", err)
	}
	out.Diff, out.Changed = d, d != ""
	return out, nil
}
