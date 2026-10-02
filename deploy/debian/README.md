# VRX Debian packaging

`vrx/` is a single source package for agent, API, web and runtime meta packages.
This is an initial, fail-closed checkpoint, not an installable appliance release.
The `stage/` payload must be prepared from the repository build and a verified
product VPP manifest; missing inputs stop the build. Maintainer scripts do not
fetch code, overwrite daemon configuration or start VPP. Debhelper-generated
remove/purge handling preserves application data. Services are not enabled or
started on installation, and require the successful firstboot marker.

Remaining before release: firstboot database/migrations/bootstrap, nginx TLS and
WebSocket configuration, base nftables policy, signed APT publication and clean
resolute install/remove/reinstall testing. No live acceptance has run.
