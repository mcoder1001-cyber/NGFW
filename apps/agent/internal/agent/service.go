package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

type summaryCounts = scheduler.Summary

// mode of an internal transaction.
type mode int

const (
	modeTxn    mode = iota // Apply RPC
	modeResync             // agent start / VPP reconnect
	modeRevert             // confirm timeout
)

// Service implements the vrx.v1.Dataplane semantics (docs/contracts/proto.md) on top of the
// scheduler. The gRPC adapter (server.go) only translates.
type Service struct {
	// netdevKind: the af_packet veth rule's Linux netdev lookup (D-105), nil = no check
	netdevKind desired.NetdevKind
	owner      string
	version    string
	log        *slog.Logger
	vpp        vpp.Client
	sched      *scheduler.Scheduler
	st         *state
	bus        *bus
	metrics    *metrics
	now        func() time.Time
	claimsTxn  func() (flush func() error) // TD-11c: Wiring.ClaimsTxn, set by Start (nil = none)

	// txn serialises transactions (Apply, resync, revert) and guards st and timer.
	txn      chan struct{}
	timer    *time.Timer
	lastResp *vrxv1.ApplyResponse // last applyLocked result (guarded by txn)
	// owed-revert retry (guarded by txn)
	retryTimer         *time.Timer
	retryDelay         time.Duration
	retryMin, retryMax time.Duration

	mu              sync.Mutex // guards the fields below (Health snapshot)
	degraded        bool
	reconciling     bool
	lastReconcileAt time.Time
	vppVersion      string
	vrfIDs          map[string]uint32           // VRF name → table id of the stored desired state
	vrfDesc         map[string]string           // VRF name → description (D-073b)
	routeDesc       map[string]string           // "<vrf>|<prefix>" → description (D-073b)
	storedIfs       map[string]*vrxv1.Interface // stored desired `interfaces` (P08: descriptions, named NICs)
	storedDoc       *vrxv1.DesiredState         // stored desired state for DryRun's dynamic sources (TD-8; only with sources)
	beforeTxn       func()
	pendingTxn      string
	deadline        time.Time
	lastTxn         string

	// sources are the dynamic desired sources (S1, TD-8): merged into every transaction's projection
	// while they are in sync (dynsource.go).
	sources []*dynSource
	closed  bool // Close ran: no source retry is armed any more (guarded by txn)
	// holder is the goroutine that holds txn, inDesired the goroutines inside a source's Desired
	// (goid; only with sources): sync's re-entrancy guard (TD-8 review R6).
	holder    atomic.Uint64
	inDesired sync.Map
}

// ServiceConfig builds a Service.
type ServiceConfig struct {
	Owner     string
	Version   string
	Logger    *slog.Logger
	VPP       vpp.Client
	Scheduler *scheduler.Scheduler
	StateDir  string
	Metrics   *metrics
	Now       func() time.Time
	// BeforeTxn runs at the start of every transaction (P08: subsystems.Wiring.BeforeTxn).
	BeforeTxn func()
	// NetdevKind is the Linux netdev lookup of the af_packet veth rule (D-105; subsystems.Wiring.NetdevKind).
	// nil skips the check (unit tests of other domains).
	NetdevKind desired.NetdevKind
	// Events is the event bus (TD-8: the agent creates it before the wiring, whose Env.Publish feeds
	// it); nil = a new one.
	Events *bus
	// Sources are the dynamic desired sources (S1, TD-8; subsystems.Wiring.DynamicSources). Their
	// descriptors must be registered with Scheduler.
	Sources []subsystems.DynamicSource
}

