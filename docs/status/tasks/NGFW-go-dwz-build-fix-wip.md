# NGFW native Debian DWZ build failure fix

Branch: codex/ngfw-go-dwz-fix-20261003.
Worktree: /root/Documents/Codex/2026-10-03/check-out-latest-code-from-git/work/NGFW-go-dwz-fix.
Fresh fetched origin/main base: 07fbfb1929da9781671f1e470ce23ff7279ba9a5,
tree 837465bc9147f20bb15bf60c29c70404807d1c1b (merged PR126).
Owned ONLY deploy/debian/ngfw/debian/rules and this WIP.

Observed root actual native build /tmp/ngfw-final-native-build.log fails at
 dh_dwz -a: all three CGO_ENABLED=0 product Go binaries contain compressed
 .debug_abbrev; dwz refuses optimization and then its multifile command exits1.
No .deb was produced by that attempt. Source prepare.sh builds exactly
ngfw-agent, ngfw-startupgen and ngfw-vppcheck with CGO_ENABLED=0.

Minimal override invokes dh_dwz -Xngfw-agent -Xngfw-startupgen -Xngfw-vppcheck.
This excludes those unsupported Go binaries; other native processing, stripping,
debug handling and the remaining standard debhelper sequence remain enabled.
No DEB_BUILD_OPTIONS=nostrip workaround or gate weakening. No changes to pins,
privileges, services, package control metadata, staged payload or host state.

Actual developer checks:
```text
make -n -f deploy/debian/ngfw/debian/rules override_dh_dwz
dh_dwz -Xngfw-agent -Xngfw-startupgen -Xngfw-vppcheck
git diff --check
(exit0, no output)
```
No mirrored tests added. Root owns the private failed stage
work/NGFW-native-final-bda4; developer did not mutate it or claim rebuilt packages.
Independent R7 review and immediate publication via root required. Root will
resume/rebuild through standard dpkg-buildpackage after reviewed source recipe,
preserving prior staged-source provenance and recording the exact new recipe SHA.
Full actual four-.deb build and unchanged mandatory gate evidence remain pending.
Next: root publishes source checkpoint, obtains independent review, retries
private build and verifies the real output package/control/hash inventory.
