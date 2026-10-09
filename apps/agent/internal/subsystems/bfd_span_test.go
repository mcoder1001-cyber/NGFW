package subsystems

import "testing"

func TestBfdIDSpanUnscopedAndInvalid(t *testing.T) {
	for _, test := range []struct {
		name, mode, base string
		lo, hi           uint32
	}{
		{"all", "all", "", 0, ^uint32(0)}, {"invalid", "invalid", "", 1, 0}, {"slot", "", "7000", 7000, 7999},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(EnvIDRange, test.mode)
			t.Setenv(EnvTableBase, test.base)
			lo, hi := BfdIDSpan()
			if lo != test.lo || hi != test.hi {
				t.Fatalf("span %d-%d, want %d-%d", lo, hi, test.lo, test.hi)
			}
		})
	}
}