// NewService loads the persisted state and returns a service. It does not touch VPP; call
// Resync once VPP is connected.
func NewService(cfg ServiceConfig) (*Service, error) {
	st, err := loadState(cfg.StateDir, cfg.Owner)
	if err != nil {
		return nil, err
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Metrics == nil {
		cfg.Metrics = newMetrics()
	}
	if cfg.Events == nil {
		cfg.Events = newBus()
	}
	if err := checkSources(cfg.Scheduler, cfg.Sources); err != nil {
		return nil, err
	}
	s := &Service{
		owner: cfg.Owner, version: cfg.Version, log: cfg.Logger, vpp: cfg.VPP, sched: cfg.Scheduler,
		st: st, bus: cfg.Events, metrics: cfg.Metrics, now: cfg.Now, txn: make(chan struct{}, 1),
		retryMin: revertRetryFloor, retryMax: revertRetryMax, beforeTxn: cfg.BeforeTxn, netdevKind: cfg.NetdevKind,
	}
	s.sources = newDynSources(cfg.Sources, s.metrics)
	s.refreshSnapshotLocked()
	return s, nil
}

func (s *Service) lock(ctx context.Context) error {
	select {
	case s.txn <- struct{}{}:
		if len(s.sources) > 0 {
			s.holder.Store(goid())
		}
		return nil
	case <-ctx.Done():
		return status.FromContextError(ctx.Err()).Err()
	}
}

func (s *Service) unlock() {
	if len(s.sources) > 0 {
		s.holder.Store(0)
	}
	<-s.txn
}

// refreshSnapshotLocked copies what Health/DryRun need out of st (caller holds txn or is the
// constructor).
func (s *Service) refreshSnapshotLocked() {
	ids := map[string]uint32{}
	for name, v := range s.st.desired.GetVrfs() {
		if v.Id != nil {
			ids[name] = v.GetId()
		}
	}
	vrfDesc, routeDesc := map[string]string{}, map[string]string{}
	for name, v := range s.st.desired.GetVrfs() {
		if v.Description != nil {
			vrfDesc[name] = v.GetDescription()
		}
	}
	for _, r := range s.st.desired.GetRouting().GetStatic() {
		if r.Description == nil {
			continue
		}
		vrf := r.GetVrf()
		if vrf == "" {
			vrf = "default"
		}
		if p, err := core.CanonNetPrefix(r.GetPrefix()); err == nil {
			routeDesc[vrf+"|"+p] = r.GetDescription()
		}
	}
	ifs := map[string]*vrxv1.Interface{}
	for name, itf := range s.st.desired.GetInterfaces() {
		ifs[name] = proto.Clone(itf).(*vrxv1.Interface)
	}
	s.mu.Lock()
	s.vrfDesc, s.routeDesc = vrfDesc, routeDesc
	s.storedIfs = ifs
	s.vrfIDs = ids
	s.pendingTxn = s.st.meta.PendingTxnID
	s.deadline = time.Time{}
	if s.st.meta.ConfirmDeadline != nil {
		s.deadline = *s.st.meta.ConfirmDeadline
	}
	s.lastTxn = s.st.meta.LastTxnID
	s.mu.Unlock()
	s.metrics.setPending(s.st.meta.PendingTxnID != "")
	desired.SetIpfixExporterNames(s.st.desired.GetServices()) // F-ipfix-sflow: names from the stored state only
	if len(s.sources) > 0 {
		doc := proto.Clone(s.st.desired).(*vrxv1.DesiredState)
		s.mu.Lock()
		s.storedDoc = doc
		s.mu.Unlock()
	}
}

func (s *Service) resolveVRF(name string) (uint32, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.vrfIDs[name]
	return id, ok
}

// ---- request validation -------------------------------------------------------------------

func (s *Service) checkOwner(owner string) error {
	if owner != "" && owner != s.owner {
		return status.Errorf(codes.InvalidArgument, "owner %q does not match this agent's owner %q", owner, s.owner)
	}
	return nil
}

func implemented(key string) bool { _, ok := domainDescriptors[key]; return ok }

// checkSubsystems validates names: unknown → INVALID_ARGUMENT, not implemented → UNIMPLEMENTED.
func checkSubsystems(subsystems []string) error {
	for _, k := range subsystems {
		if !isRootKey(k) {
			return status.Errorf(codes.InvalidArgument, "unknown subsystem %q (dotted sub-keys are not accepted in v1)", k)
		}
		if !implemented(k) {
			return status.Errorf(codes.Unimplemented, "subsystem %q is not implemented by this agent build (implemented: %v)", k, implementedDomains())
		}
	}
	return nil
}

// authoritative returns the domains a transaction manages (D-041, contract §2 table).
func authoritative(ds *vrxv1.DesiredState, subsystems []string) []string {
	if len(subsystems) > 0 {
		return union(subsystems, nil)
	}
	var out []string
	for _, k := range implementedDomains() {
		if domainPresent(ds, k) {
			out = append(out, k)
		}
	}
	return out
}

func fingerprint(ds *vrxv1.DesiredState, subsystems []string) string {
	b, _ := proto.MarshalOptions{Deterministic: true}.Marshal(ds)
	h := sha256.New()
	h.Write(b)
	for _, k := range subsystems {
		h.Write([]byte{0})
		h.Write([]byte(k))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ---- Apply --------------------------------------------------------------------------------

// Apply implements the Apply RPC.
func (s *Service) Apply(ctx context.Context, req *vrxv1.ApplyRequest) (*vrxv1.ApplyResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	hasApply, hasConfirm := req.GetTxnId() != "", req.GetConfirmTxnId() != ""
	switch {
	case !hasApply && !hasConfirm:
		return nil, status.Error(codes.InvalidArgument, "txn_id (apply) or confirm_txn_id (confirm) is required")
	case !hasApply && (req.DesiredState != nil || len(req.GetSubsystems()) > 0 || req.GetConfirmTimeoutSec() > 0):
		return nil, status.Error(codes.InvalidArgument, "desired_state, subsystems and confirm_timeout_sec need a txn_id")
	case hasApply && hasConfirm && req.GetTxnId() == req.GetConfirmTxnId():
		return nil, status.Error(codes.InvalidArgument, "txn_id and confirm_txn_id must differ")
	}
	if hasApply {
		if err := checkSubsystems(req.GetSubsystems()); err != nil {
			return nil, err
		}
	}
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	defer s.unlock()

	var fp string
	if hasApply {
		fp = fingerprint(req.GetDesiredState(), req.GetSubsystems())
		if prev, resp, ok := s.st.recall(req.GetTxnId()); ok {
			if prev != fp {
				return nil, status.Errorf(codes.Aborted, "txn_id %q was already used with a different desired state", req.GetTxnId())
			}
			s.log.Info("apply: repeated txn_id, returning the stored response", "txn_id", req.GetTxnId())
			return resp, nil
		}
	}
	if hasConfirm {
		if s.st.meta.PendingTxnID != req.GetConfirmTxnId() {
			return nil, status.Errorf(codes.FailedPrecondition, "transaction %q is not pending confirmation", req.GetConfirmTxnId())
		}
		// M1: after the deadline (also after a restart, before the first resync) or once the
		// revert is owed, a confirm is too late — the transaction reverts.
		if s.st.meta.Reverting || (s.st.meta.ConfirmDeadline != nil && !s.now().Before(*s.st.meta.ConfirmDeadline)) {
			return nil, status.Errorf(codes.FailedPrecondition, "transaction %q passed its confirm deadline and is being reverted", req.GetConfirmTxnId())
		}
		if err := s.confirmLocked(); err != nil {
			return nil, status.Errorf(codes.Internal, "confirm: %v", err)
		}
		if !hasApply {
			return &vrxv1.ApplyResponse{TxnId: req.GetConfirmTxnId(), Status: vrxv1.ApplyStatus_APPLY_STATUS_CONFIRMED, AppliedAt: timestamppb.New(s.now())}, nil
		}
	} else if s.st.meta.PendingTxnID != "" {
		if !s.st.meta.Reverting {
			return nil, status.Errorf(codes.FailedPrecondition, "transaction %q is pending confirmation: confirm it (confirm_txn_id) or let it revert", s.st.meta.PendingTxnID)
		}
		// N2: an owed revert (deadline passed, revert not yet successful) is superseded by a new
		// apply: the stored desired state already is the confirmed baseline, the new document
		// becomes the target, and the owed revert is dropped.
		s.log.Warn("new transaction supersedes the owed confirm revert", "reverted_txn_id", s.st.meta.PendingTxnID, "txn_id", req.GetTxnId())
		s.stopRetryLocked()
		s.st.meta.PendingTxnID = ""
		s.st.meta.ConfirmDeadline = nil
		s.st.meta.Reverting = false
		s.refreshSnapshotLocked()
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	domains := authoritative(req.GetDesiredState(), req.GetSubsystems())
	resp := s.applyLocked(ctx, modeTxn, req.GetTxnId(), req.GetDesiredState(), domains, req.GetConfirmTimeoutSec())
	s.st.remember(req.GetTxnId(), fp, resp)
	if err := s.st.save(); err != nil {
		s.log.Error("persist state", "err", err)
	}
	return resp, nil
}

// confirmLocked makes the pending transaction the confirmed baseline.
func (s *Service) confirmLocked() error {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	txn := s.st.meta.PendingTxnID
	s.st.confirm = proto.Clone(s.st.desired).(*vrxv1.DesiredState)
	s.st.meta.ConfirmedManaged = append([]string(nil), s.st.meta.Managed...)
	s.st.meta.LastTxnID = txn
	s.st.meta.PendingTxnID = ""
	s.st.meta.ConfirmDeadline = nil
	s.refreshSnapshotLocked()
	s.log.Info("transaction confirmed", "txn_id", txn)
	return s.st.save()
}

// applyLocked runs one transaction (caller holds txn).
func (s *Service) applyLocked(ctx context.Context, m mode, txnID string, ds *vrxv1.DesiredState, domains []string, confirmSec uint32) *vrxv1.ApplyResponse {
	if ds == nil {
		ds = &vrxv1.DesiredState{}
	}
	start := s.now()
	s.setReconciling(true)
	defer s.setReconciling(false)
	s.bus.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_RECONCILE_START, TxnId: txnID, Message: fmt.Sprintf("%s %v", modeName(m), domains)})
	log := s.log.With("txn_id", txnID, "mode", modeName(m), "domains", domains)
	log.Info("reconcile start")
	flushClaims, endClaims := s.claimsBatch() // TD-11c: one keyed-claim batch per transaction
	defer endClaims()

	resp := &vrxv1.ApplyResponse{TxnId: txnID}
	if s.beforeTxn != nil {
		s.beforeTxn()
	}
	pj := project(ds, domains, s.resolveVRF, s.netdevKind)
	var res *scheduler.TxnResult
	if pj.hasErrors() {
		resp.Status = vrxv1.ApplyStatus_APPLY_STATUS_FAILED
		resp.Validation = report(txnID, pj, nil)
		resp.Message = "validation failed"
		resp.Summary = &vrxv1.ApplySummary{}
	} else {
		// S1 (TD-8): the sources in sync join the transaction; one that makes it fail is left out.
		view := ds // resync and revert apply the stored document itself
		if m == modeTxn && len(s.activeSources()) > 0 {
			view = mergeDomains(s.st.desired, ds, domains)
		}
		var left []leftOut
		res, left = s.applySources(ctx, pj.kvs, scopeOf(domains), domains, view, scheduler.ApplyOptions{Resync: m != modeTxn})
		fillResponse(resp, res, pj)
		s.leaveOutLocked(resp, txnID, left, log)
	}
	resp.AppliedAt = timestamppb.New(s.now())

	// TD-11c (review F2): the claims reach the snapshot before the outcome is recorded. A failed write
	// is never APPLIED: DEGRADED, so the desired state is not merged and the API re-applies running;
	// the records stay in memory and in the journal and are written at the next transaction end.
	claimsErr := flushClaims()
	if claimsErr != nil {
		log.Error("persist claim stores", "err", claimsErr)
		claimsNotPersisted(resp, claimsErr)
	}
	switch resp.Status {
	case vrxv1.ApplyStatus_APPLY_STATUS_APPLIED:
		s.setDegraded(false, "")
		switch m {
		case modeTxn:
			s.st.desired = mergeDomains(s.st.desired, ds, domains)
			s.st.meta.Managed = union(s.st.meta.Managed, domains)
			if confirmSec > 0 {
				deadline := start.Add(time.Duration(confirmSec) * time.Second)
				resp.ConfirmDeadline = timestamppb.New(deadline)
				s.st.meta.PendingTxnID = txnID
				s.st.meta.ConfirmDeadline = &deadline
				s.armTimerLocked(txnID, deadline)
			} else {
				s.st.confirm = proto.Clone(s.st.desired).(*vrxv1.DesiredState)
				s.st.meta.ConfirmedManaged = append([]string(nil), s.st.meta.Managed...)
				s.st.meta.LastTxnID = txnID
			}
		case modeRevert:
			s.st.meta.Managed = domains
			s.st.meta.ConfirmedManaged = domains
			s.st.meta.PendingTxnID = ""
			s.st.meta.ConfirmDeadline = nil
			s.st.meta.Reverting = false
		}
	case vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED:
		s.setDegraded(true, resp.GetMessage())
	}
	if claimsErr != nil { // a rolled-back or refused transaction keeps its status, the agent is DEGRADED
		s.setDegraded(true, resp.GetMessage())
	}
	s.refreshSnapshotLocked()
	if err := s.st.save(); err != nil {
		log.Error("persist state", "err", err)
	}

	d := s.now().Sub(start)
	s.metrics.observe(resp.GetStatus(), d, resp.GetSummary())
	if names, n := s.sched.WriteOnly(); n > 0 || len(names) > 0 {
		s.metrics.setWriteOnly(len(names), n)
	}
	s.metrics.setObjects(len(pj.kvs))
	s.mu.Lock()
	s.lastReconcileAt = s.now()
	s.mu.Unlock()
	s.bus.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_RECONCILE_DONE, TxnId: txnID, Summary: resp.GetSummary(), Message: resp.GetStatus().String()})
	var reapplied int
	if res != nil {
		reapplied = res.Reapplied
	}
	s.lastResp = resp
	log.Info("reconcile done", "status", resp.GetStatus().String(), "summary", resp.GetSummary().String(), "reapplied", reapplied, "duration", d, "err", resp.GetMessage())
	return resp
}

