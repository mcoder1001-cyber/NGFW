package cpath

import (
	"reflect"
	"strings"
	"testing"
)

func TestPointerRoundTrip(t *testing.T) {
	cases := []struct {
		segs []string
		ptr  string
	}{
		{[]string{}, ""},
		{[]string{"interfaces", "loop301", "mtu"}, "/interfaces/loop301/mtu"},
		{[]string{"interfaces", "TenGigabitEthernet0/0/0", "ipv4", "0"}, "/interfaces/TenGigabitEthernet0~10~10/ipv4/0"},
		{[]string{"objects", "a~b", "c/d~1"}, "/objects/a~0b/c~1d~01"},
		{[]string{"x", ""}, "/x/"},
	}
	for _, c := range cases {
		if got := Pointer(c.segs); got != c.ptr {
			t.Errorf("Pointer(%q) = %q, want %q", c.segs, got, c.ptr)
		}
		back, err := ParsePointer(c.ptr)
		if err != nil || !reflect.DeepEqual(back, c.segs) {
			t.Errorf("ParsePointer(%q) = %q, %v; want %q", c.ptr, back, err, c.segs)
		}
	}
}

func TestParsePointerErrors(t *testing.T) {
	for _, p := range []string{"interfaces", "/a/~2", "/a/b~"} {
		if _, err := ParsePointer(p); err == nil {
			t.Errorf("ParsePointer(%q): want an error", p)
		}
	}
}

func TestResolveWordsToPointer(t *testing.T) {
	cases := []struct {
		base, words []string
		ptr         string
	}{
		{nil, []string{"interfaces", "TenGigabitEthernet0/0/0", "mtu"}, "/interfaces/TenGigabitEthernet0~10~10/mtu"},
		{[]string{"interfaces", "loop301"}, []string{"mtu"}, "/interfaces/loop301/mtu"},
		{[]string{"interfaces", "loop301"}, []string{"..", "loop302"}, "/interfaces/loop302"},
		{[]string{"interfaces", "loop301"}, []string{"/system/hostname"}, "/system/hostname"}, // absolute pointer
		{[]string{"interfaces"}, nil, "/interfaces"},
	}
	for _, c := range cases {
		segs, err := Resolve(c.base, c.words)
		if err != nil {
			t.Fatalf("Resolve(%q, %q): %v", c.base, c.words, err)
		}
		if got := Pointer(segs); got != c.ptr {
			t.Errorf("Resolve(%q, %q) = %q, want %q", c.base, c.words, got, c.ptr)
		}
	}
	if _, err := Resolve(nil, []string{".."}); err == nil {
		t.Error("'..' at the top must fail")
	}
}

func TestTokenize(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{`set interfaces loop301 mtu 9000`, []string{"set", "interfaces", "loop301", "mtu", "9000"}},
		{`  commit   comment "two words" `, []string{"commit", "comment", "two words"}},
		{`set system banner motd "say \"hi\"\n"`, []string{"set", "system", "banner", "motd", "say \"hi\"\n"}},
		{`set x 'lit\eral'`, []string{"set", "x", `lit\eral`}},
		{`merge interfaces loop301 {"description": "a b", "ipv4": ["10.0.0.1/24"]}`, []string{"merge", "interfaces", "loop301", `{"description": "a b", "ipv4": ["10.0.0.1/24"]}`}},
		{`set a ["x]", "y"] z`, []string{"set", "a", `["x]", "y"]`, "z"}},
	}
	for _, c := range cases {
		toks, err := Tokenize(c.line)
		if err != nil {
			t.Fatalf("Tokenize(%q): %v", c.line, err)
		}
		if got := Texts(toks); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Tokenize(%q) = %q, want %q", c.line, got, c.want)
		}
	}
	for _, bad := range []string{`set "open`, `set 'open`, `merge x {"a": 1`} {
		if _, err := Tokenize(bad); err != ErrUnterminated {
			t.Errorf("Tokenize(%q): want ErrUnterminated, got %v", bad, err)
		}
	}
	toks, _ := Tokenize(`set "9000" 9000`)
	if !toks[1].Quoted || toks[2].Quoted {
		t.Errorf("quoted flags: %+v", toks)
	}
}

func TestQuoteSurvivesTokenize(t *testing.T) {
	for _, s := range []string{"plain", "two words", "", `a"b`, `back\slash`, "{json-looking", "/slash-first", "tab\there", "#hash", "line\nbreak", "TenGigabitEthernet0/0/0", "esc\x1b]0;T\x07\x1b[2K\rx", "c1\u009b", "rtl\u202e", "bad\xff", "\x7f"} {
		toks, err := Tokenize("x " + Quote(s))
		if err != nil || len(toks) != 2 || toks[1].Text != s {
			t.Errorf("Quote(%q) = %s does not round-trip: %+v %v", s, Quote(s), toks, err)
		}
	}
}

func TestQuoteMakesControlsVisible(t *testing.T) {
	q := Quote("x\x1b]52;c;AAAA\x07\u202e")
	if strings.ContainsAny(q, "\x1b\x07\u202e") || q != `"x\x1b]52;c;AAAA\x07\u202e"` {
		t.Errorf("Quote = %q", q)
	}
}
