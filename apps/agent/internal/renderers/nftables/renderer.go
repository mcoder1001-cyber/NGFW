// Package nftables is the host firewall renderer (F-host-acl-nftables, D-057): it renders `acl.host`,
// `acl.hostAttachments` and `acl.hostSettings` into ONE nftables table, `table inet vrx`, and nothing
// else — never `flush ruleset`, never another table (P10's static base policy and foreign tables
// survive). The table is replaced atomically by one `nft -f` transaction (`add table` + `delete table` +
// the full table body); `nft -c -f` validates a staged copy first; Retrieve reads `nft -j list table`.
//
// The agent drives it through one singleton scheduler descriptor, `host-acl.nftables/vrx`
// (descriptor.go), registered under Domains["acl"] (decision (a) of the task: no agent-core change).
// See README.md and docs/agent/renderers/nftables.md.
package nftables

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
)

// Renderer renders and loads one owner's host firewall table.
type Renderer struct {
	runner renderers.Runner
	paths  Paths
}

var _ renderers.Renderer = (*Renderer)(nil)

// New returns the renderer for paths. In mode netns every nft call runs inside paths.Netns (setns on a
// locked thread); in mode check Apply only writes the file (nothing is loaded).
func New(runner renderers.Runner, paths Paths) *Renderer {
	if paths.Mode == ModeNetns {
		runner = NewNetnsRunner(paths.Netns, runner)
	}
	return &Renderer{runner: runner, paths: paths}
}

// Name implements renderers.Renderer.
func (r *Renderer) Name() string { return "nftables" }

// Paths returns the renderer's paths.
func (r *Renderer) Paths() Paths { return r.paths }

// Render implements renderers.Renderer: desired is a *HostTable (nil: the table is removed).
func (r *Renderer) Render(_ context.Context, desired proto.Message) (renderers.Files, error) {
	var v *HostTable
	if desired != nil {
		var ok bool
		if v, ok = desired.(*HostTable); !ok {
			return nil, fmt.Errorf("nftables: render %T, want *HostTable", desired)
		}
	}
	content, err := RenderText(r.paths.Table, v)
	if err != nil {
		return nil, err
	}
	return renderers.Files{r.paths.RulesFile: {Mode: 0o600, Content: content}}, nil
}

// Validate implements renderers.Renderer: `nft -c -f` on a staged copy (never the live file; `-c` never
// changes the ruleset).
func (r *Renderer) Validate(ctx context.Context, files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	if _, ok := files[r.paths.RulesFile]; !ok {
		return fmt.Errorf("%w: %s missing", renderers.ErrInvalidFiles, r.paths.RulesFile)
	}
	st, err := renderers.Stage(files)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if _, err := r.nft(ctx, "-c", "-f", st.Path(r.paths.RulesFile)); err != nil {
		return fmt.Errorf("nftables: nft -c rejected the rendering: %w", err)
	}
	return nil
}

// Apply implements renderers.Renderer: write the file atomically and load it with one `nft -f`
// transaction. The kernel applies all or nothing, so a failed load leaves the old table in place; the
// previous file is restored. Mode check writes the file only.
func (r *Renderer) Apply(ctx context.Context, files renderers.Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	if err := renderers.WriteFiles(files); err != nil {
		return errors.Join(err, snap.Restore())
	}
	if r.paths.Mode == ModeCheck {
		return nil
	}
	if _, err := r.nft(ctx, "-f", r.paths.RulesFile); err != nil {
		return errors.Join(fmt.Errorf("nftables: load %s: %w", r.paths.RulesFile, err), snap.Restore())
	}
	return nil
}

// Retrieve implements renderers.Renderer: the kernel's table as a *HostTable (sets, chains, rule
// comments and verdicts; no configuration, no annotations), or a nil *HostTable when the table does not
// exist. Mode check has no kernel table: nil.
func (r *Renderer) Retrieve(ctx context.Context) (proto.Message, error) {
	k, err := r.Kernel(ctx)
	if err != nil || k == nil {
		return (*HostTable)(nil), err
	}
	return k.Table(), nil
}

// Kernel lists the table (`nft -j list table inet <table>`); nil when it does not exist (or mode check).
func (r *Renderer) Kernel(ctx context.Context) (*KernelTable, error) {
	if r.paths.Mode == ModeCheck {
		return nil, nil
	}
	out, err := r.nft(ctx, "-j", "list", "table", "inet", r.paths.Table)
	if err != nil {
		var ee *renderers.ExitError
		if errors.As(err, &ee) && bytes.Contains(ee.Output.Stderr, []byte("No such file or directory")) {
			return nil, nil
		}
		return nil, fmt.Errorf("nftables: list table inet %s: %w", r.paths.Table, err)
	}
	return ParseKernel(out.Stdout, r.paths.Table)
}

func (r *Renderer) nft(ctx context.Context, args ...string) (renderers.Output, error) {
	return r.runner.Run(ctx, renderers.Command{Path: NftBin, Args: args})
}

// ---- text --------------------------------------------------------------------------------------

