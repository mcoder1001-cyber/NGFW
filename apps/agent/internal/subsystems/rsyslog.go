package subsystems

import (
	"fmt"
	"path/filepath"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rsyslog"
)

// newRsyslog returns the rsyslog export renderer of env: for the globals owner the product include
// (/etc/rsyslog.d/50-vrx-export.conf, `systemctl restart rsyslog` — RF-4), else a standalone slot configuration
// (<slot dir>/rsyslog/rsyslog.conf, imuxsock on its log.sock — never /dev/log, no imtcp) driven by a
// DeferredController: a restart request, never a restart (the slot harness runs the instance with
// `-i <dir>/rsyslogd.pid`), and the hook that prepares the directories.
func newRsyslog(env Env) (*rsyslog.Renderer, func() error, error) {
	runner := renderers.NewSystemRunner(renderers.NewAllowlist(rsyslog.Binaries()...))
	if env.GlobalsOwner {
		return rsyslog.New(runner), nil, nil
	}
	dir := filepath.Join(slotRunDir(env.Owner), "rsyslog")
	p := rsyslog.PathsUnder(dir, 0)
	if err := p.Validate(); err != nil {
		return nil, nil, fmt.Errorf("rsyslog slot paths: %w", err)
	}
	prep := func() error {
		for _, d := range []string{dir, p.Standalone.WorkDir} {
			if err := mkdirShared(d); err != nil {
				return fmt.Errorf("rsyslog slot dir: %w", err)
			}
		}
		return nil
	}
	ctl := &rsyslog.DeferredController{PendingFile: filepath.Join(dir, "vrx.pending"), PIDFile: filepath.Join(dir, "rsyslogd.pid")}
	return rsyslog.New(runner, rsyslog.WithPaths(p), rsyslog.WithController(ctl)), prep, nil
}
