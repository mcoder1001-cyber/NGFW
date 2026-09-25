package df7

import "testing"

// TD-8b (TD-8 verify V2): WithIDs takes the wiring's range as it is — nil owns every id, the empty
// range owns none, any other range is copied.
func TestWithIDs(t *testing.T) {
	if o := BuildOptions([]Option{WithIDs(nil)}); o.IDs != nil || !o.IDs.Owns(0) || !o.IDs.Owns(^uint32(0)) {
		t.Fatalf("nil (VRX_VPP_ID_RANGE=all): %+v, want every id", o.IDs)
	}
	none := BuildOptions([]Option{WithIDs(&IDRange{Lo: 1, Hi: 0})})
	for _, id := range []uint32{0, 1, 5000, ^uint32(0)} {
		if none.IDs.Owns(id) || none.CheckID("map", id) == nil {
			t.Fatalf("the empty range owns %d", id)
		}
	}
	r := &IDRange{Lo: 5000, Hi: 5999}
	o := BuildOptions([]Option{WithIDs(r)})
	r.Lo, r.Hi = 0, ^uint32(0) // the option copied the range
	if !o.IDs.Owns(5000) || !o.IDs.Owns(5999) || o.IDs.Owns(4999) || o.IDs.Owns(6000) || o.CheckID("map", 6000) == nil {
		t.Fatalf("5000-5999: %+v", o.IDs)
	}
}
