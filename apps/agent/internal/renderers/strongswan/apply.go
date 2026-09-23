package strongswan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"ngfw/agent/internal/renderers"
)

// Apply implements renderers.Renderer:
//
//  1. parse and check the files (the same strict parser as Validate);
//  2. snapshot the three paths and write the files atomically (conf 0640, secrets 0600);
//  3. load through VICI in dependency order — authorities, shared secrets (load-shared with
//     the connection's section name as unique id), pools, connections (load-conn) — then
//     unload what charon has but the files do not (unload-conn + terminate its SAs,
//     unload-shared, unload-pool, unload-authority): the renderer owns charon's VICI config,
//     like `swanctl --load-all`;
//  4. convergence check: list-conns must show every rendered connection with the rendered
//     version, addresses, children, modes and traffic selectors; get-shared every secret id;
//     get-pools every pool; and nothing else may be loaded;
//  5. on any failure restore the snapshot and apply the previous files the same way (with a
//     context of its own), then return the error so the commit engine rolls back.
//
// Loading an unchanged connection is a no-op in charon ("updated vici connection", no start
// action is repeated), so Apply is idempotent and safe to call with the previous rendering.
// strongswan.conf changes (plugins, sockets, log) take effect when charon restarts; the
// renderer never restarts the daemon. Not safe for concurrent use: the commit engine
// serialises commits.
func (r *Renderer) Apply(ctx context.Context, files renderers.Files) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := r.checkFiles(files); err != nil {
		return err
	}
	t, err := r.parseFiles(files)
	if err != nil {
		return r.secrets.redactErr(err)
	}
	p, err := r.buildPlan(t)
	if err != nil {
		return r.secrets.redactErr(err)
	}
	r.rememberSecrets(p)
	for _, dir := range []string{filepath.Dir(r.paths.StrongswanConf), filepath.Dir(r.paths.ConnsFile())} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("strongswan: directory %s is missing (packaging/harness creates it)", dir)
		}
	}
	prev := r.previousPlan()
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	if err := renderers.WriteFiles(files); err != nil {
		return errors.Join(err, snap.Restore())
	}
	applyErr := r.load(ctx, p, t)
	if applyErr == nil {
		r.log.Info("strongswan: applied", "conns", len(p.conns), "secrets", len(p.shared), "pools", len(p.pools))
		return nil
	}
	r.log.Warn("strongswan: apply failed, rolling back", "error", r.secrets.redact(applyErr.Error()))
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	restoreErr := snap.Restore()
	if prev != nil {
		if err := r.load(rbCtx, prev.plan, prev.trees); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("strongswan: re-applying the previous files: %w", err))
		}
	} else {
		// No (valid) previous files: take back everything this Apply may have loaded.
		empty := &plan{}
		if err := r.load(rbCtx, empty, emptyTrees()); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("strongswan: unloading after a failed first apply: %w", err))
		}
	}
	return r.secrets.redactErr(errors.Join(applyErr, restoreErr))
}

func (r *Renderer) rememberSecrets(p *plan) {
	for _, s := range p.shared {
		r.secrets.add(s.data)
	}
}

type parsedPlan struct {
	plan  *plan
	trees *trees
}

// previousPlan parses the files currently on disk (nil when they are absent or invalid: the
// rollback then unloads everything).
func (r *Renderer) previousPlan() *parsedPlan {
	files := renderers.Files{}
	for path, f := range map[string]renderers.File{
		r.paths.StrongswanConf: {Mode: r.paths.ConfMode}, r.paths.ConnsFile(): {Mode: r.paths.ConfMode},
		r.paths.SecretsFile(): {Mode: r.paths.SecretMode, Secret: true},
	} {
		b, err := readBounded(path)
		if err != nil {
			return nil
		}
		f.Content = b
		files[path] = f
	}
	t, err := r.parseFiles(files)
	if err != nil {
		return nil
	}
	p, err := r.buildPlan(t)
	if err != nil {
		return nil
	}
	r.rememberSecrets(p)
	return &parsedPlan{plan: p, trees: t}
}

func readBounded(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxSettingsSize {
		return nil, fmt.Errorf("strongswan: %s is not a regular file of at most %d bytes", path, MaxSettingsSize)
	}
	return os.ReadFile(path) //nolint:gosec // one of the renderer's own paths
}

func emptyTrees() *trees {
	conns := &Section{Items: []*Item{{Kind: KindSection, Name: "connections", Section: &Section{}}}}
	return &trees{conns: conns, secrets: &Section{Items: []*Item{{Kind: KindSection, Name: "secrets", Section: &Section{}}}}}
}

