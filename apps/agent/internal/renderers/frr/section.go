package frr

import (
	"cmp"
	"embed"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"text/template"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
)

// Section renders one part of frr.conf. The framework re-renders the whole file on every
// change (frr-reload.py computes the diff against the running config), concatenating the
// sections in Order() and separating them with "!".
//
// Protocol tasks (P12 BGP, F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-srmpls,
// F-igmp-mfib) implement Section in their own package and call RegisterSection from init();
// they never edit framework files. Render receives the message given to Renderer.Render
// (a *vrxv1.DesiredState, or a *structpb.Struct during the D-055 stand-in period — use
// frr.Desired to get the typed state) and returns complete config lines in FRR syntax,
// including the block's own "exit"/"exit-vrf" lines. Every user string must pass one of the
// escaping helpers (frr.Hostname/IfName/VRFName/Description, renderers.Ident/Addr/Network…);
// the framework additionally rejects any line with a control character, an embedded newline
// or a bare "end".
type Section interface {
	// Name is a unique lower-case identifier ("bgp", "ospf", "route-map").
	Name() string
	// Order places the section; protocol sections use OrderProtocolMin..OrderProtocolMax.
	Order() int
	// Render returns the section's lines; nil/empty means the section is absent.
	Render(desired proto.Message) ([]string, error)
}

// Section order. The framework owns everything outside the protocol range.
const (
	OrderGlobals     = 0   // frr version, defaults, hostname, log, service integrated-vtysh-config
	OrderVRF         = 100 // vrf <name> … exit-vrf (with the VRF's static routes, FRR's canonical form)
	OrderInterface   = 200 // interface <name> / description / exit
	OrderStatic      = 300 // default-VRF ip route / ipv6 route (staticd)
	OrderProtocolMin = 400 // first order a registered section may use (router bgp, route-map, …)
	OrderProtocolMax = 899 // last order a registered section may use
	// 900–999 are reserved for the framework's trailing sections (line vty when it has content).
)

var (
	registryMu sync.Mutex
	registry   = map[string]Section{}

	sectionNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
)

