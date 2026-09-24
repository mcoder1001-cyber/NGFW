package safe

import (
	"bytes"
	"testing"
)

func TestStringEscapesControlsAndBidi(t *testing.T) {
	cases := map[string]string{
		"plain text ✓ فارسی":                      "plain text ✓ فارسی",
		"cli review\x1b]0;PWNED\x07\x1b[2K\rinno": `cli review\x1b]0;PWNED\x07\x1b[2K\x0dinno`,
		"csi\u009b31m c1":                         `csi\x9b31m c1`,
		"osc52 \x1b]52;c;cm0gLXJmIC8=\x07":        `osc52 \x1b]52;c;cm0gLXJmIC8=\x07`,
		"rtl \u202egnp.exe\u2069 \u2066x":         `rtl \u202egnp.exe\u2069 \u2066x`,
		"del\x7f nul\x00 bad\xff\x9b":             `del\x7f nul\x00 bad\xff\x9b`,
		"two\nlines\ttab":                         `two\x0alines\x09tab`,
	}
	for in, want := range cases {
		if got := String(in); got != want {
			t.Errorf("String(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Text("a\n\tb\x1b[1Ac"); got != "a\n\tb\\x1b[1Ac" {
		t.Errorf("Text keeps layout only: %q", got)
	}
}

func TestWriterSplitsUTF8AcrossWrites(t *testing.T) {
	var out bytes.Buffer
	w := &Writer{W: &out}
	s := []byte("ok ف\x1b[2J \u202e!")
	for i := range s { // one byte per write
		if _, err := w.Write(s[i : i+1]); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Flush()
	if got := out.String(); got != `ok ف\x1b[2J \u202e!` {
		t.Errorf("got %q", got)
	}
	for _, b := range out.Bytes() {
		if b == 0x1b {
			t.Fatal("raw ESC reached the terminal")
		}
	}
}
