package strongswan

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

// testPSK is the fixture secret (00-CONTEXT: test fixtures use VRX_TEST_PSK_<id>).
const testPSK = "VRX_TEST_PSK_RF2"

func testResolver(values map[string]string) SecretResolver {
	return SecretResolverFunc(func(_ context.Context, ref string) ([]byte, error) {
		v, ok := values[ref]
		if !ok {
			return nil, errors.New("no such secret " + ref)
		}
		return []byte(v), nil
	})
}

func unitPaths(t *testing.T) Paths {
	t.Helper()
	dir := t.TempDir()
	p := Paths{
		StrongswanConf: filepath.Join(dir, "strongswan.conf"),
		SwanctlDir:     filepath.Join(dir, "swanctl"),
		ViciSocket:     filepath.Join(dir, "charon.vici"),
		LogFile:        filepath.Join(dir, "charon.log"),
		ConfMode:       0o640,
		SecretMode:     0o600,
	}
	if err := os.MkdirAll(filepath.Join(p.SwanctlDir, "conf.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

// goldenPaths are fixed paths so golden files do not depend on the temp dir.
func goldenPaths() Paths {
	p := TestPaths("w3", "a")
	return p
}

func doc(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return ds
}

// fullDoc covers the document → model paths: IKEv2/IKEv1, psk/cert, tunnel/transport, esp/ah,
// dpd (v1 timeout), mobike, fragmentation, reauth, anti-replay off, ESN, route-based if_id,
// a hostname remote, and tunnels that are not rendered (disabled, vpp-ikev2).
const fullDoc = `{"vpn":{"ipsec":{
 "proposals":{
  "gcm":{"ike":{"encr":"aes256gcm16","dh":"curve25519"},"esp":{"encr":"aes256gcm16"}},
  "cbc":{"ike":{"encr":"aes256","integ":"sha256","prf":"prfsha256","dh":"modp2048"},"esp":{"encr":"aes128","integ":"sha256","dh":"modp2048"}},
  "ah":{"ike":{"encr":"aes128","integ":"sha1","dh":"ecp256"},"esp":{"encr":"null","integ":"sha384"}}
 },
 "tunnels":{
  "w3-site-a":{"enabled":true,"description":"HQ \"primary\" # uplink","ikeVersion":2,"mode":"tunnel","protocol":"esp",
   "localAddr":"10.3.250.1","remoteAddr":"10.3.250.2","auth":{"method":"psk","secretRef":"psk/w3-site-a"},
   "proposal":"gcm","localTs":["10.3.1.0/24"],"remoteTs":["10.3.2.9/24","2001:db8:3::/64"],
   "dpd":{"enabled":true,"delaySec":10,"action":"restart"},"mobike":false,"fragmentation":"yes",
   "rekey":{"ikeSec":14400,"espSec":3600},"startAction":"trap","closeAction":"none"},
  "w3.site-b":{"ikeVersion":1,"mode":"transport","localAddr":"10.3.250.1","remoteAddr":"10.3.250.3",
   "localId":"gw-a.example.net","remoteId":"C=CH, O=Example, CN=gw-b",
   "auth":{"method":"psk","secretRef":"psk/w3-site-b"},"proposal":"cbc",
   "dpd":{"enabled":true,"delaySec":30,"timeoutSec":150,"action":"clear"},
   "rekey":{"ikeSec":28800,"espSec":1800,"espBytes":1000000000,"espPackets":5000000,"reauth":true},
   "startAction":"none","esn":true,"antiReplay":false},
  "w3-cert-c":{"ikeVersion":2,"protocol":"ah","localAddr":"2001:db8:3::1","remoteAddr":"peer-c.example.net",
   "auth":{"method":"cert","certificate":"gw-a","remoteCa":"lab-ca"},"proposal":"ah",
   "localTs":["2001:db8:3:1::/64"],"remoteTs":["2001:db8:3:2::/64"],"startAction":"none","dpd":{"enabled":false}},
  "w3-route-d":{"localAddr":"10.3.250.1","remoteAddr":"10.3.250.4","auth":{"method":"psk","secretRef":"psk/w3-route-d"},
   "proposal":"gcm","routeBased":{"ipipInterface":"ipip3"},"startAction":"start"},
  "w3-off":{"enabled":false,"localAddr":"10.3.250.1","remoteAddr":"10.3.250.5","auth":{"method":"psk","secretRef":"psk/none"},"proposal":"gcm"},
  "w3-native":{"engine":"vpp-ikev2","localAddr":"10.3.250.1","remoteAddr":"10.3.250.6","auth":{"method":"psk","secretRef":"psk/none"},"proposal":"gcm"}
 }}}}`

var fullSecrets = map[string]string{
	"psk/w3-site-a":  testPSK + "_site_a",
	"psk/w3-site-b":  testPSK + "_site_b with spaces, \"quotes\" and # hash",
	"psk/w3-route-d": testPSK + "_route_d",
}

func ifIDs(name string) (uint32, uint32, bool) {
	if name == "ipip3" {
		return 3001, 3001, true
	}
	return 0, 0, false
}

func newTestRenderer(t *testing.T, p Paths, opts ...Option) *Renderer {
	t.Helper()
	base := []Option{
		WithPaths(p), WithSecretResolver(testResolver(fullSecrets)), WithIfIDMapper(ifIDs),
		WithDaemonConfig(DaemonConfig{Plugins: DefaultPlugins(), LogLevel: 1, QuietJournal: true}),
	}
	return New(append(base, opts...)...)
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil { //nolint:gosec // test data
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // test data
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from golden:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

func TestRenderGoldenFull(t *testing.T) {
	r := newTestRenderer(t, goldenPaths())
	files, err := r.Render(context.Background(), doc(t, fullDoc))
	if err != nil {
		t.Fatal(err)
	}
	p := goldenPaths()
	checkGolden(t, "full-vrx.conf", files[p.ConnsFile()].Content)
	checkSecretsGolden(t, "full-vrx-secrets.conf", files[p.SecretsFile()].Content, map[string]string{
		"ike-w3-site-a": fullSecrets["psk/w3-site-a"], "ike-w3+site-b": fullSecrets["psk/w3-site-b"], "ike-w3-route-d": fullSecrets["psk/w3-route-d"],
	})
	checkGolden(t, "full-strongswan.conf", files[p.StrongswanConf].Content)
	if f := files[p.SecretsFile()]; !f.Secret || f.Mode != 0o600 {
		t.Errorf("secrets file: Secret=%v mode=%v, want Secret 0600", f.Secret, f.Mode)
	}
	if f := files[p.ConnsFile()]; f.Secret || f.Mode != 0o640 {
		t.Errorf("vrx.conf: Secret=%v mode=%v, want plain 0640", f.Secret, f.Mode)
	}
	// Structural validation accepts what Render produced.
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatalf("Validate(Render()) = %v", err)
	}
	// The PSKs are only in the secrets file, and only as base64.
	for ref, psk := range fullSecrets {
		for path, f := range files {
			if bytes.Contains(f.Content, []byte(psk)) {
				t.Errorf("plaintext of %s found in %s", ref, path)
			}
		}
		if !bytes.Contains(files[p.SecretsFile()].Content, []byte(b64(psk))) {
			t.Errorf("base64 of %s missing from the secrets file", ref)
		}
	}
}

// checkSecretsGolden compares the secrets file with its golden after masking the secret
// values (a committed golden must not hold key material, not even fixture keys in base64 —
// gitleaks would rightly flag it) and checks each decoded value separately.
func checkSecretsGolden(t *testing.T, name string, got []byte, want map[string]string) {
	t.Helper()
	tree, err := ParseSettings(name, got)
	if err != nil {
		t.Fatal(err)
	}
	secs := tree.Sub("secrets").Sections()
	if len(secs) != len(want) {
		t.Errorf("%s: %d secrets, want %d", name, len(secs), len(want))
	}
	for _, it := range secs {
		v, _ := it.Section.Get("secret")
		b, err := decodeSecret(v)
		if err != nil || string(b) != want[it.Name] {
			t.Errorf("%s: secret of %s does not decode to the resolved value (err %v)", name, it.Name, err)
		}
	}
	checkGolden(t, name, []byte(secretAssignRe.ReplaceAllString(string(got), "${1}"+Redacted)))
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// TestRenderStructInput: the D-055 stand-in (*structpb.Struct document) renders the same bytes.
func TestRenderStructInput(t *testing.T) {
	r := newTestRenderer(t, goldenPaths())
	typed, err := r.Render(context.Background(), doc(t, fullDoc))
	if err != nil {
		t.Fatal(err)
	}
	st := &structpb.Struct{}
	if err := protojson.Unmarshal([]byte(fullDoc), st); err != nil {
		t.Fatal(err)
	}
	loose, err := r.Render(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	for p, f := range typed {
		if !bytes.Equal(f.Content, loose[p].Content) {
			t.Errorf("%s differs between typed and struct input", p)
		}
	}
}

// modelExtras covers template paths the document cannot produce yet: several children,
// pools, authorities, encap, if_id without route-based, rekey bytes/packets on one child.
func modelExtras() *Model {
	zero := uint32(0)
	return &Model{
		Conns: []Conn{{
			Name: "w3-multi", Version: 2, LocalAddrs: []string{"10.3.250.1"}, RemoteAddrs: []string{"%any"},
			Proposals: []string{"aes128gcm16-prfsha256-ecp256", "aes256-sha384-prfsha384-ecp384"},
			Local:     Auth{Method: "pubkey", ID: "@gw-a.example.net", Certs: []string{"gw-a.pem"}},
			Remote:    Auth{Method: "pubkey", CACerts: []string{"lab-ca.pem"}},
			Encap:     true, DPDDelay: 20, RekeyTime: 3600, Pools: []string{"w3-pool4", "w3-pool6"},
			Children: []Child{
				{Name: "w3-net2", LocalTS: []string{"10.3.1.0/24"}, RemoteTS: []string{"10.3.2.0/24"}, ESPProposals: []string{"aes128gcm16-ecp256"}, Mode: "tunnel", StartAction: "trap", DPDAction: "trap", RekeyTime: 1200, IfIDIn: 7, IfIDOut: 8},
				{Name: "w3-net1", ESPProposals: []string{"aes256-sha256", "aes128-sha1"}, Mode: "transport", CloseAction: "start", RekeyBytes: 1 << 30, RekeyPackets: 1 << 20, ReplayWindow: &zero},
			},
		}},
		Pools:       []Pool{{Name: "w3-pool6", Addrs: "2001:db8:3:ff::/112"}, {Name: "w3-pool4", Addrs: "10.3.99.0/24", DNS: []string{"10.3.0.53", "2001:db8:3::53"}}},
		Authorities: []Authority{{Name: "lab-ca", CACert: "lab-ca.pem"}},
	}
}

func TestRenderGoldenModelExtras(t *testing.T) {
	r := newTestRenderer(t, goldenPaths())
	files, err := r.RenderModel(modelExtras())
	if err != nil {
		t.Fatal(err)
	}
	p := goldenPaths()
	checkGolden(t, "extras-vrx.conf", files[p.ConnsFile()].Content)
	checkSecretsGolden(t, "extras-vrx-secrets.conf", files[p.SecretsFile()].Content, nil)
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatalf("Validate(RenderModel()) = %v", err)
	}
}

func TestRenderGoldenEmptyAndProduct(t *testing.T) {
	r := New(WithSecretResolver(testResolver(nil)))
	files, err := r.Render(context.Background(), &vrxv1.DesiredState{})
	if err != nil {
		t.Fatal(err)
	}
	p := ProductPaths()
	checkGolden(t, "empty-vrx.conf", files[p.ConnsFile()].Content)
	checkSecretsGolden(t, "empty-vrx-secrets.conf", files[p.SecretsFile()].Content, nil)
	checkGolden(t, "product-strongswan.conf", files[p.StrongswanConf].Content)
	for path, f := range files {
		if f.Owner != "root:root" {
			t.Errorf("%s owner %q, want root:root", path, f.Owner)
		}
	}
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatal(err)
	}
}

func TestRenderDeterministic(t *testing.T) {
	r := newTestRenderer(t, goldenPaths())
	a, err := r.Render(context.Background(), doc(t, fullDoc))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		b, err := r.Render(context.Background(), doc(t, fullDoc))
		if err != nil {
			t.Fatal(err)
		}
		for p := range a {
			if !bytes.Equal(a[p].Content, b[p].Content) {
				t.Fatalf("render %d of %s differs", i, p)
			}
		}
	}
}

// ------------------------------------------------------------------------------ hostile input

// hostile strings (RF-2 prompt, README "Tests", helpers_template_test.go).
var hostile = []string{
	`"; rm -rf /`,
	"}\ninclude /etc/passwd\n{",
	"include /etc/passwd",
	"#",
	"a # b",
	"a\nb",
	"a\r\nb",
	"a\x00b",
	"a\x1bb",
	"a b",
	"\xff\xfe",
	"ünïcødé-مرحبا",
	strings.Repeat("A", 5*1024),
	"${charon.plugins}",
	"a:b",
	"a.b.c",
	"a,b",
	`a"b`,
	`a\b`,
	"a}b",
	"a{b",
	" leading",
	"%any",
}

// tunnelWith returns fullDoc's first tunnel with one field replaced.
func tunnelWith(t *testing.T, name string, mutate func(tun map[string]any)) *structpb.Struct {
	t.Helper()
	tun := map[string]any{
		"localAddr": "10.3.250.1", "remoteAddr": "10.3.250.2", "proposal": "gcm",
		"auth":    map[string]any{"method": "psk", "secretRef": "psk/w3-site-a"},
		"localTs": []any{"10.3.1.0/24"}, "remoteTs": []any{"10.3.2.0/24"},
	}
	mutate(tun)
	st, err := structpb.NewStruct(map[string]any{"vpn": map[string]any{"ipsec": map[string]any{
		"proposals": map[string]any{"gcm": map[string]any{"ike": map[string]any{"encr": "aes256gcm16", "dh": "curve25519"}, "esp": map[string]any{"encr": "aes256gcm16"}}},
		"tunnels":   map[string]any{name: tun},
	}}})
	if err != nil {
		return nil // structpb cannot hold invalid UTF-8: such input never reaches the agent
	}
	return st
}

// TestHostileStringsRejectedOrEscaped puts every hostile string into every user field. The
// result must be an error, or files that (a) pass the strict round-trip parser and Validate,
// (b) contain exactly one connection with one child, and (c) never contain the hostile string
// outside a quoted value or a comment.
func TestHostileStringsRejectedOrEscaped(t *testing.T) {
	fields := map[string]func(tun map[string]any, s string){
		"name":        nil, // the map key
		"description": func(tun map[string]any, s string) { tun["description"] = s },
		"localId":     func(tun map[string]any, s string) { tun["localId"] = s },
		"remoteId":    func(tun map[string]any, s string) { tun["remoteId"] = s },
		"remoteAddr":  func(tun map[string]any, s string) { tun["remoteAddr"] = s },
		"localAddr":   func(tun map[string]any, s string) { tun["localAddr"] = s },
		"localTs":     func(tun map[string]any, s string) { tun["localTs"] = []any{s} },
		"remoteTs":    func(tun map[string]any, s string) { tun["remoteTs"] = []any{s} },
		"secretRef":   func(tun map[string]any, s string) { tun["auth"] = map[string]any{"method": "psk", "secretRef": s} },
		"certificate": func(tun map[string]any, s string) { tun["auth"] = map[string]any{"method": "cert", "certificate": s} },
		"remoteCa": func(tun map[string]any, s string) {
			tun["auth"] = map[string]any{"method": "cert", "certificate": "gw", "remoteCa": s}
		},
		"proposal":      func(tun map[string]any, s string) { tun["proposal"] = s },
		"mode":          func(tun map[string]any, s string) { tun["mode"] = s },
		"startAction":   func(tun map[string]any, s string) { tun["startAction"] = s },
		"fragmentation": func(tun map[string]any, s string) { tun["fragmentation"] = s },
		"dpd.action":    func(tun map[string]any, s string) { tun["dpd"] = map[string]any{"enabled": true, "action": s} },
		"psk-value":     nil, // the resolved secret
	}
	for field, set := range fields {
		for _, h := range hostile {
			name, value := "w3-hostile", h
			var st *structpb.Struct
			secrets := map[string]string{"psk/w3-site-a": testPSK + "_hostile"}
			switch field {
			case "name":
				name = h
				st = tunnelWith(t, name, func(map[string]any) {})
			case "psk-value":
				secrets["psk/w3-site-a"] = h
				st = tunnelWith(t, name, func(map[string]any) {})
			default:
				st = tunnelWith(t, name, func(tun map[string]any) { set(tun, value) })
			}
			if st == nil {
				continue // structpb cannot hold invalid UTF-8 keys/values
			}
			r := newTestRenderer(t, goldenPaths(), WithSecretResolver(testResolver(secrets)))
			files, err := r.Render(context.Background(), st)
			if err != nil {
				if !errors.Is(err, ErrInput) && !errors.Is(err, renderers.ErrUnsafe) {
					t.Errorf("%s=%q: error %v is neither ErrInput nor ErrUnsafe", field, clip(h), err)
				}
				if field == "psk-value" && strings.Contains(err.Error(), h) && len(h) > 3 {
					t.Errorf("%s: error leaks the secret: %v", field, err)
				}
				continue
			}
			if err := r.Validate(context.Background(), files); err != nil {
				t.Errorf("%s=%q: rendered but Validate failed: %v", field, clip(h), err)
				continue
			}
			tree, err := ParseSettings("vrx.conf", files[goldenPaths().ConnsFile()].Content)
			if err != nil {
				t.Fatal(err)
			}
			conns := tree.Sub("connections").Sections()
			if len(conns) != 1 || len(conns[0].Section.Sub("children").Sections()) != 1 {
				t.Errorf("%s=%q: structure changed: %d connections", field, clip(h), len(conns))
			}
			// Where does the hostile string appear? Only inside a quoted value, a comment, or
			// (secret) nowhere.
			for p, f := range files {
				for _, line := range strings.Split(string(f.Content), "\n") {
					if !strings.Contains(line, h) || len(h) < 2 {
						continue
					}
					trim := strings.TrimLeft(line, "\t")
					quoted := strings.Contains(trim, ` = "`)
					bareToken := tokenRe.MatchString(h) // validated host/file name: safe unquoted
					if field == "psk-value" || !(quoted || bareToken || strings.HasPrefix(trim, "# ")) {
						t.Errorf("%s=%q appears raw in %s: %q", field, clip(h), p, clip(line))
					}
				}
			}
		}
	}
}

// TestRenderRejects: semantic errors are ErrInput and name the field.
func TestRenderRejects(t *testing.T) {
	cases := map[string]func(tun map[string]any){
		"unknown proposal":     func(tun map[string]any) { tun["proposal"] = "nope" },
		"family mismatch":      func(tun map[string]any) { tun["remoteAddr"] = "2001:db8::2" },
		"same address":         func(tun map[string]any) { tun["remoteAddr"] = "10.3.250.1" },
		"no traffic selectors": func(tun map[string]any) { delete(tun, "localTs") },
		"route-based w/o if_id": func(tun map[string]any) {
			tun["routeBased"] = map[string]any{"ipipInterface": "nope"}
		},
		"inline psk":       func(tun map[string]any) { tun["auth"] = map[string]any{"method": "psk", "secretRef": testPSK} },
		"password ref":     func(tun map[string]any) { tun["auth"] = map[string]any{"method": "psk", "secretRef": "password/x"} },
		"unresolvable":     func(tun map[string]any) { tun["auth"] = map[string]any{"method": "psk", "secretRef": "psk/missing"} },
		"ikev1 mobike":     func(tun map[string]any) { tun["ikeVersion"] = 1; tun["mobike"] = true },
		"ike version 3":    func(tun map[string]any) { tun["ikeVersion"] = 3 },
		"ikev1 aead ike":   func(tun map[string]any) { tun["ikeVersion"] = 1 },
		"start to any":     func(tun map[string]any) { tun["remoteAddr"] = "%any"; tun["startAction"] = "start" },
		"all-numeric host": func(tun map[string]any) { tun["remoteAddr"] = "203.0.113.999" },
		"dn with quote":    func(tun map[string]any) { tun["remoteId"] = `CN=a"b` },
		"description 256":  func(tun map[string]any) { tun["description"] = strings.Repeat("d", 256) },
		"dpd delay 0":      func(tun map[string]any) { tun["dpd"] = map[string]any{"enabled": true, "delaySec": 0} },
		"rekey too short":  func(tun map[string]any) { tun["rekey"] = map[string]any{"ikeSec": 10} },
		"engine unknown":   func(tun map[string]any) { tun["engine"] = "racoon" },
		"ts not cidr":      func(tun map[string]any) { tun["localTs"] = []any{"10.3.1.0"} },
		"transport+route": func(tun map[string]any) {
			tun["mode"] = "transport"
			tun["routeBased"] = map[string]any{"ipipInterface": "ipip3"}
		},
		"ah with aead":       func(tun map[string]any) { tun["protocol"] = "ah" },
		"protocol unknown":   func(tun map[string]any) { tun["protocol"] = "gre" },
		"short psk resolved": nil,
	}
	for name, mutate := range cases {
		secrets := map[string]string{"psk/w3-site-a": testPSK}
		st := tunnelWith(t, "w3-t", func(tun map[string]any) {
			if mutate != nil {
				mutate(tun)
			}
		})
		if name == "short psk resolved" {
			secrets["psk/w3-site-a"] = "short"
		}
		r := newTestRenderer(t, goldenPaths(), WithSecretResolver(testResolver(secrets)), WithIfIDMapper(ifIDs))
		_, err := r.Render(context.Background(), st)
		if err == nil {
			t.Errorf("%s: rendered, want an error", name)
			continue
		}
		if name != "unresolvable" && !errors.Is(err, ErrInput) {
			t.Errorf("%s: %v is not ErrInput", name, err)
		}
		if strings.Contains(err.Error(), testPSK) {
			t.Errorf("%s: error leaks the PSK: %v", name, err)
		}
	}
}

func TestDuplicatePSKIdentitiesRejected(t *testing.T) {
	st := tunnelWith(t, "w3-a", func(map[string]any) {})
	tuns := st.Fields["vpn"].GetStructValue().Fields["ipsec"].GetStructValue().Fields["tunnels"].GetStructValue()
	tuns.Fields["w3-b"] = proto.Clone(tuns.Fields["w3-a"]).(*structpb.Value) //nolint:forcetypeassert // test
	r := newTestRenderer(t, goldenPaths(), WithSecretResolver(testResolver(map[string]string{"psk/w3-site-a": testPSK})))
	if _, err := r.Render(context.Background(), st); err == nil || !strings.Contains(err.Error(), "same identities") {
		t.Fatalf("err = %v, want same-identities rejection", err)
	}
}

func TestConnNameMapping(t *testing.T) {
	for in, want := range map[string]string{"site.a": "site+a", "a-b_c": "a-b_c", "x.y.z": "x+y+z"} {
		got, err := ConnName(in)
		if err != nil || got != want || TunnelName(got) != in {
			t.Errorf("ConnName(%q) = %q, %v; TunnelName back = %q", in, got, err, TunnelName(got))
		}
	}
	for _, bad := range []string{"", ".a", "a b", "include", "INCLUDE", "a:b", strings.Repeat("a", 65), "a+b"} {
		if _, err := ConnName(bad); err == nil {
			t.Errorf("ConnName(%q) accepted", bad)
		}
	}
}

func TestIdentityShapes(t *testing.T) {
	ok := map[string]string{
		"10.3.250.1": "10.3.250.1", "2001:DB8::1": "2001:db8::1", "gw.example.net": "gw.example.net", "@gw.example.net": "@gw.example.net",
		"vpn@example.net": "vpn@example.net", "@#0a1b2c": "@#0a1b2c", "C=CH, O=Example, CN=gw": "C=CH, O=Example, CN=gw", "/C=CH/CN=gw": "/C=CH/CN=gw", "%any": "%any",
	}
	for in, want := range ok {
		if got, err := Identity(in); err != nil || got != want {
			t.Errorf("Identity(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range append([]string{"", "a b", "CN=a\"b", "CN=a#b", "CN=a\\b", "1.2.3", "fe80::1%eth0", "@", "a@", strings.Repeat("a", 256)}, hostile...) {
		if bad == "%any" || bad == "a.b.c" { // valid identities (%any, an FQDN)
			continue
		}
		if _, err := Identity(bad); err == nil {
			t.Errorf("Identity(%q) accepted", clip(bad))
		}
	}
}

func TestProposalGrammar(t *testing.T) {
	good := map[string][]string{
		"ike": {"aes128gcm16-prfsha256-ecp256", "aes256-sha256-ecp256", "aes256-sha256-prfsha256-modp2048", "chacha20poly1305-prfsha512-curve25519"},
		"esp": {"aes128gcm16", "aes128gcm16-ecp256", "aes256-sha256", "aes256-sha256-modp2048-esn", "null-sha256"},
		"ah":  {"sha256", "sha384-ecp384", "sha1-noesn"},
	}
	bad := map[string][]string{
		"ike": {"aes128gcm16-ecp256", "aes256-ecp256", "aes128gcm16-sha256-prfsha256-ecp256", "aes256-sha256", "aes256-sha256-ecp256;x", "des-sha1-modp768", ""},
		"esp": {"aes256", "aes128gcm16-sha256", "aes256-sha256-foo", "aes256-sha256-ecp256-esn-esn"},
		"ah":  {"aes256-sha256", ""},
	}
	for kind, ps := range good {
		for _, p := range ps {
			if err := CheckProposal(kind, p); err != nil {
				t.Errorf("CheckProposal(%s, %q) = %v", kind, p, err)
			}
		}
	}
	for kind, ps := range bad {
		for _, p := range ps {
			if err := CheckProposal(kind, p); err == nil {
				t.Errorf("CheckProposal(%s, %q) accepted", kind, p)
			}
		}
	}
	if p, err := IKEProposal("aes128gcm16", "", "", "ecp256"); err != nil || p != "aes128gcm16-prfsha256-ecp256" {
		t.Errorf("AEAD IKE proposal without PRF = %q, %v", p, err)
	}
}

func TestDaemonConfigChecks(t *testing.T) {
	for name, d := range map[string]DaemonConfig{
		"no vici":     {Plugins: []string{"random", "kernel-netlink"}},
		"bad plugin":  {Plugins: []string{"vici", "x y"}},
		"log level 2": {Plugins: DefaultPlugins(), LogLevel: 2},
		"empty":       {},
	} {
		r := New(WithDaemonConfig(d))
		if _, err := r.Render(context.Background(), nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := New(WithPaths(Paths{StrongswanConf: "etc/strongswan.conf"})).Render(context.Background(), nil); err == nil {
		t.Error("relative path accepted")
	}
	if _, err := New(WithChecker(Checker{ViciSocket: ProductPaths().ViciSocket})).Render(context.Background(), nil); err == nil {
		t.Error("checker on the live socket accepted")
	}
}
