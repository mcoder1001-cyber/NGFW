package strongswan

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ngfw/agent/internal/renderers"
)

// runChecker loads a staged copy of files into the scratch charon with
// `swanctl --load-all --noprompt --file <staged swanctl.conf> --uri unix://<scratch socket>`
// (fixed argv; the rendered content travels through the staged files only). The staged
// vrx.conf has every start_action rewritten to none so the scratch charon never initiates or
// installs trap policies; everything else is exactly what Apply would load.
func (r *Renderer) runChecker(ctx context.Context, files renderers.Files) error {
	if r.checker.Runner == nil || !pathRe.MatchString(r.checker.ViciSocket) {
		return fmt.Errorf("strongswan: checker needs a runner and an absolute scratch VICI socket path")
	}
	check := make(renderers.Files, len(files))
	for p, f := range files {
		check[p] = f
	}
	conns, err := ParseSettings("vrx.conf", files[r.paths.ConnsFile()].Content)
	if err != nil {
		return err
	}
	if cs := conns.Sub("connections"); cs != nil {
		for _, c := range cs.Sections() {
			if children := c.Section.Sub("children"); children != nil {
				for _, ch := range children.Sections() {
					for _, it := range ch.Section.Items {
						if it.Kind == KindKey && it.Name == "start_action" {
							it.Value = "none"
						}
					}
				}
			}
		}
	}
	staged, err := conns.Serialize()
	if err != nil {
		return err
	}
	f := check[r.paths.ConnsFile()]
	f.Content = staged
	check[r.paths.ConnsFile()] = f
	// The packaged top-level swanctl.conf: a constant, never user data.
	check[r.paths.SwanctlConf()] = renderers.File{Mode: 0o600, Content: []byte("include conf.d/vrx.conf\ninclude conf.d/vrx-secrets.conf\n")}
	st, err := renderers.Stage(check)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	// swanctl resolves x509/ etc. relative to its directory; make the staged one complete.
	for _, d := range []string{"x509", "x509ca", "pubkey", "private"} {
		if err := os.MkdirAll(filepath.Join(st.Path(r.paths.SwanctlDir), d), 0o700); err != nil {
			return fmt.Errorf("strongswan: staging %s: %w", d, err)
		}
	}
	out, err := r.checker.Runner.Run(ctx, renderers.Command{
		Path:    SwanctlBin,
		Args:    []string{"--load-all", "--noprompt", "--file", st.Path(r.paths.SwanctlConf()), "--uri", "unix://" + r.checker.ViciSocket},
		Timeout: validateTimeout,
	})
	if err != nil {
		return fmt.Errorf("%w: swanctl --load-all rejected the files: %s", ErrDaemon, r.toolMessage(out, err, st.Dir))
	}
	return nil
}

// toolMessage condenses a failed tool run into one redacted line.
func (r *Renderer) toolMessage(out renderers.Output, err error, stagingDir string) string {
	var parts []string
	for _, b := range [][]byte{out.Stdout, out.Stderr} {
		if s := strings.TrimSpace(string(bytes.ToValidUTF8(b, []byte("?")))); s != "" {
			parts = append(parts, s)
		}
	}
	msg := strings.Join(parts, " | ")
	if msg == "" {
		msg = err.Error()
	}
	if stagingDir != "" {
		msg = strings.ReplaceAll(msg, stagingDir, "<staging>")
	}
	msg = r.secrets.redact(strings.Join(strings.Fields(msg), " "))
	if len(msg) > 1024 {
		msg = msg[:1024] + "..."
	}
	return msg
}
