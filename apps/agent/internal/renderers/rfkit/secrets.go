package rfkit

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Secrets (00-CONTEXT rule 10, D-051). The configuration document carries references
// "<kind>/<name>" only; a renderer resolves them at Render time. The plaintext then lives in
// exactly one place — the rendered daemon file (a File marked Secret, mode 0600/0640) — and
// in the daemon. Everything a renderer returns (errors, Retrieve state, events, tool output)
// passes through a Redactor that masks every value it resolved.

// Redacted replaces a secret wherever the renderer reports something.
const Redacted = "<redacted>"

// SecretResolver returns the plaintext of a D-051 secret reference ("password/snmp-ro").
type SecretResolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

// SecretResolverFunc adapts a function to SecretResolver.
type SecretResolverFunc func(ctx context.Context, ref string) (string, error)

// Resolve implements SecretResolver.
func (f SecretResolverFunc) Resolve(ctx context.Context, ref string) (string, error) {
	return f(ctx, ref)
}

// Errors of the secret path.
var (
	// ErrNoSecretResolver: the desired state references a secret and no resolver is configured.
	ErrNoSecretResolver = errors.New("rfkit: no secret resolver configured")
	// ErrSecretRef: the reference is malformed or of the wrong kind.
	ErrSecretRef = errors.New("rfkit: invalid secret reference")
	// ErrSecretValue: the resolved value cannot be written into the daemon's syntax safely.
	ErrSecretValue = errors.New("rfkit: secret value not usable")
)

var secretRefRe = regexp.MustCompile(`^(psk|key|cert|password|token)/[A-Za-z0-9_.-]{1,64}$`)

// CheckRef validates ref against D-051 and, when kinds is non-empty, its kind.
func CheckRef(ref string, kinds ...string) error {
	if !secretRefRe.MatchString(ref) {
		return fmt.Errorf("%w: %q must match %s", ErrSecretRef, ref, secretRefRe)
	}
	if len(kinds) > 0 {
		kind, _, _ := strings.Cut(ref, "/")
		if !slices.Contains(kinds, kind) {
			return fmt.Errorf("%w: %q must be of kind %s", ErrSecretRef, ref, strings.Join(kinds, "|"))
		}
	}
	return nil
}

// Secrets resolves references during one Render and remembers the values for redaction.
// Errors never contain a value.
type Secrets struct {
	Ctx      context.Context
	Resolver SecretResolver
	used     map[string]string
}

// Resolve returns the plaintext of ref after checking the reference (kinds) and the value
// (valid). valid describes the accepted syntax in its error; it must not echo the value.
func (s *Secrets) Resolve(ref string, valid func(string) error, kinds ...string) (string, error) {
	if err := CheckRef(ref, kinds...); err != nil {
		return "", err
	}
	if v, ok := s.used[ref]; ok {
		return v, nil
	}
	if s.Resolver == nil {
		return "", fmt.Errorf("%w: %s", ErrNoSecretResolver, ref)
	}
	ctx := s.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.Resolver.Resolve(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("rfkit: resolve secret %s: %w", ref, err)
	}
	if v == "" || v == Redacted || strings.Contains(v, Redacted) {
		return "", fmt.Errorf("%w: %s resolved to an empty or reserved value", ErrSecretValue, ref)
	}
	if valid != nil {
		if err := valid(v); err != nil {
			return "", fmt.Errorf("%w: %s: %v", ErrSecretValue, ref, err)
		}
	}
	if s.used == nil {
		s.used = map[string]string{}
	}
	s.used[ref] = v
	return v, nil
}

// Values returns every plaintext resolved so far.
func (s *Secrets) Values() []string {
	out := make([]string, 0, len(s.used))
	for _, v := range s.used {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}

// Redactor masks remembered secret values (longest first) in text. The set only grows, so a
// rotated secret stays masked as well. Safe for concurrent use.
type Redactor struct {
	mu     sync.Mutex
	values map[string]bool
}

// minLineSecret is the shortest line of a multi-line secret that is masked on its own.
const minLineSecret = 12

// Add remembers values (empty strings are ignored). For a multi-line value (a PEM key) every
// line of at least minLineSecret characters that is not PEM armour ("-----BEGIN …") is also
// remembered on its own, so the secret stays masked when a tool's output is re-wrapped.
func (r *Redactor) Add(values ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = map[string]bool{}
	}
	for _, v := range values {
		if v == "" {
			continue
		}
		r.values[v] = true
		if !strings.Contains(v, "\n") {
			continue
		}
		for _, l := range strings.Split(v, "\n") {
			l = strings.TrimSpace(l)
			if len(l) >= minLineSecret && !strings.HasPrefix(l, "-----") {
				r.values[l] = true
			}
		}
	}
}

// Redact returns s with every remembered value replaced by Redacted. A value is replaced only
// where it stands as a whole token — not preceded or followed by a letter, digit, '_', '.' or
// '-' — so a short or common secret never blanks unrelated words ("ro" inside "router";
// review L1). Longest values first.
func (r *Redactor) Redact(s string) string {
	r.mu.Lock()
	vals := make([]string, 0, len(r.values))
	for v := range r.values {
		vals = append(vals, v)
	}
	r.mu.Unlock()
	slices.SortFunc(vals, func(a, b string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	for _, v := range vals {
		s = replaceToken(s, v)
	}
	return s
}

func isTokenByte(c byte) bool {
	return c == '_' || c == '.' || c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// replaceToken replaces the whole-token occurrences of v in s by Redacted.
func replaceToken(s, v string) string {
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(s[i:], v)
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		j += i
		end := j + len(v)
		left := j == 0 || !isTokenByte(s[j-1]) || !isTokenByte(v[0])
		right := end == len(s) || !isTokenByte(s[end]) || !isTokenByte(v[len(v)-1])
		b.WriteString(s[i:j])
		if left && right {
			b.WriteString(Redacted)
		} else {
			b.WriteString(v)
		}
		i = end
	}
}

// Error returns err with its message redacted; errors.Is/As still see the chain.
func (r *Redactor) Error(err error) error {
	if err == nil {
		return nil
	}
	msg := r.Redact(err.Error())
	if msg == err.Error() {
		return err
	}
	return &redactedError{msg: msg, err: err}
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }
