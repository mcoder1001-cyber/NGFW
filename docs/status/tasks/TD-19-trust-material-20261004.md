# Exact repository trust material for owner review

Read-only HTTPS GET and private GnuPG2.4 inspection, 2026-10-04. No APT,
installation, global trust/keyring or production default changes. Existing
installer fail-closed owner-supplied fingerprint behavior remains unchanged.

Official [FRR homepage](https://deb.frrouting.org/) independently lists only
`4A56C7738BB3F81595A805D2A832769908F13ED1`,
`3D9968AC9AE7BE1169288DDB1FD5839895F57FDA`,
`BBC9ACA9D13025A2C186FF7F741E92A1F6E3975B`.
Its linked [canonical key endpoint](https://deb.frrouting.org/frr/keys.gpg)
download additionally contains `A90FC36D9429409798E9C2D874DEED43AB194DBF`,
and repeats the BBC certificate. These are observed downloaded identities,
not separately published authorization. Public certificate bundle checksum (SHA-256):
`bf10935b9296e2ce7c5d9855fa29ef30c35810b0fc4b1f53005494a04a33554d`.

Actual private gpgv verification of [resolute
InRelease](https://deb.frrouting.org/frr/dists/resolute/InRelease) returned0,
VALIDSIG signing/primary `A90FC36D9429409798E9C2D874DEED43AB194DBF`, timestamp
2026-08-31 01:33:21 UTC, RSA/SHA256. InRelease SHA256:
`39f9edda794e019d282e4500dfd18b72226632e1875c93cbc4e693a117ba0528`.
This establishes downloaded byte/signature consistency only. Current separately
published three certificates cannot validate this fourth signer.

[NodeSource canonical public key](https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key)
has primary `6F71F525282841EEDAF851B42F59B5F99B1BE0B4`, subkey
`0FA5ECC8C0CA58863C0AC5867E9656125E955B26`. Public certificate bundle checksum (SHA-256):
`b42e0321dabdc24e892115da705cf061167eac12a317f23d329862d0aa0a271d`.
Private dearmor and actual gpgv of [Node22 nodistro
InRelease](https://deb.nodesource.com/node_22.x/dists/nodistro/InRelease)
returned0, VALIDSIG signing/primary
`6F71F525282841EEDAF851B42F59B5F99B1BE0B4`, timestamp2026-10-01 20:59:48 UTC,
RSA/SHA256. InRelease SHA256:
`c696cbd32421aa5a04d79a80f82d7b43c948e314792b954ed57f69bbe4867fdf`.
No independent official fingerprint publication located in bounded research.
[Vendor support table](https://github.com/nodesource/distributions/blob/master/DEV_README.md)
ends at Ubuntu24.04, so trust approval would not prove Ubuntu26.04 compatibility.

## Concrete decision alternatives

1. Owner explicitly authorizes official HTTPS endpoints as the initial trust
   anchor for the exact observed primary fingerprints above: select/canonicalize
   the four FRR certificates (remove duplicate), retain strict full-set identity
   validation, pin NodeSource exact primary and verify repository signatures
   against those pins. Continue requiring exact runtime package versions and
   digest manifests. Such approval changes the trust provenance and requires
   an explicit recorded owner answer; it has not occurred here.
2. Keep existing administrator-supplied fingerprints and refuse unattended
   repository setup until independently published authority is obtained.

Neither alternative permits trusting changing downloaded identities by default,
accepting an arbitrary future fourth signer, weakening validation, or treating
installation/runtime compatibility as tested. Current code follows option2.

Raw material retained only in ignored private `.scratch/trust-20261004` under
Packaging-complete. It may be re-fetched to reproduce; no private material exists.

## Signed index consistency

Actual canonical Packages downloads matched their SHA256 and byte lengths in
the successfully verified respective InRelease SHA256 sections:
FRR index21289 bytes,
`1abdb00c4ae56a1c993df91db5392832bbf83b4ca93e86b161386268324aa477`;
NodeSource index88661 bytes,
`5eb85fea1d479f5381331df50d45ea2de57be872a5a73e87d56a3d72894fe666`.

Signed FRR index advertises amd64 frr10.7.1-0~ubuntu26.1,6839342 bytes,
package SHA256 `5378bd6c6d76daf10d76775dd098e5fbc080f7206654646ad36239dc0f810132`.
Signed NodeSource index advertises amd64 Node22.23.2-1nodesource1 matching the
repository's documented host patch pin,37827592 bytes,
SHA256 `eed0c5f0ab411f28783f81f051fcf7928ae8bf833e2df11f48f3fa78270025cb`.
A newer22.23.3 entry also exists; no version upgrade decision is made here.
No .deb download or installation performed. This digest chain does not establish
independent trust ownership or Debian policy/target-runtime compatibility.
