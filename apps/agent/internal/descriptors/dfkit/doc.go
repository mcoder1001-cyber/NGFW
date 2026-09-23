// Package dfkit holds the helpers shared by the DF-8 descriptor packages (dhcp, dns, ipfix,
// flowprobe, sflow, pcap, trace, lcp): the structpb stand-in codec for typed specs (D-055),
// interface resolution and claims through DF-1's iface package (D-069/D-071), the globals-owner
// role (D-071), the boot-identity store for non-idempotent write-only adds (D-076) and errors.
//
// It contains no descriptor itself. Everything here talks to VPP only through the generated
// bindings in apps/agent/binapi and the vpp.Client contract.
package dfkit
