package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"ngfw/cli/internal/api"
	"ngfw/cli/internal/cpath"
	"ngfw/cli/internal/lineedit"
	"ngfw/cli/internal/render"
)

func init() {
	register(&Command{
		Words: []string{"show", "interfaces"}, Args: "[<name>]", Where: inBoth,
		Summary: "Interfaces as retrieved from VPP by the agent, with counters",
		Ops:     []string{"State_interfaces"}, Run: showInterfaces, Complete: completeInterfaceNames,
		Example: "show interfaces loop301",
	})
	register(&Command{
		Words: []string{"show", "ip", "route"}, Args: "[<vrf>]", Where: inBoth,
		Summary: "Routes retrieved from the data plane (connected + static), optionally of one VRF",
		Ops:     []string{"State_routes"}, Run: showRoutes,
	})
	register(&Command{
		Words: []string{"show", "bgp", "summary"}, Where: inBoth,
		Summary: "BGP neighbour summary",
		NoREST:  "no REST endpoint yet: the API has no BGP state route (FRR state arrives with P12) — exits 10",
		Run: func(*App, context.Context, []cpath.Token) error {
			return notImplemented("show bgp summary: the API has no BGP state endpoint yet (P12)")
		},
	})
	register(&Command{
		Words: []string{"show", "ipsec", "sa"}, Where: inBoth,
		Summary: "IPsec security associations",
		NoREST:  "no REST endpoint yet: the API has no IPsec SA state route (P11) — exits 10",
		Run: func(*App, context.Context, []cpath.Token) error {
			return notImplemented("show ipsec sa: the API has no IPsec SA state endpoint yet (P11)")
		},
	})
	register(&Command{
		Words: []string{"show", "system"}, Where: inBoth,
		Summary: "API and agent health, running revision, pending commit, running↔data-plane sync state",
		Ops:     []string{"State_system"}, Run: showSystem,
	})
	register(&Command{
		Words: []string{"show", "configuration"}, Args: "[<path>] [json|text|set]", Where: inBoth,
		Summary: "Running configuration (redacted), whole or at a path",
		Ops:     []string{"Config_running", "Config_runningAt"}, Run: showRunning, Complete: completeShowPath,
		Example: "show configuration interfaces loop301 set",
	})
	register(&Command{
		Words: []string{"show", "configuration", "candidate"}, Args: "[<path>] [json|text|set]", Where: inBoth,
		Summary: "Candidate configuration (what `commit` would apply)",
		Ops:     []string{"Config_candidate", "Config_candidateAt"}, Run: showCandidate, Complete: completeShowPath,
	})
	register(&Command{
		Words: []string{"show", "configuration", "diff"}, Where: inBoth,
		Summary: "Uncommitted changes: candidate vs running, as - / + set lines",
		Ops:     []string{"Config_diff"}, Run: showDiff,
	})
	register(&Command{
		Words: []string{"show", "revisions"}, Args: "[<count>]", Where: inBoth,
		Summary: "Commit history, newest first",
		Ops:     []string{"Config_revisions"}, Run: showRevisions,
	})
	register(&Command{
		Words: []string{"show", "revision"}, Args: "<rev> [json|text|set]", Where: inBoth,
		Summary: "One revision with its (redacted) configuration",
		Ops:     []string{"Config_revision"}, Run: showRevision,
	})
	register(&Command{
		Words: []string{"show", "commit", "pending"}, Where: inBoth,
		Summary: "The confirmed commit waiting for `confirm`, if any",
		Ops:     []string{"Config_pending"}, Run: showPending,
	})
	register(&Command{
		Words: []string{"show", "lock"}, Where: inBoth,
		Summary: "Who holds the candidate (single writer)",
		Ops:     []string{"Config_lock"}, Run: showLock,
	})
	register(&Command{
		Words: []string{"show", "drift"}, Where: inBoth,
		Summary: "Running configuration vs what the agent retrieves from the data plane",
		Ops:     []string{"State_drift"}, Run: showDrift,
	})
	register(&Command{
		Words: []string{"show", "whoami"}, Where: inBoth,
		Summary: "The authenticated user, effective role and credential type",
		Ops:     []string{"Auth_me"}, Run: whoami,
	})
	register(&Command{
		Words: []string{"ping"}, Args: "<host>", Where: inOp,
		Summary: "Ping from the data plane (the API answers 501 until the agent implements actions)",
		Ops:     []string{"Actions_run"}, Run: func(a *App, ctx context.Context, args []cpath.Token) error { return action(a, ctx, "ping", args) },
	})
	register(&Command{
		Words: []string{"traceroute"}, Args: "<host>", Where: inOp,
		Summary: "Traceroute from the data plane (the API answers 501 until the agent implements actions)",
		Ops:     []string{"Actions_run"}, Run: func(a *App, ctx context.Context, args []cpath.Token) error { return action(a, ctx, "traceroute", args) },
	})
	register(&Command{
		Words: []string{"login"}, Args: "[<user>]", Where: inOp, Public: true,
		Summary: "Log in (password prompted without echo); one-shot use stores the 15-min access token in a 0600 session file",
		Ops:     []string{"Auth_login"}, Run: loginCmd,
	})
	register(&Command{
		Words: []string{"logout"}, Where: inOp,
		Summary: "Revoke the refresh token (interactive) and remove the session file",
		Ops:     []string{"Auth_logout"}, Run: logoutCmd,
	})
	register(&Command{
		Words: []string{"api-key", "create"}, Args: "<name> [role admin|operator|readonly] [expires <days>] [file <path>]", Where: inOp,
		Summary: "Create an API key (shown once; with `file` it is written to a new 0600 file instead)",
		Ops:     []string{"Auth_createApiKey"}, Run: apiKeyCreate,
		Complete: func(_ *App, _ context.Context, args []string, _ string) []lineedit.Candidate {
			if len(args) > 0 && args[len(args)-1] == "role" {
				return cands("admin", "operator", "readonly")
			}
			if len(args) >= 1 {
				return cands("role", "expires", "file")
			}
			return []lineedit.Candidate{{Text: "<name>", Help: "a label for the key"}}
		},
	})
	register(&Command{
		Words: []string{"api-key", "list"}, Where: inOp,
		Summary: "Your API keys (never the key itself)",
		Ops:     []string{"Auth_apiKeys"}, Run: apiKeyList,
	})
	register(&Command{
		Words: []string{"api-key", "delete"}, Args: "<id>", Where: inOp,
		Summary: "Revoke an API key",
		Ops:     []string{"Auth_deleteApiKey"}, Run: apiKeyDelete,
	})
	register(&Command{
		Words: []string{"configure"}, Where: inOp,
		Summary: "Enter configuration mode (one-shot: `vrx configure <config command>`)",
		NoREST:  "local: switches the shell mode; the candidate lives in the API",
		Run: func(a *App, _ context.Context, _ []cpath.Token) error {
			a.mode, a.edit = ModeConfig, nil
			if a.interactive {
				fmt.Fprintln(a.Stdout, "Entering configuration mode (changes go to the candidate; `commit` applies them).")
			}
			return nil
		},
	})
	register(&Command{
		Words: []string{"help"}, Args: "[<command>]", Where: inBoth, Public: true,
		Summary: "List commands, or explain one",
		NoREST:  "local",
		Run:     helpCmd,
	})
}

