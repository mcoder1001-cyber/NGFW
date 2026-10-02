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


New fixture checkpoint: six refusal/blocked-host tests (five controlled GPG semantics plus one ACTUAL GPG malformed-input refusal) and six real-certificate tests. Five controlled cases PASS2.130s after raw private-parent hardening; actual GPG malformed refusal PASS0.055s. Controlled partial-import2 aborts before export/publication, export2 refuses, secret/parser errors abort before import, nonregular/FIFO/size/pins/private-parent fail beforeGPG, foreign existing output preserved, either FRR-import or Node-identity failure exercises actual entry with fake preflight/tools and NO APT/install/mv execution. All private GPG/staging paths observed removed.

Actual REAL fixture generation in local environment FAILED: GPG exit2, gpg-agent could not start/connect, key generation `No agent running`. Initial full invocation:5 refusal tests PASS plus RealCertificates.setUpClass ERROR, overall FAILED(errors1). This is NOT positive import/export acceptance; six real-certificate cases remain NOT RUN locally. There is no skip/bypass/agent-error waiver. Full new fixture runner requires exactly12 actual completed tests and zero non-success; hosted fixture run on normal Linux is mandatory before positive acceptance.

Real pending cases explicitly cover full named set with duplicate+extra exclusion/subkey preservation, missing primary, conflicting duplicate revocation, expired and cert-only nonsigning primary, secret/raw-malformed input. Export fixtures require actual nonempty successful exports so negatives cannot pass on silently empty fixture data. Original nine raw tests passed4.007s, untouched; raw verify_repo_key exact byte identity preserved. Original provisioning36 and Node trust gate remain unchanged. Source callback/installer/lab whole-task acceptance is not supplied here.

Next: publish this source/test checkpoint, independently review R1/R2/R4/R7/R8, prepare a new strict hosted12 certificate-fixture gate without weakening original36, then compose onto actualmain782 preserving unrelated script20 Go-module fix. ShellCheck local NOT RUN (unavailable); Bash syntax/whitespace PASS. No host/network/APT/repository install or global GPG home mutation occurred. New source is frozen for review, not wholeTD19DONE.
