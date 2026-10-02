# TD-19 work checkpoint

Base8f07ce68; branch task/TD19-artifact-preflight-20261002.
Read AGENTS/shared context/contributing/decision policy, actual board scope and
review verdicts. No standalone TD-19 prompt exists; row scope is authoritative.
Findings:00-add-repos executes floatingFDio installer;20 pins oldGo1.23.4 and
uses @latest generators;40 executes unpinned containerlab installer;tools/lab
uses hardcoded upstream filenames and performs remote sysctl changes before
artifact verification. Existing original manifest verifier and seven shipping
package contract available. Authoritative third-party keys/checksums absent.
Next source: non-mutating explicit artifact preflight in00, mandatory original
install gate before existing repo setup, and remove floatingFDio. Tests run only
fixtures; no host install/network/key/service commands authorized for execution.
Next command: bash -n scripts/00-add-repos.sh; original verify missing-artifact
refusal; fixture command-order checks. Remote publication pending manager.
