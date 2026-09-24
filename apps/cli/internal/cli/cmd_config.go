package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"ngfw/cli/internal/api"
	"ngfw/cli/internal/cpath"
	"ngfw/cli/internal/jschema"
	"ngfw/cli/internal/lineedit"
)

func init() {
	register(&Command{
		Words: []string{"set"}, Args: "<path> <value>", Where: inBoth,
		Summary: "Set the value at a path in the candidate (value checked against the schema first); on a list of scalars it appends",
		Ops:     []string{"Config_putAt", "Config_candidateAt"}, Run: setCmd, Complete: completeSet,
		Example: "set interfaces loop301 mtu 9000",
	})
	register(&Command{
		Words: []string{"merge"}, Args: "<path> <json-object>", Where: inBoth,
		Summary: "RFC 7386 merge patch at a path (null deletes a member); the merged result is schema-checked first",
		Ops:     []string{"Config_patchAt", "Config_patchRoot", "Config_candidateAt", "Config_candidate"}, Run: mergeCmd, Complete: completePath,
		Example: `merge interfaces loop301 {"enabled":true,"description":"lab"}`,
	})
	register(&Command{
		Words: []string{"delete"}, Args: "<path> [<list-value>]", Where: inBoth,
		Summary: "Remove the node at a path from the candidate (with a value: remove that item of a list of scalars)",
		Ops:     []string{"Config_deleteAt", "Config_candidateAt"}, Run: deleteCmd, Complete: completeDelete,
		Example: "delete interfaces loop301 ipv4 10.3.1.1/24",
	})
	register(&Command{
		Words: []string{"edit"}, Args: "<path>", Where: inConfig,
		Summary: "Move the edit level (later paths are relative to it; a path starting with / is absolute)",
		NoREST:  "local (the path is checked against the schema)", Run: editCmd, Complete: completeContainers,
	})
	register(&Command{
		Words: []string{"up"}, Where: inConfig, Summary: "Move the edit level up one element",
		NoREST: "local", Run: func(_ context.Context, a *App, _ []cpath.Token) error {
			if len(a.edit) > 0 {
				a.edit = a.edit[:len(a.edit)-1]
			}
			return nil
		},
	})
	register(&Command{
		Words: []string{"top"}, Where: inConfig, Summary: "Move the edit level to the top",
		NoREST: "local", Run: func(_ context.Context, a *App, _ []cpath.Token) error { a.edit = nil; return nil },
	})
	register(&Command{
		Words: []string{"show"}, Args: "[<path>] [json|text|set]", Where: inConfig,
		Summary: "Candidate configuration at the edit level or a path",
		Ops:     []string{"Config_candidate", "Config_candidateAt"}, Run: configShow, Complete: completeShowPath,
	})
	register(&Command{
		Words: []string{"compare"}, Where: inConfig,
		Summary: "Uncommitted changes (same as `show configuration diff`)",
		Ops:     []string{"Config_diff"}, Run: showDiff,
	})
	register(&Command{
		Words: []string{"validate"}, Where: inBoth,
		Summary: "Three-tier validation of the candidate (schema → semantic → agent dry-run); nothing is applied",
		Ops:     []string{"Config_validate"}, Run: validateCmd,
	})
	register(&Command{
		Words: []string{"commit"}, Args: "[confirm <sec>] [comment <text>]", Where: inBoth,
		Summary: "Apply the candidate atomically; with `confirm <sec>` the change reverts unless `confirm` follows in time",
		Ops:     []string{"Config_commit"}, Run: commitCmd,
		Complete: func(_ context.Context, _ *App, args []string, _ string) []lineedit.Candidate {
			return commitOptions(args)
		},
		Example: `commit confirm 60 comment "mtu 9000 on loop301"`,
	})
	register(&Command{
		Words: []string{"confirm"}, Where: inBoth,
		Summary: "Confirm the pending commit (cancels the auto-revert and records the revision)",
		Ops:     []string{"Config_confirm"}, Run: confirmCmd,
	})
	register(&Command{
		Words: []string{"rollback"}, Args: "<rev> [confirm <sec>] [comment <text>]", Where: inBoth,
		Summary: "Apply an old revision as a new revision",
		Ops:     []string{"Config_rollback"}, Run: rollbackCmd,
		Complete: func(ctx context.Context, a *App, args []string, _ string) []lineedit.Candidate {
			if len(args) == 0 {
				return a.revisionCandidates(ctx)
			}
			return commitOptions(args[1:])
		},
	})
	register(&Command{
		Words: []string{"discard"}, Where: inBoth,
		Summary: "Drop the candidate and release the lock",
		Ops:     []string{"Config_discard"}, Run: discardCmd,
	})
	register(&Command{
		Words: []string{"exit"}, Where: inConfig,
		Summary: "Leave configuration mode (the candidate stays until commit/discard)",
		Ops:     []string{"Config_diff"}, Run: exitConfig,
	})
}

