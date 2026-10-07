package ravpn

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
)

// Record Linux's down, unaddressed automatic fallback devices before TAPs or
// daemon activation. Arbitrary existing devices never become an owned baseline.
var kernelFallbackKinds = map[string]string{"ip6tnl0": "ip6tnl", "tunl0": "ipip", "gre0": "gre", "gretap0": "gretap", "erspan0": "erspan", "sit0": "sit", "ip6gre0": "ip6gre", "ip6gretap0": "ip6gretap", "ip6erspan0": "ip6erspan", "vti0": "vti", "ip6_vti0": "vti6"}

type observedLink struct {
	Name  string   `json:"ifname"`
	Index uint32   `json:"ifindex"`
	Flags []string `json:"flags"`
	Info  struct {
		Kind string `json:"info_kind"`
		Data struct {
			ID uint32 `json:"if_id"`
		} `json:"info_data"`
	} `json:"linkinfo"`
}

func recordKernelLinks(ctx context.Context, instance string) ([]KernelLink, error) {
	data, err := command(ctx, "/usr/bin/nsenter", nil, "--net="+filepath.Join(InstanceRoot, instance, "netns"), "--", "/usr/sbin/ip", "-j", "-d", "link", "show")
	if err != nil {
		return nil, ErrBoundary
	}
	var links []observedLink
	if json.Unmarshal(data, &links) != nil {
		return nil, ErrBoundary
	}
	var result []KernelLink
	seen := map[string]bool{}
	hasLoopback := false
	for _, link := range links {
		if seen[link.Name] {
			return nil, ErrBoundary
		}
		seen[link.Name] = true
		if link.Name == "lo" && link.Info.Kind == "" {
			hasLoopback = true
			continue
		}
		if kind := kernelFallbackKinds[link.Name]; kind == "" || kind != link.Info.Kind || link.Index == 0 || slices.Contains(link.Flags, "UP") {
			return nil, ErrBoundary
		}
		result = append(result, KernelLink{Name: link.Name, Kind: link.Info.Kind, Index: link.Index})
	}
	if !hasLoopback || len(result) > 16 {
		return nil, ErrBoundary
	}
	data, err = command(ctx, "/usr/bin/nsenter", nil, "--net="+filepath.Join(InstanceRoot, instance, "netns"), "--", "/usr/sbin/ip", "-j", "address", "show")
	if err != nil {
		return nil, ErrBoundary
	}
	var addresses []struct {
		Name   string            `json:"ifname"`
		Values []json.RawMessage `json:"addr_info"`
	}
	if json.Unmarshal(data, &addresses) != nil {
		return nil, ErrBoundary
	}
	for _, entry := range addresses {
		if entry.Name != "lo" && len(entry.Values) != 0 {
			return nil, ErrBoundary
		}
	}
	return result, nil
}
func matchesKernelLink(plan *NetworkPlan, observed observedLink) bool {
	for _, link := range plan.KernelLinks {
		if link.Name == observed.Name {
			return link.Index == observed.Index && link.Kind == observed.Info.Kind && !slices.Contains(observed.Flags, "UP")
		}
	}
	return false
}
