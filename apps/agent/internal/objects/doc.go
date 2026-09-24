// Package objects is the agent side of the firewall object model (F-object-model, WBS D5.1):
// the `objects` configuration domain (addresses, address groups, services, service groups,
// schedules, zones, tags) as an agent-local store, the FQDN resolver, and the expansion library
// the consumers (F-acl, F-host-acl-nftables, later NAT) use to turn object references into
// prefixes and port ranges. There are no VPP objects here.
//
// Stable API for the consumers (docs/agent/objects.md):
//
//	Expand(doc, ref, opts…)     address object/group → Addresses{V4, V6 []netip.Prefix, Unresolved}
//	ExpandService(doc, ref, …)  service object/group → []PortSpec (the shape of acl.Rule)
//	ExpandServiceSpec(spec)     inline ServiceSpec of an ACL rule → []PortSpec
//	Active(schedule, now, loc)  is a schedule active at an instant
//	ZoneInterfaces(doc, zone)   interfaces of a zone
//	RuntimeFor(stateDir, owner) the running agent's Runtime: Snapshot() (applied objects document),
//	                            FQDN (lookup for WithFQDN), FQDNStates, Subscribe (FQDN changes)
//
// # Domain realisation
//
// `objects` is an implemented domain (subsystems.Domains["objects"]) realised by the agent-local
// descriptor family objects.address, objects.address-group, objects.service,
// objects.service-group, objects.schedule, objects.zone and objects.tag. Key = "<descriptor>/<name>";
// value = an ObjectsConfig holding exactly that one entry in that kind's map (the configuration
// message itself, so Retrieve returns the applied objects document unchanged). Create/Update/Delete
// write the Store (persisted in the agent state dir, <dir>/objects-<owner>.json); Retrieve reads it.
// Apply, rollback, confirm-revert and resync therefore work through the scheduler like every other
// domain, and after an agent restart Retrieve comes from the persisted store (D-063: real agent
// state, not an echo of the request).
//
// # FQDN resolver
//
// Every `type: fqdn` address object of the store is resolved by the agent itself: Go's resolver
// (PreferGo, the system's /etc/resolv.conf; no exec, no shell), A and AAAA, refreshed on a fixed
// interval clamped to [30 s, 1 h] (Go's resolver reports no TTL; a Lookup that does is honoured
// with the same clamp). A failed refresh keeps the last good answers; results are persisted in
// <dir>/objects-fqdn-<owner>.json, so a restarted agent reloads them and re-queries only what is
// due, spread out (no query storm). A name that never resolved expands to nothing (Unresolved),
// never to an error.
package objects
