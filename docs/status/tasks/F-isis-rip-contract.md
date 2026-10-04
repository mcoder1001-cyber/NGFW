# F-isis-rip additive contract

Existing object shapes retained. IS-IS interfaces add optional protobuf bool ipv4/ipv6 tags 6/7 with schema defaults true; omitted old wire values preserve both families in consumers. IS-IS adds areaPasswordRef/domainPasswordRef tags 6/7 constrained password references. RoutingConfig tag14 adds separate IPv6-only ripng; RipConfig tag6 adds version2 only. EventKind21 names IS-IS adjacency changes. BFD owns IsisInterface tag8 independently.

RIPng uses a dedicated config message and shared interface/redistribution messages. The shared redistribution wire superset excludes the RIP source in the schema, with precise contract drift annotation. RIPng redistribution into other protocols remains outside scope. Runtime secret delivery remains governed by PENDING-secret-channel; no storage, socket permissions or privilege change.
