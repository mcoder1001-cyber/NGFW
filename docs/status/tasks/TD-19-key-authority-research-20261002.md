# TD-19 third-party key authority and Ubuntu 26.04 research

Checked 2026-10-02 23:06 UTC. Docs-only phase, branch
`task/TD19-key-authority-research-20261002`, worktree
`NGFW-TD19-key-authority-research`, exact base
`344c919083f9e5ae7ab177efcdcbda8f1a6b702f`. Owned file: this report only.
No installer, pin, workflow, board, decision, frozen PR or main changes.
Parent CI manager requested current primary evidence before further code.

## Existing source and acceptance boundary

`scripts/00-add-repos.sh` requires administrator-provided exact primary sets
`VRX_FRR_KEY_FINGERPRINTS` and `VRX_NODESOURCE_KEY_FINGERPRINTS` before network/APT.
Its reviewed parser rejects duplicate/unmatched primaries and unsupported
validity, checks secret packets, and validates both downloads before keyring
writes. Original BLOCK and corrected current-main composition reviews remain
unchanged in `TD-19-repository-key-review.md` and
`TD-19-key-current-main-review.md`. Existing fallback comment saying resolute
may be unpublished is stale. Central `DEFERRED-ACCEPTANCE.md` still correctly
marks target provisioning/boot NOT RUN; published availability below does not
prove installation or runtime compatibility. TD-19 remains unfinished.

## FRRouting: documented authority now exists, bundle mismatch remains

[Official repository page](https://deb.frrouting.org/) independently publishes
these three primary fingerprints:

- `4A56C7738BB3F81595A805D2A832769908F13ED1`
- `3D9968AC9AE7BE1169288DDB1FD5839895F57FDA`
- `BBC9ACA9D13025A2C186FF7F741E92A1F6E3975B`

It explicitly lists Ubuntu 26.04/resolute, stable/10/10.7 channels,
`10.7.1-0~ubuntu26.1` including amd64. The moving stable channel is not a fixed
package version. Read-only GET of [Release](https://deb.frrouting.org/frr/dists/resolute/Release)
confirmed codename resolute, Ubuntu26.04 description, date 2026-08-31 01:33:21 UTC,
and components frr-stable/frr-rc/frr-10/frr-10.6/frr-10.7.
This metadata was inspected, not signature-verified or installed.

[Downloaded public bundle](https://deb.frrouting.org/frr/keys.gpg), 16112 bytes,
SHA256 `bf10935b9296e2ce7c5d9855fa29ef30c35810b0fc4b1f53005494a04a33554d`,
has FIVE pub records: the documented three, an additional
`A90FC36D9429409798E9C2D874DEED43AB194DBF` (Jafar launchpad identity), and a second
copy of `BBC9ACA9D13025A2C186FF7F741E92A1F6E3975B`. The extra primary is not
independently authorized by that page. Actual unchanged `verify_repo_key`
extracted into a private temporary probe, using documented three pins and
real GPG, returned exit1 `downloaded primary key set differs from trusted pins`;
no dearmored output was created. Do not weaken exact-set validation or pin the
extra/duplicate merely because the download contains them.

## NodeSource: endpoint observed, independent pin/support unresolved

[Official DEV_README](https://github.com/nodesource/distributions/blob/master/DEV_README.md)
documents nodistro and its Ubuntu matrix ends at 24.04, including Node22.
[Official setup22 source](https://github.com/nodesource/distributions/blob/master/scripts/deb/setup_22.x)
uses the same public-key endpoint and node_22.x/nodistro, but does not independently
publish a fingerprint; broad Debian-based OS detection is not a Ubuntu26.04
support guarantee. Checked upstream master tree
`9b431d8ae0f10df272598585855c6eca6c0e1bd2` through official GitHub tree API:
no key/gpg-named paths. Setup22 blob identity
`2d8bdb9c95cde288a45797769289daeb3c63814c` (blob, not commit).

[Downloaded key](https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key),
1717 bytes, SHA256
`b42e0321dabdc24e892115da705cf061167eac12a317f23d329862d0aa0a271d`,
contains primary `6F71F525282841EEDAF851B42F59B5F99B1BE0B4`, RSA2048,
NSolid identity. This is an extracted observation, NOT an independently
published trusted pin. Primary created 2016-05-23; UID packet timestamp changed
2026-01-14. Do not infer key rotation from changed UID alone.
[Node22 Release](https://deb.nodesource.com/node_22.x/dists/nodistro/Release)
was readable, dated 2026-10-01 20:59:48 UTC, codename/suite nodistro, including
amd64; it does not declare supported Ubuntu releases. No independently
published fingerprint or explicit Ubuntu26.04 support statement was located
in the inspected official sources/searches. This is a bounded negative finding,
not proof no such publication exists. A user issue mentioning Ubuntu26.04 was
excluded as support authority.

## Inspection and next decision

GnuPG2.4.4 `--no-options --homedir PRIVATE0700 --batch --with-colons
--with-fingerprint --show-keys FILE` succeeded for both downloads. All primary
records had validity `-` (unknown ownertrust in empty private home), blank expiry,
and signing capability directly or via subkeys. This is not authenticated
repository ownership, future validity, installed APT signature acceptance or
cryptographic signature audit. `--list-packets` exited0 for both and contained
no secret-key/subkey packets. All inspection was temporary; no host keyring,
APT, service, installation, upstream script execution or external mutation.
Reported hashes are reproducibility observations, not independent trust roots.

Proposed bounded next code option, requiring fresh security review: preserve
raw-bundle secret/size checks, import only into private temporary GPG home,
select/export exactly the documented FRR certificates (excluding extra and
deduplicating repeated packets), then run existing exact-set validator on that
selected output. Alternative: obtain an official dedicated certificate endpoint
or upstream corrected bundle. Do not silently accept all downloaded identities.
NodeSource must retain explicit trusted-administrator pin refusal until an
independent official publication/immutable key blob or separately authorized
trust source exists. Ubuntu26.04 Node22 install/boot remains NOT RUN.
No code implementation or activation is authorized by this research report.

Checkpoint: local commit reported to manager for publication; no remote SHA
claimed. Next command after handoff: manager reviews authority/mismatch and
assigns an independently reviewed certificate-selection scope, or obtains
upstream corrected bundle and NodeSource authority evidence. No new tests of
unchanged installer, full CI or target acceptance were claimed.
