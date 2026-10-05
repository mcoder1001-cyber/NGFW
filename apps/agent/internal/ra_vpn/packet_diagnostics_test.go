package ravpn

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Raw kernel XFRM JSON never leaves memory: only public selectors and counters
// are projected before persistence. Encryption/authentication key fields are
// never logged, returned, or written to an artifact.
func collectPrivatePacketDiagnostics(t *testing.T, plan *NetworkPlan) {
	t.Helper()
	public := map[string]any{}
	for _, request := range []struct {
		name   string
		args   []string
		fields []string
	}{
		{"states", []string{"-j", "-s", "xfrm", "state"}, []string{"src", "dst", "proto", "spi", "mode", "if_id", "lifetime-current", "stats"}},
		{"policies", []string{"-j", "xfrm", "policy"}, []string{"src", "dst", "dir", "priority", "if_id", "action", "index"}},
		{"routes", []string{"-j", "route", "show", "table", "all"}, []string{"dst", "gateway", "dev", "table", "prefsrc", "scope", "type"}},
	} {
		args := append([]string{"--net=" + filepath.Join(InstanceRoot, plan.Instance, "netns"), "--", "/usr/sbin/ip"}, request.args...)
		// #nosec G204 -- fixed nsenter/ip tools and literal read-only requests in the validated owned fixture namespace.
		raw, err := exec.Command("/usr/bin/nsenter", args...).Output()
		if err != nil || len(raw) > 1<<20 {
			public[request.name] = "bounded read refused"
			clear(raw)
			continue
		}
		var entries []map[string]json.RawMessage
		if json.Unmarshal(raw, &entries) != nil {
			public[request.name] = "decode refused"
			clear(raw)
			continue
		}
		result := []map[string]json.RawMessage{}
		for _, entry := range entries {
			filtered := map[string]json.RawMessage{}
			for _, key := range request.fields {
				if value, ok := entry[key]; ok {
					filtered[key] = value
				}
			}
			result = append(result, filtered)
		}
		public[request.name] = result
		clear(raw)
	}
	data, err := json.MarshalIndent(public, "", "  ")
	if err != nil {
		return
	}
	root := os.Getenv("NGFW_RA_EVIDENCE_ROOT")
	if root == "" {
		return
	}
	file, err := os.OpenFile(filepath.Join(root, plan.Instance+"-packet-metadata.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Error("own public packet metadata file refused")
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error("own public packet metadata close refused")
		}
	}()
	if _, err := file.Write(data); err != nil {
		t.Error("own public packet metadata write refused")
	}
}
