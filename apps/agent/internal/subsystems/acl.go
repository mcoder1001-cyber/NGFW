package subsystems

// F-acl: the `acl` domain (Domains["acl"]) — DF-4's acl plugin descriptors with the F-acl tracker
// on acl.acl / acl.macip-acl (internal/actions/acl), the projection environment of
// internal/desired/acl.go, and the re-projection watcher: every 60 s it checks whether a rule's
// schedule turned on or off since the applied projection, and on every FQDN change of an object an
// applied rule uses it asks the agent for a resync of its stored desired state (A5 seam
// Wiring.RequestResync, questions Q1). subsystems.go only carries the domain constant, the Domains
// entry and one registerACL call.

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	aclstate "ngfw/agent/internal/actions/acl"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/scheduler"
)

// Environment of the acl domain (read once at start).
const (
	// EnvACLReproject is the schedule check interval in seconds (default 60; clamped to 5–3600).
	EnvACLReproject = "VRX_ACL_REPROJECT_SEC"
	// EnvStatsSocket is the VPP stats segment socket (the agent's own setting, same default).
	EnvStatsSocket = "VRX_AGENT_VPP_STATS_SOCKET"
)

// ACL re-projection timing.
const (
	aclReprojectDefault = 60 * time.Second
	aclReprojectMin     = 5 * time.Second
	aclReprojectMax     = time.Hour
	// aclResyncGap is the minimum time between two resync requests (bursts are coalesced; review L5:
	// a resync re-reads every ACL, so FQDN churn must not trigger one more often than D-132's 30 s).
	aclResyncGap = 30 * time.Second
)

// aclDescriptors is Domains["acl"]: the applied-configuration record, then DF-4's six in registration order.
func aclDescriptors() []string {
	return []string{
		aclstate.NameConfig, descacl.NameACL, descacl.NameMacipACL, descacl.NameInterfaceBinding, descacl.NameEtypeWhitelist,
		descacl.NameMacipInterfaceBinding, descacl.NameStatsEnable,
	}
}

// registerACL registers the acl family with the persisted claim store (KeyedClaims("acl"), D-080),
// installs the projection environment and starts the re-projection watcher.
func (w *Wiring) registerACL(r scheduler.Registry) error {
	claims, err := w.KeyedClaims("acl")
	if err != nil {
		return err
	}
	c, owner := w.env.Client, w.env.Owner
	log := w.env.Log.With("family", "acl")
	statsSock := os.Getenv(EnvStatsSocket)
	if statsSock == "" {
		statsSock = "/run/vpp/stats.sock"
	}
	interval, err := aclReprojectInterval(log)
	if err != nil {
		return err
	}
	rt := aclstate.Open(aclstate.Config{StateDir: w.env.StateDir, Owner: owner, Client: c, StatsSocket: statsSock, GlobalsOwner: w.env.GlobalsOwner, Log: log})
	opts := []descacl.Option{descacl.WithEtypeClaims(claims)}
	r.Register(rt.ConfigDescriptor()) // the applied acl configuration (review H1)
	r.Register(rt.Tracker().WrapACL(descacl.NewACL(c, owner)))
	r.Register(rt.Tracker().WrapMacip(descacl.NewMacipACL(c, owner)))
	r.Register(descacl.NewInterfaceBinding(c, owner, opts...))
	r.Register(descacl.NewEtypeWhitelist(c, owner, opts...))
	r.Register(descacl.NewMacipBinding(c, owner, opts...))
	r.Register(descacl.NewStatsEnable(c))

	env := desired.ACLEnv{Owner: owner, GlobalsOwner: w.env.GlobalsOwner, Record: rt.Record(), Location: time.Local}
	objrt := w.ObjectModel()
	if objrt != nil {
		env.FQDN = objrt.FQDN
	}
	desired.SetACLEnv(env)

	wt := newACLWatcher(rt, w.RequestResync, w.env.Resync != nil, log, time.Now, time.Local)
	wt.start(interval)
	unsubscribe := func() {}
	if objrt != nil {
		unsubscribe = objrt.Subscribe(wt.fqdnChanged)
	}
	w.OnClose(func() {
		unsubscribe()
		wt.close()
		rt.Close()
	})
	log.Info("acl domain wired", "stats", statsSock, "reproject_every", interval.String(), "globals_owner", w.env.GlobalsOwner, "resync_hook", w.env.Resync != nil)
	return nil
}

// ACLRuntime returns this agent's acl runtime (tracker, counters, bindings).
func (w *Wiring) ACLRuntime() *aclstate.Runtime {
	return aclstate.RuntimeFor(w.env.StateDir, w.env.Owner)
}

