package strongswan

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"ngfw/agent/internal/renderers"
)

// Escaping for the strongSwan settings grammar (strongswan.conf(5), swanctl.conf(5); the
// lexer is src/libstrongswan/settings/settings_lexer.l):
//
//   - Section names and keys are NAME tokens: any printable character except
//     `. , : { } = " #`, blanks and line breaks. `.` separates the path of a lookup
//     ("connections.<conn>.version"), `:` starts a section reference (inheritance) and the word
//     `include` followed by a blank starts an include statement wherever a name may appear.
//     The renderer is stricter: names are [A-Za-z0-9_+-]{1,64}, never "include".
//   - Unquoted values end at a newline, `#` or `}`; quoted values ("…") end at the next
//     unescaped `"` and understand `\n \r \t` and `\<c>` escapes. Every user string that is
//     not a validated token (address, prefix, keyword) is rendered quoted with `\` and `"`
//     escaped, after rejecting control characters (so no escape can form a line break).
//   - PSKs are never quoted: they are rendered as `0s<base64>` (swanctl's base64 form).

// ErrInput is wrapped by every error about the desired state (as opposed to I/O or daemon
// errors), so the commit engine reports it as a validation issue.
var ErrInput = errors.New("strongswan: invalid desired state")

// MaxNameLen bounds section names (connections, children, secrets, pools).
const MaxNameLen = 64

// nameRe is the renderer's subset of the settings NAME token.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9_+-]{1,64}$`)

// SectionName validates a section name (connection, child, secret, pool, authority).
func SectionName(s string) (string, error) {
	if !nameRe.MatchString(s) {
		return "", fmt.Errorf("%w: name %q must match %s", renderers.ErrUnsafe, clip(s), nameRe)
	}
	if strings.EqualFold(s, "include") {
		return "", fmt.Errorf("%w: name %q is the settings include keyword", renderers.ErrUnsafe, s)
	}
	return s, nil
}

// MaxConnNameLen bounds a connection name so that every name derived from it — the secret
// section and VICI shared-key id "ike-<conn>" — stays within MaxNameLen (RF-2 review L1).
const MaxConnNameLen = MaxNameLen - len("ike-")

// ConnName maps a document object name (objectName: [A-Za-z0-9][A-Za-z0-9_.-]{0,62}) to a
// strongSwan section name: `.` is a settings path separator, so it becomes `+` — which
// objectName never contains, so the mapping is bijective (TunnelName reverses it).
func ConnName(objectName string) (string, error) {
	if !objectNameRe.MatchString(objectName) {
		return "", fmt.Errorf("%w: name %q must match %s", ErrInput, clip(objectName), objectNameRe)
	}
	if len(objectName) > MaxConnNameLen {
		return "", fmt.Errorf("%w: name %q is %d characters; tunnel names are limited to %d so that the derived secret name ike-<name> fits %d", ErrInput, clip(objectName), len(objectName), MaxConnNameLen, MaxNameLen)
	}
	return SectionName(strings.ReplaceAll(objectName, ".", "+"))
}

// TunnelName is the inverse of ConnName.
func TunnelName(connName string) string { return strings.ReplaceAll(connName, "+", ".") }

var objectNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// Quote returns s as a settings quoted string: `"` + s with `\` and `"` escaped + `"`. Control
// characters, invalid UTF-8 and Unicode line separators are rejected (renderers.Line).
func Quote(s string) (string, error) {
	if len(s) > MaxValueLen {
		return "", fmt.Errorf("%w: value of %d bytes exceeds %d", renderers.ErrUnsafe, len(s), MaxValueLen)
	}
	return renderers.Quoted(s)
}

// MaxValueLen bounds a quoted value (identities are ≤ 255 in the schema).
const MaxValueLen = 1024