func modeName(m mode) string {
	switch m {
	case modeResync:
		return "resync"
	case modeRevert:
		return "revert"
	default:
		return "apply"
	}
}

func (s *Service) setReconciling(v bool) {
	s.mu.Lock()
	s.reconciling = v
	s.mu.Unlock()
}

func (s *Service) setDegraded(v bool, why string) {
	s.mu.Lock()
	was := s.degraded
	s.degraded = v
	s.mu.Unlock()
	s.metrics.setDegraded(v)
	if v && !was {
		s.bus.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_DEGRADED, Message: why})
		s.log.Error("agent degraded", "why", why)
	}
}

// ---- confirm timer and resync -----------------------------------------------------------

func (s *Service) armTimerLocked(txnID string, deadline time.Time) {
	if s.timer != nil {
		s.timer.Stop()
	}
	d := deadline.Sub(s.now())
	if d < 0 {
		d = 0
	}
	s.timer = time.AfterFunc(d, func() { s.revert(txnID) })
	s.log.Info("confirm timer armed", "txn_id", txnID, "deadline", deadline.UTC().Format(time.RFC3339), "in", d)
}

// revert re-applies the confirmed baseline when txnID is still pending (confirm timeout).
func (s *Service) revert(txnID string) {
	_ = s.lock(context.Background())
	defer s.unlock()
	s.revertLocked(txnID)
}

