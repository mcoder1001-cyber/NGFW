package frr

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Secrets (00-CONTEXT rule 10, D-051, D-072; RF-1 review M2). The document carries only
// secret references "<kind>/<name>"; a section resolves them at render time through
// RenderContext.Secret. The plaintext then exists only in frr.conf (mode 0640, frr:frr, the
// File is marked Secret) and in FRR itself. Everything the renderer hands back — Retrieve,
// State, Show output, DryRun diffs, error messages — passes through the redactor, which masks
// (1) every value resolved by this renderer and (2) every match of a secret-bearing FRR
// config pattern (built-in + RegisterRedaction), with "<redacted>". frr-reload.py runs with
// --log-level critical, because at info/error level it writes the lines it applies (and the
// commands that failed) to its --logfile.

// Redacted replaces secrets in everything the renderer returns.
const Redacted = "<redacted>"

// SecretResolver returns the plaintext of a secret reference ("password/bgp-peer1").
type SecretResolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

// SecretResolverFunc adapts a function to SecretResolver.
type SecretResolverFunc func(ctx context.Context, ref string) (string, error)

// Resolve implements SecretResolver.
func (f SecretResolverFunc) Resolve(ctx context.Context, ref string) (string, error) {
	return f(ctx, ref)
}

// ErrNoSecretResolver is returned when a section needs a secret and none is configured.
var ErrNoSecretResolver = errors.New("frr: no secret resolver configured")

var (
	// secretRefRe is the D-051 reference form.
	secretRefRe = regexp.MustCompile(`^(psk|key|cert|password|token)/[A-Za-z0-9_.-]{1,64}$`)
	// secretValueRe is what FRR can carry as one WORD token without quoting problems: no
	// blanks, no '|' (CLI pipe), no quotes/backslash (so masking inside JSON output is safe),
	// no '!'/'#' at the start.
	secretValueRe = regexp.MustCompile(`^[A-Za-z0-9_.,:;@%+=/~^*()\[\]{}<>?$&-]{1,128}$`)
)

// builtinRedactions mask the secret argument of FRR's secret-bearing commands wherever the
// line appears (running-config, frr-reload diffs with "no " prefixes, vtysh error echoes).
// Group 1 is the secret.
var builtinRedactions = []*regexp.Regexp{
	// password [clear|md5] <secret>: vty/enable passwords, `neighbor X password`, `isis password`.
	regexp.MustCompile(`(?:^|[\s"])(?:enable )?password (?:(?:clear|md5) )?([^\s"]+)`),
	regexp.MustCompile(`ip ospf authentication-key ([^\s"]+)`),
	regexp.MustCompile(`ip ospf message-digest-key \d+ md5 ([^\s"]+)`),
	regexp.MustCompile(`ospf6 authentication key-id \d+ hash-algo \S+ key ([^\s"]+)`),
	regexp.MustCompile(`key-string ([^\s"]+)`),
	regexp.MustCompile(`(?:area|domain)-password (?:clear|md5) ([^\s"]+)`),
	regexp.MustCompile(`ip rip authentication string ([^\s"]+)`),
}

var (
	redactMu   sync.Mutex
	redactions = slices.Clone(builtinRedactions)
)

// RegisterRedaction adds a pattern whose capture group 1 is a secret (protocol sections with
// secret-bearing commands not covered by the built-ins). Panics without exactly one group.
func RegisterRedaction(re *regexp.Regexp) {
	if re == nil || re.NumSubexp() != 1 {
		panic("frr: RegisterRedaction needs a pattern with exactly one capture group (the secret)")
	}
	redactMu.Lock()
	defer redactMu.Unlock()
	redactions = append(redactions, re)
}

// RedactPatterns masks group 1 of every registered secret pattern in s.
func RedactPatterns(s string) string {
	redactMu.Lock()
	res := slices.Clone(redactions)
	redactMu.Unlock()
	for _, re := range res {
		idx := re.FindAllStringSubmatchIndex(s, -1)
		for k := len(idx) - 1; k >= 0; k-- {
			a, b := idx[k][2], idx[k][3]
			if a >= 0 && s[a:b] != Redacted {
				s = s[:a] + Redacted + s[b:]
			}
		}
	}
	return s
}

// secretSet remembers the values this renderer resolved, to mask them literally.
type secretSet struct {
	mu     sync.Mutex
	values map[string]bool
}

func (ss *secretSet) add(vs ...string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.values == nil {
		ss.values = map[string]bool{}
	}
	for _, v := range vs {
		ss.values[v] = true
	}
}

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
	return RedactPatterns(s)
}

// RenderContext is what a Section renders from.
type RenderContext struct {
	// Ctx is the Render call's context (for the secret resolver).
	Ctx context.Context
	// Input is the message given to Renderer.Render (*vrxv1.DesiredState or *structpb.Struct).
	Input proto.Message
	// Desired is the typed desired state; Ext the D-055 stand-in fields.
	Desired *vrxv1.DesiredState
	Ext     *Extensions

	mapIf    InterfaceMapper
	resolver SecretResolver
	used     map[string]string
	model    *Model // framework model, built once per Render
}

// redactedError masks secrets in Error() and keeps the chain for errors.Is/As.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

func redactErr(err error, ss *secretSet) error {
	if err == nil {
		return nil
	}
	msg := ss.redact(err.Error())
	if msg == err.Error() {
		return err
	}
	return &redactedError{msg: msg, err: err}
}

// MapInterface maps a VPP interface name to its Linux name (the renderer's InterfaceMapper),
// for protocol lines such as `neighbor … update-source <if>` and per-interface blocks.
func (rc *RenderContext) MapInterface(vppName string) (string, bool) {
	if rc.mapIf == nil {
		return NoMapper(vppName)
	}
	return rc.mapIf(vppName)
}

// Secret resolves a D-051 reference to its plaintext, validated as a single FRR WORD token.
// Errors never contain the value.
func (rc *RenderContext) Secret(ref string) (string, error) {
	if !secretRefRe.MatchString(ref) {
		return "", fmt.Errorf("%w: secret reference %q must match %s", ErrInput, ref, secretRefRe)
	}
	if v, ok := rc.used[ref]; ok {
		return v, nil
	}
	if rc.resolver == nil {
		return "", fmt.Errorf("%w: %s", ErrNoSecretResolver, ref)
	}
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := rc.resolver.Resolve(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("frr: resolve secret %s: %w", ref, err)
	}
	if !secretValueRe.MatchString(v) || v == Redacted {
		return "", fmt.Errorf("%w: secret %s is not usable in frr.conf (1–128 printable characters without blanks, quotes, backslash or '|')", ErrInput, ref)
	}
	if rc.used == nil {
		rc.used = map[string]string{}
	}
	rc.used[ref] = v
	return v, nil
}

// secretValues are the plaintexts resolved during this render.
func (rc *RenderContext) secretValues() []string {
	out := make([]string, 0, len(rc.used))
	for _, v := range rc.used {
		out = append(out, v)
	}
	return out
}
