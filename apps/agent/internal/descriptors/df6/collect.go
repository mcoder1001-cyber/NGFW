package df6

import (
	"errors"
	"io"
)

// Collect drains a generated dump stream: recv is the stream's Recv, which ends with io.EOF.
func Collect[T any](recv func() (T, error)) ([]T, error) {
	var out []T
	for {
		d, err := recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, err
		}
		out = append(out, d)
	}
}
