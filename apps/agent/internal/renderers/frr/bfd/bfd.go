// Package bfd owns the FRR BFD profile section. Protocols create dynamic peers through their own BFD attachment lines.
package bfd

import (
	"fmt"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"sort"
)

// Reader identifies the bounded FRR BFD peer observation.
const Reader = "bfdPeers"

func init() {
	frr.RegisterSection(Section{})
	frr.RegisterInterfaceLines("bfd-profiles", InterfaceLines)
	frr.RegisterStateReader(frr.StateReader{Key: Reader, Command: "show bfd peers json"})
}

// Section renders named FRR BFD profiles.
type Section struct{}

// Name identifies this renderer section.
func (Section) Name() string { return "bfd" }

// Order places profiles after protocol configuration.
func (Section) Order() int { return 700 }

// Render validates and emits configured profiles with millisecond timers.
func (Section) Render(rc *frr.RenderContext) ([]string, error) {
	profiles := rc.Desired.GetRouting().GetBfd().GetProfiles()
	if len(profiles) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	out := []string{"bfd"}
	for _, name := range names {
		n, e := renderers.Ident(name)
		if e != nil {
			return nil, e
		}
		p := profiles[name]
		tx, rx, m := p.GetDesiredMinTxUs(), p.GetRequiredMinRxUs(), p.GetDetectMultiplier()
		if tx == 0 {
			tx = 300000
		}
		if rx == 0 {
			rx = 300000
		}
		if m == 0 {
			m = 3
		}
		if tx%1000 != 0 || rx%1000 != 0 {
			return nil, fmt.Errorf("BFD profile %s: FRR intervals require whole milliseconds", n)
		}
		out = append(out, " profile "+n, fmt.Sprintf("  transmit-interval %d", tx/1000), fmt.Sprintf("  receive-interval %d", rx/1000), fmt.Sprintf("  detect-multiplier %d", m), " exit")
	}
	return append(out, "exit"), nil
}

// InterfaceLines extends existing protocol BFD attachments with a named profile.
func InterfaceLines(rc *frr.RenderContext) (map[string][]string, error) {
	out := map[string][]string{}
	add := func(name, profile, line string) error {
		if profile == "" {
			return nil
		}
		p, e := renderers.Ident(profile)
		if e != nil {
			return e
		}
		linux, ok := rc.MapInterface(name)
		if !ok {
			return fmt.Errorf("BFD profile interface %s has no Linux mapping", name)
		}
		if _, e := frr.IfName(linux); e != nil {
			return e
		}
		out[linux] = append(out[linux], line+p)
		return nil
	}
	for name, i := range rc.Desired.GetRouting().GetOspf().GetInterfaces() {
		if i.GetBfd() {
			if e := add(name, i.GetBfdProfile(), " ip ospf bfd profile "); e != nil {
				return nil, e
			}
		}
	}
	for name, i := range rc.Desired.GetRouting().GetIsis().GetInterfaces() {
		if i.GetBfd() {
			if e := add(name, i.GetBfdProfile(), " isis bfd profile "); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}
