package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"ngfw/cli/internal/api"
	"ngfw/cli/internal/cpath"
	"ngfw/cli/internal/jschema"
	"ngfw/cli/internal/lineedit"
)

// complete is the REPL's Tab handler: command words, then the command's own argument completion (config paths and
// enum values from the live JSON Schema, map keys from the candidate).
func (a *App) complete(line string) ([]lineedit.Candidate, int) {
	all, start := a.candidates(context.Background(), line)
	out := make([]lineedit.Candidate, 0, len(all))
	for _, c := range all {
		if !strings.HasPrefix(c.Text, "<") { // <placeholders> are help, not text to insert
			out = append(out, c)
		}
	}
	return out, start
}

// help is the REPL's `?` handler.
func (a *App) help(line string) string {
	cands, _ := a.candidates(context.Background(), line)
	if len(cands) == 0 {
		return "  <Enter>  (nothing more expected)"
	}
	return strings.TrimRight(lineedit.List(cands, 110), "\n")
}

// candidates returns everything that may follow line (the last word filtered by its prefix) and where that word
// starts.
func (a *App) candidates(ctx context.Context, line string) ([]lineedit.Candidate, int) {
	toks, err := cpath.Tokenize(line)
	if err != nil {
		return nil, len(line)
	}
	partial := ""
	start := len(line)
	if len(toks) > 0 && !strings.HasSuffix(line, " ") && !strings.HasSuffix(line, "\t") {
		last := toks[len(toks)-1]
		if last.Quoted {
			return nil, len(line)
		}
		partial, start = last.Text, last.Start
		toks = toks[:len(toks)-1]
	}
	mode := a.mode
	if mode == ModeConfig && len(toks) > 0 && toks[0].Text == "run" {
		mode, toks = ModeOperational, toks[1:]
	}
	var out []lineedit.Candidate
	cmd, n, _ := match(mode, toks)
	full := cmd != nil && n == len(toks)
	// command words that can follow what was typed
	seen := map[string]bool{}
	for _, c := range Commands() {
		if !available(c.Where, mode) || len(c.Words) <= len(toks) || !wordsMatch(c.Words, toks) {
			continue
		}
		w := c.Words[len(toks)]
		if seen[w] || !strings.HasPrefix(w, partial) {
			continue
		}
		seen[w] = true
		help := c.Summary
		if len(c.Words) > len(toks)+1 {
			help = "…"
		}
		out = append(out, lineedit.Candidate{Text: w, Help: help})
	}
	if len(toks) == 0 && mode == ModeConfig && strings.HasPrefix("run", partial) {
		out = append(out, lineedit.Candidate{Text: "run", Help: "run an operational command"})
	}
	if cmd != nil && (full || n < len(toks) || len(out) == 0) && cmd.Complete != nil {
		args := cpath.Texts(toks[n:])
		for _, c := range cmd.Complete(a, ctx, args, partial) {
			if strings.HasPrefix(c.Text, partial) || strings.HasPrefix(c.Text, "<") {
				out = append(out, c)
			}
		}
	}
	return out, start
}

func wordsMatch(words []string, toks []cpath.Token) bool {
	for i, t := range toks {
		if i >= len(words) || t.Quoted {
			return false
		}
		if words[i] != t.Text && !(i < len(toks) && strings.HasPrefix(words[i], t.Text) && uniquePrefix(t.Text, i, toks)) {
			return false
		}
	}
	return true
}

func uniquePrefix(w string, depth int, toks []cpath.Token) bool {
	hits := map[string]bool{}
	for _, c := range registry {
		if len(c.Words) > depth && strings.HasPrefix(c.Words[depth], w) {
			hits[c.Words[depth]] = true
		}
	}
	return len(hits) == 1
}

// candidateDoc caches the candidate for completion (2 s), so Tab does not hit the API per key press.
func (a *App) candidateDoc(ctx context.Context) any {
	if a.candCache != nil && time.Since(a.candCacheAt) < 2*time.Second {
		return a.candCache
	}
	var v any
	if _, err := a.call(ctx, api.Call{Op: "Config_candidate"}, &v); err != nil {
		return nil
	}
	a.candCache, a.candCacheAt = v, time.Now()
	return v
}

func valueAt(doc any, segs []string) any {
	cur := doc
	for _, s := range segs {
		switch x := cur.(type) {
		case map[string]any:
			cur = x[s]
		case []any:
			i, err := strconv.Atoi(s)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			cur = x[i]
		default:
			return nil
		}
	}
	return cur
}

