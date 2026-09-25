package snmpagent

import (
	"context"
	"sort"
)

// PlaypenOID is net-snmp's netSnmpPlaypen (NET-SNMP-MIB: "for local experiments"), the documented
// PLACEHOLDER under which VRX-MIB lives until the product owner registers a Private Enterprise Number
// (docs/status/tasks/F-snmp-questions.md). Change VRXMIBOID and deploy/snmp/VRX-MIB.txt together.
const PlaypenOID = ".1.3.6.1.4.1.8072.9999.9999"

// VRXMIBOID is the registered subtree: vrxMIB = netSnmpPlaypen.7853.
var VRXMIBOID = MustOID(PlaypenOID + ".7853")

// Subtrees of VRX-MIB (deploy/snmp/VRX-MIB.txt).
var (
	oidAgent   = VRXMIBOID.Append(1) // vrxAgent scalars
	oidIfEntry = VRXMIBOID.Append(2, 1)
)

// Agent scalars (vrxAgent.N.0).
const (
	agentVersion      = 1
	agentVppConnected = 2
	agentRevision     = 3
	agentCommits      = 4
	agentDegraded     = 5
)

// vrxIfEntry columns.
const (
	colIndex      = 1
	colName       = 2
	colAdmin      = 3
	colOper       = 4
	colInOctets   = 5
	colOutOctets  = 6
	colInPkts     = 7
	colOutPkts    = 8
	colInErrors   = 9
	colOutErrors  = 10
	lastIfColumn  = colOutErrors
	statusUp      = 1
	statusDown    = 2
	truthValTrue  = 1
	truthValFalse = 2
)

// Interface is one VPP interface row.
type Interface struct {
	SwIfIndex           uint32
	Name                string
	AdminUp, OperUp     bool
	InOctets, OutOctets uint64
	InPkts, OutPkts     uint64
	InErrors, OutErrors uint64
}

// AgentInfo is the vrxAgent group.
type AgentInfo struct {
	Version      string
	VppConnected bool
	Revision     string // last applied transaction id
	Commits      uint32 // applied transactions seen (Counter32)
	Degraded     bool
}

// Snapshot is one consistent view of the MIB.
type Snapshot struct {
	Agent      AgentInfo
	Interfaces []Interface
}

// Source provides snapshots (the product: stats segment + interface dump + agent state file).
type Source interface {
	Snapshot(ctx context.Context) (Snapshot, error)
}

// SourceFunc adapts a function.
type SourceFunc func(ctx context.Context) (Snapshot, error)

// Snapshot implements Source.
func (f SourceFunc) Snapshot(ctx context.Context) (Snapshot, error) { return f(ctx) }

func integer(v int64) Value      { return Value{Type: TypeInteger, Int: v} }
func counter64(v uint64) Value   { return Value{Type: TypeCounter64, U64: v} }
func counter32(v uint32) Value   { return Value{Type: TypeCounter32, Int: int64(v)} }
func octetString(s string) Value { return Value{Type: TypeOctetString, Str: []byte(s)} }

func status(up bool) Value {
	if up {
		return integer(statusUp)
	}
	return integer(statusDown)
}

func truth(b bool) Value {
	if b {
		return integer(truthValTrue)
	}
	return integer(truthValFalse)
}

// view is a snapshot flattened into lexicographically sorted varbinds.
type view []VarBind

// buildView flattens s. The table index is sw_if_index+1 (an SNMP index is never 0).
func buildView(s Snapshot) view {
	v := view{
		{oidAgent.Append(agentVersion, 0), octetString(s.Agent.Version)},
		{oidAgent.Append(agentVppConnected, 0), truth(s.Agent.VppConnected)},
		{oidAgent.Append(agentRevision, 0), octetString(s.Agent.Revision)},
		{oidAgent.Append(agentCommits, 0), counter32(s.Agent.Commits)},
		{oidAgent.Append(agentDegraded, 0), truth(s.Agent.Degraded)},
	}
	ifs := append([]Interface(nil), s.Interfaces...)
	sort.Slice(ifs, func(a, b int) bool { return ifs[a].SwIfIndex < ifs[b].SwIfIndex })
	for col := uint32(colIndex); col <= lastIfColumn; col++ {
		for _, i := range ifs {
			idx := i.SwIfIndex + 1
			var val Value
			switch col {
			case colIndex:
				val = integer(int64(idx))
			case colName:
				val = octetString(i.Name)
			case colAdmin:
				val = status(i.AdminUp)
			case colOper:
				val = status(i.OperUp)
			case colInOctets:
				val = counter64(i.InOctets)
			case colOutOctets:
				val = counter64(i.OutOctets)
			case colInPkts:
				val = counter64(i.InPkts)
			case colOutPkts:
				val = counter64(i.OutPkts)
			case colInErrors:
				val = counter64(i.InErrors)
			case colOutErrors:
				val = counter64(i.OutErrors)
			}
			v = append(v, VarBind{oidIfEntry.Append(col, idx), val})
		}
	}
	sort.Slice(v, func(a, b int) bool { return v[a].Name.Compare(v[b].Name) < 0 })
	return v
}

// get answers one Get (exact match).
func (v view) get(name OID) VarBind {
	i := sort.Search(len(v), func(i int) bool { return v[i].Name.Compare(name) >= 0 })
	if i < len(v) && v[i].Name.Compare(name) == 0 {
		return v[i]
	}
	// An OID inside a known object (scalar or column) without the instance is noSuchInstance.
	if name.HasPrefix(VRXMIBOID) {
		return VarBind{Name: name, Value: Value{Type: TypeNoSuchInstance}}
	}
	return VarBind{Name: name, Value: Value{Type: TypeNoSuchObject}}
}

// next answers one GetNext search range.
func (v view) next(r searchRange) VarBind {
	i := sort.Search(len(v), func(i int) bool {
		c := v[i].Name.Compare(r.Start)
		return c > 0 || (c == 0 && r.Include)
	})
	if i < len(v) && (len(r.End) == 0 || v[i].Name.Compare(r.End) < 0) {
		return v[i]
	}
	return VarBind{Name: r.Start, Value: Value{Type: TypeEndOfMibView}}
}
