package subsystems

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	pppoedesc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
)

const pppoeCarrierHelper = "/usr/lib/ngfw/pppoe-carrier.py"

type pppoeCarrierHost struct{ runner renderers.Runner }

func (h *pppoeCarrierHost) call(ctx context.Context, target any, args ...string) error {
	if h == nil || h.runner == nil {
		return errors.New("PPPoE kernel helper is unavailable")
	}
	out, err := h.runner.Run(ctx, renderers.Command{Path: pppoe.Python3Bin,
		Args: append([]string{"-I", pppoeCarrierHelper}, args...), Timeout: 15 * time.Second})
	if err != nil {
		// Helper stderr may contain kernel/daemon text. Do not forward arbitrary
		// process output into RPC state, logs or the audit trail.
		return errors.New("PPPoE kernel helper operation failed")
	}
	if len(out.Stdout) > 1<<20 {
		return errors.New("PPPoE kernel helper receipt exceeds its bound")
	}
	if target == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(out.Stdout))
	if err := dec.Decode(target); err != nil {
		return errors.New("PPPoE kernel helper returned an invalid receipt")
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("PPPoE kernel helper returned trailing receipt data")
	}
	return nil
}

func (h *pppoeCarrierHost) Provision(ctx context.Context, spec pppoe.CarrierSpec) (pppoedesc.CarrierLease, error) {
	var lease pppoedesc.CarrierLease
	if err := spec.Validate(); err != nil {
		return lease, err
	}
	body, err := json.Marshal(spec)
	if err != nil {
		return lease, err
	}
	err = h.call(ctx, &lease, "provision", spec.Owner, spec.Logical, "--spec-json", string(body))
	if err == nil {
		err = lease.Validate()
	}
	if err == nil && !reflect.DeepEqual(lease.Spec, spec) {
		err = errors.New("PPPoE provision specification changed")
	}
	return lease, err
}

func (h *pppoeCarrierHost) Inventory(ctx context.Context, owner string) ([]pppoedesc.CarrierLease, error) {
	var leases []pppoedesc.CarrierLease
	if err := h.call(ctx, &leases, "list", owner); err != nil {
		return nil, err
	}
	if len(leases) > 1024 {
		return nil, errors.New("PPPoE carrier inventory exceeds its bound")
	}
	for _, lease := range leases {
		if err := lease.Validate(); err != nil {
			return nil, err
		}
		if lease.Spec.Owner != owner {
			return nil, errors.New("foreign PPPoE carrier in inventory")
		}
	}
	return leases, nil
}

func (h *pppoeCarrierHost) Remove(ctx context.Context, lease pppoedesc.CarrierLease) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	return h.call(ctx, nil, "delete", lease.Token, lease.Generation)
}

func (h *pppoeCarrierHost) Prepare(ctx context.Context, lease pppoedesc.CarrierLease, physicalMAC string) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	s := lease.Spec
	var result pppoedesc.CarrierLease
	if err := h.call(ctx, &result, "prepare", lease.Token, lease.Generation,
		"--physical-mac", physicalMAC, "--local4", s.Host4, "--peer4", s.Peer4,
		"--local6", s.Host6, "--peer6", s.Peer6); err != nil {
		return err
	}
	if err := result.Validate(); err != nil {
		return err
	}
	if !sameCarrierLease(lease, result) {
		return errors.New("PPPoE namespace changed during preparation")
	}
	return nil
}

func sameCarrierLease(a, b pppoedesc.CarrierLease) bool {
	return a.Token == b.Token && a.Generation == b.Generation && a.Boot == b.Boot &&
		reflect.DeepEqual(a.Namespace, b.Namespace) && reflect.DeepEqual(a.Spec, b.Spec)
}

type carrierVerification struct {
	Verified   bool              `json:"verified"`
	Token      string            `json:"token"`
	Generation string            `json:"generation"`
	Boot       string            `json:"boot"`
	Namespace  []uint64          `json:"namespace"`
	Links      map[string]uint32 `json:"links"`
	MTU        uint32            `json:"mtu"`
}

func (h *pppoeCarrierHost) Verify(ctx context.Context, lease pppoedesc.CarrierLease) (carrierVerification, error) {
	var result carrierVerification
	if err := lease.Validate(); err != nil {
		return result, err
	}
	if err := h.call(ctx, &result, "verify", lease.Token, lease.Generation); err != nil {
		return result, err
	}
	if !result.Verified || result.Token != lease.Token || result.Generation != lease.Generation || result.Boot != lease.Boot ||
		!reflect.DeepEqual(result.Namespace, lease.Namespace) || result.MTU != lease.Spec.MTU {
		return result, errors.New("PPPoE forwarding identity/MTU verification failed")
	}
	for _, name := range []string{lease.Spec.RawHost(), lease.Spec.TransitHost(), "ppp0"} {
		if result.Links[name] == 0 {
			return result, fmt.Errorf("PPPoE forwarding interface identity missing")
		}
	}
	return result, nil
}

func (h *pppoeCarrierHost) Configure(ctx context.Context, lease pppoedesc.CarrierLease) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	return h.call(ctx, nil, "configure", lease.Token, lease.Generation)
}

func (h *pppoeCarrierHost) Withdraw(ctx context.Context, lease pppoedesc.CarrierLease) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	return h.call(ctx, nil, "withdraw", lease.Token, lease.Generation)
}
