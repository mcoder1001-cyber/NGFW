package frr

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
)

// Seam S2 (wave-BC-numbers.md, D-119 M4; P12): protocol and linux-cp lines inside the framework's `interface X … exit`
// block. FRR prints every per-interface command (`ip address`, `ip ospf …`, `ip router isis …`, `ip pim`, interface BFD)
// inside one `interface X` block, so a section that rendered its own block would fight the framework's description
// block in frr-reload's diff. Instead a package registers an InterfaceLinesFunc; the framework calls every registered
// function (sorted by name) once per render and renders one block per Linux interface: the description first, then
// each function's lines in name order. A block is rendered when an interface has a description or lines.

// InterfaceLinesFunc returns, per Linux interface name (as FRR sees it: map VPP names with rc.MapInterface), the lines
// to render inside its `interface` block — complete lines with one leading blank (" ip address 10.0.0.1/24"), no
// `interface`/`exit` of their own. Every user string must pass the framework's validators, like any section line.
type InterfaceLinesFunc func(rc *RenderContext) (map[string][]string, error)

var (
	ifLinesMu  sync.Mutex
	ifLinesReg = map[string]InterfaceLinesFunc{}
)

// RegisterInterfaceLines adds a per-interface line producer to the global registry used by every Renderer created
// without WithInterfaceLines. It panics on a nil function or an invalid or duplicate name (init()-time programming
// errors), like RegisterSection.
func RegisterInterfaceLines(name string, fn InterfaceLinesFunc) {
	if fn == nil || !sectionNameRe.MatchString(name) {
		panic(fmt.Sprintf("frr: RegisterInterfaceLines(%q): invalid name or nil function", name))
	}
	ifLinesMu.Lock()
	defer ifLinesMu.Unlock()
	if _, dup := ifLinesReg[name]; dup {
		panic(fmt.Sprintf("frr: interface lines %q registered twice", name))
	}
	ifLinesReg[name] = fn
}

// NamedInterfaceLines is one producer for WithInterfaceLines.
type NamedInterfaceLines struct {
	Name string
	Fn   InterfaceLinesFunc
}

// WithInterfaceLines replaces the globally registered interface-line producers by extra (tests, or a renderer that must
// not pick up init()-time registrations).
func WithInterfaceLines(extra ...NamedInterfaceLines) Option {
	return func(r *Renderer) { r.ifLines = append([]NamedInterfaceLines{}, extra...) }
}

func registeredInterfaceLines() []NamedInterfaceLines {
	ifLinesMu.Lock()
	out := make([]NamedInterfaceLines, 0, len(ifLinesReg))
	for n, fn := range ifLinesReg {
		out = append(out, NamedInterfaceLines{Name: n, Fn: fn})
	}
	ifLinesMu.Unlock()
	slices.SortFunc(out, func(a, b NamedInterfaceLines) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

// mergeInterfaceLines adds the producers' lines to the model's interface blocks (creating blocks for interfaces without
// a description), validating every interface name and every line.
func mergeInterfaceLines(m *Model, producers []NamedInterfaceLines, rc *RenderContext) error {
	if len(producers) == 0 {
		return nil
	}
	idx := map[string]int{}
	for i, itf := range m.Interfaces {
		idx[itf.Name] = i
	}
	for _, p := range producers {
		byIf, err := p.Fn(rc)
		if err != nil {
			return fmt.Errorf("frr: interface lines %q: %w", p.Name, err)
		}
		for _, name := range slices.Sorted(maps.Keys(byIf)) {
			lines := byIf[name]
			if len(lines) == 0 {
				continue
			}
			if _, err := IfName(name); err != nil {
				return fmt.Errorf("%w: interface lines %q: %w", ErrInput, p.Name, err)
			}
			for i, l := range lines {
				if err := checkLine("interface lines "+p.Name, i, l); err != nil {
					return err
				}
				if !strings.HasPrefix(l, " ") || strings.TrimSpace(l) == "" || strings.TrimSpace(l) == "exit" {
					return fmt.Errorf("frr: interface lines %q: line %q must be one indented command inside the block", p.Name, l)
				}
			}
			k, ok := idx[name]
			if !ok {
				m.Interfaces = append(m.Interfaces, Interface{Name: name})
				k = len(m.Interfaces) - 1
				idx[name] = k
			}
			m.Interfaces[k].Lines = append(m.Interfaces[k].Lines, lines...)
		}
	}
	slices.SortFunc(m.Interfaces, func(a, b Interface) int { return cmp.Compare(a.Name, b.Name) })
	return nil
}