func (s *Service) revertLocked(txnID string) {
	if s.st.meta.PendingTxnID == "" || s.st.meta.PendingTxnID != txnID {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = nil
	first := !s.st.meta.Reverting
	// H3: make the revert durable BEFORE attempting it: from now on the stored desired state is the
	// confirmed baseline and the transaction stays pending with "revert owed", so a failed attempt
	// (VPP down, rollback) is retried by every resync and nothing re-applies the unconfirmed config.
	s.st.desired = proto.Clone(s.st.confirm).(*vrxv1.DesiredState)
	s.st.meta.Managed = union(s.st.meta.Managed, s.st.meta.ConfirmedManaged)
	s.st.meta.Reverting = true
	s.refreshSnapshotLocked()
	if err := s.st.save(); err != nil {
		s.log.Error("persist state", "err", err)
	}
	if first {
		s.log.Warn("confirm timeout: reverting to the last confirmed state", "txn_id", txnID)
		s.bus.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_CONFIRM_REVERTED, TxnId: txnID, Message: "confirm timeout expired; reverting to the last confirmed state"})
		s.metrics.reverts.Add(1)
	} else {
		s.log.Warn("retrying the owed confirm revert", "txn_id", txnID)
	}
	var domains []string
	for _, d := range s.st.meta.Managed {
		if implemented(d) {
			domains = append(domains, d)
		}
	}
	resp := s.applyLocked(context.Background(), modeRevert, "", s.st.desired, domains, 0)
	if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		d := s.scheduleRetryLocked(txnID)
		s.setDegraded(true, fmt.Sprintf("confirm revert of %s failed, retrying in %v (and on every resync; a new Apply supersedes it): %s", txnID, d, resp.GetMessage()))
	} else {
		s.stopRetryLocked()
	}
	s.refreshSnapshotLocked()
	if err := s.st.save(); err != nil {
		s.log.Error("persist state", "err", err)
	}
}

