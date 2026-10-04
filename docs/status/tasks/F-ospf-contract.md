# F-ospf additive contract completion

RoutingConfig.ospf6 uses reserved wave-BC allocation 13; OspfInterface.auth uses 9;
EventKind.OSPF_NEIGHBOR_CHANGED uses 20. Separate Ospf6Config and Ospf6Interface share area/timer semantics,
without carrying v2 authentication, BFD or unsupported IPv6 NBMA in its schema.
MD5 key references remain password/<name>; no inline secret. Fields are additive and existing shapes unchanged.

Auth.type is proto optional per D039. The existing drift-guard policy for shared Redistribute excludes IPv4 ospf under ospf6.
