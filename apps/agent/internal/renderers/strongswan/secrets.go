package strongswan

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Secrets (00-CONTEXT rule 10, D-051; P11 "secret handling contract"). The document carries
// only references "psk/<name>"; Render resolves them through the injected SecretResolver. The
// plaintext then exists in memory, in the VICI load-shared message (never logged) and — as
// base64 — in vrx-secrets.conf (mode 0600, marked Secret so Files.Redacted hides it). It is
// never in vrx.conf, strongswan.conf, argv, Retrieve/State, DryRun, events or errors: every
// string the renderer hands back passes through the redactor, which masks every value this
// renderer resolved (plaintext, base64 and hex forms) and every `secret = …` assignment.

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

// secretSet remembers the values this renderer resolved, to mask them literally.
type secretSet struct {
	mu     sync.Mutex
	values map[string]bool
}

// add remembers v in every form the renderer could emit (plaintext, base64, hex).
func (ss *secretSet) add(v []byte) {
	if len(v) == 0 {
		return
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.values == nil {
		ss.values = map[string]bool{}
	}
	ss.values[string(v)] = true
	ss.values[base64.StdEncoding.EncodeToString(v)] = true
	ss.values[hex.EncodeToString(v)] = true
}

// redact masks every remembered value and every secret assignment in s.
func (ss *secretSet) redact(s string) string {
	ss.mu.Lock()
	vals := make([]string, 0, len(ss.values))
	for v := range ss.values {
		vals = append(vals, v)
	}
	ss.mu.Unlock()
	// Longest first, so a value containing another is masked whole.
	slices.SortFunc(vals, func(a, b string) int { return len(b) - len(a) })
	for _, v := range vals {
		s = strings.ReplaceAll(s, v, Redacted)
	}
	return secretAssignRe.ReplaceAllString(s, "${1}"+Redacted)
}

// redactedError masks secrets in Error() and keeps the chain for errors.Is/As.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

func (ss *secretSet) redactErr(err error) error {
	if err == nil {
		return nil
	}
	msg := ss.redact(err.Error())
	if msg == err.Error() {
		return err
	}
	return &redactedError{msg: msg, err: err}
}