func cands(words ...string) []lineedit.Candidate {
	out := make([]lineedit.Candidate, len(words))
	for i, w := range words {
		out[i] = lineedit.Candidate{Text: w}
	}
	return out
}

// ---- show interfaces ----

type ifaceState struct {
	RetrievedAt string `json:"retrievedAt"`
	CountersAt  string `json:"countersAt"`
	Items       []struct {
		Name     string         `json:"name"`
		Config   map[string]any `json:"config"`
		Counters map[string]any `json:"counters"`
	} `json:"items"`
}

func showInterfaces(a *App, ctx context.Context, args []cpath.Token) error {
	if len(args) > 1 {
		return usagef("show interfaces [<name>]")
	}
	var st ifaceState
	raw, err := a.call(ctx, api.Call{Op: "State_interfaces"}, &st)
	if err != nil {
		return err
	}
	if len(args) == 1 {
		name := args[0].Text
		for _, it := range st.Items {
			if it.Name == name {
				return a.emit(it, func(w io.Writer) {
					fmt.Fprintf(w, "Interface %s (retrieved %s)\n", it.Name, st.RetrievedAt)
					fmt.Fprint(w, indentText(render.Text(it.Config), "  "))
					if it.Counters != nil {
						fmt.Fprintf(w, "Counters (%s)\n", st.CountersAt)
						fmt.Fprint(w, indentText(render.Text(it.Counters), "  "))
					}
				})
			}
		}
		return &ExitErr{Code: ExitNotFound, Err: fmt.Errorf("interface %q is not in the data plane", name)}
	}
	return a.emit(raw, func(w io.Writer) {
		rows := make([][]string, 0, len(st.Items))
		for _, it := range st.Items {
			c := it.Config
			addrs := append(strList(c["ipv4"]), strList(c["ipv6"])...)
			rows = append(rows, []string{it.Name, onOff(c["enabled"]), scalarOr(c["mtu"], "-"), scalarOr(c["vrf"], "default"), strings.Join(addrs, ","), counter(it.Counters, "rx", "packets"), counter(it.Counters, "tx", "packets")})
		}
		table(w, []string{"NAME", "ADMIN", "MTU", "VRF", "ADDRESSES", "RX-PKTS", "TX-PKTS"}, rows)
		if len(rows) == 0 {
			fmt.Fprintln(w, "(the agent retrieved no interfaces of this owner)")
		}
	})
}

