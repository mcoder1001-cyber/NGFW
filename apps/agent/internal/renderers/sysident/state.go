package sysident

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ObservedState contains only read-only host facts; no configured banner or secret.
// ResolverStatus describes runtime-file observation, never inferred daemon health.
type ObservedState struct {
	Hostname, Timezone, ResolverStatus                                          string
	UptimeSeconds                                                               *float64
	KernelHostname                                                              *string
	ConfiguredNameServers, ConfiguredSearchDomains, ObservedNameServers, Errors []string
}

// readBounded prevents malformed host files from allocating unbounded memory.
func readBounded(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // fixed injected renderer paths, read only
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil {
		return "", err
	}
	if len(b) > 16384 {
		return "", os.ErrInvalid
	}
	return string(b), nil
}

// State reads this descriptor's installed files. Slot agents never return the
// shared machine's hostname or resolver configuration as their own identity.
func (d *Descriptor) State(procRoot, resolvedRuntime string) ObservedState {
	st := ObservedState{ResolverStatus: "slot-only"}
	if d.provisioned != nil {
		if err := d.provisioned(); err != nil {
			return ObservedState{ResolverStatus: "unavailable", Errors: []string{"provisioning"}}
		}
	}
	if v, err := readBounded(d.paths.Hostname); err == nil {
		st.Hostname = strings.TrimSpace(v)
	} else {
		st.Errors = append(st.Errors, "hostname")
	}
	if target, err := filepath.EvalSymlinks(d.paths.Localtime); err == nil {
		rel, err := filepath.Rel(d.paths.ZoneinfoDir, target)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel) {
			st.Timezone = filepath.ToSlash(rel)
		} else {
			st.Errors = append(st.Errors, "timezone")
		}
	} else {
		st.Errors = append(st.Errors, "timezone")
	}
	if v, err := readBounded(d.paths.ResolvedDropIn); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(v))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "DNS=") {
				st.ConfiguredNameServers = strings.Fields(strings.TrimPrefix(line, "DNS="))
			}
			if strings.HasPrefix(line, "Domains=") {
				st.ConfiguredSearchDomains = strings.Fields(strings.TrimPrefix(line, "Domains="))
			}
		}
	} else {
		st.Errors = append(st.Errors, "resolver-config")
	}
	if v, err := readBounded(filepath.Join(procRoot, "uptime")); err == nil {
		fields := strings.Fields(v)
		if len(fields) > 0 {
			if n, err := strconv.ParseFloat(fields[0], 64); err == nil && n >= 0 && n < 1e12 {
				st.UptimeSeconds = &n
			}
		}
	}
	if st.UptimeSeconds == nil {
		st.Errors = append(st.Errors, "uptime")
	}
	if !d.paths.SetKernelHostname {
		return st
	}
	if v, err := readBounded(filepath.Join(procRoot, "sys/kernel/hostname")); err == nil {
		v = strings.TrimSpace(v)
		st.KernelHostname = &v
	} else {
		st.Errors = append(st.Errors, "kernel-hostname")
	}
	st.ResolverStatus = "unavailable"
	if v, err := readBounded(resolvedRuntime); err == nil {
		st.ResolverStatus = "observed"
		for _, line := range strings.Split(v, "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && f[0] == "nameserver" {
				st.ObservedNameServers = append(st.ObservedNameServers, f[1])
			}
		}
	} else {
		st.Errors = append(st.Errors, "resolver-runtime")
	}
	return st
}
