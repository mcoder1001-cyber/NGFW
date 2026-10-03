# TD-19 official upstream signer authority follow-up

Docs-only envelope/checkpoint. Branch task/TD19-frr-upstream-authority-20261002,
isolated NGFW-TD19-frr-upstream-authority, exact base
78218e6da8b623debf2542bebbe15af2fcc5a809. Own only this report.
Earlier b0f12494/8c639171 research covered homepage, downloaded public bundle,
metadata signer and Packages digest. That established raw signature consistency,
not independent authority for A90FC36D9429409798E9C2D874DEED43AB194DBF.
The canonical certificate transformation fixes mechanics, not signer authority.

New bounded read-only scope: inspect official FRRouting repositories, packaging/
build/publish source, release documents and attributable commits for separately
published full fingerprint or trust rotation. No source/pin/gate/default trust,
PR90/main/board or host installation changes; no keyserver trust substitution.
Negative evidence must name actual inspected paths, not claim all upstream
history searched. NodeSource Ubuntu26.04 support remains unresolved.

Initial search: official frr README and releases link deb.frrouting.org; no full
A90 fingerprint found by exact indexed official-domain search. This is not an
exhaustive repository search. Research continuing, not authority resolution.
Remote publication not claimed; local checkpoint sent to parent manager.


## Completed bounded inspection — 2026-10-03 00:02 UTC

No independent publication of full A90 fingerprint or explicit repository-key
rotation was found in the following inspected official sources. This is a
bounded negative result, not proof all upstream history lacks authorization.
Public read-only GitHub APIs/connectors and primary web search were used; no
upstream code, build, installer or host mutation was executed.

[FRRouting organization repository inventory](https://api.github.com/orgs/FRRouting/repos?per_page=100)
returned ten visible repositories: frr, topotests, frr-www, gentoo-overlay,
frrbot, frr-gsoc, frr-testing, frr-mibs, netdef-ci-github-app, zebra-historic.
No separately named Debian repository publisher appeared in that inventory.
Recursive current master tree inspections were complete (truncated=false):
frr tree `ebfd886fd46b19f9886b0872b39ad3edf5e6c053`, frr-www tree
`ccebf3626db33f9e8dbae2c7f0a6cfd21afd7201`. These are tree object IDs, not
commit IDs. Paths were selected for packaging/release/publish/key handling;
source below was fetched at those tree snapshots, not merely inferred from
filenames. The full contents of all repository files/history were not searched.

| Inspected official path | Concrete finding |
| --- | --- |
| [frr README](https://github.com/FRRouting/frr/blob/master/README.md) | Links official APT repository, releases and community channels; no signer fingerprint. |
| [frr release procedure](https://github.com/FRRouting/frr/blob/master/doc/developer/frr-release-procedure.rst) | Staging uses NetDEF CI; publication coordinates with Debian maintainer Jafar Al-Gharaibeh and updates repository webpage. No full key fingerprint or rotation record. |
| [Debian build helper](https://github.com/FRRouting/frr/blob/master/tools/build-debian-package.sh) | Builds with dpkg-buildpackage -uc/-us; no repository signing identity. Read only, never executed. |
| [README.Maintainer](https://github.com/FRRouting/frr/blob/master/debian/README.Maintainer) | Signing-key/watch handling remains a TODO; no published key identity. |
| [debian/watch](https://github.com/FRRouting/frr/blob/master/debian/watch) | Tracks release source archives; no signer authorization. |
| [debian/gbp.conf](https://github.com/FRRouting/frr/blob/master/debian/gbp.conf) | No A90/key-rotation declaration. |
| [Ubuntu source build guide](https://github.com/FRRouting/frr/blob/master/doc/developer/building-frr-for-ubuntu2x04.rst) | No repository signing-key identity. Source-build instructions do not establish package-repository trust. |
| [Debian Dockerfile](https://github.com/FRRouting/frr/blob/master/docker/debian/Dockerfile) | Downloads keys.asc directly; no independently pinned full fingerprint. Another bundle endpoint is not independent identity publication. |
| [Release announcement template](https://github.com/FRRouting/frr/blob/master/doc/developer/release-announcement-template.md) | No full signer identity/rotation declaration. |
| [Website README](https://github.com/FRRouting/frr-www/blob/master/README.md) | No full signer identity/rotation declaration. |
| [Website 10.7.1 announcement](https://github.com/FRRouting/frr-www/blob/master/content/release/10.7.1.md) | Dated 2026-08-25, links official APT/RPM packages and Docker; no A90 publication. |
| [Website maintainers](https://github.com/FRRouting/frr-www/blob/master/content/community/maintainers.md) | Names Jafar/@jafaral; name or GitHub association does not bind that person to A90. |

Representative immutable blob identities: release procedure
`04c82d67ba5bd7a595d0fcfec578aa9de3a1fbda`, Debian build helper
`7ae21272ad19cda8ed0a2f1f514fe5fe7f66b55d`, website10.7.1
`2789c69cb8b453074f33c3365ac2750471440a6f`, Debian Dockerfile
`b317b0598d9f4a9c118def48079720aec8eef168`.

Tree path inspection found no .asc/.gpg/signing/keyring/reprepro/aptly publisher
candidate in frr, and only nginx/frrouting.ssl.tar.gpg in frr-www. That encrypted
TLS-related asset was NOT downloaded/decrypted and is not repository key evidence.
Official GitHub commit search (message index, not source diff/history audit)
returned total_count0, incomplete_results=false for full A90 in frr and frr-www,
and AB194DBF in frr. Exact indexed official-domain web searches likewise found
no attributable full-fingerprint publication. Search absence is not exhaustive
negative proof, and unauthenticated/public visibility may omit private CI source.

## Remaining authority channel and action

The release procedure explicitly assigns repository publication coordination,
so a concrete remaining channel is an official repository-page correction or
maintainer-published trust/rotation statement containing the FULL fingerprint
and its repository/signing scope, ideally immutable upstream source/change.
A matching UID, maintainer name, keys.asc/keys.gpg bytes, self-signature, current
VALIDSIG or website package availability is insufficient independent evidence.
No message was sent to upstream; no new authority was assumed. NetDEF CI is
referenced by official release documentation but its private signing setup was
not accessed or asserted audited. Do not derive authorization from private
implementation guesses.

Strict operator trust/refusal must remain. Current resolute A90 signature can
only be described as cryptographically consistent with raw downloaded bytes,
not authorized by the three separately published identities. Canonical selection
mechanics and hosted13 fixtures do not close this authority gap. NodeSource
independent key authority/Ubuntu26.04 support remain unresolved; target
installation/bootstrap acceptance remains NOT RUN and TD-19 is not DONE.

Local checkpoint sent for manager publication; initial853f512e was separately
published by manager as353b2e2bccc159b3d80f79300d38a5a5cb308104 on
`task/TD19-frr-upstream-authority-source-20261002`. Final publication not claimed
until manager confirms. No product source changed or unnecessary full tests run.