func commitOptions(args []string) []lineedit.Candidate {
	if n := len(args); n > 0 {
		switch args[n-1] {
		case "confirm":
			return []lineedit.Candidate{{Text: "<sec>", Help: "1…3600 seconds until the automatic revert"}}
		case "comment":
			return []lineedit.Candidate{{Text: "<text>", Help: "quoted comment stored with the revision"}}
		}
	}
	return []lineedit.Candidate{{Text: "confirm", Help: "auto-revert unless confirmed within <sec>"}, {Text: "comment", Help: "comment for the revision"}}
}

// target resolves path words at the edit level and walks the schema there.
func (a *App) target(ctx context.Context, words []cpath.Token) ([]string, *jschema.Node, error) {
	segs, err := wordsToSegs(a.edit, words)
	if err != nil {
		return nil, nil, err
	}
	d, err := a.Schema(ctx)
	if err != nil {
		return nil, nil, err
	}
	n, err := d.Root().Walk(segs)
	if err != nil {
		return nil, nil, usagef("path %s: %v", cpath.Pointer(segs), err)
	}
	return segs, n, nil
}

// candidateAt reads the candidate node (nil, false when absent).
func (a *App) candidateAt(ctx context.Context, segs []string) (any, bool, error) {
	var v any
	c := api.Call{Op: "Config_candidate"}
	if len(segs) > 0 {
		c = api.Call{Op: "Config_candidateAt", Params: pathParam(segs)}
	}
	_, err := a.call(ctx, c, &v)
	if err != nil {
		var ae *api.Error
		if errors.As(err, &ae) && ae.Status == 404 {
			return nil, false, nil
		}
		return nil, false, err
	}
	return v, true, nil
}

type editOut struct {
	Pointer                   string `json:"pointer"`
	Before                    any    `json:"before"`
	After                     any    `json:"after"`
	DiscardedStaleCandidateOf string `json:"discardedStaleCandidateOf"`
}

func (a *App) edited(raw json.RawMessage) error {
	var e editOut
	_ = json.Unmarshal(raw, &e)
	a.candCache = nil
	return a.emit(raw, func(w io.Writer) {
		if e.DiscardedStaleCandidateOf != "" {
			fmt.Fprintf(w, "note: took over a stale lock; the candidate of %s had privileged changes and was discarded\n", e.DiscardedStaleCandidateOf)
		}
	})
}

func setCmd(ctx context.Context, a *App, args []cpath.Token) error {
	if len(args) < 2 {
		return usagef("set <path> <value>")
	}
	segs, node, err := a.target(ctx, args[:len(args)-1])
	if err != nil {
		return err
	}
	if len(segs) == 0 {
		return usagef("set needs a path below the top (use `merge / {…}` for the whole document)")
	}
	val := args[len(args)-1]
	if node.IsLeafList() && !strings.HasPrefix(val.Text, "[") {
		item, err := node.Items().Coerce(val.Text, val.Quoted)
		if err != nil {
			return usagef("%s: %v", cpath.Words(segs), err)
		}
		cur, _, err := a.candidateAt(ctx, segs)
		if err != nil {
			return err
		}
		list, _ := cur.([]any)
		for _, x := range list {
			if fmt.Sprint(x) == fmt.Sprint(item) {
				return a.emit(map[string]any{"pointer": cpath.Pointer(segs), "unchanged": true}, func(w io.Writer) {
					fmt.Fprintf(w, "%s already contains %v\n", cpath.Words(segs), item)
				})
			}
		}
		next := append(append([]any{}, list...), item)
		if issues := node.Validate(next, ""); len(issues) > 0 {
			return usagef("%s: %v", cpath.Words(segs), jschema.IssuesError(issues))
		}
		raw, err := a.call(ctx, api.Call{Op: "Config_putAt", Params: pathParam(segs), Body: next}, nil)
		if err != nil {
			return err
		}
		return a.edited(raw)
	}
	v, err := node.Coerce(val.Text, val.Quoted)
	if err != nil {
		return usagef("%s: %v", cpath.Words(segs), err)
	}
	if v == nil {
		return usagef("%s: use `delete` to remove a value", cpath.Words(segs))
	}
	raw, err := a.call(ctx, api.Call{Op: "Config_putAt", Params: pathParam(segs), Body: v}, nil)
	if err != nil {
		return err
	}
	return a.edited(raw)
}