func strList(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		out = append(out, fmt.Sprint(x))
	}
	return out
}

func onOff(v any) string {
	switch v {
	case true:
		return "up"
	case false:
		return "down"
	}
	return "-"
}

func scalarOr(v any, def string) string {
	if v == nil {
		return def
	}
	return render.Scalar(v)
}

// counter finds a counter in the agent's per-interface counters (field names vary: rxPackets, rx.packets…).
func counter(c map[string]any, dir, kind string) string {
	if c == nil {
		return "-"
	}
	for k, v := range c {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, dir) && strings.Contains(lk, kind) {
			return render.Scalar(v)
		}
	}
	if sub, ok := c[dir].(map[string]any); ok {
		if v, ok := sub[kind]; ok {
			return render.Scalar(v)
		}
	}
	return "-"
}

func indentText(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n") + "\n"
}

func completeInterfaceNames(a *App, ctx context.Context, args []string, _ string) []lineedit.Candidate {
	if len(args) > 0 {
		return nil
	}
	var st ifaceState
	if _, err := a.call(ctx, api.Call{Op: "State_interfaces"}, &st); err != nil {
		return nil
	}
	out := make([]lineedit.Candidate, 0, len(st.Items))
	for _, it := range st.Items {
		out = append(out, lineedit.Candidate{Text: cpath.Quote(it.Name)})
	}
	return out
}

// ---- show ip route ----

type routePage struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
	Items    []struct {
		VRF      string `json:"vrf"`
		Prefix   string `json:"prefix"`
		Origin   string `json:"origin"`
		NextHops []struct {
			Address   string `json:"address"`
			Interface string `json:"interface"`
		} `json:"nextHops"`
		Distance *int `json:"distance"`
	} `json:"items"`
}

func showRoutes(a *App, ctx context.Context, args []cpath.Token) error {
	if len(args) > 1 {
		return usagef("show ip route [<vrf>]")
	}
	var all routePage
	for page := 1; ; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "pageSize": {"500"}}
		if len(args) == 1 {
			q.Set("vrf", args[0].Text)
		}
		var p routePage
		if _, err := a.call(ctx, api.Call{Op: "State_routes", Query: q}, &p); err != nil {
			return err
		}
		all.Items = append(all.Items, p.Items...)
		all.Total, all.PageSize, all.Page = p.Total, len(all.Items), 1
		if len(p.Items) == 0 || len(all.Items) >= p.Total {
			break
		}
	}
	return a.emit(all, func(w io.Writer) {
		rows := make([][]string, 0, len(all.Items))
		for _, r := range all.Items {
			hops := make([]string, 0, len(r.NextHops))
			for _, h := range r.NextHops {
				s := h.Address
				if h.Interface != "" {
					if s != "" {
						s += " via "
					}
					s += h.Interface
				}
				hops = append(hops, s)
			}
			d := "-"
			if r.Distance != nil {
				d = strconv.Itoa(*r.Distance)
			}
			rows = append(rows, []string{r.VRF, r.Prefix, r.Origin, d, strings.Join(hops, ", ")})
		}
		table(w, []string{"VRF", "PREFIX", "ORIGIN", "DIST", "NEXT-HOPS"}, rows)
		fmt.Fprintf(w, "%d route(s)\n", len(rows))
	})
}

