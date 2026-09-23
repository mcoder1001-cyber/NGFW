package strongswan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ngfw/agent/internal/renderers"
)

func TestParseSettingsRoundTrip(t *testing.T) {
	src := "# header\nconnections {\n\ta {\n\t\t# note: \"x\" } { #\n\t\tversion = 2\n\t\tid = \"C=CH, O=a \\\"b\\\" \\\\ c\"\n\t\tchildren {\n\t\t}\n\t}\n}\n\nsecrets {\n}\n"
	tree, err := RoundTrip("x", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	a := tree.Sub("connections").Sub("a")
	if v, _ := a.Get("id"); v != `C=CH, O=a "b" \ c` {
		t.Errorf("id = %q", v)
	}
	if v, _ := a.Get("version"); v != "2" {
		t.Errorf("version = %q", v)
	}
}

// TestParseSettingsRejects: everything outside the canonical subset, in particular the
// constructs strongSwan's own lexer would treat as structure.
func TestParseSettingsRejects(t *testing.T) {
	bad := map[string]string{
		"include":             "include /etc/passwd\n",
		"include in section":  "a {\n\tinclude /etc/passwd\n}\n",
		"include section":     "include {\n}\n",
		"reference":           "a : b {\n}\n",
		"dotted name":         "a.b {\n}\n",
		"multi-token value":   "a = b c\n",
		"value with brace":    "a = b}\n",
		"value with hash":     "a = b#c\n",
		"trailing comment":    "a = b # c\n",
		"bad escape":          "a = \"b\\nc\"\n",
		"unterminated quote":  "a = \"b\n",
		"unescaped quote":     "a = \"b\"c\"\n",
		"unbalanced close":    "}\n",
		"unclosed":            "a {\n",
		"no final newline":    "a = b",
		"CR":                  "a = b\r\n",
		"space indent":        "a {\n  b = c\n}\n",
		"wrong depth":         "a {\n\t\tb = c\n}\n",
		"trailing blank":      "a = b \n",
		"duplicate key":       "a = b\na = c\n",
		"duplicate section":   "a {\n}\na {\n}\n",
		"NUL":                 "a = b\x00\n",
		"ESC in comment":      "# a\x1b[2J\n",
		"U+2028":              "# a b\n",
		"invalid UTF-8":       "# \xff\n",
		"empty key":           " = b\n",
		"key with colon":      "a:b = c\n",
		"indented empty line": "a {\n\t\n}\n",
		"deep nesting":        strings.Repeat("a {\n", 9) + strings.Repeat("}\n", 9),
	}
	for name, src := range bad {
		if _, err := RoundTrip(name, []byte(src)); err == nil {
			t.Errorf("%s: accepted %q", name, src)
		} else if !errors.Is(err, ErrSyntax) {
			t.Errorf("%s: %v is not ErrSyntax", name, err)
		}
	}
	// Errors never quote line content (the secrets file goes through the same parser).
	_, err := RoundTrip("vrx-secrets.conf", []byte("secrets {\n\tike-a {\n\t\tsecret = VRX_TEST_PSK_RF2 leaked\n\t}\n}\n"))
	if err == nil || strings.Contains(err.Error(), "VRX_TEST_PSK") {
		t.Errorf("parse error leaks content: %v", err)
	}
}

func TestRoundTripRequiresCanonicalForm(t *testing.T) {
	// A quoted value that did not need escaping in a different way re-serialises identically;
	// a bare value written quoted for a key that is bare-typed is caught by Validate instead.
	if _, err := RoundTrip("x", []byte("a = \"b\"\n")); err != nil {
		t.Errorf("quoted value: %v", err)
	}
}

// TestValidateRejectsTamperedFiles: Validate re-checks files it did not render.
func TestValidateRejectsTamperedFiles(t *testing.T) {
	r := newTestRenderer(t, goldenPaths())
	files, err := r.Render(context.Background(), doc(t, fullDoc))
	if err != nil {
		t.Fatal(err)
	}
	p := goldenPaths()
	edit := func(path, old, new string) renderers.Files {
		out := renderers.Files{}
		for k, v := range files {
			out[k] = v
		}
		f := out[path]
		if !strings.Contains(string(f.Content), old) {
			t.Fatalf("fixture: %q not in %s", old, path)
		}
		f.Content = []byte(strings.Replace(string(f.Content), old, new, 1))
		out[path] = f
		return out
	}
	cases := map[string]renderers.Files{
		"unknown key":          edit(p.ConnsFile(), "\t\tversion = 2\n", "\t\tversion = 2\n\t\tupdown = /tmp/x\n"),
		"bad proposal":         edit(p.ConnsFile(), "aes256gcm16-prfsha256-curve25519", "aes256gcm16-curve25519"),
		"unmasked ts":          edit(p.ConnsFile(), "10.3.1.0/24", "10.3.1.1/24"),
		"quoted bare key":      edit(p.ConnsFile(), "mode = tunnel", "mode = \"tunnel\""),
		"unquoted id":          edit(p.ConnsFile(), "id = \"10.3.250.2\"", "id = 10.3.250.2"),
		"child w/o proposals":  edit(p.ConnsFile(), "\t\t\t\tah_proposals = sha384\n", ""),
		"secret quoted":        edit(p.SecretsFile(), "secret = 0s", "secret = \"0s"),
		"secret not base64":    edit(p.SecretsFile(), "secret = 0s", "secret = 0x"),
		"secret ids mismatch":  edit(p.SecretsFile(), "id-remote = \"10.3.250.2\"", "id-remote = \"10.3.250.9\""),
		"orphan secret":        edit(p.SecretsFile(), "ike-w3-site-a {", "ike-w3-other {"),
		"log level 4":          edit(p.StrongswanConf, "default = 1", "default = 4"),
		"foreign vici socket":  edit(p.StrongswanConf, "socket = unix:///run/vrx-test/w3/swan/a/charon.vici\n\t\t}", "socket = unix:///var/run/charon.vici\n\t\t}"),
		"plugin list w/o vici": edit(p.StrongswanConf, " vici\"", "\""),
		"not canonical":        edit(p.ConnsFile(), "\t\tversion = 2\n", "\t\tversion  = 2\n"),
	}
	for name, fs := range cases {
		if err := r.Validate(context.Background(), fs); err == nil {
			t.Errorf("%s: Validate accepted tampered files", name)
		} else if strings.Contains(err.Error(), testPSK) {
			t.Errorf("%s: error leaks the PSK: %v", name, err)
		}
	}
	// Wrong file set.
	delete(files, p.SecretsFile())
	if err := r.Validate(context.Background(), files); err == nil {
		t.Error("missing secrets file accepted")
	}
}
