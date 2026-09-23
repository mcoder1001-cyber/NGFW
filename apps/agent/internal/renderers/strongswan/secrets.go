package strongswan

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// Secrets (00-CONTEXT rule 10, D-051; P11 "secret handling contract"). The document carries
// only references "psk/<name>"; Render resolves them through the injected SecretResolver. The
// plaintext then exists in memory, in the VICI load-shared request (never logged; its String
// is redacted) and — as base64 — in vrx-secrets.conf (mode 0600, marked Secret so
// Files.Redacted hides it). It never reaches vrx.conf, strongswan.conf or argv, and Retrieve,
// State and events carry no key material by construction (VICI never returns it).
//
// Redaction is by field, never by text replacement of the secret value (RF-2 review L2: a
// global replace corrupts state that happens to contain the PSK's text and turns GET into an
// oracle): the only secret-bearing text shapes are `secret = …` assignments, which are masked
// in errors and tool output; parse errors never quote lines; malformed secret references are
// not echoed (they may be pasted secrets); resolver errors are the secret store's own text.

// Redacted replaces secrets in everything the renderer returns.
const Redacted = "<redacted>"

// SecretResolver returns the plaintext of a secret reference ("psk/site-a").
type SecretResolver interface {
	Resolve(ctx context.Context, ref string) ([]byte, error)
}

// SecretResolverFunc adapts a function to SecretResolver.
type SecretResolverFunc func(ctx context.Context, ref string) ([]byte, error)

// Resolve implements SecretResolver.
func (f SecretResolverFunc) Resolve(ctx context.Context, ref string) ([]byte, error) {
	return f(ctx, ref)
}

// ErrNoSecretResolver is returned when a tunnel needs a secret and none is configured.
var ErrNoSecretResolver = errors.New("strongswan: no secret resolver configured")

// secretRefRe is the D-051 reference form.
var secretRefRe = regexp.MustCompile(`^(psk|key|cert|password|token)/[A-Za-z0-9_.-]{1,64}$`)

// MaxPSKLen bounds a pre-shared key.
const MaxPSKLen = 1024

// MinPSKLen is the shortest PSK accepted (charon itself accepts shorter ones; a PSK is the
// whole authentication of a site-to-site tunnel).
const MinPSKLen = 8

func checkPSK(v []byte) error {
	switch {
	case len(v) < MinPSKLen:
		return fmt.Errorf("pre-shared key shorter than %d bytes", MinPSKLen)
	case len(v) > MaxPSKLen:
		return fmt.Errorf("pre-shared key longer than %d bytes", MaxPSKLen)
	case string(v) == Redacted:
		return errors.New("pre-shared key is the redaction marker")
	}
	return nil
}

// secretAssignRe masks the value of any `secret = …` line (settings files, VICI dumps).
var secretAssignRe = regexp.MustCompile(`(?m)(\bsecret\s*=\s*)([^\s#}]+)`)

// secretSet masks the secret-bearing fields of text the renderer returns.
type secretSet struct{}

// redact masks every `secret = …` assignment in s.
func (secretSet) redact(s string) string {
	return secretAssignRe.ReplaceAllString(s, "${1}"+Redacted)
}

// redactedError masks secrets in Error() and keeps the chain for errors.Is/As.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

func (ss secretSet) redactErr(err error) error {
	if err == nil {
		return nil
	}
	msg := ss.redact(err.Error())
	if msg == err.Error() {
		return err
	}
	return &redactedError{msg: msg, err: err}
}
