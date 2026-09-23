package df2

// IDRange is the closed range of numeric ids (FIB table ids, ABF policy ids) this agent owns
// on a shared VPP. Untagged objects are attributed by their id: on the shared lab host a
// worker owns its slot's range (VRX_VPP_TABLE_BASE..+999); in production the agent owns
// every id, which the nil range expresses.
type IDRange struct{ Lo, Hi uint32 }

// Owns reports whether id is inside the range; a nil range owns everything.
func (r *IDRange) Owns(id uint32) bool { return r == nil || (id >= r.Lo && id <= r.Hi) }

// Collect drains a generated dump stream: recv is the stream's Recv, which ends with io.EOF.
func Collect[T any](recv func() (T, error)) ([]T, error) {
	var out []T
	for {
		d, err := recv()
		if err != nil {
			if isEOF(err) {
				return out, nil
			}
			return nil, err
		}
		out = append(out, d)
	}
}