// Unquote reverses Quote (the strict parser's view of a quoted value: only `\\` and `\"`).
func Unquote(q string) (string, error) {
	if len(q) < 2 || q[0] != '"' || q[len(q)-1] != '"' {
		return "", errors.New("not a quoted string")
	}
	var b strings.Builder
	body := q[1 : len(q)-1]
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch c {
		case '\\':
			if i+1 >= len(body) || (body[i+1] != '\\' && body[i+1] != '"') {
				return "", errors.New("unsupported escape in quoted string")
			}
			i++
			b.WriteByte(body[i])
		case '"':
			return "", errors.New("unescaped quote in quoted string")
		default:
			b.WriteByte(c)
		}
	}
	if _, err := renderers.Line(b.String()); err != nil {
		return "", err
	}
	return b.String(), nil
}

// tokenRe is a bare (unquoted) value the renderer emits: keywords, numbers, times, addresses,
// prefixes, proposals, comma lists of those, %any, unix:// URIs and absolute paths.
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9_.:/%,|+@=-]+$`)

// maxTokenLen bounds a bare value (a comma list of 64 IPv6 prefixes fits).
const maxTokenLen = 4096

// Token validates a bare value.
func Token(s string) (string, error) {
	if len(s) > maxTokenLen || !tokenRe.MatchString(s) {
		return "", fmt.Errorf("%w: bare value %q must match %s (at most %d bytes)", renderers.ErrUnsafe, clip(s), tokenRe, maxTokenLen)
	}
	return s, nil
}

// clip shortens a value for an error message (hostile 5 KB inputs stay readable).
func clip(s string) string {
	if len(s) > 48 {
		return s[:48] + "…"
	}
	return s
}

// ------------------------------------------------------------------------------ identities

var (
	fqdnRe  = regexp.MustCompile(`^@?[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+-]{1,64}@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	keyidRe = regexp.MustCompile(`^@#[0-9A-Fa-f]{2,128}$`)
	// dnRe: RDNs "attr=value" separated by "," or "/" (leading "/" form), values without the
	// characters strongSwan's DN parser treats specially or that could confuse a reader.
	// Length is bounded by Identity (255 bytes).
	dnRe = regexp.MustCompile(`^(?:/)?[A-Za-z][A-Za-z0-9.]*=[A-Za-z0-9 ._@*()'+:?-]+(?:(?:, ?|/)[A-Za-z][A-Za-z0-9.]*=[A-Za-z0-9 ._@*()'+:?-]+)*$`)
)

// Identity validates an IKE identity in one of the shapes strongSwan parses unambiguously:
// IP address, FQDN (optionally with a leading @), e-mail (RFC 822), key id (@#hex), X.509 DN
// ("C=CH, O=example, CN=gw") or %any. It returns the canonical text (addresses canonical).
func Identity(s string) (string, error) {
	switch {
	case s == "":
		return "", fmt.Errorf("%w: empty identity", ErrInput)
	case len(s) > 255:
		return "", fmt.Errorf("%w: identity of %d bytes exceeds 255", ErrInput, len(s))
	case s == "%any":
		return s, nil
	}
	if a, err := netip.ParseAddr(s); err == nil {
		if a.Zone() != "" {
			return "", fmt.Errorf("%w: identity %q has a zone", ErrInput, s)
		}
		return a.String(), nil
	}
	if strings.ContainsRune(s, '=') {
		if dnRe.MatchString(s) && !strings.Contains(s, "  ") && strings.TrimSpace(s) == s {
			return s, nil
		}
		return "", fmt.Errorf("%w: identity %q is not a distinguished name of the form \"C=CH, O=Org, CN=name\"", ErrInput, clip(s))
	}
	if keyidRe.MatchString(s) || emailRe.MatchString(s) {
		return s, nil
	}
	if fqdnRe.MatchString(s) && len(strings.TrimPrefix(s, "@")) <= 253 && !allDigitsAndDots(strings.TrimPrefix(s, "@")) {
		return s, nil
	}
	return "", fmt.Errorf("%w: identity %q is not an IP address, FQDN, e-mail, @#keyid or DN", ErrInput, clip(s))
}

func allDigitsAndDots(s string) bool {
	return strings.Trim(s, "0123456789.") == ""
}

