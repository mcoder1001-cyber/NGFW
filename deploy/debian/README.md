# NGFW Debian packaging

`ngfw/` is a single source package for agent, API, web and runtime meta packages.
This is an initial, fail-closed checkpoint, not an installable appliance release.
The `stage/` payload must be prepared from the repository build and a verified
product VPP manifest; missing inputs stop the build. Maintainer scripts do not
fetch code, overwrite daemon configuration or start VPP. Debhelper-generated
remove/purge handling preserves application data. Services are not enabled or
started on installation, are neither stopped nor restarted on upgrade (D-253:
`dh_installsystemd --no-start --no-stop-on-upgrade`; the operator restarts agent
and API), are stopped only on removal, and require the successful firstboot marker.
`ngfw-firstboot.service` wants (never requires) PostgreSQL, Valkey and nftables and
checks them itself on a fresh appliance (D-254), so their explicit restart does not
propagate to VPP, the agent or the API.

Remaining before release: firstboot database/migrations/bootstrap, nginx TLS and
WebSocket configuration, base nftables policy, signed APT publication and clean
resolute install/remove/reinstall testing. No live acceptance has run.

## NGFW runtime naming boundary

New source builds produce `ngfw-agent`, `ngfw-api`, `ngfw-web` and `ngfw-meta`
packages, use `ngfw-*.service` units, `NGFW_*` environment variables and
`/etc/ngfw`, `/var/lib/ngfw` and `/run/ngfw` paths. They are a new naming
boundary; existing pre-rename packages, units, environment files and persisted data
are not migrated or removed automatically by this change. Operators must plan
and validate any migration separately before installing on an existing device.
The source rename performs no host installation or service action.

Patched VPP source builds now carry the `+ngfw` local revision suffix. Upstream
version/tag/commit and locked dependency input hashes remain pinned. Existing
pre-rename binaries and their manifests keep their original identity and provenance;
new NGFW releases require a fresh build and newly generated manifests.