// pathCandidates lists what can follow segs in the schema: properties, existing map keys (+ a <name> hint),
// existing array indices.
func (a *App) pathCandidates(ctx context.Context, segs []string, containersOnly bool) (*jschema.Node, []lineedit.Candidate) {
	d, err := a.Schema(ctx)
	if err != nil {
		return nil, nil
	}
	node, err := d.Root().Walk(segs)
	if err != nil {
		return nil, nil
	}
	var out []lineedit.Candidate
	for _, p := range node.Properties() {
		if containersOnly && !p.Node.IsContainer() {
			continue
		}
		help := p.Node.Title()
		if h := p.Node.Help(); h != "" {
			if help != "" {
				help += " — "
			}
			help += h
		}
		if !p.Node.IsContainer() {
			help = strings.TrimSpace(help + " [" + p.Node.TypeHint() + "]")
		}
		out = append(out, lineedit.Candidate{Text: cpath.Quote(p.Name), Help: help, More: false})
	}
	if mv := node.MapValue(); mv != nil && (!containersOnly || mv.IsContainer()) {
		cur := valueAt(a.candidateDoc(ctx), segs)
		if m, ok := cur.(map[string]any); ok {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				out = append(out, lineedit.Candidate{Text: cpath.Quote(k), Help: "(existing)"})
			}
		}
		label := "name"
		help := "new entry"
		if kn := node.KeyNode(); kn != nil {
			label = strings.ToLower(strings.ReplaceAll(kn.Label("name"), " ", "-"))
			if h := kn.TypeHint(); h != "" {
				help = "new entry: " + h
			}
		}
		out = append(out, lineedit.Candidate{Text: "<" + label + ">", Help: help})
	}
	if node.Has("array") && node.Items() != nil && node.Items().IsContainer() {
		if arr, ok := valueAt(a.candidateDoc(ctx), segs).([]any); ok {
			for i := range arr {
				out = append(out, lineedit.Candidate{Text: strconv.Itoa(i), Help: "(existing item)"})
			}
			out = append(out, lineedit.Candidate{Text: strconv.Itoa(len(arr)), Help: "new item"})
		} else {
			out = append(out, lineedit.Candidate{Text: "0", Help: "new item"})
		}
	}
	return node, out
}

// valueCandidates: enum values / booleans, else a typed placeholder.
func valueCandidates(node *jschema.Node) []lineedit.Candidate {
	target := node
	if node.IsLeafList() {
		target = node.Items()
	}
	var out []lineedit.Candidate
	for _, e := range target.Enum() {
		out = append(out, lineedit.Candidate{Text: cpath.Quote(e)})
	}
	if len(out) > 0 {
		return out
	}
	help := target.Help()
	if node.IsLeafList() {
		help = strings.TrimSpace("appends to the list; " + help)
	}
	return []lineedit.Candidate{{Text: "<" + strings.ReplaceAll(target.TypeHint(), " ", "") + ">", Help: help}}
}

func (a *App) segsFor(args []string, base []string) ([]string, bool) {
	segs, err := cpath.Resolve(base, args)
	if err != nil {
		return nil, false
	}
	return segs, true
}

func completePath(a *App, ctx context.Context, args []string, _ string) []lineedit.Candidate {
	segs, ok := a.segsFor(args, a.edit)
	if !ok {
		return nil
	}
	_, out := a.pathCandidates(ctx, segs, false)
	return out
}

func completeContainers(a *App, ctx context.Context, args []string, _ string) []lineedit.Candidate {
	segs, ok := a.segsFor(args, a.edit)
	if !ok {
		return nil
	}
	_, out := a.pathCandidates(ctx, segs, true)
	return out
}

func completeSet(a *App, ctx context.Context, args []string, _ string) []lineedit.Candidate {
	segs, ok := a.segsFor(args, a.edit)
	if !ok || len(segs) == 0 {
		if ok {
			_, out := a.pathCandidates(ctx, segs, false)
			return out
		}
		return nil
	}
	d, err := a.Schema(ctx)
	if err != nil {
		return nil
	}
	node, err := d.Root().Walk(segs)
	if err != nil {
		return nil // past the value: nothing more
	}
	if node.IsContainer() {
		_, out := a.pathCandidates(ctx, segs, false)
		return append(out, lineedit.Candidate{Text: "<json>", Help: "the whole " + strings.Join(node.Types(), "|") + " as JSON"})
	}
	return valueCandidates(node)
}

