package subsystems

import (
	"strings"
	"testing"
)

// The close seam runs the closers of one Wiring once, last registered first, and leaves other Wirings alone.
func TestWiringCloseSeam(t *testing.T) {
	var got []string
	a, b := &Wiring{}, &Wiring{}
	a.OnClose(func() { got = append(got, "a1") })
	a.OnClose(func() { got = append(got, "a2") })
	b.OnClose(func() { got = append(got, "b1") })
	a.Close()
	a.Close()
	if strings.Join(got, ",") != "a2,a1" {
		t.Fatalf("closers %v", got)
	}
	b.Close()
	if strings.Join(got, ",") != "a2,a1,b1" {
		t.Fatalf("closers %v", got)
	}
	(&Wiring{}).Close() // nothing registered: no-op
}