func aclReprojectInterval(log *slog.Logger) (time.Duration, error) {
	s := strings.TrimSpace(os.Getenv(EnvACLReproject))
	if s == "" {
		return aclReprojectDefault, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s=%q: want a number of seconds", EnvACLReproject, s)
	}
	d := time.Duration(n) * time.Second
	clamped := min(max(d, aclReprojectMin), aclReprojectMax)
	if clamped != d {
		log.Warn("ACL re-projection interval clamped", "env", EnvACLReproject, "value", s, "used", clamped.String())
	}
	return clamped, nil
}

// aclWatcher asks for a resync when what VPP holds no longer matches what a projection would
// produce now: a schedule of an applied rule turned on or off, or an FQDN object an applied rule
// uses changed addresses. It compares against the APPLIED expansion (tracker fingerprint → record),
// never against a DryRun.
type aclWatcher struct {
	rt     *aclstate.Runtime
	resync func()
	hooked bool
	log    *slog.Logger
	now    func() time.Time
	loc    *time.Location

	mu        sync.Mutex
	last      time.Time
	pending   *time.Timer
	warned    bool
	requested int
	stopCh    chan struct{}
	done      chan struct{}
}

func newACLWatcher(rt *aclstate.Runtime, resync func(), hooked bool, log *slog.Logger, now func() time.Time, loc *time.Location) *aclWatcher {
	return &aclWatcher{rt: rt, resync: resync, hooked: hooked, log: log, now: now, loc: loc}
}

func (w *aclWatcher) start(interval time.Duration) {
	w.stopCh, w.done = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(w.done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-w.stopCh:
				return
			case <-t.C:
				w.check()
			}
		}
	}()
}

func (w *aclWatcher) close() {
	if w.stopCh != nil {
		close(w.stopCh)
		<-w.done
		w.stopCh = nil
	}
	w.mu.Lock()
	if w.pending != nil {
		w.pending.Stop()
		w.pending = nil
	}
	w.mu.Unlock()
}

// check runs one schedule check.
func (w *aclWatcher) check() {
	if reason, ok := w.scheduleChanged(w.now()); ok {
		w.request(reason)
	}
}

// scheduleChanged reports the first applied rule whose schedule state differs from the projection.
func (w *aclWatcher) scheduleChanged(now time.Time) (string, bool) {
	for _, a := range w.rt.Tracker().ACLs() {
		exp, ok := w.rt.AppliedExpansion(a.Name, a.Fingerprint)
		if !ok {
			continue
		}
		for _, r := range exp.Rules {
			if r.Schedule == "" || r.Status == vrxv1.AclRuleStatus_ACL_RULE_STATUS_DISABLED || r.Status == vrxv1.AclRuleStatus_ACL_RULE_STATUS_UNSPECIFIED {
				continue
			}
			on, err := objects.Active(exp.Schedules[r.Schedule], now, w.loc)
			if err != nil {
				continue
			}
			if applied := r.Status != vrxv1.AclRuleStatus_ACL_RULE_STATUS_SCHEDULE_INACTIVE; on != applied {
				state := "off"
				if on {
					state = "on"
				}
				return fmt.Sprintf("list %q rule %d: schedule %q turned %s", a.Name, r.Sequence, r.Schedule, state), true
			}
		}
	}
	return "", false
}

// fqdnChanged is the objects runtime's change notification (resolver goroutine; kept short).
func (w *aclWatcher) fqdnChanged(ch objects.Change) {
	changed := map[string]bool{}
	for _, o := range ch.Objects {
		changed[o] = true
	}
	for _, a := range w.rt.Tracker().ACLs() {
		exp, ok := w.rt.AppliedExpansion(a.Name, a.Fingerprint)
		if !ok {
			continue
		}
		for _, r := range exp.Rules {
			for _, f := range r.FQDN {
				if changed[f] {
					w.request(fmt.Sprintf("list %q rule %d: FQDN object %q (%s) now resolves to %v", a.Name, r.Sequence, f, ch.Host, ch.Addresses))
					return
				}
			}
		}
	}
}

// request asks for a resync, at most once per aclResyncGap (a burst becomes one delayed request).
func (w *aclWatcher) request(reason string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.hooked {
		if !w.warned {
			w.warned = true
			w.log.Warn("acl re-projection needed but the agent's resync hook is not wired (F-acl questions Q1); it is applied with the next commit", "reason", reason)
		}
		return
	}
	if w.pending != nil {
		return
	}
	wait := aclResyncGap - w.now().Sub(w.last)
	if w.last.IsZero() || wait <= 0 {
		w.fireLocked(reason)
		return
	}
	w.pending = time.AfterFunc(wait, func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.pending = nil
		w.fireLocked(reason)
	})
}

func (w *aclWatcher) fireLocked(reason string) {
	w.last = w.now()
	w.requested++
	w.log.Info("acl re-projection: resync requested", "reason", reason)
	go w.resync()
}

// Requested is the number of resync requests made (tests).
func (w *aclWatcher) Requested() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.requested
}
