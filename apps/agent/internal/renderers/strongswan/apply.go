package strongswan

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/strongswan/govici/vici"

	"ngfw/agent/internal/renderers"
)

// Apply implements renderers.Renderer:
//
//  1. parse and check the files (the same strict parser as Validate);
//  2. snapshot the three paths and write the files atomically (conf 0640, secrets 0600);
//  3. load through VICI in dependency order — authorities, shared secrets (load-shared with
//     the connection's section name as unique id), pools, connections (load-conn, with
//     start_action "start" withheld, see plan.starts) — then unload what charon has and the
//     files do not (unload-conn + terminate its SAs, unload-shared, unload-pool,
//     unload-authority), limited to this renderer's owner prefix;
//  4. config convergence: list-conns per rendered connection must show the rendered version,
//     addresses, children, modes and traffic selectors; get-shared/get-pools the owned sets;
//  5. SA convergence (RF-2 review H1): established SAs are negotiated from the config that was
//     loaded when they came up, and charon keeps using it on rekey. For every connection whose
//     security-relevant parts changed against the previous files (fingerprint: addresses,
//     proposals, auth, identities, the PSK; per child: selectors, ESP/AH proposals, mode,
//     if_ids, replay window) the affected IKE_SAs / CHILD_SAs are terminated by unique id
//     (DELETE sent to the peer; forced if the peer does not answer); independently, every live
//     SA that visibly contradicts the new config (selector outside the configured ones,
//     identity or IKE version mismatch, child no longer configured) is terminated too. Then
//     start_action=start children without a CHILD_SA are initiated (non-blocking; a peer that
//     is down does not fail the commit). Success requires list-sas to show none of the
//     terminated SAs and no contradicting SA. Soft changes (DPD, MOBIKE, lifetimes, actions)
//     never disturb an established SA (review M1); they apply to the next negotiation.
//  6. on any failure restore the snapshot and apply the previous files the same way (with a
//     context of its own), then return the error so the commit engine rolls back.
//
// The return value can be an *ActionRequired (not a failure, nothing is rolled back) when the
// running charon's plugin set differs from the rendered strongswan.conf (review L5): charon
// must be restarted for strongswan.conf changes. Not safe for concurrent use: the commit
// engine serialises commits.
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
	for _, dir := range []string{filepath.Dir(r.paths.StrongswanConf), filepath.Dir(r.paths.ConnsFile())} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("strongswan: directory %s is missing (packaging/harness creates it)", dir)
		}
	}
	prev := r.previousPlan()
	var prevPlan *plan
	if prev != nil {
		prevPlan = prev.plan
	}
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	if err := renderers.WriteFiles(files); err != nil {
		return errors.Join(err, snap.Restore())
	}
	rep, applyErr := r.load(ctx, p, t, prevPlan)
	if applyErr == nil {
		r.log.Info("strongswan: applied", "conns", len(p.conns), "secrets", len(p.shared), "pools", len(p.pools),
			"reestablished", strings.Join(rep.reestablished, ","), "terminated_sas", rep.terminated)
		return r.pluginCheck(ctx, t)
	}
	r.log.Warn("strongswan: apply failed, rolling back", "error", r.secrets.redact(applyErr.Error()))
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	restoreErr := snap.Restore()
	// The SAs were negotiated from the failed plan only if its SA reconciliation ran; otherwise
	// they still belong to the previous files and must not flap on rollback.
	rbOld := prevPlan
	if rep.reconciled || prevPlan == nil {
		rbOld = p
	}
	if prev != nil {
		if _, err := r.load(rbCtx, prev.plan, prev.trees, rbOld); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("strongswan: re-applying the previous files: %w", err))
		}
	} else {
		// No (valid) previous files: take back everything this Apply may have loaded.
		if _, err := r.load(rbCtx, &plan{starts: map[string][]string{}, fp: map[string]connFP{}}, emptyTrees(), p); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("strongswan: unloading after a failed first apply: %w", err))
		}
	}
	return r.secrets.redactErr(errors.Join(applyErr, restoreErr))
}

// ActionRequired is returned by Apply when the files were applied but charon must be
// restarted for strongswan.conf to take effect (shared duck-typed shape with the kea, unbound
// and chrony renderers, D-079). It is re-derived on every Apply from the running daemon, so it
// persists until charon runs the rendered plugin set.
type ActionRequired struct {
	Daemon string // "charon"
	Unit   string // systemd unit in the product ("strongswan")
	Action string // "restart"
	Reason string
}

func (e *ActionRequired) Error() string {
	return fmt.Sprintf("strongswan: %s must be %sed (%s): %s", e.Daemon, e.Action, e.Unit, e.Reason)
}

// NeedsRestart reports the unit and action.
func (e *ActionRequired) NeedsRestart() (unit, action string) { return e.Unit, e.Action }

// pluginCheck compares charon's loaded plugins with the rendered load list.
func (r *Renderer) pluginCheck(ctx context.Context, t *trees) error {
	want, _ := t.conf.Sub("charon").Get("load")
	s, err := r.open(ctx)
	if err != nil {
		return nil //nolint:nilerr // the apply itself succeeded; the next Apply re-checks
	}
	defer s.Close()
	stats, err := s.call(ctx, "stats", nil)
	if err != nil {
		return nil //nolint:nilerr // as above
	}
	loaded := strs(stats, "plugins")
	var missing []string
	for _, p := range strings.Fields(want) {
		if !slices.Contains(loaded, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &ActionRequired{Daemon: "charon", Unit: "strongswan", Action: "restart",
		Reason: "strongswan.conf loads plugins charon is not running: " + strings.Join(missing, " ")}
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

// applyReport says what load did to established SAs.
type applyReport struct {
	reestablished []string // connections whose SAs were terminated because they no longer matched
	terminated    int
	reconciled    bool // reconcileSAs ran (SAs may have been (re)negotiated from this plan)
}

// load makes charon's VICI state equal the plan and verifies config and SAs. old is the plan
// the currently established SAs were (presumably) negotiated from (nil: unknown).
func (r *Renderer) load(ctx context.Context, p *plan, t *trees, old *plan) (*applyReport, error) {
	s, err := r.open(ctx)
	if err != nil {
		return &applyReport{}, err
	}
	defer s.Close()

	for _, a := range p.authorities {
		if _, err := s.call(ctx, "load-authority", a.msg); err != nil {
			return &applyReport{}, fmt.Errorf("load authority %s: %w", a.name, err)
		}
	}
	for _, sh := range p.shared {
		if _, err := s.call(ctx, "load-shared", sh.message()); err != nil {
			return &applyReport{}, fmt.Errorf("load %s: %w", sh, err)
		}
	}
	for _, pl := range p.pools {
		if _, err := s.call(ctx, "load-pool", pl.msg); err != nil {
			return &applyReport{}, fmt.Errorf("load pool %s: %w", pl.name, err)
		}
	}
	for _, c := range p.conns {
		if _, err := s.call(ctx, "load-conn", c.msg); err != nil {
			return &applyReport{}, fmt.Errorf("load connection %s: %w", c.name, err)
		}
	}
	if err := r.unloadStale(ctx, s, p); err != nil {
		return &applyReport{}, err
	}
	if err := r.converged(ctx, s, p, t); err != nil {
		return &applyReport{}, err
	}
	return r.reconcileSAs(ctx, s, p, t, old)
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
		if slices.Contains(want, c) || !r.owns(c) {
			continue
		}
		if _, err := s.call(ctx, "unload-conn", msg("name", c)); err != nil {
			return fmt.Errorf("unload connection %s: %w", c, err)
		}
		// unload-conn only undoes start actions; SAs of a manually initiated or responder
		// connection would keep running without a config. A truncated listing is fine here
		// (review L3): the loop runs again on the next Apply for the rest.
		sas, _, err := s.listSAs(ctx, c)
		if err != nil {
			return err
		}
		for _, sa := range sas {
			if err := s.terminateIKE(ctx, str(sa, "uniqueid")); err != nil {
				return fmt.Errorf("terminate SAs of unloaded connection %s: %w", c, err)
			}
		}
	}
	shared, err := s.getShared(ctx)
	if err != nil {
		return err
	}
	for _, id := range shared {
		if r.ownsShared(id) && !slices.ContainsFunc(p.shared, func(x sharedSecret) bool { return x.id == id }) {
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
		if r.owns(pl) && !slices.Contains(names(p.pools), pl) {
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
		if r.owns(a) && !slices.Contains(names(p.authorities), a) {
			if _, err := s.call(ctx, "unload-authority", msg("name", a)); err != nil {
				return fmt.Errorf("unload authority %s: %w", a, err)
			}
		}
	}
	return nil
}

// converged verifies charon holds exactly the plan's configuration (RF-1 review H2: success
// is only reported when the daemon took everything).
func (r *Renderer) converged(ctx context.Context, s *session, p *plan, t *trees) error {
	var problems []string
	loaded, err := s.getConns(ctx)
	if err != nil {
		return err
	}
	want := names(p.conns)
	for _, c := range loaded {
		if r.owns(c) && !slices.Contains(want, c) {
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
	owned := 0
	for _, id := range shared {
		if r.ownsShared(id) {
			owned++
		}
	}
	for _, sh := range p.shared {
		if !slices.Contains(shared, sh.id) {
			problems = append(problems, "shared secret "+sh.id+" not loaded")
		}
	}
	if owned != len(p.shared) {
		problems = append(problems, fmt.Sprintf("%d owned shared secrets loaded, %d rendered", owned, len(p.shared)))
	}
	pools, err := s.getPools(ctx)
	if err != nil {
		return err
	}
	var ownedPools []string
	for _, pl := range pools {
		if r.owns(pl) {
			ownedPools = append(ownedPools, pl)
		}
	}
	if !sameSet(ownedPools, names(p.pools)) {
		problems = append(problems, fmt.Sprintf("pools %v loaded, %v rendered", ownedPools, names(p.pools)))
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

// ------------------------------------------------------------------------------ SA convergence

// connCfg is what a live SA is checked against.
type connCfg struct {
	version      string // "1", "2" or "0"
	localID      string // "" = any
	remoteID     string
	children     map[string]childCfg
	ikeChanged   bool
	changedChild map[string]bool
}

type childCfg struct {
	local, remote []netip.Prefix // nil = dynamic (not checked)
	mode          string
}

func cfgFromTree(name string, conn *Section, p, old *plan) connCfg {
	c := connCfg{children: map[string]childCfg{}, changedChild: map[string]bool{}}
	c.version, _ = conn.Get("version")
	c.localID, _ = conn.Sub("local").Get("id")
	c.remoteID, _ = conn.Sub("remote").Get("id")
	for _, ch := range conn.Sub("children").Sections() {
		cc := childCfg{mode: "tunnel"}
		if m, ok := ch.Section.Get("mode"); ok {
			cc.mode = m
		}
		if v, ok := ch.Section.Get("local_ts"); ok {
			cc.local = prefixes(v)
		}
		if v, ok := ch.Section.Get("remote_ts"); ok {
			cc.remote = prefixes(v)
		}
		c.children[ch.Name] = cc
	}
	if old != nil {
		if ofp, ok := old.fp[name]; ok {
			nfp := p.fp[name]
			c.ikeChanged = ofp.ike != nfp.ike
			for child, f := range ofp.children {
				if nf, ok := nfp.children[child]; !ok || nf != f {
					c.changedChild[child] = true
				}
			}
		}
	}
	return c
}

func prefixes(list string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range splitList(list) {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
		}
	}
	return out
}

// tsWithin reports whether every selector of an SA lies inside one configured prefix.
// Selectors charon reports with a port/protocol suffix ("10.3.1.0/24[tcp/80]") are compared by
// their address part.
func tsWithin(sa []string, cfg []netip.Prefix) bool {
	if cfg == nil {
		return true
	}
	for _, s := range sa {
		addr, _, _ := strings.Cut(s, "[")
		p, err := netip.ParsePrefix(addr)
		if err != nil {
			a, aerr := netip.ParseAddr(addr)
			if aerr != nil {
				return false
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		if !slices.ContainsFunc(cfg, func(c netip.Prefix) bool { return c.Bits() <= p.Bits() && c.Contains(p.Addr()) }) {
			return false
		}
	}
	return true
}

var (
	deadIKEStates   = []string{"DELETING", "DESTROYING"}
	deadChildStates = []string{"DELETING", "DELETED", "DESTROYING"}
)

// mismatchIKE explains why a live IKE_SA contradicts its connection ("" = it does not).
func (c connCfg) mismatchIKE(sa *vici.Message) string {
	established := str(sa, "state") == "ESTABLISHED"
	switch {
	case c.ikeChanged:
		return "negotiated from a changed connection (addresses, proposals, auth, identities or PSK)"
	case c.version != "0" && c.version != "" && str(sa, "version") != c.version:
		return "IKE version " + str(sa, "version")
	case c.localID != "" && established && str(sa, "local-id") != c.localID:
		return "local identity " + str(sa, "local-id")
	case c.remoteID != "" && established && str(sa, "remote-id") != c.remoteID:
		return "remote identity " + str(sa, "remote-id")
	}
	return ""
}

// mismatchChild explains why a live CHILD_SA contradicts the config ("" = it does not).
func (c connCfg) mismatchChild(child *vici.Message) string {
	name := str(child, "name")
	cc, ok := c.children[name]
	switch {
	case !ok:
		return "child " + name + " is no longer configured"
	case c.changedChild[name]:
		return "negotiated from a changed child (selectors, proposals, mode or if_ids)"
	case !strings.EqualFold(str(child, "mode"), cc.mode):
		return "mode " + str(child, "mode")
	case !tsWithin(strs(child, "local-ts"), cc.local):
		return fmt.Sprintf("local selectors %v outside %v", strs(child, "local-ts"), cc.local)
	case !tsWithin(strs(child, "remote-ts"), cc.remote):
		return fmt.Sprintf("remote selectors %v outside %v", strs(child, "remote-ts"), cc.remote)
	}
	return ""
}

func childSections(sa *vici.Message) []*vici.Message {
	var out []*vici.Message
	if cs := sub(sa, "child-sas"); cs != nil {
		for _, k := range cs.Keys() {
			if c := sub(cs, k); c != nil {
				out = append(out, c)
			}
		}
	}
	return out
}

// reconcileSAs terminates SAs that no longer match, initiates missing start children and
// verifies the result (see Apply, step 5).
func (r *Renderer) reconcileSAs(ctx context.Context, s *session, p *plan, t *trees, old *plan) (*applyReport, error) {
	rep := &applyReport{reconciled: true}
	deadIKE, deadChild := map[string]bool{}, map[string]bool{}
	cfgs := map[string]connCfg{}
	for _, it := range t.conns.Sub("connections").Sections() {
		c := cfgFromTree(it.Name, it.Section, p, old)
		cfgs[it.Name] = c
		sas, truncated, err := s.listSAs(ctx, it.Name)
		if err != nil {
			return rep, err
		}
		if truncated {
			return rep, fmt.Errorf("%w: connection %s has more than %d IKE_SAs; cannot verify them", ErrTooLarge, it.Name, MaxSAsPerConn)
		}
		flapped := false
		for _, sa := range sas {
			id := str(sa, "uniqueid")
			if slices.Contains(deadIKEStates, str(sa, "state")) {
				continue
			}
			if why := c.mismatchIKE(sa); why != "" {
				r.log.Info("strongswan: terminating IKE_SA that no longer matches its connection", "conn", it.Name, "ike_id", id, "reason", why)
				if err := s.terminateIKE(ctx, id); err != nil {
					return rep, err
				}
				deadIKE[it.Name+"#"+id], flapped = true, true
				rep.terminated++
				continue
			}
			for _, ch := range childSections(sa) {
				cid := str(ch, "uniqueid")
				if slices.Contains(deadChildStates, str(ch, "state")) {
					continue
				}
				if why := c.mismatchChild(ch); why != "" {
					r.log.Info("strongswan: terminating CHILD_SA that no longer matches its config", "conn", it.Name, "child_id", cid, "reason", why)
					if err := s.terminateChild(ctx, cid); err != nil {
						return rep, err
					}
					deadChild[it.Name+"#"+cid], flapped = true, true
					rep.terminated++
				}
			}
		}
		if flapped {
			rep.reestablished = append(rep.reestablished, it.Name)
		}
	}
	// Start children that have no CHILD_SA (and no IKE_SA still connecting).
	for conn, children := range p.starts {
		sas, _, err := s.listSAs(ctx, conn)
		if err != nil {
			return rep, err
		}
		connecting := false
		have := map[string]bool{}
		for _, sa := range sas {
			st := str(sa, "state")
			if slices.Contains(deadIKEStates, st) {
				continue
			}
			if st == "CONNECTING" {
				connecting = true
			}
			for _, ch := range childSections(sa) {
				if !slices.Contains(deadChildStates, str(ch, "state")) {
					have[str(ch, "name")] = true
				}
			}
		}
		if connecting {
			continue
		}
		for _, child := range children {
			if have[child] {
				continue
			}
			if err := s.initiateAsync(ctx, conn, child); err != nil {
				return rep, err
			}
		}
	}
	// Verify: nothing terminated is still alive, nothing alive contradicts the config.
	deadline := time.Now().Add(saVerifyTimeout)
	for {
		problems, err := r.saProblems(ctx, s, cfgs, deadIKE, deadChild)
		if err != nil {
			return rep, err
		}
		if len(problems) == 0 {
			return rep, nil
		}
		if time.Now().After(deadline) {
			return rep, fmt.Errorf("%w: live SAs do not match the applied configuration: %s", ErrDaemon, strings.Join(problems, "; "))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

const saVerifyTimeout = 10 * time.Second

func (r *Renderer) saProblems(ctx context.Context, s *session, cfgs map[string]connCfg, deadIKE, deadChild map[string]bool) ([]string, error) {
	var problems []string
	for name, c := range cfgs {
		sas, _, err := s.listSAs(ctx, name)
		if err != nil {
			return nil, err
		}
		// SAs not terminated above were negotiated after the change: only the visible checks.
		c.ikeChanged, c.changedChild = false, map[string]bool{}
		for _, sa := range sas {
			id := str(sa, "uniqueid")
			if deadIKE[name+"#"+id] {
				problems = append(problems, fmt.Sprintf("%s IKE_SA #%s still %s after terminate", name, id, str(sa, "state")))
				continue
			}
			if slices.Contains(deadIKEStates, str(sa, "state")) {
				continue
			}
			if why := c.mismatchIKE(sa); why != "" {
				problems = append(problems, fmt.Sprintf("%s IKE_SA #%s: %s", name, id, why))
			}
			for _, ch := range childSections(sa) {
				cid := str(ch, "uniqueid")
				if deadChild[name+"#"+cid] {
					problems = append(problems, fmt.Sprintf("%s CHILD_SA #%s still %s after terminate", name, cid, str(ch, "state")))
					continue
				}
				if slices.Contains(deadChildStates, str(ch, "state")) {
					continue
				}
				if why := c.mismatchChild(ch); why != "" {
					problems = append(problems, fmt.Sprintf("%s CHILD_SA #%s: %s", name, cid, why))
				}
			}
		}
	}
	slices.Sort(problems)
	return problems, nil
}

// Impact reports, per connection, what applying files would do to its established SAs
// compared with the files on disk: "reestablish" (security-relevant change: its SAs are
// terminated and re-negotiated), "add", "remove" or "update" (soft change: SAs untouched).
// Unchanged connections are omitted. For the commit engine / UI ("tunnel will re-establish").
func (r *Renderer) Impact(files renderers.Files) (map[string]string, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	if err := r.checkFiles(files); err != nil {
		return nil, err
	}
	t, err := r.parseFiles(files)
	if err != nil {
		return nil, r.secrets.redactErr(err)
	}
	p, err := r.buildPlan(t)
	if err != nil {
		return nil, r.secrets.redactErr(err)
	}
	out := map[string]string{}
	prev := r.previousPlan()
	if prev == nil {
		prev = &parsedPlan{plan: &plan{starts: map[string][]string{}, fp: map[string]connFP{}}}
	}
	oldConns := map[string]string{}
	for _, c := range prev.plan.conns {
		oldConns[c.name] = c.msg.String()
	}
	for name := range prev.plan.fp {
		if _, ok := p.fp[name]; !ok {
			out[name] = "remove"
		}
	}
	for _, c := range p.conns {
		ofp, ok := prev.plan.fp[c.name]
		switch {
		case !ok:
			out[c.name] = "add"
		case ofp.ike != p.fp[c.name].ike || !mapsEqual(ofp.children, p.fp[c.name].children):
			out[c.name] = "reestablish"
		case oldConns[c.name] != c.msg.String() || !slices.Equal(prev.plan.starts[c.name], p.starts[c.name]):
			out[c.name] = "update"
		}
	}
	return out, nil
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}
