// Package dfkit holds the helpers shared by the DF-8 descriptor packages (dhcp, dns, ipfix,
// flowprobe, sflow, prom, pcap, trace, lcp): the structpb stand-in codec for typed specs
// (D-055), the interface name/tag table, the VPP process identity and the "applied" cache
// used by object types VPP has no dump for.
//
// It contains no descriptor itself. Everything here talks to VPP only through the generated
// bindings in apps/agent/binapi and the vpp.Client contract.
package dfkit
