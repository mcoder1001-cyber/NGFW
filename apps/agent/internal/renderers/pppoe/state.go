package pppoe

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// maxStateFile bounds a hook state file (it holds a handful of short lines).
const maxStateFile = 64 << 10

// ReadState parses the ip-up/ip-down hook's state file for one session's host interface into the proto
// state. A missing file is the "down" state (the session never came up). The IPv6 side ("<hostif>.state6", see
// ReadIPv6) fills the ipv6 field and makes an IPv6-only session "up". fail and lastErr come from the
// agent's supervisor (pppd exit tracking), not the hook, so they are passed in.
func (r *Renderer) ReadState(hostIf string, failCount uint32, lastErr string) (*ngfwv1.PppoeSessionState, error) {
	st := &ngfwv1.PppoeSessionState{Phase: "down", FailCount: failCount, LastError: lastErr}
	b, err := os.ReadFile(filepath.Join(r.paths.StateDir, hostIf+".state")) //nolint:gosec // StateDir is a fixed product path
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("pppoe: read state of %q: %w", hostIf, err)
	}
	if len(b) > maxStateFile {
		b = b[:maxStateFile]
	}
	kv := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if ok {
			kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	switch kv["phase"] {
	case "up":
		st.Phase = "up"
	case "down":
		st.Phase = "down"
	}
	if v := kv["local"]; v != "" {
		st.LocalIpv4 = v + "/32"
	}
	st.PeerIpv4 = kv["peer"]
	var dns []string
	for _, k := range []string{"dns1", "dns2"} {
		if v := kv[k]; v != "" {
			dns = append(dns, v)
		}
	}
	sort.Strings(dns)
	st.Dns = dns
	if v := kv["at"]; v != "" && st.Phase == "up" {
		if ts, err := time.Parse(time.RFC3339, v); err == nil {
			st.Since = timestamppb.New(ts)
		}
	}
	// IPv6 runs its own NCP (IPv6CP): an IPv6-only session is up too, and its addresses/prefix are reported.
	v6, err := r.ReadIPv6(hostIf)
	if err != nil {
		return nil, err
	}
	if v6.Up {
		st.Ipv6 = v6.Summary()
		if st.Phase != "up" {
			st.Phase = "up"
			if !v6.Since.IsZero() {
				st.Since = timestamppb.New(v6.Since)
			}
		}
	}
	if failCount > 0 && st.Phase != "up" {
		st.Phase = "failed"
	}
	return st, nil
}
