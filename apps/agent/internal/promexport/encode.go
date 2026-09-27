// Package promexport renders VRX data-plane metrics in the Prometheus text exposition format (version
// 0.0.4), stdlib only (like internal/agent/metrics.go — no prometheus/client_golang, D-063). The families
// here are served through the manager's metrics-collector hook on the agent's existing /metrics endpoint,
// and, when management.prometheus is enabled, on an external allow-listed listener (listener.go). The
// values come from a StatsSource (the VPP stats segment on the box; a fake in tests).
package promexport

import (
	"bufio"
	"io"
	"sort"
	"strconv"
	"strings"
)

// writer accumulates one family at a time and escapes label values per the text format.
type writer struct {
	w   *bufio.Writer
	err error
}

func newWriter(w io.Writer) *writer { return &writer{w: bufio.NewWriter(w)} }

func (e *writer) flush() error {
	if e.err != nil {
		return e.err
	}
	return e.w.Flush()
}

// family writes the HELP/TYPE header once per metric name.
func (e *writer) family(name, typ, help string) {
	if e.err != nil {
		return
	}
	_, e.err = e.w.WriteString("# HELP " + name + " " + help + "\n# TYPE " + name + " " + typ + "\n")
}

// sample writes one line: name{labels} value. Labels are sorted; values escaped.
func (e *writer) sample(name string, labels map[string]string, value string) {
	if e.err != nil {
		return
	}
	var b strings.Builder
	b.WriteString(name)
	if len(labels) > 0 {
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(k)
			b.WriteString(`="`)
			b.WriteString(escapeLabel(labels[k]))
			b.WriteByte('"')
		}
		b.WriteByte('}')
	}
	b.WriteByte(' ')
	b.WriteString(value)
	b.WriteByte('\n')
	_, e.err = e.w.WriteString(b.String())
}

func (e *writer) gauge(name string, labels map[string]string, v float64) {
	e.sample(name, labels, strconv.FormatFloat(v, 'g', -1, 64))
}

func (e *writer) counter(name string, labels map[string]string, v uint64) {
	e.sample(name, labels, strconv.FormatUint(v, 10))
}

// escapeLabel escapes backslash, double-quote and newline (the only characters the format reserves in a
// label value). Everything else — including a hostile interface name — is written verbatim but harmless.
func escapeLabel(s string) string {
	if !strings.ContainsAny(s, "\\\"\n") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(s)
}