// mergePatch applies an RFC 7386 merge patch (client-side preview for validation; the API applies it for real).
func mergePatch(target, patch any) any {
	p, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	t, ok := target.(map[string]any)
	if !ok {
		t = map[string]any{}
	}
	out := make(map[string]any, len(t))
	for k, v := range t {
		out[k] = v
	}
	for k, v := range p {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = mergePatch(out[k], v)
		}
	}
	return out
}

func mergeCmd(ctx context.Context, a *App, args []cpath.Token) error {
	if len(args) < 1 {
		return usagef("merge <path> <json-object>")
	}
	segs, node, err := a.target(ctx, args[:len(args)-1])
	if err != nil {
		return err
	}
	var patch any
	if err := json.Unmarshal([]byte(args[len(args)-1].Text), &patch); err != nil {
		return usagef("merge: the last argument must be JSON: %v", err)
	}
	cur, _, err := a.candidateAt(ctx, segs)
	if err != nil {
		return err
	}
	if issues := node.Validate(mergePatch(cur, patch), ""); len(issues) > 0 {
		return usagef("%s: %v", pathLabel(segs), jschema.IssuesError(issues))
	}
	c := api.Call{Op: "Config_patchRoot", Body: patch}
	if len(segs) > 0 {
		c = api.Call{Op: "Config_patchAt", Params: pathParam(segs), Body: patch}
	}
	if patch == nil {
		// a JSON null body is "delete" in RFC 7386; send it as such
		c.Body = json.RawMessage("null")
	}
	raw, err := a.call(ctx, c, nil)
	if err != nil {
		return err
	}
	return a.edited(raw)
}

func pathLabel(segs []string) string {
	if len(segs) == 0 {
		return "(top)"
	}
	return cpath.Words(segs)
}

func deleteCmd(ctx context.Context, a *App, args []cpath.Token) error {
	if len(args) < 1 && len(a.edit) == 0 {
		return usagef("delete <path> [<list-value>]")
	}
	segs, _, err := a.target(ctx, args)
	if err == nil {
		if len(segs) == 0 {
			return usagef("cannot delete the whole configuration")
		}
		raw, err := a.call(ctx, api.Call{Op: "Config_deleteAt", Params: pathParam(segs)}, nil)
		if err != nil {
			return err
		}
		return a.edited(raw)
	}
	// `delete <leaf-list path> <value>`: remove that item
	if len(args) < 2 {
		return err
	}
	lsegs, lnode, lerr := a.target(ctx, args[:len(args)-1])
	if lerr != nil || !lnode.IsLeafList() {
		return err
	}
	val := args[len(args)-1]
	item, cerr := lnode.Items().Coerce(val.Text, val.Quoted)
	if cerr != nil {
		item = val.Text
	}
	cur, _, gerr := a.candidateAt(ctx, lsegs)
	if gerr != nil {
		return gerr
	}
	list, _ := cur.([]any)
	for i, x := range list {
		if fmt.Sprint(x) == fmt.Sprint(item) {
			raw, err := a.call(ctx, api.Call{Op: "Config_deleteAt", Params: pathParam(append(lsegs, strconv.Itoa(i)))}, nil)
			if err != nil {
				return err
			}
			return a.edited(raw)
		}
	}
	return &ExitErr{Code: ExitNotFound, Err: fmt.Errorf("%s does not contain %v", cpath.Words(lsegs), item)}
}