var (
	setNameRe   = regexp.MustCompile(`^a[46]_[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	chainNameRe = regexp.MustCompile(`^(in|out|fwd)_[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	commentRe   = regexp.MustCompile(`^vrx:(@[a-z-]+|[A-Za-z0-9][A-Za-z0-9_.-]{0,62}:[0-9]{1,10})/[0-9]{1,4}:[0-9a-f]{8}$`)
)

// RenderText is the file `nft -f` loads for table (v nil or without chains: the table is removed). It
// checks v first (Validate): the value may come from the agent's store, so nothing in it is trusted.
func RenderText(table string, v *HostTable) ([]byte, error) {
	if !tableRe.MatchString(table) {
		return nil, fmt.Errorf("%w: table name %q", renderers.ErrUnsafe, table)
	}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# vrx-agent host firewall (F-host-acl-nftables): table inet %s, rendered from acl.host*.\n", table)
	b.WriteString("# One nft -f transaction replaces the whole table; hand edits are overwritten by the next apply.\n")
	fmt.Fprintf(&b, "add table inet %s\ndelete table inet %s\n", table, table)
	if len(v.GetChains()) > 0 {
		fmt.Fprintf(&b, "table inet %s {\n\tcomment \"vrx-agent host firewall\"\n", table)
		for _, s := range v.GetSets() {
			fmt.Fprintf(&b, "\tset %s {\n\t\ttype %s\n\t\tflags interval\n", s.GetName(), s.GetType())
			if len(s.GetElements()) > 0 {
				fmt.Fprintf(&b, "\t\telements = { %s }\n", strings.Join(s.GetElements(), ", "))
			}
			b.WriteString("\t}\n")
		}
		for _, c := range v.GetChains() {
			fmt.Fprintf(&b, "\tchain %s {\n\t\ttype filter hook %s priority %d; policy %s;\n", c.GetName(), c.GetHook(), c.GetPriority(), c.GetPolicy())
			for _, rl := range c.GetRules() {
				fmt.Fprintf(&b, "\t\t%s comment %q\n", rl.GetText(), rl.GetComment())
			}
			b.WriteString("\t}\n")
		}
		b.WriteString("}\n")
	}
	out := []byte(b.String())
	if err := renderers.CheckRendered(out); err != nil {
		return nil, err
	}
	return out, nil
}

// Validate checks every token of v that reaches the rendered file: names, types, hooks, policies,
// priorities, set elements (canonical prefixes), comments and rule texts (see checkRuleText).
func (v *HostTable) Validate() error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", renderers.ErrUnsafe, fmt.Sprintf(format, a...))
	}
	for _, s := range v.GetSets() {
		if !setNameRe.MatchString(s.GetName()) {
			return bad("set name %q", s.GetName())
		}
		want := map[byte]string{'4': "ipv4_addr", '6': "ipv6_addr"}[s.GetName()[1]]
		if s.GetType() != want {
			return bad("set %s type %q, want %q", s.GetName(), s.GetType(), want)
		}
		for _, e := range s.GetElements() {
			p, err := netip.ParsePrefix(e)
			if err != nil || p.Masked().String() != e || (p.Addr().Is4() != (want == "ipv4_addr")) {
				return bad("set %s element %q", s.GetName(), e)
			}
		}
	}
	for _, c := range v.GetChains() {
		switch {
		case !chainNameRe.MatchString(c.GetName()):
			return bad("chain name %q", c.GetName())
		case hookPrefix[c.GetHook()] == "" || !strings.HasPrefix(c.GetName(), hookPrefix[c.GetHook()]+"_"):
			return bad("chain %s hook %q", c.GetName(), c.GetHook())
		case c.GetPolicy() != "accept" && c.GetPolicy() != "drop":
			return bad("chain %s policy %q", c.GetName(), c.GetPolicy())
		case c.GetPriority() < -500 || c.GetPriority() > 500:
			return bad("chain %s priority %d", c.GetName(), c.GetPriority())
		}
		for _, rl := range c.GetRules() {
			if !commentRe.MatchString(rl.GetComment()) {
				return bad("rule comment %q", rl.GetComment())
			}
			if err := checkRuleText(rl.GetText()); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkRuleText is the backstop for rule texts: printable ASCII, no statement separators or comments
// (`;`, `#`), no escapes, quoted strings only of nft-safe tokens, braces balanced and flat, and a verdict
// at the end. Build only produces texts that pass; a text from anywhere else that does not is refused.
func checkRuleText(t string) error {
	bad := func(why string) error { return fmt.Errorf("%w: rule text %q: %s", renderers.ErrUnsafe, t, why) }
	if t == "" {
		return bad("empty")
	}
	depth, inQuote := 0, false
	for i := 0; i < len(t); i++ {
		c := t[i]
		switch {
		case c < 0x20 || c > 0x7e:
			return bad("control or non-ASCII character")
		case c == ';' || c == '#' || c == '\\':
			return bad(fmt.Sprintf("character %q", c))
		case c == '"':
			inQuote = !inQuote
		case inQuote && !(c == ' ' || c == ':' || c == '_' || c == '.' || c == '-' || c == '@' ||
			(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')):
			return bad(fmt.Sprintf("character %q inside quotes", c))
		case !inQuote && c == '{':
			if depth++; depth > 1 {
				return bad("nested braces")
			}
		case !inQuote && c == '}':
			if depth--; depth < 0 {
				return bad("unbalanced braces")
			}
		}
	}
	if inQuote || depth != 0 {
		return bad("unbalanced quotes or braces")
	}
	for _, v := range []string{" accept", " drop", " reject"} {
		if strings.HasSuffix(" "+t, v) {
			return nil
		}
	}
	return bad("no verdict at the end")
}