// Host validates a remote/local address as strongSwan takes it in local_addrs/remote_addrs:
// an IP address (canonicalised), an RFC 1123 hostname (resolved by charon) or %any.
func Host(s string) (string, error) {
	if s == "%any" {
		return s, nil
	}
	if a, err := netip.ParseAddr(s); err == nil {
		if a.Zone() != "" {
			return "", fmt.Errorf("%w: address %q has a zone", ErrInput, s)
		}
		return a.String(), nil
	}
	if len(s) <= 253 && fqdnRe.MatchString(s) && !strings.HasPrefix(s, "@") && !allDigitsAndDots(s) {
		return s, nil
	}
	return "", fmt.Errorf("%w: %q is not an IP address, hostname or %%any", ErrInput, clip(s))
}

// TrafficSelector validates a CIDR traffic selector and returns the masked network.
func TrafficSelector(s string) (string, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "", fmt.Errorf("%w: traffic selector %q: %v", ErrInput, clip(s), err)
	}
	return p.Masked().String(), nil
}

// ------------------------------------------------------------------------------ proposals

// Algorithm keywords (packages/schema/src/domains/vpn.ts, strongSwan proposal keywords).
var (
	AEADCiphers   = []string{"aes128gcm8", "aes128gcm12", "aes128gcm16", "aes192gcm16", "aes256gcm8", "aes256gcm12", "aes256gcm16", "chacha20poly1305"}
	IKEEncryption = append([]string{"aes128", "aes192", "aes256", "aes128ctr", "aes256ctr"}, append(slices.Clone(AEADCiphers), "3des")...)
	ESPEncryption = append(slices.Clone(IKEEncryption), "null")
	Integrity     = []string{"sha1", "sha256", "sha384", "sha512", "md5", "aesxcbc", "aescmac"}
	PRFs          = []string{"prfsha1", "prfsha256", "prfsha384", "prfsha512", "prfmd5", "prfaesxcbc", "prfaescmac"}
	DHGroups      = []string{"modp768", "modp1024", "modp1536", "modp2048", "modp3072", "modp4096", "modp6144", "modp8192", "ecp256", "ecp384", "ecp521", "modp1024s160", "modp2048s224", "modp2048s256", "ecp192", "ecp224", "curve25519", "curve448"}
)

// DefaultAEADPRF is the PRF rendered for an IKE proposal with an AEAD cipher and no PRF:
// AEAD transforms carry no integrity algorithm to derive the PRF from, and charon rejects an
// AEAD IKE proposal without one.
const DefaultAEADPRF = "prfsha256"

func isAEAD(encr string) bool { return slices.Contains(AEADCiphers, encr) }

func oneOf(field, v string, set []string) error {
	if !slices.Contains(set, v) {
		return fmt.Errorf("%w: %s %q is not one of %s", ErrInput, field, clip(v), strings.Join(set, "|"))
	}
	return nil
}

// IKEProposal builds the strongSwan IKE proposal keyword ("aes256-sha256-prfsha256-ecp256",
// "aes128gcm16-prfsha256-ecp256").
func IKEProposal(encr, integ, prf, dh string) (string, error) {
	if err := oneOf("ike.encr", encr, IKEEncryption); err != nil {
		return "", err
	}
	parts := []string{encr}
	switch {
	case isAEAD(encr) && integ != "":
		return "", fmt.Errorf("%w: ike.integ must be unset for the AEAD cipher %s", ErrInput, encr)
	case !isAEAD(encr) && integ == "":
		return "", fmt.Errorf("%w: ike.integ is required for %s", ErrInput, encr)
	case integ != "":
		if err := oneOf("ike.integ", integ, Integrity); err != nil {
			return "", err
		}
		parts = append(parts, integ)
	}
	if prf == "" && isAEAD(encr) {
		prf = DefaultAEADPRF
	}
	if prf != "" {
		if err := oneOf("ike.prf", prf, PRFs); err != nil {
			return "", err
		}
		parts = append(parts, prf)
	}
	if err := oneOf("ike.dh", dh, DHGroups); err != nil {
		return "", err
	}
	return strings.Join(append(parts, dh), "-"), nil
}