// Revert retry backoff (N2): independent of VPP reconnects.
const (
	revertRetryFloor = 5 * time.Second
	revertRetryMax   = 60 * time.Second
)

// scheduleRetryLocked arms the owed-revert retry timer with exponential backoff and returns the delay.
func (s *Service) scheduleRetryLocked(txnID string) time.Duration {
	if s.retryDelay == 0 {
		s.retryDelay = s.retryMin
	} else if s.retryDelay *= 2; s.retryDelay > s.retryMax {
		s.retryDelay = s.retryMax
	}
	if s.retryTimer != nil {
		s.retryTimer.Stop()
	}
	d := s.retryDelay
	s.retryTimer = time.AfterFunc(d, func() { s.revert(txnID) })
	return d
}

func (s *Service) stopRetryLocked() {
	if s.retryTimer != nil {
		s.retryTimer.Stop()
		s.retryTimer = nil
	}
	s.retryDelay = 0
}

// Resync re-applies the stored desired state of every managed domain (agent start, VPP
// reconnect), then resumes or fires a pending confirm timer. It emits RECONCILE_START/DONE with
// an empty txn_id.
func (s *Service) Resync(ctx context.Context) *vrxv1.ApplyResponse {
	if err := s.lock(ctx); err != nil {
		return nil
	}
	defer s.unlock()
	// A pending transaction whose deadline passed (while the agent was down, or whose revert is
	// owed): converge straight to the confirmed baseline, never re-apply the unconfirmed config.
	if p := s.st.meta.PendingTxnID; p != "" && (s.st.meta.Reverting || (s.st.meta.ConfirmDeadline != nil && !s.st.meta.ConfirmDeadline.After(s.now()))) {
		s.revertLocked(p)
		return s.lastResp
	}
	var domains []string
	for _, d := range s.st.meta.Managed {
		if implemented(d) {
			domains = append(domains, d)
		}
	}
	resp := s.applyLocked(ctx, modeResync, "", s.st.desired, domains, 0)
	if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		s.setDegraded(true, "resync failed: "+resp.GetMessage())
	}
	if p := s.st.meta.PendingTxnID; p != "" && s.st.meta.ConfirmDeadline != nil && s.timer == nil {
		s.armTimerLocked(p, *s.st.meta.ConfirmDeadline)
	}
	return resp
}

