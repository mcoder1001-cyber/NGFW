package strongswan

import (
	"errors"
	"strings"
)

// canonicalRA converts only this package's generated compact template into the
// existing strict settings grammar. It is never an arbitrary config parser.
// RoundTrip then rejects constructs outside the shared authenticated subset.
func canonicalRA(input string) ([]byte, error) {
	var output strings.Builder
	depth := 0
	emit := func(segment string) error {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil
		}
		if segment == "}" {
			depth--
			if depth < 0 {
				return errors.New("invalid generated RA settings")
			}
		}
		if strings.HasSuffix(segment, "{") {
			output.WriteString(strings.Repeat("\t", depth))
			output.WriteString(segment)
			output.WriteByte('\n')
			depth++
			return nil
		}
		if segment != "}" {
			parts := strings.SplitN(segment, "=", 2)
			if len(parts) != 2 {
				return errors.New("invalid generated RA settings")
			}
			key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if !strings.HasPrefix(value, "\"") && strings.ContainsAny(value, " \t") {
				var err error
				value, err = Quote(value)
				if err != nil {
					return errors.New("invalid generated RA settings")
				}
			}
			segment = key + " = " + value
		}
		output.WriteString(strings.Repeat("\t", depth))
		output.WriteString(segment)
		output.WriteByte('\n')
		return nil
	}
	for _, line := range strings.Split(strings.TrimSuffix(input, "\n"), "\n") {
		start := 0
		quoted, escaped := false, false
		for index, char := range line {
			if escaped {
				escaped = false
				continue
			}
			if quoted && char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				quoted = !quoted
				continue
			}
			if quoted {
				continue
			}
			if char == '{' {
				if emit(line[start:index+1]) != nil {
					return nil, errors.New("invalid generated RA settings")
				}
				start = index + 1
			}
			if char == '}' {
				if emit(line[start:index]) != nil || emit("}") != nil {
					return nil, errors.New("invalid generated RA settings")
				}
				start = index + 1
			}
		}
		if quoted || escaped || emit(line[start:]) != nil {
			return nil, errors.New("invalid generated RA settings")
		}
	}
	if depth != 0 {
		return nil, errors.New("invalid generated RA settings")
	}
	data := []byte(output.String())
	if _, err := RoundTrip("remote-access", data); err != nil {
		return nil, errors.New("invalid generated RA settings")
	}
	return data, nil
}
