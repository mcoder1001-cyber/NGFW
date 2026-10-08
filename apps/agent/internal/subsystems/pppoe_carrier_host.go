package subsystems

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

func (h *pppoeCarrierHost) direct(ctx context.Context, target any, args ...string) error {
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

// call crosses the privileged boundary only through the fixed packaged broker.
// The agent may enqueue a finite validated request; it never enters a namespace.
func (h *pppoeCarrierHost) call(ctx context.Context, target any, args ...string) error {
	if len(args) == 0 {
		return errors.New("empty carrier operation")
	}
	request := map[string]any{"op": args[0]}
	token := ""
	switch args[0] {
	case "provision":
		if len(args) != 5 || args[3] != "--spec-json" {
			return errors.New("invalid provision request")
		}
		var spec pppoe.CarrierSpec
		if json.Unmarshal([]byte(args[4]), &spec) != nil || spec.Validate() != nil || spec.Owner != args[1] || spec.Logical != args[2] {
			return errors.New("invalid provision specification")
		}
		token = spec.Token()
		request["owner"], request["logical"], request["spec"] = spec.Owner, spec.Logical, spec
	case "list":
		if len(args) != 2 {
			return errors.New("invalid inventory request")
		}
		digest := sha256.Sum256([]byte("inventory\x00" + args[1]))
		token = "ngp-" + hex.EncodeToString(digest[:6])
		request["owner"] = args[1]
	case "verify", "inspect", "delete", "withdraw", "configure", "prepare":
		if len(args) < 3 {
			return errors.New("incomplete carrier request")
		}
		token = args[1]
		request["token"], request["generation"] = token, args[2]
		if args[0] == "prepare" {
			if len(args) != 13 {
				return errors.New("invalid prepare request")
			}
			request["physical_mac"] = args[4]
			request["transit"] = map[string]string{"local4": args[6], "peer4": args[8], "local6": args[10], "peer6": args[12]}
		} else if args[0] == "configure" {
			if len(args) != 4 {
				return errors.New("missing carrier default route policy")
			}
			request["accept_default_route"] = args[3] == "true"
		} else if len(args) != 3 {
			return errors.New("invalid carrier request")
		}
	default:
		return errors.New("unapproved carrier operation")
	}
	return h.broker(ctx, target, token, request)
}

type carrierBrokerReceipt struct {
	Token   string          `json:"token"`
	Nonce   string          `json:"nonce"`
	Boot    string          `json:"boot"`
	Expires float64         `json:"expires"`
	Hash    string          `json:"request_sha256"`
	OK      bool            `json:"ok"`
	Result  json.RawMessage `json:"result"`
}

func (h *pppoeCarrierHost) broker(ctx context.Context, target any, token string, request any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var nonceBytes [16]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		return err
	}
	nonce := hex.EncodeToString(nonceBytes[:])
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	var queued carrierBrokerReceipt
	if err = h.direct(ctx, &queued, "broker-queue", token, nonce, "--request-json", string(body)); err != nil {
		return err
	}
	if queued.Token != token || queued.Nonce != nonce || queued.Boot == "" || len(queued.Hash) != 64 || queued.Expires <= 0 {
		return errors.New("invalid carrier broker receipt")
	}
	if _, err = h.runner.Run(ctx, renderers.Command{Path: pppoe.SystemctlBin, Args: []string{"start", "ngfw-pppoe-broker@" + token + ".service"}, Timeout: 10 * time.Second}); err != nil {
		return errors.New("carrier broker service failed")
	}
	var result carrierBrokerReceipt
	if err = h.direct(ctx, &result, "broker-result", token, nonce); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if !result.OK || result.Token != queued.Token || result.Nonce != queued.Nonce || result.Boot != queued.Boot || result.Hash != queued.Hash || result.Expires != queued.Expires {
		return errors.New("carrier broker result identity or operation failed")
	}
	if target == nil {
		return nil
	}
	return json.Unmarshal(result.Result, target)
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
	PPPAddresses []string          `json:"ppp_addresses"`
	Verified     bool              `json:"verified"`
	Token        string            `json:"token"`
	Generation   string            `json:"generation"`
	Boot         string            `json:"boot"`
	Namespace    []uint64          `json:"namespace"`
	Links        map[string]uint32 `json:"links"`
	MTU          uint32            `json:"mtu"`
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

func (h *pppoeCarrierHost) Configure(ctx context.Context, lease pppoedesc.CarrierLease, acceptDefault bool) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	return h.call(ctx, nil, "configure", lease.Token, lease.Generation, fmt.Sprint(acceptDefault))
}

func (h *pppoeCarrierHost) Withdraw(ctx context.Context, lease pppoedesc.CarrierLease) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	return h.call(ctx, nil, "withdraw", lease.Token, lease.Generation)
}
