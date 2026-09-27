# Renderer: sysident (system identity)

Package `apps/agent/internal/renderers/sysident` (F-system-identity, WBS D0.14, DEC-system-identity / D-152).
It applies the `system` schema domain on the box: host name, time zone, login/MOTD banners and the router's own DNS client.

## Files

| input | rendered into | notes |
|---|---|---|
| `system.hostname` | `/etc/hostname` + `sethostname(2)` | the kernel name is set only by the globals owner (`Paths.SetKernelHostname`) |
| `system.timezone` | `/etc/localtime` → `/usr/share/zoneinfo/<zone>` | atomic symlink replace; the zone file must exist (tzdata) |
| `system.banner.login` | `/etc/issue`, `/etc/issue.net` | text + one trailing newline; empty file when unset |
| `system.banner.motd` | `/etc/motd` | same |
| `system.dns.servers`, `.searchDomains` | `/etc/systemd/resolved.conf.d/vrx.conf` (`DNS=`, `Domains=`) | also carries the embedded render input (`# vrx-input: <base64>`) |

Every path comes from `Paths` (`paths.go`). `ProductPaths()` is the Ubuntu 26.04 layout. `PathsUnder(base)` mirrors it
under `base/etc/…`, keeps the host's zoneinfo (read only) and never sets the kernel name. Tests and test slots use it.

## Descriptor `system.identity/vrx`

One singleton scheduler descriptor (D-109 d). Its Value is `sysident.Input(system)`: the `SystemConfig` with the schema
defaults filled in (`vrx`, `UTC`, VRF `default`) and empty banners dropped. The projection (`desired/system_identity.go`)
always adds it while `system` is authoritative, because every field has a default.

- **Validator** (TD-13, `scheduler.Validator`): `Check`. It verifies an RFC 1123 host name and an IANA zone name (no `..`)
  whose zone file exists. Banners may hold printable text, LF and TAB only: no C0/C1 controls, no DEL and no bidi
  override/isolate marks (D-049). Servers must be IP addresses (max 8), search domains names (max 6), and the VRF must be `default`.
  A finding names the leaf (`/system/timezone`, `/system/banner/login`, …). The projection runs the same checks except
  the zone-file lookup, so DryRun reports them too.
- **Stage**: `StageDaemon` (host configuration, after VPP objects).
- **Create / Update**: renders the files and writes only those whose content differs, atomically, with a snapshot and
  restore on error. It re-points `/etc/localtime` only when the target differs and calls `sethostname` only when the
  kernel name differs. **An unchanged document writes nothing**: an agent restart's resync touches no file.
- **Delete**: removes the resolver drop-in only. The host keeps its last name, zone and banners.
- **Retrieve**: decodes the embedded input from the drop-in and re-renders it. When every file, the symlink and (owner
  only) the kernel name match, the input is the Value. Otherwise a drift `Struct` lists the differences, and the
  scheduler runs Update.
- **Ownership** (TD-11b): `RecordsNoOwnership`. The drop-in's embedded input is the ownership fact.

## Who renders where

`subsystems/system_identity.go`: the globals owner (D-071, the product agent on a real box) uses `ProductPaths()`.
Every other agent renders into `<slot dir>/sysident/etc/…` (`/run/vrx-test/<owner>`, or `VRX_HOST_SERVICES_DIR`). It
never touches the host's identity (docs/lab/shared-host-rules.md).

## Daemons

No daemon checker exists, so the input is validated structurally. systemd-resolved reads drop-ins at start. A DNS
change is logged as `systemd-resolved must be restarted` (unit, action). The agent does not restart host daemons itself
(D-079, PENDING-agent-privileges: the same hand-off as unbound/chrony). getty re-reads `/etc/issue` for every prompt,
and sshd serves `/etc/issue.net` only when `Banner /etc/issue.net` is configured (F-hardening-lite).
Note: agetty expands backslash escapes (`\n`, `\l`, …) in `/etc/issue`, and the renderer writes the banner verbatim.