func completeDelete(a *App, ctx context.Context, args []string, _ string) []lineedit.Candidate {
	segs, ok := a.segsFor(args, a.edit)
	if !ok {
		return nil
	}
	d, err := a.Schema(ctx)
	if err != nil {
		return nil
	}
	node, err := d.Root().Walk(segs)
	if err != nil {
		return nil
	}
	if node.IsLeafList() {
		var out []lineedit.Candidate
		if arr, ok := valueAt(a.candidateDoc(ctx), segs).([]any); ok {
			for _, x := range arr {
				out = append(out, lineedit.Candidate{Text: cpath.Quote(fmt.Sprint(x)), Help: "(existing item)"})
			}
		}
		return append(out, lineedit.Candidate{Text: "<Enter>", Help: "delete the whole list"})
	}
	if !node.IsContainer() {
		return nil
	}
	_, out := a.pathCandidates(ctx, segs, false)
	return out
}

func completeShowPath(a *App, ctx context.Context, args []string, _ string) []lineedit.Candidate {
	base := []string(nil)
	if a.mode == ModeConfig {
		base = a.edit
	}
	fmts := []lineedit.Candidate{{Text: "json", Help: "output format"}, {Text: "text", Help: "output format (default)"}, {Text: "set", Help: "output format: set commands"}}
	if n := len(args); n > 0 && (args[n-1] == "json" || args[n-1] == "text" || args[n-1] == "set") {
		return nil
	}
	segs, ok := a.segsFor(args, base)
	if !ok {
		return fmts
	}
	d, err := a.Schema(ctx)
	if err != nil {
		return fmts
	}
	node, err := d.Root().Walk(segs)
	if err != nil || !node.IsContainer() {
		return fmts
	}
	_, out := a.pathCandidates(ctx, segs, false)
	return append(out, fmts...)
}

func (a *App) revisionCandidates(ctx context.Context) []lineedit.Candidate {
	var out struct {
		Items []revisionMeta `json:"items"`
	}
	if _, err := a.call(ctx, api.Call{Op: "Config_revisions", Query: map[string][]string{"limit": {"20"}}}, &out); err != nil {
		return nil
	}
	c := make([]lineedit.Candidate, 0, len(out.Items))
	for _, r := range out.Items {
		c = append(c, lineedit.Candidate{Text: strconv.Itoa(r.ID), Help: shortTime(r.CreatedAt) + " " + r.Comment})
	}
	return c
}

// helpCmd prints the command list of the current mode, or one command's syntax and REST mapping.
func helpCmd(a *App, _ context.Context, args []cpath.Token) error {
	mode := a.mode
	if len(args) > 0 {
		cmd, n, _ := match(mode, args)
		if cmd == nil || n != len(args) {
			return usagef("no command %q (try `help`)", cpath.Words(cpath.Texts(args)))
		}
		return a.emit(cmdDoc(cmd), func(w io.Writer) {
			fmt.Fprintf(w, "%s %s\n  %s\n", cmd.Name(), cmd.Args, cmd.Summary)
			for _, id := range cmd.Ops {
				if op, err := api.Lookup(id); err == nil {
					fmt.Fprintf(w, "  REST: %s %s\n", op.Method, op.Path)
				}
			}
			if cmd.NoREST != "" {
				fmt.Fprintf(w, "  REST: none — %s\n", cmd.NoREST)
			}
			if cmd.Example != "" {
				fmt.Fprintf(w, "  example: %s\n", cmd.Example)
			}
		})
	}
	var docs []map[string]any
	rows := [][]string{}
	for _, c := range Commands() {
		if !available(c.Where, mode) {
			continue
		}
		docs = append(docs, cmdDoc(c))
		rows = append(rows, []string{strings.TrimSpace(c.Name() + " " + c.Args), c.Summary})
	}
	return a.emit(docs, func(w io.Writer) {
		if mode == ModeConfig {
			fmt.Fprintln(w, "Configuration mode (`run <command>` runs an operational command):")
		} else {
			fmt.Fprintln(w, "Operational mode:")
		}
		for _, r := range rows {
			fmt.Fprintf(w, "  %-58s %s\n", r[0], r[1])
		}
		fmt.Fprintln(w, "Tab completes commands, paths and values; ? lists what may follow.")
	})
}

func cmdDoc(c *Command) map[string]any {
	ops := []map[string]string{}
	for _, id := range c.Ops {
		if op, err := api.Lookup(id); err == nil {
			ops = append(ops, map[string]string{"operationId": id, "method": op.Method, "path": op.Path})
		}
	}
	return map[string]any{"command": c.Name(), "args": c.Args, "summary": c.Summary, "rest": ops, "noRest": c.NoREST}
}
