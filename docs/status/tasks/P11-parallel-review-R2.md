# R2 security review — P11 input verification

Reviewer /root, independent of developer.
Original reviewed27d3d800; final correctedlocal2dbdff2405e845da47aa149bb72cdedbce60649c.
Final tree406ff6de90fb16708d6523816eb5003f5f5b93b6.
Published candidatefa26c70072a9b125a129fe9594a218bd52f15512 inPR101.

Initial MAJOR finding: optional digest in the shared snapshot helper meant public verify(..., None) skipped the required source trust comparison. The command-line required flag did not enforce the Python-call boundary. Approval was withheld. Developer corrected verify to reject missing, non-string or malformed expected digests before snapshot work. Direct None/empty/integer negative cases and empty CLI digest are now covered. Independent full fixture rerun succeeded. Finding RESOLVED.

Private copies are bounded, regular-file checked and non-following. Source and metadata readback compare file identities to detect replacement during copying. Tar decompression is bounded before extended-header/member parsing; paths, duplicate members, links, devices and unexpected roots are rejected. Source bytes are never extracted or executed. Existing complete VPP install gate is invoked with fixed arguments and enabled tests on the private copy. Debian metadata is read without executing package scripts. No privilege, socket, service or authentication changes.

Source expected_origin is explicitly an expected URL, not observed provenance. Caller must obtain the expected digest independently through a trusted channel. Output retains release_approved false; synthetic fixture success does not approve the old source release, plugin ABI, real package build or appliance installation. No new dependency or GPL linkage introduced.

Evidence: independent original9fixture cases PASS18.056s; missing-digest finding then corrected; independent final9cases PASS35.523s on2dbdff24, including the new trust-boundary negatives; git diff --check exit0. Detailed test output is in separate T1 report.

Verdict APPROVE for corrected bounded source slice only.
Required remaining fresh reviewers and complete gate still apply.