// ---- show system ----

type systemState struct {
	API struct {
		Version   string `json:"version"`
		StartedAt string `json:"startedAt"`
		WSClients int    `json:"wsClients"`
	} `json:"api"`
	Agent           map[string]any `json:"agent"`
	RunningRevision *int           `json:"runningRevision"`
	PendingCommit   map[string]any `json:"pendingCommit"`
	Sync            struct {
		State  string  `json:"state"`
		Reason string  `json:"reason"`
		TxnID  *string `json:"txnId"`
		Since  string  `json:"since"`
	} `json:"sync"`
}

func showSystem(a *App, ctx context.Context, args []cpath.Token) error {
	if len(args) > 0 {
		return usagef("show system takes no arguments")
	}
	var s systemState
	raw, err := a.call(ctx, api.Call{Op: "State_system"}, &s)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		fmt.Fprintf(w, "API        version %s, up since %s, %d stream client(s)\n", s.API.Version, s.API.StartedAt, s.API.WSClients)
		if s.Agent["reachable"] == true {
			keys := make([]string, 0, len(s.Agent))
			for k := range s.Agent {
				if k != "reachable" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			fmt.Fprintln(w, "agent      reachable")
			for _, k := range keys {
				fmt.Fprintf(w, "  %-18s %s\n", k, compact(s.Agent[k]))
			}
		} else {
			fmt.Fprintf(w, "agent      UNREACHABLE: %v\n", s.Agent["error"])
		}
		rev := "none"
		if s.RunningRevision != nil {
			rev = strconv.Itoa(*s.RunningRevision)
		}
		fmt.Fprintf(w, "running    revision %s\n", rev)
		if s.PendingCommit != nil {
			fmt.Fprintf(w, "pending    commit %v waiting for confirm until %v\n", s.PendingCommit["txnId"], s.PendingCommit["deadline"])
		} else {
			fmt.Fprintln(w, "pending    none")
		}
		fmt.Fprintf(w, "sync       %s — %s (since %s)\n", s.Sync.State, s.Sync.Reason, s.Sync.Since)
	})
}

func compact(v any) string {
	switch v.(type) {
	case map[string]any, []any:
		b, _ := json.Marshal(v)
		if len(b) > 100 {
			return string(b[:99]) + "…"
		}
		return string(b)
	}
	return render.Scalar(v)
}

// ---- show configuration ----

// splitFormat removes a trailing json|text|set word.
func splitFormat(args []cpath.Token) ([]cpath.Token, string) {
	if n := len(args); n > 0 && !args[n-1].Quoted {
		switch args[n-1].Text {
		case "json", "text", "set":
			return args[:n-1], args[n-1].Text
		}
	}
	return args, "text"
}

func (a *App) printConfig(segs []string, raw json.RawMessage, format string) error {
	if a.jsonOut || format == "json" {
		if !a.jsonOut {
			var v any
			_ = json.Unmarshal(raw, &v)
			b, _ := json.MarshalIndent(v, "", "  ")
			fmt.Fprintln(a.Stdout, string(b))
			return nil
		}
		return a.emit(raw, nil)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	if format == "set" {
		for _, l := range render.Set(segs, v) {
			fmt.Fprintln(a.Stdout, l)
		}
		return nil
	}
	fmt.Fprint(a.Stdout, render.Text(v))
	return nil
}

func wordsToSegs(base []string, args []cpath.Token) ([]string, error) {
	segs, err := cpath.Resolve(base, cpath.Texts(args))
	if err != nil {
		return nil, usagef("%v", err)
	}
	return segs, nil
}

func showRunning(a *App, ctx context.Context, args []cpath.Token) error {
	args, format := splitFormat(args)
	segs, err := wordsToSegs(nil, args)
	if err != nil {
		return err
	}
	c := api.Call{Op: "Config_running"}
	if len(segs) > 0 {
		c = api.Call{Op: "Config_runningAt", Params: pathParam(segs)}
	}
	raw, err := a.call(ctx, c, nil)
	if err != nil {
		return err
	}
	return a.printConfig(segs, raw, format)
}

func showCandidate(a *App, ctx context.Context, args []cpath.Token) error {
	args, format := splitFormat(args)
	segs, err := wordsToSegs(nil, args)
	if err != nil {
		return err
	}
	return a.showCandidateAt(ctx, segs, format)
}

func (a *App) showCandidateAt(ctx context.Context, segs []string, format string) error {
	c := api.Call{Op: "Config_candidate"}
	if len(segs) > 0 {
		c = api.Call{Op: "Config_candidateAt", Params: pathParam(segs)}
	}
	raw, err := a.call(ctx, c, nil)
	if err != nil {
		return err
	}
	return a.printConfig(segs, raw, format)
}

type diffOut struct {
	BaseRevision *int            `json:"baseRevision"`
	Changes      []render.Change `json:"changes"`
}

func showDiff(a *App, ctx context.Context, args []cpath.Token) error {
	if len(args) > 0 {
		return usagef("show configuration diff takes no arguments")
	}
	var d diffOut
	raw, err := a.call(ctx, api.Call{Op: "Config_diff"}, &d)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		base := "none"
		if d.BaseRevision != nil {
			base = strconv.Itoa(*d.BaseRevision)
		}
		if len(d.Changes) == 0 {
			fmt.Fprintf(w, "no uncommitted changes (running revision %s)\n", base)
			return
		}
		fmt.Fprintf(w, "candidate vs running revision %s: %d change(s)\n", base, len(d.Changes))
		for _, l := range render.Diff(d.Changes) {
			fmt.Fprintln(w, l)
		}
	})
}

// ---- revisions ----

type revisionMeta struct {
	ID        int     `json:"id"`
	CreatedAt string  `json:"createdAt"`
	Author    *string `json:"author"`
	Comment   string  `json:"comment"`
	ParentID  *int    `json:"parentId"`
	Hash      string  `json:"hash"`
	TxnID     *string `json:"txnId"`
	Kind      string  `json:"kind"`
}

func showRevisions(a *App, ctx context.Context, args []cpath.Token) error {
	limit := "20"
	if len(args) == 1 {
		if n, err := strconv.Atoi(args[0].Text); err != nil || n < 1 || n > 500 {
			return usagef("count must be 1…500")
		}
		limit = args[0].Text
	} else if len(args) > 1 {
		return usagef("show revisions [<count>]")
	}
	var out struct {
		Items []revisionMeta `json:"items"`
		Total int            `json:"total"`
	}
	raw, err := a.call(ctx, api.Call{Op: "Config_revisions", Query: url.Values{"limit": {limit}}}, &out)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		rows := make([][]string, 0, len(out.Items))
		for _, r := range out.Items {
			author := "-"
			if r.Author != nil {
				author = *r.Author
			}
			rows = append(rows, []string{strconv.Itoa(r.ID), shortTime(r.CreatedAt), author, r.Kind, short(r.Hash, 12), r.Comment})
		}
		table(w, []string{"REV", "DATE", "AUTHOR", "KIND", "HASH", "COMMENT"}, rows)
		fmt.Fprintf(w, "%d of %d revision(s)\n", len(rows), out.Total)
	})
}

func shortTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04:05Z")
}