// load makes charon's VICI state equal the plan and verifies it.
func (r *Renderer) load(ctx context.Context, p *plan, t *trees) error {
	s, err := r.open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	for _, a := range p.authorities {
		if _, err := s.call(ctx, "load-authority", a.msg); err != nil {
			return fmt.Errorf("load authority %s: %w", a.name, err)
		}
	}
	for _, sh := range p.shared {
		if _, err := s.call(ctx, "load-shared", sh.message()); err != nil {
			return fmt.Errorf("load %s: %w", sh, err)
		}
	}
	for _, pl := range p.pools {
		if _, err := s.call(ctx, "load-pool", pl.msg); err != nil {
			return fmt.Errorf("load pool %s: %w", pl.name, err)
		}
	}
	for _, c := range p.conns {
		if _, err := s.call(ctx, "load-conn", c.msg); err != nil {
			return fmt.Errorf("load connection %s: %w", c.name, err)
		}
	}
	if err := r.unloadStale(ctx, s, p); err != nil {
		return err
	}
	return r.converged(ctx, s, p, t)
}

func names(ms []namedMsg) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.name
	}
	return out
}

func (r *Renderer) unloadStale(ctx context.Context, s *session, p *plan) error {
	loaded, err := s.getConns(ctx)
	if err != nil {
		return err
	}
	want := names(p.conns)
	for _, c := range loaded {
		if slices.Contains(want, c) {
			continue
		}
		if _, err := s.call(ctx, "unload-conn", msg("name", c)); err != nil {
			return fmt.Errorf("unload connection %s: %w", c, err)
		}
		// unload-conn only undoes start actions; SAs of a manually initiated or responder
		// connection would keep running without a config.
		sas, err := s.listSAs(ctx, c)
		if err != nil {
			return err
		}
		if len(sas) > 0 {
			if _, err := s.call(ctx, "terminate", msg("ike", c, "force", "yes", "timeout", "-1")); err != nil {
				return fmt.Errorf("terminate SAs of unloaded connection %s: %w", c, err)
			}
		}
	}
	shared, err := s.getShared(ctx)
	if err != nil {
		return err
	}
	for _, id := range shared {
		if !slices.ContainsFunc(p.shared, func(x sharedSecret) bool { return x.id == id }) {
			if _, err := s.call(ctx, "unload-shared", msg("id", id)); err != nil {
				return fmt.Errorf("unload shared secret %s: %w", id, err)
			}
		}
	}
	pools, err := s.getPools(ctx)
	if err != nil {
		return err
	}
	for _, pl := range pools {
		if !slices.Contains(names(p.pools), pl) {
			if _, err := s.call(ctx, "unload-pool", msg("name", pl)); err != nil {
				return fmt.Errorf("unload pool %s: %w", pl, err)
			}
		}
	}
	auths, err := s.getAuthorities(ctx)
	if err != nil {
		return err
	}
	for _, a := range auths {
		if !slices.Contains(names(p.authorities), a) {
			if _, err := s.call(ctx, "unload-authority", msg("name", a)); err != nil {
				return fmt.Errorf("unload authority %s: %w", a, err)
			}
		}
	}
	return nil
}

// converged verifies charon holds exactly the plan (RF-1 review H2: success is only reported
// when the daemon took everything).
func (r *Renderer) converged(ctx context.Context, s *session, p *plan, t *trees) error {
	var problems []string
	loaded, err := s.getConns(ctx)
	if err != nil {
		return err
	}
	want := names(p.conns)
	for _, c := range loaded {
		if !slices.Contains(want, c) {
			problems = append(problems, "connection "+c+" still loaded")
		}
	}
	for _, it := range t.conns.Sub("connections").Sections() {
		got, err := s.listConn(ctx, it.Name)
		if err != nil {
			return err
		}
		if got == nil {
			problems = append(problems, "connection "+it.Name+" not loaded")
			continue
		}
		if d := summaryFromTree(it.Section).diff(summaryFromVICI(got)); d != "" {
			problems = append(problems, "connection "+it.Name+": "+d)
		}
	}
	shared, err := s.getShared(ctx)
	if err != nil {
		return err
	}
	for _, sh := range p.shared {
		if !slices.Contains(shared, sh.id) {
			problems = append(problems, "shared secret "+sh.id+" not loaded")
		}
	}
	if len(shared) != len(p.shared) {
		problems = append(problems, fmt.Sprintf("%d shared secrets loaded, %d rendered", len(shared), len(p.shared)))
	}
	pools, err := s.getPools(ctx)
	if err != nil {
		return err
	}
	if !sameSet(pools, names(p.pools)) {
		problems = append(problems, fmt.Sprintf("pools %v loaded, %v rendered", pools, names(p.pools)))
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: not converged after loading: %s", ErrDaemon, strings.Join(problems, "; "))
	}
	return nil
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}