// ---- Retrieve / DryRun / Health -------------------------------------------------------------

// Retrieve implements the Retrieve RPC.
func (s *Service) Retrieve(ctx context.Context, req *vrxv1.RetrieveRequest) (*vrxv1.RetrieveResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if err := checkSubsystems(req.GetSubsystems()); err != nil {
		return nil, err
	}
	domains := union(req.GetSubsystems(), nil)
	if len(domains) == 0 {
		domains = implementedDomains()
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	// Always dump the VRFs too: they name the tables of interface bindings and routes.
	kvs, err := s.sched.Retrieve(ctx, scopeOf(union(domains, []string{"vrfs"})))
	if err != nil {
		if errors.Is(err, vpp.ErrDisconnected) {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "retrieve: %v", err)
	}
	var live desired.Live
	if in := union(domains, nil); contains(in, subsystems.Interfaces) {
		tbl, err := s.interfaceTable(ctx)
		if err != nil {
			if errors.Is(err, vpp.ErrDisconnected) {
				return nil, status.Error(codes.Unavailable, err.Error())
			}
			return nil, status.Errorf(codes.Internal, "retrieve: %v", err)
		}
		live = tbl
	}
	s.mu.Lock()
	stored := s.storedIfs
	s.mu.Unlock()
	ds := assemble(kvs, domains, func(id uint32) (string, bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for n, v := range s.vrfIDs {
			if v == id {
				return n, true
			}
		}
		return "", false
	}, stored, live)
	s.addDescriptions(ds)
	return &vrxv1.RetrieveResponse{DesiredState: ds, Subsystems: domains, Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// addDescriptions fills VRF and static-route descriptions — VPP cannot store them (D-073b) —
// from the stored desired state for objects that actually exist.
func (s *Service) addDescriptions(ds *vrxv1.DesiredState) {
	s.mu.Lock()
	vrfDesc, routeDesc := s.vrfDesc, s.routeDesc
	s.mu.Unlock()
	for name, v := range ds.GetVrfs() {
		if d, ok := vrfDesc[name]; ok {
			v.Description = proto.String(d)
		}
	}
	for _, r := range ds.GetRouting().GetStatic() {
		if d, ok := routeDesc[r.GetVrf()+"|"+r.GetPrefix()]; ok {
			r.Description = proto.String(d)
		}
	}
}

// DryRun implements the DryRun RPC: validation + plan, nothing applied, no events.
func (s *Service) DryRun(ctx context.Context, req *vrxv1.DryRunRequest) (*vrxv1.ValidationReport, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if err := checkSubsystems(req.GetSubsystems()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	domains := authoritative(req.GetDesiredState(), req.GetSubsystems())
	pj := project(req.GetDesiredState(), domains, s.resolveVRF, s.netdevKind)
	if pj.hasErrors() {
		return report(req.GetTxnId(), pj, nil), nil
	}
	plan, err := s.planSources(ctx, pj, domains, req.GetDesiredState())
	if err != nil {
		if errors.Is(err, vpp.ErrDisconnected) {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "plan: %v", err)
	}
	return report(req.GetTxnId(), pj, plan), nil
}

// Health implements the Health RPC (no VPP round trip).
func (s *Service) Health() *vrxv1.HealthResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := &vrxv1.HealthResponse{
		AgentVersion: s.version, VppConnected: s.vpp.Connected(), Owner: s.owner,
		Subsystems: implementedDomains(), LastTxnId: s.lastTxn, PendingConfirmTxnId: s.pendingTxn,
		Degraded: s.degraded, ReconcileInProgress: s.reconciling,
	}
	if h.VppConnected {
		h.VppVersion = s.vppVersion
	}
	if !s.deadline.IsZero() && s.pendingTxn != "" {
		h.ConfirmDeadline = timestamppb.New(s.deadline)
	}
	if !s.lastReconcileAt.IsZero() {
		h.LastReconcileAt = timestamppb.New(s.lastReconcileAt)
	}
	return h
}

// SetVPPVersion records the version string reported by show_version.
func (s *Service) SetVPPVersion(v string) {
	s.mu.Lock()
	s.vppVersion = v
	s.mu.Unlock()
}

// events returns the event bus (StreamEvents, link events).
func (s *Service) events() *bus { return s.bus }

// Close stops the confirm timer (the pending state stays persisted; a restart resumes it).
func (s *Service) Close() {
	_ = s.lock(context.Background())
	defer s.unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.stopRetryLocked()
	s.closed = true
	for _, ds := range s.sources {
		s.stopSourceRetryLocked(ds)
	}
}

// ---- response building ------------------------------------------------------------------------

var opPB = map[string]vrxv1.ApplyOperation{
	scheduler.OpCreate:   vrxv1.ApplyOperation_APPLY_OPERATION_CREATE,
	scheduler.OpUpdate:   vrxv1.ApplyOperation_APPLY_OPERATION_UPDATE,
	scheduler.OpDelete:   vrxv1.ApplyOperation_APPLY_OPERATION_DELETE,
	scheduler.OpRecreate: vrxv1.ApplyOperation_APPLY_OPERATION_RECREATE,
}

var codePB = map[scheduler.ResultCode]vrxv1.ObjectResultCode{
	scheduler.CodeOK:                vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_OK,
	scheduler.CodeFailed:            vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_FAILED,
	scheduler.CodeSkipped:           vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_SKIPPED,
	scheduler.CodeReverted:          vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_REVERTED,
	scheduler.CodeRevertFailed:      vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_REVERT_FAILED,
	scheduler.CodeDependencyMissing: vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_DEPENDENCY_MISSING,
	scheduler.CodeInvalid:           vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_INVALID,
}

var statusPB = map[scheduler.Outcome]vrxv1.ApplyStatus{
	scheduler.OutcomeApplied:    vrxv1.ApplyStatus_APPLY_STATUS_APPLIED,
	scheduler.OutcomeFailed:     vrxv1.ApplyStatus_APPLY_STATUS_FAILED,
	scheduler.OutcomeRolledBack: vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK,
	scheduler.OutcomeDegraded:   vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED,
}

func fillResponse(resp *vrxv1.ApplyResponse, res *scheduler.TxnResult, pj *projected) {
	resp.Status = statusPB[res.Outcome]
	resp.Summary = summaryPB(res.Summary)
	if res.Err != nil {
		resp.Message = res.Err.Error()
	}
	for _, r := range res.Results {
		or := &vrxv1.ObjectResult{Key: string(r.Key), Op: opPB[r.Op], Code: codePB[r.Code], Pointer: pj.pointers[r.Key], Subsystem: domainOf(r.Key.Descriptor())}
		if r.Err != nil {
			or.Message = r.Err.Error()
		}
		resp.Results = append(resp.Results, or)
	}
	if res.Outcome == scheduler.OutcomeFailed && res.Plan != nil {
		resp.Validation = report(resp.GetTxnId(), pj, res.Plan)
	}
}

// report builds a ValidationReport from projection issues and (optionally) a plan.
func report(txnID string, pj *projected, plan *scheduler.TxnPlan) *vrxv1.ValidationReport {
	rep := &vrxv1.ValidationReport{TxnId: txnID, Summary: &vrxv1.ApplySummary{}}
	for _, is := range pj.issues {
		rep.Errors = append(rep.Errors, &vrxv1.ValidationIssue{Pointer: is.pointer, Message: is.message, Severity: is.severity, Rule: is.rule})
	}
	if plan != nil {
		for _, is := range plan.Issues {
			rule := "agent.invalid"
			if is.Code == scheduler.CodeDependencyMissing {
				rule = "agent.dependency-missing"
			}
			rep.Errors = append(rep.Errors, &vrxv1.ValidationIssue{Pointer: pj.pointers[is.Key], Message: is.String(), Severity: vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR, Rule: rule})
		}
	}
	sort.SliceStable(rep.Errors, func(i, j int) bool {
		a, b := rep.Errors[i], rep.Errors[j]
		ea, eb := a.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR, b.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR
		if ea != eb {
			return ea
		}
		return a.GetPointer() < b.GetPointer()
	})
	rep.Ok = true
	for _, e := range rep.Errors {
		if e.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			rep.Ok = false
		}
	}
	if plan != nil && rep.Ok {
		for _, op := range plan.Ops {
			rep.Plan = append(rep.Plan, &vrxv1.ObjectResult{Key: string(op.Key), Op: opPB[op.Op], Pointer: pj.pointers[op.Key], Subsystem: domainOf(op.Key.Descriptor())})
		}
		rep.Summary = summaryPB(plan.Summary())
	}
	return rep
}

// claimsBatch opens one batch of the keyed claim stores for a transaction (TD-11c, D-133: claims are
// journaled before the VPP write and compacted into the snapshot by end) and returns end and a
// function the caller defers: it ends the batch when the transaction panicked before end ran
// (review F4), so the stores never stay in batch mode. applyLocked and TD-8's syncLocked both use it
// (review F5). No hook (unit tests of other domains): both are no-ops.
func (s *Service) claimsBatch() (end func() error, cleanup func()) {
	if s.claimsTxn == nil {
		return func() error { return nil }, func() {}
	}
	flush, ended := s.claimsTxn(), false
	end = func() error { ended = true; return flush() }
	cleanup = func() {
		if !ended {
			_ = flush()
		}
	}
	return end, cleanup
}

// claimsNotPersisted records a failed end of the claim batch in resp (review F2): APPLIED becomes
// DEGRADED — the caller must not record the outcome as applied — and the message says why. The
// records stay in memory and in the journal; the next transaction end writes them.
func claimsNotPersisted(resp *vrxv1.ApplyResponse, err error) {
	msg := "claim stores not persisted: " + err.Error()
	if resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		resp.Status = vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED
	}
	if resp.GetMessage() != "" {
		msg = resp.GetMessage() + "; " + msg
	}
	resp.Message = msg
}
