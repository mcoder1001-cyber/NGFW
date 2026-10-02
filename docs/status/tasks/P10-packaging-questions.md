# P10 release questions and bounded dependencies

1. Licensing metadata: no repository LICENSE exists. Debian copyright cannot
   truthfully assign a license. Obtain the owner-approved source/copyright terms
   before release; never invent a permissive or proprietary grant.
2. Approved agent capabilities are exactly NET_ADMIN/SYS_ADMIN/IPC_LOCK (P10).
   rsyslog ProductPaths sets KeyOwner root:syslog and shared file writing applies
   owner changes. Changing group ownership may require CAP_CHOWN, absent from the
   approved unit. Distro daemon directory ownership may independently require
   privilege/permission compatibility. Do not widen the capability boundary; verify
   on the target and resolve the dependent security decision before declaring those
   paths functional. Existing precise capture/rsyslog TLS paths are provisioned.
3. Static base policy must identify management and VPP punt interfaces explicitly.
   LCP names are configurable logical names, not a trustworthy vpp* prefix. Fresh
   default has no dataplane device. Proposed packaging set uses explicit validated
   bootstrap interface membership and no broad wildcard; future runtime set sync
   must remain explicit until implemented/reviewed.
4. No VPP build output, debootstrap/resolute root, real reprepro release output or
   reachable appliance is present. gpg-agent cannot start/connect in this execution
   environment. These execution checks are NOT RUN/deferred; host-independent code
   development continues. Full hosted quick still required before any merge.