// ChildProposal builds an ESP ("aes256-sha256-ecp256-esn") or AH ("sha256-modp2048")
// proposal keyword; dh is the optional PFS group.
func ChildProposal(protocol, encr, integ, dh string, esn bool) (string, error) {
	var parts []string
	switch protocol {
	case "esp":
		if err := oneOf("esp.encr", encr, ESPEncryption); err != nil {
			return "", err
		}
		parts = append(parts, encr)
		switch {
		case isAEAD(encr) && integ != "":
			return "", fmt.Errorf("%w: esp.integ must be unset for the AEAD cipher %s", ErrInput, encr)
		case !isAEAD(encr) && integ == "":
			return "", fmt.Errorf("%w: esp.integ is required for %s", ErrInput, encr)
		case integ != "":
			if err := oneOf("esp.integ", integ, Integrity); err != nil {
				return "", err
			}
			parts = append(parts, integ)
		}
	case "ah":
		if integ == "" {
			return "", fmt.Errorf("%w: AH needs esp.integ (AH authenticates only)", ErrInput)
		}
		if err := oneOf("esp.integ", integ, Integrity); err != nil {
			return "", err
		}
		parts = append(parts, integ)
	default:
		return "", fmt.Errorf("%w: protocol %q is not esp|ah", ErrInput, clip(protocol))
	}
	if dh != "" {
		if err := oneOf("esp.dh", dh, DHGroups); err != nil {
			return "", err
		}
		parts = append(parts, dh)
	}
	if esn {
		parts = append(parts, "esn")
	}
	return strings.Join(parts, "-"), nil
}

// proposal grammars for Validate (checks proposals found in files, whoever rendered them).
var (
	ikeProposalRe   = regexp.MustCompile(`^(` + alt(IKEEncryption) + `)(?:-(` + alt(Integrity) + `))?(?:-(` + alt(PRFs) + `))?-(` + alt(DHGroups) + `)$`)
	espProposalRe   = regexp.MustCompile(`^(` + alt(ESPEncryption) + `)(?:-(` + alt(Integrity) + `))?(?:-(` + alt(DHGroups) + `))?(?:-(esn|noesn))?$`)
	ahProposalRe    = regexp.MustCompile(`^(` + alt(Integrity) + `)(?:-(` + alt(DHGroups) + `))?(?:-(esn|noesn))?$`)
	errBadProposal  = errors.New("proposal does not match the keyword grammar")
	errAEADIntegMix = errors.New("AEAD cipher combined with an integrity algorithm, or classic cipher without one")
)

func alt(words []string) string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = regexp.QuoteMeta(w)
	}
	// longest first so "aes128gcm16" is not matched as "aes128" + garbage
	slices.SortFunc(q, func(a, b string) int { return len(b) - len(a) })
	return strings.Join(q, "|")
}

// CheckProposal validates one proposal keyword of kind "ike", "esp" or "ah".
func CheckProposal(kind, p string) error {
	var m []string
	switch kind {
	case "ike":
		m = ikeProposalRe.FindStringSubmatch(p)
		if m != nil && isAEAD(m[1]) && m[3] == "" {
			return fmt.Errorf("%w: ike proposal %q: AEAD needs a PRF", ErrInput, clip(p))
		}
	case "esp":
		m = espProposalRe.FindStringSubmatch(p)
	case "ah":
		if ahProposalRe.MatchString(p) {
			return nil
		}
		return fmt.Errorf("%w: ah proposal %q: %v", ErrInput, clip(p), errBadProposal)
	default:
		return fmt.Errorf("%w: unknown proposal kind %q", ErrInput, kind)
	}
	if m == nil {
		return fmt.Errorf("%w: %s proposal %q: %v", ErrInput, kind, clip(p), errBadProposal)
	}
	if isAEAD(m[1]) == (m[2] != "") {
		return fmt.Errorf("%w: %s proposal %q: %v", ErrInput, kind, clip(p), errAEADIntegMix)
	}
	return nil
}