func short(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func parseRev(t cpath.Token) (string, error) {
	n, err := strconv.Atoi(t.Text)
	if err != nil || n < 1 {
		return "", usagef("revision must be a positive integer, got %q", t.Text)
	}
	return strconv.Itoa(n), nil
}

func showRevision(a *App, ctx context.Context, args []cpath.Token) error {
	args, format := splitFormat(args)
	if len(args) != 1 {
		return usagef("show revision <rev> [json|text|set]")
	}
	rev, err := parseRev(args[0])
	if err != nil {
		return err
	}
	var out struct {
		revisionMeta
		Payload json.RawMessage `json:"payload"`
	}
	raw, err := a.call(ctx, api.Call{Op: "Config_revision", Params: map[string]string{"rev": rev}}, &out)
	if err != nil {
		return err
	}
	if a.jsonOut {
		return a.emit(raw, nil)
	}
	author := "-"
	if out.Author != nil {
		author = *out.Author
	}
	fmt.Fprintf(a.Stdout, "# revision %d · %s · %s · %s · %s\n# comment: %s\n", out.ID, shortTime(out.CreatedAt), author, out.Kind, short(out.Hash, 12), out.Comment)
	return a.printConfig(nil, out.Payload, format)
}

func showPending(a *App, ctx context.Context, _ []cpath.Token) error {
	var out struct {
		Pending map[string]any `json:"pending"`
	}
	raw, err := a.call(ctx, api.Call{Op: "Config_pending"}, &out)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		if out.Pending == nil {
			fmt.Fprintln(w, "no commit is waiting for confirmation")
			return
		}
		fmt.Fprintf(w, "commit %v (%v) waiting for `confirm` until %v — comment: %v\n", out.Pending["txnId"], out.Pending["kind"], out.Pending["deadline"], out.Pending["comment"])
	})
}

func showLock(a *App, ctx context.Context, _ []cpath.Token) error {
	var out map[string]any
	raw, err := a.call(ctx, api.Call{Op: "Config_lock"}, &out)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		if out["locked"] != true {
			fmt.Fprintln(w, "candidate not locked")
			return
		}
		fmt.Fprintf(w, "candidate locked by %v since %v (last activity %v, expires %v)\n", out["owner"], out["lockedAt"], out["lastActivity"], out["expiresAt"])
	})
}

func showDrift(a *App, ctx context.Context, _ []cpath.Token) error {
	var out struct {
		Subsystems []string        `json:"subsystems"`
		Changes    []render.Change `json:"changes"`
		Ignored    []struct {
			Pointer string `json:"pointer"`
			Rule    string `json:"rule"`
		} `json:"ignored"`
	}
	raw, err := a.call(ctx, api.Call{Op: "State_drift"}, &out)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		fmt.Fprintf(w, "compared subsystems: %s\n", strings.Join(out.Subsystems, ", "))
		if len(out.Changes) == 0 {
			fmt.Fprintln(w, "no drift: the data plane matches running")
		} else {
			fmt.Fprintf(w, "%d difference(s) (- running, + data plane):\n", len(out.Changes))
			for _, l := range render.Diff(out.Changes) {
				fmt.Fprintln(w, l)
			}
		}
		if len(out.Ignored) > 0 {
			fmt.Fprintf(w, "%d pointer(s) not compared (not managed by the agent)\n", len(out.Ignored))
		}
	})
}

func whoami(a *App, ctx context.Context, _ []cpath.Token) error {
	var me map[string]any
	raw, err := a.call(ctx, api.Call{Op: "Auth_me"}, &me)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		fmt.Fprintf(w, "%v (role %v, effective %v, via %v)\n", me["username"], me["role"], me["effectiveRole"], me["via"])
	})
}

func action(a *App, ctx context.Context, name string, args []cpath.Token) error {
	if len(args) != 1 {
		return usagef("%s <host>", name)
	}
	raw, err := a.call(ctx, api.Call{Op: "Actions_run", Params: map[string]string{"action": name}}, nil)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) { fmt.Fprintln(w, string(raw)) })
}

// ---- auth ----

func loginCmd(a *App, ctx context.Context, args []cpath.Token) error {
	if len(args) > 1 {
		return usagef("login [<user>]")
	}
	user := ""
	if len(args) == 1 {
		user = args[0].Text
	}
	pw := ""
	if a.passwordFile != "" {
		var err error
		if pw, err = readSecretFile(a.passwordFile); err != nil {
			return usagef("password file: %v", err)
		}
	}
	a.client.Cred = nil
	if err := a.login(ctx, user, pw, !a.interactive); err != nil {
		return err
	}
	return a.emit(map[string]any{"user": a.username, "loggedIn": true}, func(w io.Writer) {
		if a.interactive {
			fmt.Fprintf(w, "logged in as %s\n", a.username)
		} else {
			fmt.Fprintf(w, "logged in as %s (session valid 15 min, %s)\n", a.username, a.sessionPath())
		}
	})
}

