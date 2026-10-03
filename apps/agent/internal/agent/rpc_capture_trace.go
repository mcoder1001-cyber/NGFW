package agent

// F-capture-trace: the capture case of Action plus CaptureList / CaptureRead / CaptureDelete. The work is in
// internal/actions/capture-trace; this file maps it onto gRPC. Paths and caps: VRX_CAPTURE_DIR (default
// /var/lib/vrx/captures), VRX_CAPTURE_VPP_DIR (where VPP writes, default /tmp), VRX_CAPTURE_MAX_FILES (10),
// VRX_CAPTURE_MAX_BYTES (500 MiB); the BPF filter needs the globals owner (VRX_GLOBALS_OWNER, D-071).

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	capturetrace "ngfw/agent/internal/actions/capture-trace"
	"ngfw/agent/internal/vpp"
)

var captureManagers sync.Map // *Service → *captureEntry

type captureEntry struct {
	once sync.Once
	m    *capturetrace.Manager
	err  error
}

// captureGlobalsOwner is retained for the existing CNAT RPCs. Capture itself uses
// the role resolved at agent startup, not this legacy environment lookup.
func captureGlobalsOwner(owner string) bool {
	g := owner == "vrx"
	switch strings.ToLower(os.Getenv("VRX_GLOBALS_OWNER")) {
	case "1", "true", "yes":
		g = true
	case "0", "false", "no":
		g = false
	}
	return g
}

func (g *server) captures() (*capturetrace.Manager, error) {
	v, _ := captureManagers.LoadOrStore(g.svc, &captureEntry{})
	e := v.(*captureEntry)
	e.once.Do(func() {
		files, _ := strconv.Atoi(os.Getenv("VRX_CAPTURE_MAX_FILES"))
		bytes, _ := strconv.ParseInt(os.Getenv("VRX_CAPTURE_MAX_BYTES"), 10, 64)
		cfg := g.svc.captureConfig
		cfg.Client, cfg.Owner, cfg.Logger = g.svc.vpp, g.svc.owner, g.svc.log
		if dir := os.Getenv("VRX_CAPTURE_DIR"); dir != "" {
			cfg.Dir = dir
		}
		cfg.VPPDir = os.Getenv("VRX_CAPTURE_VPP_DIR")
		cfg.MaxFiles, cfg.MaxBytes, cfg.OwnedFilter = files, bytes, g.svc.ownedTraceFilter
		e.m, e.err = capturetrace.New(cfg)
	})
	if e.err != nil {
		return nil, status.Error(codes.FailedPrecondition, e.err.Error())
	}
	return e.m, nil
}

// actionCapture runs one pcap capture (Action, member 3).
func (g *server) actionCapture(req *vrxv1.CaptureAction, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	plan, err := capturetrace.Validate(req)
	if err != nil {
		return captureStatus(err)
	}
	if !g.svc.vpp.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	m, err := g.captures()
	if err != nil {
		return err
	}
	if g.log != nil {
		g.log.Info("action capture", "interface", plan.Interface, "direction", plan.Direction(), "bpf", plan.BPF != "", "seconds", plan.Seconds)
	}
	return captureStatus(m.Run(stream.Context(), plan, stream.Send))
}

// CaptureList implements vrx.v1.Dataplane/CaptureList.
func (g *server) CaptureList(ctx context.Context, req *vrxv1.CaptureListRequest) (*vrxv1.CaptureListResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	m, err := g.captures()
	if err != nil {
		return nil, err
	}
	out, err := m.List(ctx)
	return out, captureStatus(err)
}

// CaptureRead implements vrx.v1.Dataplane/CaptureRead.
func (g *server) CaptureRead(req *vrxv1.CaptureReadRequest, stream grpc.ServerStreamingServer[vrxv1.CaptureChunk]) error {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return err
	}
	m, err := g.captures()
	if err != nil {
		return err
	}
	return captureStatus(m.Read(req.GetId(), stream.Send))
}

// CaptureDelete implements vrx.v1.Dataplane/CaptureDelete.
func (g *server) CaptureDelete(_ context.Context, req *vrxv1.CaptureDeleteRequest) (*vrxv1.CaptureDeleteResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	m, err := g.captures()
	if err != nil {
		return nil, err
	}
	n, err := m.Delete(req.GetId())
	if err != nil {
		return nil, captureStatus(err)
	}
	return &vrxv1.CaptureDeleteResponse{Size: n}, nil
}

func captureStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, capturetrace.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, capturetrace.ErrBusy):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, capturetrace.ErrGlobals), errors.Is(err, capturetrace.ErrRunning):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, capturetrace.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, vpp.ErrDisconnected):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

// recoverCaptures initializes and repairs interrupted captures before any RPC is needed.
func (s *Service) recoverCaptures(ctx context.Context) error {
	m, err := (&server{svc: s}).captures()
	if err != nil {
		return err
	}
	return m.Recover(ctx)
}

func (s *Service) ownedTraceFilter(context.Context) (bpf, filterFunction string) {
	return "", ""
}