func editCmd(ctx context.Context, a *App, args []cpath.Token) error {
	segs, node, err := a.target(ctx, args)
	if err != nil {
		return err
	}
	if !node.IsContainer() {
		return usagef("%s is a value, not a container: use `set`", cpath.Words(segs))
	}
	a.edit = segs
	return nil
}

func configShow(ctx context.Context, a *App, args []cpath.Token) error {
	args, format := splitFormat(args)
	segs, err := wordsToSegs(a.edit, args)
	if err != nil {
		return err
	}
	return a.showCandidateAt(ctx, segs, format)
}

type commitOut struct {
	Status          string        `json:"status"`
	TxnID           string        `json:"txnId"`
	Revision        *revisionMeta `json:"revision"`
	ConfirmDeadline string        `json:"confirmDeadline"`
	Results         []struct {
		Key     string `json:"key"`
		Op      string `json:"op"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"results"`
	Summary    map[string]float64 `json:"summary"`
	Warnings   []warning          `json:"warnings"`
	NotApplied []string           `json:"notApplied"`
	Sync       *struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	} `json:"sync"`
}

// commitQuery parses `[confirm <sec>] [comment <text>]` (also Junos' `confirmed <sec>`).
func commitQuery(args []cpath.Token) (url.Values, error) {
	q := url.Values{}
	for i := 0; i < len(args); i++ {
		k := args[i].Text
		if i+1 >= len(args) {
			return nil, usagef("%s needs a value", k)
		}
		v := args[i+1].Text
		i++
		switch k {
		case "confirm", "confirmed":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 3600 {
				return nil, usagef("confirm <sec>: 1…3600, got %q", v)
			}
			q.Set("confirm", strconv.Itoa(n))
		case "comment":
			if len(v) > 1024 {
				return nil, usagef("comment: at most 1024 characters")
			}
			q.Set("comment", v)
		default:
			return nil, usagef("unknown option %q (confirm <sec>, comment <text>)", k)
		}
	}
	return q, nil
}

func commitCmd(ctx context.Context, a *App, args []cpath.Token) error {
	q, err := commitQuery(args)
	if err != nil {
		return err
	}
	return a.commitLike(ctx, api.Call{Op: "Config_commit", Query: q})
}

func rollbackCmd(ctx context.Context, a *App, args []cpath.Token) error {
	if len(args) < 1 {
		return usagef("rollback <rev> [confirm <sec>] [comment <text>]")
	}
	rev, err := parseRev(args[0])
	if err != nil {
		return err
	}
	q, err := commitQuery(args[1:])
	if err != nil {
		return err
	}
	return a.commitLike(ctx, api.Call{Op: "Config_rollback", Params: map[string]string{"rev": rev}, Query: q})
}

func confirmCmd(ctx context.Context, a *App, _ []cpath.Token) error {
	return a.commitLike(ctx, api.Call{Op: "Config_confirm"})
}

func (a *App) commitLike(ctx context.Context, c api.Call) error {
	var out commitOut
	raw, err := a.call(ctx, c, &out)
	if err != nil {
		return err
	}
	a.candCache = nil
	return a.emit(raw, func(w io.Writer) { printCommit(w, &out) })
}