func logoutCmd(a *App, ctx context.Context, _ []cpath.Token) error {
	if a.refreshCookie != "" {
		_, _ = a.call(ctx, api.Call{Op: "Auth_logout", Cookie: a.refreshCookie}, nil)
		a.refreshCookie = ""
	}
	removed := false
	if !a.noSession {
		if err := os.Remove(a.sessionPath()); err == nil {
			removed = true
		}
	}
	if a.credSource == "login" || a.credSource == "session" {
		a.client.Cred = nil
	}
	return a.emit(map[string]any{"loggedOut": true, "sessionFileRemoved": removed}, func(w io.Writer) {
		fmt.Fprintln(w, "logged out")
	})
}

func apiKeyCreate(a *App, ctx context.Context, args []cpath.Token) error {
	if len(args) < 1 {
		return usagef("api-key create <name> [role …] [expires <days>] [file <path>]")
	}
	body := map[string]any{"name": args[0].Text}
	file := ""
	for i := 1; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return usagef("%s needs a value", args[i].Text)
		}
		v := args[i+1].Text
		switch args[i].Text {
		case "role":
			body["role"] = v
		case "expires":
			n, err := strconv.Atoi(v)
			if err != nil {
				return usagef("expires <days>: %q is not a number", v)
			}
			body["expiresInDays"] = n
		case "file":
			file = v
		default:
			return usagef("unknown option %q (role, expires, file)", args[i].Text)
		}
	}
	var out struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Role      string  `json:"role"`
		ExpiresAt *string `json:"expiresAt"`
		Key       string  `json:"key"`
	}
	if file != "" {
		// create the file first (O_EXCL, 0600) so a key is never created without a place to put it
		f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // user-chosen path
		if err != nil {
			return usagef("key file: %v", err)
		}
		if _, err := a.call(ctx, api.Call{Op: "Auth_createApiKey", Body: body}, &out); err != nil {
			_ = f.Close()
			_ = os.Remove(file)
			return err
		}
		_, werr := f.WriteString(out.Key + "\n")
		cerr := f.Close()
		if werr != nil || cerr != nil {
			return fmt.Errorf("writing %s failed; delete key %s with `api-key delete %s`", file, out.ID, out.ID)
		}
		out.Key = ""
		return a.emit(map[string]any{"id": out.ID, "name": out.Name, "role": out.Role, "expiresAt": out.ExpiresAt, "file": file}, func(w io.Writer) {
			fmt.Fprintf(w, "API key %s (%s, role %s) written to %s (mode 0600)\n", out.ID, out.Name, out.Role, file)
		})
	}
	raw, err := a.call(ctx, api.Call{Op: "Auth_createApiKey", Body: body}, &out)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		fmt.Fprintf(w, "API key %s (%s, role %s). It is shown once — store it now:\n%s\n", out.ID, out.Name, out.Role, out.Key)
	})
}

func apiKeyList(a *App, ctx context.Context, _ []cpath.Token) error {
	var out []map[string]any
	raw, err := a.call(ctx, api.Call{Op: "Auth_apiKeys"}, &out)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		rows := make([][]string, 0, len(out))
		for _, k := range out {
			rows = append(rows, []string{fmt.Sprint(k["id"]), fmt.Sprint(k["name"]), fmt.Sprint(k["role"]), fmt.Sprint(k["expiresAt"]), fmt.Sprint(k["lastUsedAt"])})
		}
		table(w, []string{"ID", "NAME", "ROLE", "EXPIRES", "LAST-USED"}, rows)
	})
}

func apiKeyDelete(a *App, ctx context.Context, args []cpath.Token) error {
	if len(args) != 1 {
		return usagef("api-key delete <id>")
	}
	if _, err := a.call(ctx, api.Call{Op: "Auth_deleteApiKey", Params: map[string]string{"id": args[0].Text}}, nil); err != nil {
		return err
	}
	return a.emit(map[string]any{"deleted": args[0].Text}, func(w io.Writer) { fmt.Fprintf(w, "API key %s deleted\n", args[0].Text) })
}