// RegisterSection adds a protocol section to the global registry used by every Renderer
// created without WithSections. It panics on a nil section, an invalid or duplicate name, a
// framework name, or an order outside OrderProtocolMin..OrderProtocolMax — registration
// happens in init(), so these are programming errors.
func RegisterSection(s Section) {
	if s == nil {
		panic("frr: RegisterSection(nil)")
	}
	name := s.Name()
	if !sectionNameRe.MatchString(name) {
		panic(fmt.Sprintf("frr: section name %q must match %s", name, sectionNameRe))
	}
	if o := s.Order(); o < OrderProtocolMin || o > OrderProtocolMax {
		panic(fmt.Sprintf("frr: section %q order %d outside %d..%d", name, o, OrderProtocolMin, OrderProtocolMax))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[name]; dup || isFrameworkSection(name) {
		panic(fmt.Sprintf("frr: section %q registered twice", name))
	}
	registry[name] = s
}

// RegisteredSections returns the registered protocol sections sorted by (Order, Name).
func RegisteredSections() []Section {
	registryMu.Lock()
	out := make([]Section, 0, len(registry))
	for _, s := range registry {
		out = append(out, s)
	}
	registryMu.Unlock()
	sortSections(out)
	return out
}

func sortSections(ss []Section) {
	slices.SortStableFunc(ss, func(a, b Section) int {
		return cmp.Or(cmp.Compare(a.Order(), b.Order()), cmp.Compare(a.Name(), b.Name()))
	})
}

// ---------------------------------------------------------------------------- framework

//go:embed templates/*.tmpl
var templateFS embed.FS

var tmpl = template.Must(renderers.NewTemplate("frr").Funcs(funcs()).ParseFS(templateFS, "templates/*.tmpl"))

// DefaultVersion is the FRR version written in the `frr version` line (the packaged FRR on
// Ubuntu 26.04; frr-reload.py never deletes this line and the daemons accept any value).
const DefaultVersion = "10.7.1"

var versionRe = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}(\.[0-9]{1,3})?$`)

var frameworkNames = []string{"globals", "vrfs", "interfaces", "static"}

func isFrameworkSection(name string) bool { return slices.Contains(frameworkNames, name) }

// frameworkSection renders one framework template from the Model built out of the input.
type frameworkSection struct {
	name    string
	order   int
	version string
	mapIf   InterfaceMapper
}

func (s frameworkSection) Name() string { return s.name }
func (s frameworkSection) Order() int   { return s.order }

func (s frameworkSection) Render(desired proto.Message) ([]string, error) {
	ds, ext, err := Desired(desired)
	if err != nil {
		return nil, err
	}
	m, err := BuildModel(ds, ext, s.mapIf)
	if err != nil {
		return nil, err
	}
	out, err := renderers.ExecuteTemplate(tmpl, s.name, struct {
		Version string
		Model   *Model
	}{s.version, m})
	if err != nil {
		return nil, err
	}
	return splitLines(string(out)), nil
}

func frameworkSections(version string, mapIf InterfaceMapper) []Section {
	return []Section{
		frameworkSection{name: "globals", order: OrderGlobals, version: version, mapIf: mapIf},
		frameworkSection{name: "vrfs", order: OrderVRF, version: version, mapIf: mapIf},
		frameworkSection{name: "interfaces", order: OrderInterface, version: version, mapIf: mapIf},
		frameworkSection{name: "static", order: OrderStatic, version: version, mapIf: mapIf},
	}
}

// splitLines splits rendered text into lines, trimming trailing blanks and dropping empty
// lines (template whitespace is cosmetic; the renderer inserts the "!" separators).
func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimRight(l, " \t")
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// checkLine is the per-line backstop applied to every section's output, framework or not.
func checkLine(section string, i int, line string) error {
	if strings.ContainsAny(line, "\n\r") {
		return fmt.Errorf("%w: section %q line %d contains a line break", renderers.ErrUnsafe, section, i+1)
	}
	if err := renderers.CheckRendered([]byte(line)); err != nil {
		return fmt.Errorf("section %q line %d: %w", section, i+1, err)
	}
	if strings.TrimSpace(line) == "end" {
		return fmt.Errorf("%w: section %q line %d is a bare \"end\" (would truncate frr.conf)", renderers.ErrUnsafe, section, i+1)
	}
	return nil
}

// assemble renders all sections in order into the complete frr.conf.
func assemble(sections []Section, desired proto.Message) ([]byte, error) {
	ss := slices.Clone(sections)
	sortSections(ss)
	seen := map[string]bool{}
	var b strings.Builder
	for _, s := range ss {
		name := s.Name()
		if seen[name] {
			return nil, fmt.Errorf("frr: section %q appears twice", name)
		}
		seen[name] = true
		lines, err := s.Render(desired)
		if err != nil {
			return nil, fmt.Errorf("frr: section %q: %w", name, err)
		}
		var kept []string
		for i, l := range lines {
			if err := checkLine(name, i, l); err != nil {
				return nil, err
			}
			if strings.TrimSpace(l) != "" {
				kept = append(kept, strings.TrimRight(l, " \t"))
			}
		}
		if len(kept) == 0 {
			continue
		}
		for _, l := range kept {
			b.WriteString(l)
			b.WriteByte('\n')
		}
		b.WriteString("!\n")
	}
	b.WriteString("end\n")
	out := []byte(b.String())
	if err := renderers.CheckRendered(out); err != nil {
		return nil, err
	}
	return out, nil
}

// renderVtyshConf renders vtysh.conf.
func renderVtyshConf() ([]byte, error) {
	return renderers.ExecuteTemplate(tmpl, "vtysh.conf", nil)
}
