package ikev2

import (
	"math"
	"testing"
)

func TestSACounterJoinRefusesForeignSPICollision(t *testing.T) {
	want := map[uint32]map[uint32]bool{0x12345678: {0xabc001: true}}
	for _, id := range []uint32{0x80abc001, 0xc0abc001, 0xc0abc801, 0x83abc001} {
		if !ownedCounterEntry(want, 0x12345678, id) {
			t.Fatalf("owned native SAD ID %#x rejected", id)
		}
	}
	for _, id := range []uint32{0x80abd001, 0xc0abb001, 0x80abc002, 0x00abc001, 0x80abc801} {
		if ownedCounterEntry(want, 0x12345678, id) {
			t.Fatalf("foreign colliding SPI accepted from SAD ID %#x", id)
		}
	}
	if ownedCounterEntry(want, 0x87654321, 0xc0abc001) {
		t.Fatal("foreign SPI accepted at owned index")
	}
}

func TestSaturatingCounterBounds(t *testing.T) {
	for _, tc := range []struct {
		previous int64
		value    uint64
		want     int64
	}{
		{0, 0, 0}, {12, 34, 46}, {math.MaxInt64 - 1, 1, math.MaxInt64},
		{math.MaxInt64 - 1, 2, math.MaxInt64}, {0, math.MaxUint64, math.MaxInt64}, {-1, 2, 2},
	} {
		if got := saturatingCounter(tc.previous, tc.value); got != tc.want {
			t.Fatalf("%d + %d: got %d want %d", tc.previous, tc.value, got, tc.want)
		}
	}
}
