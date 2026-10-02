# Signed appliance APT repository

After the product packages and VPP runtime have passed their gates:

```
scripts/publish-apt.sh /path/to/verified-vpp /path/to/vrx-debs /path/to/new-apt-output
```

The script verifies the VPP install gate, includes exactly the seven shipping
runtime packages and four matching-version VRX packages, checks vrx-meta's VPP
pin, signs reprepro Release metadata and verifies InRelease. The local signing
key lives under the user's protected config directory; it is never committed,
printed or copied to the repository. The exported archive keyring is public.
Keep the source/signing host trusted: unsigned input manifests detect corruption,
not malicious writes. The manager publishes the completed output to
`/srv/vrx-artifacts/apt/`; `RELEASE-READY` is created only after signature validation.
Provision the public trust anchor through a separately authenticated channel.
No unsigned-repository/allow-insecure APT switches are supported.

Repository generation and installation have not yet run on a release builder.
