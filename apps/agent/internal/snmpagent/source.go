package snmpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"go.fd.io/govpp/adapter/statsclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
)

// DefaultStatsSocket is VPP's stats segment.
const DefaultStatsSocket = "/run/vpp/stats.sock"

// IfStatus is the admin/oper state of one interface (from the VPP interface dump).
type IfStatus struct {
	Name            string
	AdminUp, OperUp bool
}

// ProductSource reads VRX-MIB from the VPP stats segment (its own govpp stats connection, read-only),
// the interface dump (admin/oper state) and the agent's persisted state file (running revision).
type ProductSource struct {
	// StatsSocket is the stats segment (DefaultStatsSocket).
	StatsSocket string
	// StateFile is the agent's agent-state.json (read-only here).
	StateFile string
	Version   string
	// Connected reports the VPP API connection.
	Connected func() bool
	// Status dumps admin/oper state by sw_if_index; nil = unknown (down/down).
	Status func(ctx context.Context) (map[uint32]IfStatus, error)

	mu      sync.Mutex
	conn    *core.StatsConnection
	lastTxn string
	commits uint32
	seeded  bool
}

type agentState struct {
	LastTxnID string            `json:"last_txn_id"`
	History   []json.RawMessage `json:"history"`
}

// Snapshot implements Source. A missing stats segment is not an error: the table is empty and
// vrxVppConnected says why.
func (p *ProductSource) Snapshot(ctx context.Context) (Snapshot, error) {
	s := Snapshot{Agent: AgentInfo{Version: p.Version}}
	if p.Connected != nil {
		s.Agent.VppConnected = p.Connected()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readState(&s.Agent)
	counters, err := p.interfaceCounters()
	if err != nil {
		return s, nil //nolint:nilerr // an absent stats segment is a state (empty table), not a failure
	}
	var st map[uint32]IfStatus
	if p.Status != nil && s.Agent.VppConnected {
		st, _ = p.Status(ctx)
	}
	for _, c := range counters {
		i := Interface{
			SwIfIndex: c.InterfaceIndex, Name: c.InterfaceName,
			InOctets: c.Rx.Bytes, OutOctets: c.Tx.Bytes, InPkts: c.Rx.Packets, OutPkts: c.Tx.Packets,
			InErrors: c.RxErrors, OutErrors: c.TxErrors,
		}
		if x, ok := st[c.InterfaceIndex]; ok {
			i.AdminUp, i.OperUp = x.AdminUp, x.OperUp
			if i.Name == "" {
				i.Name = x.Name
			}
		}
		if i.Name == "" {
			continue // a deleted interface's stale counter slot
		}
		s.Interfaces = append(s.Interfaces, i)
	}
	return s, nil
}

func (p *ProductSource) readState(a *AgentInfo) {
	raw, err := os.ReadFile(p.StateFile)
	if err != nil {
		a.Revision, a.Commits = p.lastTxn, p.commits
		return
	}
	var st agentState
	if json.Unmarshal(raw, &st) == nil {
		if !p.seeded {
			p.commits, p.lastTxn, p.seeded = uint32(len(st.History)), st.LastTxnID, true //nolint:gosec // bounded history
		} else if st.LastTxnID != p.lastTxn {
			p.commits++
			p.lastTxn = st.LastTxnID
		}
	}
	a.Revision, a.Commits = p.lastTxn, p.commits
}

func (p *ProductSource) interfaceCounters() ([]api.InterfaceCounters, error) {
	if p.conn == nil {
		sock := p.StatsSocket
		if sock == "" {
			sock = DefaultStatsSocket
		}
		if _, err := os.Stat(sock); err != nil {
			return nil, err
		}
		c, err := core.ConnectStats(statsclient.NewStatsClient(sock))
		if err != nil {
			return nil, fmt.Errorf("stats segment %s: %w", sock, err)
		}
		p.conn = c
	}
	var st api.InterfaceStats
	if err := p.conn.GetInterfaceStats(&st); err != nil {
		p.conn.Disconnect()
		p.conn = nil
		return nil, err
	}
	return st.Interfaces, nil
}

// Close drops the stats connection.
func (p *ProductSource) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		p.conn.Disconnect()
		p.conn = nil
	}
}