func printCommit(w io.Writer, out *commitOut) {
	switch out.Status {
	case "pending":
		fmt.Fprintf(w, "commit %s applied, NOT confirmed: it reverts automatically at %s unless you run `confirm`\n", out.TxnID, out.ConfirmDeadline)
	case "unchanged":
		_, _ = fmt.Fprintln(w, "nothing to commit: the candidate equals running")
	case "confirmed":
		fmt.Fprintf(w, "commit %s confirmed", out.TxnID)
	default:
		fmt.Fprintf(w, "commit %s: %s", out.TxnID, out.Status)
	}
	if out.Revision != nil {
		fmt.Fprintf(w, " — revision %d", out.Revision.ID)
		if out.Revision.Comment != "" {
			fmt.Fprintf(w, " (%s)", out.Revision.Comment)
		}
	}
	if out.Status != "pending" && out.Status != "unchanged" {
		_, _ = fmt.Fprintln(w)
	}
	if len(out.Summary) > 0 {
		parts := make([]string, 0, len(out.Summary))
		for _, k := range sortedKeys(out.Summary) {
			parts = append(parts, fmt.Sprintf("%s %v", k, out.Summary[k]))
		}
		fmt.Fprintf(w, "  objects: %s\n", strings.Join(parts, ", "))
	}
	for _, r := range out.Results {
		line := fmt.Sprintf("  %s %s: %s", r.Op, r.Key, r.Code)
		if r.Message != "" {
			line += " — " + r.Message
		}
		fmt.Fprintln(w, line)
	}
	if len(out.NotApplied) > 0 {
		fmt.Fprintf(w, "  not applied (agent does not implement): %s — stored in running, enforced when supported\n", strings.Join(out.NotApplied, ", "))
	}
	printWarnings(w, out.Warnings)
	if out.Sync != nil {
		fmt.Fprintf(w, "  sync: %s", out.Sync.State)
		if out.Sync.Reason != "" {
			fmt.Fprintf(w, " (%s)", out.Sync.Reason)
		}
		fmt.Fprintln(w)
	}
}

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func validateCmd(ctx context.Context, a *App, _ []cpath.Token) error {
	var out struct {
		OK       bool      `json:"ok"`
		Warnings []warning `json:"warnings"`
		Plan     []struct {
			Key       string `json:"key"`
			Op        string `json:"op"`
			Subsystem string `json:"subsystem"`
		} `json:"plan"`
		NotApplied []string `json:"notApplied"`
	}
	raw, err := a.call(ctx, api.Call{Op: "Config_validate"}, &out)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		fmt.Fprintf(w, "validation ok: %d planned operation(s)\n", len(out.Plan))
		for _, p := range out.Plan {
			fmt.Fprintf(w, "  %s %s (%s)\n", p.Op, p.Key, p.Subsystem)
		}
		if len(out.NotApplied) > 0 {
			fmt.Fprintf(w, "  not applied (agent does not implement): %s\n", strings.Join(out.NotApplied, ", "))
		}
		printWarnings(w, out.Warnings)
	})
}

type warning = struct {
	Pointer string `json:"pointer"`
	Message string `json:"message"`
}

// printWarnings prints real warnings one per line and folds the agent's coverage notes ("… is not implemented by
// this agent build", "… is not applied") into one line of pointers — stored in running, not enforced (D-P06-15/16).
func printWarnings(w io.Writer, ws []warning) {
	var notEnforced []string
	for _, x := range ws {
		m := strings.ToLower(x.Message)
		if strings.Contains(m, "not implemented by this agent") || strings.Contains(m, "not applied") || strings.Contains(m, "not by this agent build") {
			notEnforced = append(notEnforced, x.Pointer)
			continue
		}
		fmt.Fprintf(w, "  warning %s: %s\n", x.Pointer, x.Message)
	}
	if len(notEnforced) > 0 {
		fmt.Fprintf(w, "  stored but not enforced by this agent build (%d): %s  (details: --json)\n", len(notEnforced), strings.Join(notEnforced, " "))
	}
}

func discardCmd(ctx context.Context, a *App, _ []cpath.Token) error {
	var out struct {
		Discarded bool `json:"discarded"`
	}
	raw, err := a.call(ctx, api.Call{Op: "Config_discard"}, &out)
	if err != nil {
		return err
	}
	a.candCache = nil
	return a.emit(raw, func(w io.Writer) {
		if out.Discarded {
			fmt.Fprintln(w, "candidate discarded; lock released")
		} else {
			fmt.Fprintln(w, "nothing to discard; lock released")
		}
	})
}

func exitConfig(ctx context.Context, a *App, _ []cpath.Token) error {
	if len(a.edit) > 0 {
		a.edit = a.edit[:0]
		return nil
	}
	a.mode = ModeOperational
	var d diffOut
	if _, err := a.call(ctx, api.Call{Op: "Config_diff"}, &d); err == nil && len(d.Changes) > 0 {
		fmt.Fprintf(a.Stdout, "note: the candidate keeps %d uncommitted change(s) — `commit` or `discard` them\n", len(d.Changes))
	}
	return nil
}
