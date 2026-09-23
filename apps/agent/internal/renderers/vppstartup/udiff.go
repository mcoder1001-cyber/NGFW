package vppstartup

import (
	"errors"
	"fmt"
	"strings"
)

// maxDiffCells bounds the LCS table (lines(a) × lines(b)); startup.conf files have hundreds of
// lines, so this only stops pathological input.
const maxDiffCells = 16 << 20

// ErrDiffTooLarge is returned by UnifiedDiff for inputs beyond maxDiffCells.
var ErrDiffTooLarge = errors.New("vppstartup: files too large to diff")

type editOp struct {
	kind byte // ' ', '-', '+'
	text string
	ai   int // 0-based line in a (for ' ' and '-')
	bi   int // 0-based line in b (for ' ' and '+')
}

// UnifiedDiff returns a unified diff (3 lines of context) from a to b with the given labels,
// or "" when the contents are equal. A missing final newline is marked like diff(1) does.
func UnifiedDiff(labelA, labelB string, a, b []byte) (string, error) {
	if string(a) == string(b) {
		return "", nil
	}
	la, lb := splitLines(string(a)), splitLines(string(b))
	if len(la)*len(lb) > maxDiffCells {
		return "", ErrDiffTooLarge
	}
	ops := lcsOps(la, lb)

	const ctx = 3
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", labelA, labelB)
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		// hunk: from ctx lines before the first change to ctx lines after the last change that
		// is within 2*ctx of the previous one
		start := max(i-ctx, 0)
		end := i
		for j := i; j < len(ops); j++ {
			if ops[j].kind != ' ' {
				end = j
			} else if j-end > 2*ctx {
				break
			}
		}
		stop := min(end+ctx+1, len(ops))
		var aStart, bStart, aLen, bLen int
		aStart, bStart = -1, -1
		for _, op := range ops[start:stop] {
			if op.kind != '+' {
				if aStart < 0 {
					aStart = op.ai
				}
				aLen++
			}
			if op.kind != '-' {
				if bStart < 0 {
					bStart = op.bi
				}
				bLen++
			}
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", hunkRange(aStart, aLen, ops[start:stop], true), hunkRange(bStart, bLen, ops[start:stop], false))
		for _, op := range ops[start:stop] {
			out.WriteByte(op.kind)
			out.WriteString(strings.TrimSuffix(op.text, "\n"))
			out.WriteByte('\n')
			if !strings.HasSuffix(op.text, "\n") {
				out.WriteString("\\ No newline at end of file\n")
			}
		}
		i = stop
	}
	return out.String(), nil
}

// hunkRange formats "start,len" (1-based); an empty side names the line before the hunk.
func hunkRange(start, n int, ops []editOp, sideA bool) string {
	if n == 0 {
		// position just before: the line index of the first op on the other side
		pos := 0
		for _, op := range ops {
			if sideA {
				pos = op.ai
			} else {
				pos = op.bi
			}
			break
		}
		return fmt.Sprintf("%d,0", pos)
	}
	if n == 1 {
		return fmt.Sprintf("%d", start+1)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}

// splitLines splits keeping the "\n" on each line (the last may lack it).
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lcsOps computes a minimal edit script via the LCS table; deletions come before insertions
// inside a change block, like diff(1).
func lcsOps(a, b []string) []editOp {
	n, m := len(a), len(b)
	w := m + 1
	t := make([]int32, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				t[i*w+j] = t[(i+1)*w+j+1] + 1
			} else {
				t[i*w+j] = max(t[(i+1)*w+j], t[i*w+j+1])
			}
		}
	}
	var ops []editOp
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			ops = append(ops, editOp{' ', a[i], i, j})
			i++
			j++
		case i < n && (j == m || t[(i+1)*w+j] >= t[i*w+j+1]):
			ops = append(ops, editOp{'-', a[i], i, j})
			i++
		default:
			ops = append(ops, editOp{'+', b[j], i, j})
			j++
		}
	}
	return ops
}
