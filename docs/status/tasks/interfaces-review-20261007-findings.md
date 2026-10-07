# Independent source findings — pending verification

Reviewed source: c450739df14ee4abd97f021b71d5a8104222fa00 + a3d2322f7284f49ff5d58173a18c6fbd4e824f6f. Report branch only; no product changes.

1. R6 MAJOR — InterfacesPage.tsx host link fallback and InterfaceDrawer.tsx inventory link: host LinkUp=false conflates down and unknown (carrier read fails or VFIO has no netdev). A no-netdev PCI NIC can appear definitely down despite no observation. Developer asked for conservative translated down-or-unknown and regression; real engine state should continue to take precedence.
2. R1/R3 candidate edge — state.controller.ts PCI lookup prefers the whole running value before candidate. Existing running logical row without physical plus newly staged candidate physical metadata may fail correlation and create duplicate inventory row. Developer asked to test distinct running/candidate documents and scan their physical views safely.
3. R1/R6 full screen regression — all InterfacesPage tests together stalled >3 minutes with no first EN case result; own process was terminated explicitly (exit143). Targeted new EN/FA tests pass. Developer must resolve/reproduce and pass mandatory quick gate before merge.
4. R7 pending source completion — final task and contract reports absent at first checkpoint, WIP explicitly says unverified; developer asked for truthful output and limits. Manager owns D240 policy and lab-only acceptance deferral.
5. R1/R4 bounded host discovery — existing direct-device PCI resolver omits virtio child path; manager authorized minimal read-only resolver fix + fixtures. Native vmxnet3 unique MAC can correlate safely with matching physical driver; virtual virtio/TAP lookalikes must remain unmerged absent exact name or PCI evidence. Multiport per-PCI dedup and non-PCI virtual/USB bounds need explicit documentation.

Own actual evidence:

```text
pnpm install --frozen-lockfile --prefer-offline -> Done in14.2s
TURBO_CACHE_DIR=/root/.cache/ngfw-turbo pnpm exec turbo run build --filter=@ngfw/proto --filter=@ngfw/schema --filter=@ngfw/yang --filter=@ngfw/api-client --filter=@ngfw/ui-kit --concurrency=2
Tasks:12 successful,12 total; generated tracked files unchanged
TMPDIR=/root/ngfw-review-tmp/interfaces-review tools/ci.sh check --base3ddb1680e
check PASSED(0m13s); gitleaks no leaks
pnpm --filter @ngfw/api exec vitest run src/state
Test Files4 passed; Tests15 passed
pnpm --filter @ngfw/web exec vitest run src/domains/interfaces/InterfacesPage.test.tsx -t 'automatically displays'
Test Files1 passed; Tests2 passed/12 excluded; EN3896ms; FA2747ms
```

The full unchanged quick gate is required independently on the final integration tree; manager requested no duplicate heavy gate during source development. Unit frontend evidence above is not live stack or hardware proof.
