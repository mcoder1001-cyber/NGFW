package strongswan

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/strongswan/govici/vici"

	"ngfw/agent/internal/renderers"
)

// trees are the parsed rendered files.
type trees struct {
	conf, conns, secrets *Section
}

// parseFiles round-trips and semantically checks the three files.
func (r *Renderer) parseFiles(files renderers.Files) (*trees, error) {
	var t trees
	var err error
	if t.conf, err = RoundTrip("strongswan.conf", files[r.paths.StrongswanConf].Content); err != nil {
		return nil, err
	}
	if t.conns, err = RoundTrip("vrx.conf", files[r.paths.ConnsFile()].Content); err != nil {
		return nil, err
	}
	if t.secrets, err = RoundTrip("vrx-secrets.conf", files[r.paths.SecretsFile()].Content); err != nil {
		return nil, err
	}
	if err := r.checkConf(t.conf); err != nil {
		return nil, err
	}
	if err := checkConns(t.conns); err != nil {
		return nil, err
	}
	if err := checkSecrets(t.secrets, t.conns); err != nil {
		return nil, err
	}
	return &t, nil
}

// ------------------------------------------------------------------------------ semantic checks

// keyCheck validates one value.
type keyCheck func(v string) error

var (
	secondsRe = regexp.MustCompile(`^[0-9]{1,7}s$`)
	uintRe    = regexp.MustCompile(`^[0-9]{1,20}$`)
	certFile  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}\.pem$`)
)

func enum(vals ...string) keyCheck {
	return func(v string) error {
		if !slices.Contains(vals, v) {
			return fmt.Errorf("not one of %s", strings.Join(vals, "|"))
		}
		return nil
	}
}

func matches(re *regexp.Regexp) keyCheck {
	return func(v string) error {
		if !re.MatchString(v) {
			return fmt.Errorf("does not match %s", re)
		}
		return nil
	}
}

func listOf(each func(string) error) keyCheck {
	return func(v string) error {
		items := splitList(v)
		if len(items) == 0 || len(items) > 64 {
			return errors.New("list must have 1..64 elements")
		}
		for _, it := range items {
			if err := each(it); err != nil {
				return err
			}
		}
		return nil
	}
}

func hostCheck(v string) error {
	h, err := Host(v)
	if err == nil && h != v {
		return fmt.Errorf("%q is not canonical (%s)", v, h)
	}
	return err
}

func tsCheck(v string) error {
	s, err := TrafficSelector(v)
	if err == nil && s != v {
		return fmt.Errorf("traffic selector %q is not a masked network (%s)", v, s)
	}
	return err
}

func idCheck(v string) error {
	s, err := Identity(v)
	if err == nil && s != v {
		return fmt.Errorf("identity %q is not canonical (%s)", v, s)
	}
	return err
}

func proposalCheck(kind string) keyCheck {
	return listOf(func(p string) error { return CheckProposal(kind, p) })
}

var (
	connKeys = map[string]keyCheck{
		"version": enum("0", "1", "2"), "local_addrs": listOf(hostCheck), "remote_addrs": listOf(hostCheck),
		"proposals": proposalCheck("ike"), "dpd_delay": matches(secondsRe), "dpd_timeout": matches(secondsRe),
		"mobike": enum("yes", "no"), "encap": enum("yes", "no"), "fragmentation": enum("yes", "no", "force", "accept"),
		"rekey_time": matches(secondsRe), "reauth_time": matches(secondsRe), "pools": listOf(func(s string) error { _, err := SectionName(s); return err }),
	}
	authKeys = map[string]keyCheck{
		"auth": enum("psk", "pubkey"), "id": idCheck,
		"certs": listOf(func(s string) error { return matches(certFile)(s) }), "cacerts": listOf(func(s string) error { return matches(certFile)(s) }),
	}
	childKeys = map[string]keyCheck{
		"local_ts": listOf(tsCheck), "remote_ts": listOf(tsCheck), "esp_proposals": proposalCheck("esp"), "ah_proposals": proposalCheck("ah"),
		"mode": enum("tunnel", "transport"), "start_action": enum("none", "trap", "start"), "close_action": enum("none", "trap", "start"),
		"dpd_action": enum("clear", "trap", "restart"), "rekey_time": matches(secondsRe), "rekey_bytes": matches(uintRe),
		"rekey_packets": matches(uintRe), "replay_window": matches(uintRe), "if_id_in": matches(uintRe), "if_id_out": matches(uintRe),
	}
	poolKeys = map[string]keyCheck{
		"addrs": func(v string) error { _, err := netip.ParsePrefix(v); return err },
		"dns":   listOf(func(s string) error { _, err := netip.ParseAddr(s); return err }),
	}
	authorityKeys = map[string]keyCheck{"cacert": matches(certFile)}
	// quotedKeys are the keys whose values are free text and therefore always quoted.
	quotedKeys = map[string]bool{"id": true, "id-local": true, "id-remote": true, "load": true}
)

// checkKeys validates every key of sec against allowed; the section may hold only the listed
// subsections (checked by the caller) and comments/blank lines.
func checkKeys(where string, sec *Section, allowed map[string]keyCheck, subs ...string) error {
	for _, it := range sec.Items {
		switch it.Kind {
		case KindKey:
			chk, ok := allowed[it.Name]
			if !ok {
				return fmt.Errorf("%w: %s: unknown key %s", ErrInput, where, it.Name)
			}
			if it.Quoted != quotedKeys[it.Name] {
				return fmt.Errorf("%w: %s.%s: quoting does not match the key type", ErrInput, where, it.Name)
			}
			if err := chk(it.Value); err != nil {
				return fmt.Errorf("%w: %s.%s: %v", ErrInput, where, it.Name, err)
			}
		case KindSection:
			if !slices.Contains(subs, it.Name) && !slices.Contains(subs, "*") {
				return fmt.Errorf("%w: %s: unexpected section %s", ErrInput, where, it.Name)
			}
		}
	}
	return nil
}

func checkConns(root *Section) error {
	if err := checkKeys("vrx.conf", root, nil, "connections", "pools", "authorities"); err != nil {
		return err
	}
	conns := root.Sub("connections")
	if conns == nil {
		return fmt.Errorf("%w: vrx.conf: no connections section", ErrInput)
	}
	if err := checkKeys("connections", conns, nil, "*"); err != nil {
		return err
	}
	pools := map[string]bool{}
	if p := root.Sub("pools"); p != nil {
		if err := checkKeys("pools", p, nil, "*"); err != nil {
			return err
		}
		for _, it := range p.Sections() {
			pools[it.Name] = true
			if err := checkKeys("pools."+it.Name, it.Section, poolKeys); err != nil {
				return err
			}
			if _, ok := it.Section.Get("addrs"); !ok {
				return fmt.Errorf("%w: pools.%s: addrs missing", ErrInput, it.Name)
			}
		}
	}
	if a := root.Sub("authorities"); a != nil {
		if err := checkKeys("authorities", a, nil, "*"); err != nil {
			return err
		}
		for _, it := range a.Sections() {
			if err := checkKeys("authorities."+it.Name, it.Section, authorityKeys); err != nil {
				return err
			}
		}
	}
	if len(conns.Sections()) > MaxConns {
		return fmt.Errorf("%w: more than %d connections", ErrInput, MaxConns)
	}
	for _, it := range conns.Sections() {
		where := "connections." + it.Name
		c := it.Section
		if err := checkKeys(where, c, connKeys, "local", "remote", "children"); err != nil {
			return err
		}
		for _, k := range []string{"local_addrs", "remote_addrs", "proposals"} {
			if _, ok := c.Get(k); !ok {
				return fmt.Errorf("%w: %s: %s missing", ErrInput, where, k)
			}
		}
		if v, ok := c.Get("pools"); ok {
			for _, p := range splitList(v) {
				if !pools[p] {
					return fmt.Errorf("%w: %s.pools: no pool %s", ErrInput, where, p)
				}
			}
		}
		for _, side := range []string{"local", "remote"} {
			a := c.Sub(side)
			if a == nil {
				return fmt.Errorf("%w: %s: %s section missing", ErrInput, where, side)
			}
			if err := checkKeys(where+"."+side, a, authKeys); err != nil {
				return err
			}
			if _, ok := a.Get("auth"); !ok {
				return fmt.Errorf("%w: %s.%s: auth missing", ErrInput, where, side)
			}
		}
		children := c.Sub("children")
		if children == nil || len(children.Sections()) == 0 {
			return fmt.Errorf("%w: %s: no children", ErrInput, where)
		}
		if err := checkKeys(where+".children", children, nil, "*"); err != nil {
			return err
		}
		for _, ch := range children.Sections() {
			cw := where + ".children." + ch.Name
			if err := checkKeys(cw, ch.Section, childKeys); err != nil {
				return err
			}
			_, esp := ch.Section.Get("esp_proposals")
			_, ah := ch.Section.Get("ah_proposals")
			if esp == ah {
				return fmt.Errorf("%w: %s: exactly one of esp_proposals / ah_proposals", ErrInput, cw)
			}
		}
	}
	return nil
}

// checkSecrets validates the secrets file and that every PSK connection has its secret with
// the connection's identities as owners (and no secret is orphaned).
func checkSecrets(root, conns *Section) error {
	if err := checkKeys("vrx-secrets.conf", root, nil, "secrets"); err != nil {
		return err
	}
	secs := root.Sub("secrets")
	if secs == nil {
		return fmt.Errorf("%w: vrx-secrets.conf: no secrets section", ErrInput)
	}
	if err := checkKeys("secrets", secs, nil, "*"); err != nil {
		return err
	}
	for _, it := range secs.Sections() {
		where := "secrets." + it.Name
		if !strings.HasPrefix(it.Name, "ike-") {
			return fmt.Errorf("%w: %s: only ike-<connection> secrets are rendered", ErrInput, where)
		}
		for _, k := range it.Section.Keys() {
			switch k.Name {
			case "secret":
				if k.Quoted {
					return fmt.Errorf("%w: %s.secret must be base64 (0s…)", ErrInput, where)
				}
				if _, err := decodeSecret(k.Value); err != nil {
					return fmt.Errorf("%w: %s.secret: %v", ErrInput, where, err)
				}
			case "id-local", "id-remote":
				if !k.Quoted {
					return fmt.Errorf("%w: %s.%s must be quoted", ErrInput, where, k.Name)
				}
				if err := idCheck(k.Value); err != nil {
					return fmt.Errorf("%w: %s.%s: %v", ErrInput, where, k.Name, err)
				}
			default:
				return fmt.Errorf("%w: %s: unknown key %s", ErrInput, where, k.Name)
			}
		}
		if _, ok := it.Section.Get("secret"); !ok {
			return fmt.Errorf("%w: %s: secret missing", ErrInput, where)
		}
		conn := conns.Sub("connections").Sub(strings.TrimPrefix(it.Name, "ike-"))
		if conn == nil || authOf(conn, "local") != "psk" {
			return fmt.Errorf("%w: %s has no PSK connection %s", ErrInput, where, strings.TrimPrefix(it.Name, "ike-"))
		}
	}
	for _, it := range conns.Sub("connections").Sections() {
		if authOf(it.Section, "local") != "psk" && authOf(it.Section, "remote") != "psk" {
			continue
		}
		where := "connections." + it.Name
		if authOf(it.Section, "local") != "psk" || authOf(it.Section, "remote") != "psk" {
			return fmt.Errorf("%w: %s: psk must be used on both sides", ErrInput, where)
		}
		s := secs.Sub("ike-" + it.Name)
		if s == nil {
			return fmt.Errorf("%w: %s uses psk but vrx-secrets.conf has no ike-%s", ErrInput, where, it.Name)
		}
		lid, rid := expectedIDs(it.Section)
		if v, _ := s.Get("id-local"); v != lid {
			return fmt.Errorf("%w: secrets.ike-%s.id-local does not match the connection's local identity", ErrInput, it.Name)
		}
		if v, _ := s.Get("id-remote"); v != rid {
			return fmt.Errorf("%w: secrets.ike-%s.id-remote does not match the connection's remote identity", ErrInput, it.Name)
		}
	}
	return nil
}

func authOf(conn *Section, side string) string {
	v, _ := conn.Sub(side).Get("auth")
	return v
}

// expectedIDs are the identities charon matches PSK owners against: the explicit ids, else
// the first local address and %any.
func expectedIDs(conn *Section) (local, remote string) {
	local, ok := conn.Sub("local").Get("id")
	if !ok {
		addrs, _ := conn.Get("local_addrs")
		local = splitList(addrs)[0]
	}
	remote, ok = conn.Sub("remote").Get("id")
	if !ok {
		remote = "%any"
	}
	return local, remote
}

func decodeSecret(v string) ([]byte, error) {
	if !strings.HasPrefix(v, "0s") {
		return nil, errors.New("must be base64 with the 0s prefix")
	}
	b, err := base64.StdEncoding.DecodeString(v[2:])
	if err != nil {
		return nil, errors.New("invalid base64")
	}
	if err := checkPSK(b); err != nil {
		return nil, err
	}
	return b, nil
}

func (r *Renderer) checkConf(root *Section) error {
	if err := checkKeys("strongswan.conf", root, nil, "charon", "swanctl"); err != nil {
		return err
	}
	ch := root.Sub("charon")
	if ch == nil {
		return fmt.Errorf("%w: strongswan.conf: no charon section", ErrInput)
	}
	level := matches(regexp.MustCompile(`^(-1|0|1)$`))
	if err := checkKeys("charon", ch, map[string]keyCheck{
		"load_modular": enum("no"), "load": listOfWords(pluginRe, true), "install_routes": enum("yes", "no"),
		"port": matches(uintRe), "port_nat_t": matches(uintRe),
	}, "plugins", "filelog", "journal", "syslog"); err != nil {
		return err
	}
	uri := "unix://" + r.paths.ViciSocket
	socketIs := func(v string) error {
		if v != uri {
			return fmt.Errorf("must be %s (the socket Apply uses)", uri)
		}
		return nil
	}
	plugins := ch.Sub("plugins")
	if plugins == nil || plugins.Sub("vici") == nil {
		return fmt.Errorf("%w: charon.plugins.vici missing", ErrInput)
	}
	if err := checkKeys("charon.plugins", plugins, nil, "vici"); err != nil {
		return err
	}
	if err := checkKeys("charon.plugins.vici", plugins.Sub("vici"), map[string]keyCheck{"socket": socketIs}); err != nil {
		return err
	}
	if fl := ch.Sub("filelog"); fl != nil {
		if err := checkKeys("charon.filelog", fl, nil, "vrx"); err != nil {
			return err
		}
		if v := fl.Sub("vrx"); v != nil {
			if err := checkKeys("charon.filelog.vrx", v, map[string]keyCheck{
				"path": matches(pathRe), "default": level, "append": enum("yes"), "flush_line": enum("yes"),
			}); err != nil {
				return err
			}
		}
	}
	if j := ch.Sub("journal"); j != nil {
		if err := checkKeys("charon.journal", j, map[string]keyCheck{"default": level}); err != nil {
			return err
		}
	}
	if s := ch.Sub("syslog"); s != nil {
		if err := checkKeys("charon.syslog", s, nil, "daemon"); err != nil {
			return err
		}
		if d := s.Sub("daemon"); d != nil {
			if err := checkKeys("charon.syslog.daemon", d, map[string]keyCheck{"default": level}); err != nil {
				return err
			}
		}
	}
	if sw := root.Sub("swanctl"); sw != nil {
		if err := checkKeys("swanctl", sw, map[string]keyCheck{"socket": socketIs, "load": listOfWords(pluginRe, false)}); err != nil {
			return err
		}
	}
	return nil
}

func listOfWords(re *regexp.Regexp, needVici bool) keyCheck {
	return func(v string) error {
		ws := strings.Fields(v)
		if len(ws) == 0 || strings.Join(ws, " ") != v {
			return errors.New("must be single-space separated words")
		}
		for _, w := range ws {
			if !re.MatchString(w) {
				return fmt.Errorf("plugin %q does not match %s", w, re)
			}
		}
		if needVici && !slices.Contains(ws, "vici") {
			return errors.New("must load vici")
		}
		return nil
	}
}

// ------------------------------------------------------------------------------ VICI plan

// plan is what Apply loads: messages built from the parsed files exactly as swanctl builds
// them from the same files (list keys split on commas, file keys read from the swanctl
// directories, secrets decoded).
type plan struct {
	conns       []namedMsg
	shared      []sharedSecret
	pools       []namedMsg
	authorities []namedMsg
}

type namedMsg struct {
	name string
	msg  *vici.Message
}

// sharedSecret is a load-shared request; String never shows the data.
type sharedSecret struct {
	id     string
	data   []byte
	owners []string
}

func (s sharedSecret) String() string { return "shared secret " + s.id + " (" + Redacted + ")" }

func (s sharedSecret) message() *vici.Message {
	return msg("id", s.id, "type", "IKE", "data", string(s.data), "owners", s.owners)
}

// listKeys are swanctl's is_list_key set; fileListKeys its is_file_list_key set.
var (
	listKeys     = []string{"local_addrs", "remote_addrs", "proposals", "esp_proposals", "ah_proposals", "local_ts", "remote_ts", "vips", "pools", "groups", "cert_policy"}
	fileListKeys = []string{"certs", "cacerts", "pubkeys"}
)

// MaxCertFile bounds a certificate file read for load-conn / load-authority.
const MaxCertFile = 64 << 10

// buildPlan converts parsed trees to VICI messages. readFile loads certificate files (nil in
// Validate, which only checks the structure).
func (r *Renderer) buildPlan(t *trees) (*plan, error) {
	p := &plan{}
	for _, it := range t.conns.Sub("connections").Sections() {
		body, err := r.sectionMsg(it.Section)
		if err != nil {
			return nil, fmt.Errorf("connections.%s: %w", it.Name, err)
		}
		p.conns = append(p.conns, namedMsg{name: it.Name, msg: msg(it.Name, body)})
	}
	if pools := t.conns.Sub("pools"); pools != nil {
		for _, it := range pools.Sections() {
			body := vici.NewMessage()
			for _, k := range it.Section.Keys() {
				if k.Name == "addrs" {
					_ = body.Set(k.Name, k.Value)
				} else {
					_ = body.Set(k.Name, splitList(k.Value))
				}
			}
			p.pools = append(p.pools, namedMsg{name: it.Name, msg: msg(it.Name, body)})
		}
	}
	if auths := t.conns.Sub("authorities"); auths != nil {
		for _, it := range auths.Sections() {
			body := vici.NewMessage()
			v, _ := it.Section.Get("cacert")
			blob, err := r.readCert("cacert", v)
			if err != nil {
				return nil, fmt.Errorf("authorities.%s: %w", it.Name, err)
			}
			_ = body.Set("cacert", string(blob))
			p.authorities = append(p.authorities, namedMsg{name: it.Name, msg: msg(it.Name, body)})
		}
	}
	for _, it := range t.secrets.Sub("secrets").Sections() {
		v, _ := it.Section.Get("secret")
		data, err := decodeSecret(v)
		if err != nil {
			return nil, fmt.Errorf("%w: secrets.%s: %v", ErrInput, it.Name, err)
		}
		s := sharedSecret{id: it.Name, data: data}
		for _, k := range it.Section.Keys() {
			if strings.HasPrefix(k.Name, "id") {
				s.owners = append(s.owners, k.Value)
			}
		}
		p.shared = append(p.shared, s)
	}
	return p, nil
}

// sectionMsg is swanctl's add_key_values + add_sections.
func (r *Renderer) sectionMsg(sec *Section) (*vici.Message, error) {
	m := vici.NewMessage()
	for _, it := range sec.Items {
		switch {
		case it.Kind == KindKey && slices.Contains(listKeys, it.Name):
			_ = m.Set(it.Name, splitList(it.Value))
		case it.Kind == KindKey && slices.Contains(fileListKeys, it.Name):
			var blobs []string
			for _, f := range splitList(it.Value) {
				b, err := r.readCert(it.Name, f)
				if err != nil {
					return nil, err
				}
				blobs = append(blobs, string(b))
			}
			_ = m.Set(it.Name, blobs)
		case it.Kind == KindKey:
			_ = m.Set(it.Name, it.Value)
		case it.Kind == KindSection:
			sub, err := r.sectionMsg(it.Section)
			if err != nil {
				return nil, err
			}
			_ = m.Set(it.Name, sub)
		}
	}
	return m, nil
}

// readCert reads a certificate file named in a file-list key from its swanctl directory
// (F-pki-basic installs them; untested until then). Names are plain file names (certFile).
func (r *Renderer) readCert(key, name string) ([]byte, error) {
	if !certFile.MatchString(name) {
		return nil, fmt.Errorf("%w: %s file %q", ErrInput, key, clip(name))
	}
	p := filepath.Join(r.paths.CertDir(key), name)
	f, err := os.Open(p) //nolint:gosec // directory from Paths, name validated as a plain file name
	if err != nil {
		return nil, fmt.Errorf("%w: %s file %s: %v (certificates are installed by F-pki-basic)", ErrInput, key, name, err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, MaxCertFile+1))
	if err != nil {
		return nil, fmt.Errorf("strongswan: read %s: %w", p, err)
	}
	if len(b) > MaxCertFile {
		return nil, fmt.Errorf("%w: %s file %s exceeds %d bytes", ErrInput, key, name, MaxCertFile)
	}
	return b, nil
}

// connSummary is what the convergence check compares between the rendered files and charon's
// list-conns: the parts of a connection that decide which traffic it protects and with whom.
type connSummary struct {
	Version     string
	LocalAddrs  []string
	RemoteAddrs []string
	Children    map[string]childSummary
}

type childSummary struct {
	Mode              string
	LocalTS, RemoteTS []string
}

func summaryFromTree(conn *Section) connSummary {
	s := connSummary{Children: map[string]childSummary{}}
	switch v, _ := conn.Get("version"); v {
	case "1":
		s.Version = "IKEv1"
	case "2":
		s.Version = "IKEv2"
	default:
		s.Version = "0"
	}
	v, _ := conn.Get("local_addrs")
	s.LocalAddrs = splitList(v)
	v, _ = conn.Get("remote_addrs")
	s.RemoteAddrs = splitList(v)
	for _, ch := range conn.Sub("children").Sections() {
		mode, ok := ch.Section.Get("mode")
		if !ok {
			mode = "tunnel"
		}
		cs := childSummary{Mode: strings.ToUpper(mode), LocalTS: []string{"dynamic"}, RemoteTS: []string{"dynamic"}}
		if v, ok := ch.Section.Get("local_ts"); ok {
			cs.LocalTS = splitList(v)
		}
		if v, ok := ch.Section.Get("remote_ts"); ok {
			cs.RemoteTS = splitList(v)
		}
		s.Children[ch.Name] = cs
	}
	return s
}

func summaryFromVICI(m *vici.Message) connSummary {
	s := connSummary{Version: str(m, "version"), LocalAddrs: strs(m, "local_addrs"), RemoteAddrs: strs(m, "remote_addrs"), Children: map[string]childSummary{}}
	children := sub(m, "children")
	if children != nil {
		for _, name := range children.Keys() {
			c := sub(children, name)
			s.Children[name] = childSummary{Mode: strings.ToUpper(str(c, "mode")), LocalTS: strs(c, "local-ts"), RemoteTS: strs(c, "remote-ts")}
		}
	}
	return s
}

// diff describes the first difference between the desired (a) and the loaded (b) summary.
func (a connSummary) diff(b connSummary) string {
	switch {
	case a.Version != b.Version:
		return fmt.Sprintf("version %s, charon has %s", a.Version, b.Version)
	case !slices.Equal(a.LocalAddrs, b.LocalAddrs):
		return fmt.Sprintf("local_addrs %v, charon has %v", a.LocalAddrs, b.LocalAddrs)
	case !slices.Equal(a.RemoteAddrs, b.RemoteAddrs):
		return fmt.Sprintf("remote_addrs %v, charon has %v", a.RemoteAddrs, b.RemoteAddrs)
	case len(a.Children) != len(b.Children):
		return fmt.Sprintf("%d children, charon has %d", len(a.Children), len(b.Children))
	}
	for name, ac := range a.Children {
		bc, ok := b.Children[name]
		switch {
		case !ok:
			return "child " + name + " missing"
		case ac.Mode != bc.Mode:
			return fmt.Sprintf("child %s mode %s, charon has %s", name, ac.Mode, bc.Mode)
		case !slices.Equal(ac.LocalTS, bc.LocalTS):
			return fmt.Sprintf("child %s local_ts %v, charon has %v", name, ac.LocalTS, bc.LocalTS)
		case !slices.Equal(ac.RemoteTS, bc.RemoteTS):
			return fmt.Sprintf("child %s remote_ts %v, charon has %v", name, ac.RemoteTS, bc.RemoteTS)
		}
	}
	return ""
}

// atoi64 parses a decimal counter from VICI ("" → 0).
func atoi64(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
