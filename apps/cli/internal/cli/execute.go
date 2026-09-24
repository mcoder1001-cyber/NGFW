package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"ngfw/cli/internal/api"
	"ngfw/cli/internal/cpath"
)

// execute runs one command line. oneShot: `vrx <command>` from the shell (configuration verbs are allowed without
// entering configuration mode, and `configure <command>` runs one configuration-mode command).
func (a *App) execute(ctx context.Context, toks []cpath.Token, oneShot bool) error {
	if len(toks) == 0 {
		return nil
	}
	mode := a.mode
	if !toks[0].Quoted && toks[0].Text == "run" && mode == ModeConfig {
		mode, toks = ModeOperational, toks[1:]
		saved := a.mode
		a.mode = ModeOperational
		defer func() { a.mode = saved }()
	}
	if oneShot && len(toks) > 1 && !toks[0].Quoted && toks[0].Text == "configure" {
		a.mode, mode, toks = ModeConfig, ModeConfig, toks[1:]
	}
	cmd, n, words := match(mode, toks)
	if cmd == nil {
		bad := ""
		if n < len(toks) {
			bad = toks[n].Text
		}
		prefix := cpath.Words(cpath.Texts(toks[:n]))
		if prefix != "" {
			prefix += " "
		}
		if bad == "" {
			return usagef("incomplete command %q; expected: %s", strings.TrimSpace(prefix), strings.Join(words, ", "))
		}
		return usagef("unknown command %q; expected: %s", prefix+bad, strings.Join(words, ", "))
	}
	if len(cmd.Ops) > 0 && !cmd.Public {
		if err := a.ensureAuth(ctx); err != nil {
			return err
		}
	}
	return cmd.Run(ctx, a, toks[n:])
}

// ---- output helpers ----

// emit prints v: --json → the JSON document, else the text renderer.
func (a *App) emit(v any, text func(w io.Writer)) error {
	if a.jsonOut {
		var b []byte
		var err error
		switch x := v.(type) {
		case json.RawMessage:
			b = x
		case []byte:
			b = x
		default:
			if b, err = json.Marshal(v); err != nil {
				return err
			}
		}
		if len(b) == 0 {
			b = []byte("{}")
		}
		_, err = fmt.Fprintln(a.Stdout, string(b))
		return err
	}
	if text != nil {
		text(a.Stdout)
	}
	return nil
}

// call runs an operation and decodes the answer into out; raw is the undecoded body (for --json passthrough).
func (a *App) call(ctx context.Context, c api.Call, out any) (json.RawMessage, error) {
	r, err := a.client.JSON(ctx, c, out)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(r.Body), nil
}

func table(w io.Writer, header []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	_ = tw.Flush()
}

func pointerWords(p string) string {
	segs, err := cpath.ParsePointer(p)
	if err != nil || len(segs) == 0 {
		return "(top)"
	}
	return cpath.Words(segs)
}

// pathParam is the `path` parameter of the config routes: the pointer without its leading "/", each segment
// pointer-escaped (the client percent-encodes them).
func pathParam(segs []string) map[string]string {
	esc := make([]string, len(segs))
	for i, s := range segs {
		esc[i] = cpath.EscapeSegment(s)
	}
	return map[string]string{"path": strings.Join(esc, "/")}
}
