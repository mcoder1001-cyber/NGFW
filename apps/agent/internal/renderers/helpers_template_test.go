package renderers

import (
	"errors"
	"strings"
	"testing"
)

// Hostile inputs every template path must survive (RENDERER-FACTORY-TEMPLATE acceptance).
var hostile = []string{
	`"; rm -rf /`,
	"desc\nno service password-encryption",
	"desc\r\nline",
	"nul\x00byte",
	"tab\tinside",
	"esc\x1b[31m",
	"para\u2028sep",
	"\xff\xfeinvalid utf8",
}

func TestIdent(t *testing.T) {
	for _, ok := range []string{"loop200", "GigabitEthernet0/0/0", "vrf-cust_a", "peer.example.org", "fe80::1", "user@host"} {
		if got, err := Ident(ok); err != nil || got != ok {
			t.Errorf("Ident(%q) = %q, %v", ok, got, err)
		}
	}
	for _, bad := range append([]string{"", "-leading", "with space", `quo"te`, "semi;colon", "uni\u00e9", strings.Repeat("a", MaxIdentLen+1)}, hostile...) {
		if _, err := Ident(bad); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Ident(%q) accepted (%v)", bad, err)
		}
	}
}

func TestLineAndQuoted(t *testing.T) {
	if got, err := Line("Uplink to ISP (primary) - 10G"); err != nil || got != "Uplink to ISP (primary) - 10G" {
		t.Fatalf("Line: %q %v", got, err)
	}
	if got, err := Line("فارسی description"); err != nil || !strings.HasPrefix(got, "فارسی") {
		t.Fatalf("Line must accept non-ASCII text: %q %v", got, err)
	}
	got, err := Quoted(`say "hi" \ bye`)
	if err != nil || got != `"say \"hi\" \\ bye"` {
		t.Fatalf("Quoted = %s, %v", got, err)
	}
	got, err = Quoted(`"; rm -rf /`)
	if err != nil || got != `"\"; rm -rf /"` {
		t.Fatalf("Quoted hostile = %s, %v (must be escaped, not rejected)", got, err)
	}
	for _, bad := range hostile[1:] {
		if _, err := Line(bad); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Line(%q) accepted", bad)
		}
		if _, err := Quoted(bad); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Quoted(%q) accepted", bad)
		}
	}
}

func TestJSONString(t *testing.T) {
	got, err := JSONString("a\nb\"c")
	if err != nil || got != `"a\nb\"c"` {
		t.Fatalf("JSONString = %s, %v", got, err)
	}
	if _, err := JSONString("\xff"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("invalid UTF-8: %v", err)
	}
}

func TestAddrPrefixNetwork(t *testing.T) {
	if got, _ := Addr("2001:DB8:0:0:0:0:0:1"); got != "2001:db8::1" {
		t.Fatalf("Addr canonical = %q", got)
	}
	if got, _ := Addr("10.2.1.1"); got != "10.2.1.1" {
		t.Fatalf("Addr v4 = %q", got)
	}
	for _, bad := range []string{"", "10.2.1", "fe80::1%eth0", "10.2.1.1/24", "example.org", "1.2.3.4 && x"} {
		if _, err := Addr(bad); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Addr(%q) accepted", bad)
		}
	}
	if got, _ := Prefix("10.2.1.1/24"); got != "10.2.1.1/24" {
		t.Fatalf("Prefix = %q", got)
	}
	if got, _ := Network("10.2.1.1/24"); got != "10.2.1.0/24" {
		t.Fatalf("Network = %q", got)
	}
	for _, bad := range []string{"10.2.1.1", "10.2.1.1/33", "/24", "10.2.1.1/24; x"} {
		if _, err := Prefix(bad); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Prefix(%q) accepted", bad)
		}
	}
}

func TestTemplateStrictRendering(t *testing.T) {
	tmpl, err := NewTemplate("frr").Parse(
		"interface {{ident .Name}}\n description {{quoted .Desc}}\n ip address {{prefix .Addr}}\n")
	if err != nil {
		t.Fatal(err)
	}
	type iface struct{ Name, Desc, Addr string }
	out, err := Execute(tmpl, iface{"loop200", `Uplink "A"`, "10.2.1.1/24"})
	if err != nil {
		t.Fatal(err)
	}
	want := "interface loop200\n description \"Uplink \\\"A\\\"\"\n ip address 10.2.1.1/24\n"
	if string(out) != want {
		t.Fatalf("rendered:\n%s\nwant:\n%s", out, want)
	}
	// Hostile values are rejected by the helper, never written.
	for _, h := range hostile {
		if _, err := Execute(tmpl, iface{h, "d", "10.2.1.1/24"}); !errors.Is(err, ErrUnsafe) {
			t.Errorf("hostile name %q rendered: %v", h, err)
		}
		if _, err := Execute(tmpl, iface{"loop200", h, "10.2.1.1/24"}); err == nil {
			// `"; rm -rf /` is a legal description once quoted; all others must fail.
			if h != hostile[0] {
				t.Errorf("hostile description %q rendered", h)
			}
		}
	}
	// A typo in a field name fails instead of rendering an empty value.
	typo, _ := NewTemplate("typo").Parse("x {{.Nmae}}")
	if _, err := Execute(typo, map[string]string{"Name": "a"}); err == nil {
		t.Fatal("missingkey=error not in effect")
	}
}

func TestCheckRenderedBackstop(t *testing.T) {
	// A raw {{.Field}} (no helper) is caught by the output check for every control character
	// except LF and TAB, which are legal in any config file. LF injection through a raw field
	// is exactly what the backstop cannot see, which is why every user string must pass
	// through ident/line/quoted/json in the template (TestTemplateStrictRendering).
	raw, _ := NewTemplate("raw").Parse("description {{.}}\n")
	if _, err := Execute(raw, "ok description"); err != nil {
		t.Fatal(err)
	}
	for _, h := range hostile[2:] {
		if h == "tab\tinside" {
			continue // tabs are legal in rendered output
		}
		if _, err := Execute(raw, h); !errors.Is(err, ErrUnsafe) {
			t.Errorf("CheckRendered let %q through: %v", h, err)
		}
	}
	if _, err := Execute(raw, hostile[1]); err != nil {
		t.Fatalf("LF-only injection is not detectable after rendering; helpers must reject it: %v", err)
	}
	if err := CheckRendered([]byte("a\tb\nc\n")); err != nil {
		t.Fatalf("tabs and newlines must pass: %v", err)
	}
	if err := CheckRendered([]byte("line1\nbad\rline")); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("CR must be reported with its line: %v", err)
	}
	if _, err := ExecuteTemplate(raw, "raw", "fine"); err != nil {
		t.Fatal(err)
	}
}
