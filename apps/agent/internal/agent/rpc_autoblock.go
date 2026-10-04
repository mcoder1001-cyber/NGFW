package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/autoblock"
	"ngfw/agent/internal/renderers"
)

// autoBlockRuntime is guarded by the service transaction lock. PostgreSQL owns
// the set; this private cache closes the restart gap until the API republishes it.
type autoBlockRuntime struct {
	entries     []*ngfwv1.AutoBlockRuntimeEntry
	dirty       bool
	fingerprint string
}

func (s *Service) loadAutoBlock() error {
	b, err := os.ReadFile(filepath.Join(s.st.dir, "auto-block.json")) //nolint:gosec // fixed path in agent state directory
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var req ngfwv1.AutoBlockSetRequest
	if err := protojson.Unmarshal(b, &req); err != nil {
		return fmt.Errorf("auto-block cache: %w", err)
	}
	if req.GetOwner() != s.owner {
		return fmt.Errorf("auto-block cache owner mismatch")
	}
	if err := autoblock.Validate(req.GetEntries(), s.now()); err != nil {
		return err
	}
	s.autoBlock.entries = req.GetEntries()
	s.autoBlock.dirty = true
	return nil
}

func (g *server) AutoBlockSet(ctx context.Context, req *ngfwv1.AutoBlockSetRequest) (*ngfwv1.AutoBlockSetResponse, error) {
	return g.svc.AutoBlockSet(ctx, req)
}

// AutoBlockSet atomically validates and persists a full authoritative snapshot.
// Failed enforcement remains desired and is retried by the runtime loop/resync.
func (s *Service) AutoBlockSet(ctx context.Context, req *ngfwv1.AutoBlockSetRequest) (*ngfwv1.AutoBlockSetResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if err := autoblock.Validate(req.GetEntries(), s.now()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	defer s.unlock()
	if s.closed {
		return nil, status.Error(codes.Unavailable, "agent stopped")
	}
	snapshot := proto.Clone(req).(*ngfwv1.AutoBlockSetRequest)
	b, err := protojson.Marshal(snapshot)
	if err != nil {
		return nil, status.Error(codes.Internal, "cannot encode runtime snapshot")
	}
	if err := renderers.WriteFiles(renderers.Files{filepath.Join(s.st.dir, "auto-block.json"): {Mode: 0o600, Content: b}}); err != nil {
		return nil, status.Error(codes.Internal, "cannot persist runtime snapshot")
	}
	s.autoBlock.entries = snapshot.GetEntries()
	s.autoBlock.dirty = true
	n, err := s.reconcileAutoBlockLocked(ctx)
	if err != nil {
		return nil, err
	}
	return &ngfwv1.AutoBlockSetResponse{ActiveEntries: uint32(n)}, nil //nolint:gosec // bounded by autoblock.MaxEntries
}

func (s *Service) reconcileAutoBlockLocked(ctx context.Context) (int, error) {
	view, err := autoblock.Overlay(s.st.desired, s.autoBlock.entries, s.now())
	if err != nil {
		return 0, status.Error(codes.FailedPrecondition, err.Error())
	}
	n := 0
	if view.GetSecurity().GetAutoBlock().GetEnabled() {
		n = len(view.GetAcl().GetGlobalBlocking().GetLists()[autoblock.ListName].GetEntries())
	}
	// Use the existing transactional scheduler with the unmodified stored document.
	// Only ACL-domain objects are touched; no commit, revision or user config update.
	resp, _ := s.applyLocked(ctx, modeRuntime, "", s.st.desired, []string{"acl"}, 0)
	if resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		return 0, status.Errorf(codes.Unavailable, "auto-block enforcement: %s", resp.GetMessage())
	}
	s.autoBlock.dirty = false
	s.autoBlock.fingerprint = fingerprint(view, []string{"acl"})
	return n, nil
}

// Expiry is enforced even when the API is down. Reconnect/full resync use the same
// projection, recreating runtime ACLs after VPP object loss.
func (a *Agent) watchAutoBlock(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			func() {
				c, cancel := context.WithTimeout(ctx, a.svc.txnTimeout)
				defer cancel()
				if err := a.svc.lock(c); err != nil {
					return
				}
				defer a.svc.unlock()
				if a.svc.closed {
					return
				}
				view, err := autoblock.Overlay(a.svc.st.desired, a.svc.autoBlock.entries, a.svc.now())
				if err != nil {
					return
				}
				fp := fingerprint(view, []string{"acl"})
				if !a.svc.autoBlock.dirty && fp == a.svc.autoBlock.fingerprint {
					return
				}
				if _, err := a.svc.reconcileAutoBlockLocked(c); err != nil {
					a.svc.log.Warn("auto-block reconciliation failed", "err", err)
				}
			}()
		}
	}
}
