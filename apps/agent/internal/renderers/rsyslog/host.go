package rsyslog

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"ngfw/agent/internal/renderers/rfkit"
)

// HostStats is what the host's rsyslog configuration says about impstats (review M3).
type HostStats struct {
	// Loaded: some host file loads impstats (module(load="impstats") or $ModLoad impstats).
	Loaded bool
	// File is its log.file when it writes format="json" to a file ("" otherwise).
	File string
	// Source is the file that loads it.
	Source string
}

var (
	impstatsModRe    = regexp.MustCompile(`(?is)module\s*\(\s*load\s*=\s*"impstats"(.*?)\)`)
	impstatsLegacyRe = regexp.MustCompile(`(?im)^\s*\$ModLoad\s+impstats\b`)
	paramRe          = regexp.MustCompile(`(?i)\b(format|log\.file)\s*=\s*"([^"]*)"`)
)

// stripComments drops "#" comment lines (RainerScript and legacy).
func stripComments(b []byte) string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// ScanHost reads the host rsyslog files (never ConfFile itself) for an impstats load.
func ScanHost(p Paths) (HostStats, error) {
	var files []string
	for _, g := range p.HostConfigs {
		m, err := filepath.Glob(g)
		if err != nil {
			return HostStats{}, fmt.Errorf("rsyslog: host config glob %q: %w", g, err)
		}
		files = append(files, m...)
	}
	slices.Sort(files)
	for _, f := range slices.Compact(files) {
		if f == p.ConfFile {
			continue
		}
		b, err := rfkit.ReadFileLimit(f, 1<<20)
		if err != nil {
			return HostStats{}, fmt.Errorf("rsyslog: read host config %s: %w", f, err)
		}
		text := stripComments(b)
		if m := impstatsModRe.FindStringSubmatch(text); m != nil {
			hs := HostStats{Loaded: true, Source: f}
			params := map[string]string{}
			for _, pm := range paramRe.FindAllStringSubmatch(m[1], -1) {
				params[strings.ToLower(pm[1])] = pm[2]
			}
			if strings.EqualFold(params["format"], "json") && filepath.IsAbs(params["log.file"]) {
				hs.File = filepath.Clean(params["log.file"])
			}
			return hs, nil
		}
		if impstatsLegacyRe.MatchString(text) {
			return HostStats{Loaded: true, Source: f}, nil
		}
	}
	return HostStats{}, nil
}
