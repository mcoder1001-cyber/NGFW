# TD-19 FRR selection recovery checkpoint

Own branch `task/TD19-frr-certificate-selection-20261002`, isolated
`NGFW-TD19-frr-selection`, base344c919083f9e5ae7ab177efcdcbda8f1a6b702f.
Remote publication awaiting manager. Read exact researchb0/conditionaldesign769,
script and old raw-validator fixtures. Implementation not yet committed; no new
source/fixture PASS or hosted result claimed.

Chosen constrained policy recorded in envelope: explicitadminfull40 pins, fullraw
sanity/secret rejection before private import, require exact successful import,
complete named public export, unchanged fresh-context exact verifier, Nodeunchanged
and both gates before host mutation. Rawduplicate/extra tests remain untouched.
Next: implement narrow preprocessing helper, actual GPG/blocked-host fixtures,
run meaningful local negatives and document unavailable positives honestly.
No key authority invented/defaulted, no APT/network/lab or global keyring access.

Source checkpoint implements distinct FRR selector with early full40 gate, complete raw sanity/secret checks, fresh private transient homes, strict import/export exit propagation, full certificate selection then unchanged raw verifier, exclusive0600 publication to caller-owned0700 directory. Node remains unchanged and both gates precede APT. Original raw verifier byte identity PASS; old nine parser/gate tests PASS (actual output recorded in next checkpoint). New actual GPG/blocked-host fixtures still pending; no positive selection/hosted PASS claimed.
