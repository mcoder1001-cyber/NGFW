package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"ngfw/cli/internal/api"
	"ngfw/cli/internal/cpath"
	"strconv"
	"strings"
)

func showNativeIPsec(ctx context.Context, a *App, args []cpath.Token) error {
	q := url.Values{}
	if len(args) > 0 && (args[0].Quoted || (args[0].Text != "limit" && args[0].Text != "offset")) {
		q.Set("tunnel", args[0].Text)
		args = args[1:]
	}
	for len(args) > 0 {
		if len(args) < 2 || args[0].Quoted || (args[0].Text != "limit" && args[0].Text != "offset") {
			return usagef("show ipsec sa [<tunnel>] [limit <1..1000>] [offset <0..1000000>]")
		}
		flag, value := args[0].Text, args[1].Text
		if q.Has(flag) {
			return usagef("%s may be given only once", flag)
		}
		n, e := strconv.ParseUint(value, 10, 32)
		if e != nil || (flag == "limit" && (n < 1 || n > 1000)) || (flag == "offset" && n > 1000000) {
			return usagef("invalid IPsec %s value", flag)
		}
		q.Set(flag, strconv.FormatUint(n, 10))
		args = args[2:]
	}
	return showIPsecState(ctx, a, "Ipsec_sas", q)
}

func showNativeIPsecTunnels(ctx context.Context, a *App, args []cpath.Token) error {
	if len(args) > 1 {
		return usagef("show ipsec tunnels [<tunnel>]")
	}
	q := url.Values{}
	if len(args) == 1 {
		q.Set("tunnel", args[0].Text)
	}
	return showIPsecState(ctx, a, "Ipsec_tunnels", q)
}
func showIPsecState(ctx context.Context, a *App, op string, q url.Values) error {
	raw, err := a.call(ctx, api.Call{Op: op, Query: q}, nil)
	if err != nil {
		return err
	}
	return a.emit(raw, func(w io.Writer) {
		var out any
		if json.Unmarshal(raw, &out) == nil {
			formatted, _ := json.MarshalIndent(out, "", "  ")
			fmt.Fprintln(w, string(formatted))
		}
	})
}

func init() {
	for _, operation := range []string{"initiate", "rekey", "delete-sa"} {
		op := operation
		args := "<tunnel>"
		if op != "initiate" {
			args += " <spi>"
		}
		register(&Command{Words: []string{"ipsec", op}, Args: args, Where: inOp, Summary: "Native IKE runtime action on an owned tunnel", Ops: []string{"Ikev2Native_action"}, Run: func(ctx context.Context, a *App, tokens []cpath.Token) error {
			return nativeIPsecAction(ctx, a, op, tokens)
		}})
	}
}
func nativeIPsecAction(ctx context.Context, a *App, operation string, args []cpath.Token) error {
	count := 1
	if operation != "initiate" {
		count = 2
	}
	if len(args) != count {
		return usagef("ipsec %s <tunnel> [<spi>]", operation)
	}
	body := map[string]any{"ikeSpi": "0", "childSpi": uint32(0)}
	if count == 2 {
		bits := 64
		if operation == "rekey" {
			bits = 32
		}
		text := args[1].Text
		base := 10
		if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
			base = 16
			text = text[2:]
		}
		spi, e := strconv.ParseUint(text, base, bits)
		if e != nil || spi == 0 {
			return usagef("SPI must be a nonzero unsigned %d-bit decimal or 0x-prefixed hexadecimal number", bits)
		}
		if operation == "rekey" {
			body["childSpi"] = uint32(spi)
		} else {
			body["ikeSpi"] = strconv.FormatUint(spi, 10)
		}
	}
	raw, e := a.call(ctx, api.Call{Op: "Ikev2Native_action", Params: map[string]string{"tunnel": args[0].Text, "operation": operation}, Body: body}, nil)
	if e != nil {
		return e
	}
	return a.emit(raw, func(w io.Writer) { fmt.Fprintln(w, string(raw)) })
}
