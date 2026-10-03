# NGFW contract/agent rename WIP
Base0d174caf, remote unpublished; inventory/protocolsharedrootandconsumerdeveloperbeforechanges. Contractnamespace renameauthorized explicitly; behavior/fieldnumbers remainfixed. No implementationyet. Nextcommands: baselinebufdescriptor; replaceownedcanonicalsource/paths; generateproto/schema/yang; meaningfulcontract+Go tests andindependentreview. Tools/deploy/UIconsumers areseparate coordinatedbranches; no fullintegrationgateclaim untilassembled.

Canonicalproto/schema/yang renamed andgeneratedsuccessfully withactualpnpmofflinefrozeninstall; bufdescriptorbaselinevsrenamed confirms ALLfieldnumbers/types/options/servicesignatures identical afterexpectedVRXcase/name normalization (sourcecodeinfo removed only). Generatorcommands bashpackages/proto/gen.sh, pnpm--filter@ngfw/schema gen, pnpm--filter@ngfw/yang gen allEXIT0. GeneratedYangoldvrxfiles removed andcanonicalngfwfiles recreated; Go/TSbindingsactualgenerated, nothandedited. Contractscommittedbefore ownagentconsumers; consumerdeveloper cancherry-pick sharedcheckpoint intoownbranch. Agent/testsource renamedbutconsumercheckpoint follows; wholecombinedGo gate notyetclaimed.

ActualcontracttestsPASS: schema1600/76files; proto107/2files; Yang13/1file. InitialagentfullraceFAIL realinternalprotobufpanic: localgenerated*.pb.go outsideapps/agent/gen was mistakenly textreplaced, corruptingencodedrawdescriptorlengths. Preserve /tmp/ngfw-contract-rename-agent.log; priorfde source notreadyGo beforecanonicalregeneration. CorrectionregeneratesALLinternal43protos? exactfilecountinactualinternalprotoclog, usinginstalledprotoc+protoc-gen-go and canonicalSOURCEpath frombasegeneratedheader (avoids duplicate model.proto descriptorregistry). Noencodedbyte handpatching; upstreambinapi untouched. ActualcommandsincludeIrootcanonical/descriptors/packagesproto, --go_out=canonicalroot --go_opt=paths=source_relative. Rootnotifiedcause/correction. Retestagentfullrace running; no passclaimed beforeexit.

Correction followup: firstinternalgeneration stopped at nftables because standardtimestamp.proto include directory was missing; earlier shell trailingdiffcheck hid Pythonnonzero. Retestrealagentcaught remainingnftablesdescriptorpanic; preserved/tmp/ngfw-contract-rename-agent-fixed.log. Resolved usingcanonicalREADME wellknownimports /root/go/pkg/mod/github.com/bufbuild/protocompile@v0.14.1/wellknownimports. Standalonegenerator Pythoncommand nowEXIT0, COMPLETE30canonicalinternalprotos regenerated (actual/tmp/ngfw-contract-rename-internal-proto-final.log). No encodeddescriptorhandedits; finalrealGo race stillrequired. Contracttypechecks andlintallDone/EXIT0, existingrootESLintmoduletypewarning disclosed.

ALL30internalcanonicaldescriptor before/after proofPASS usingactualprotoc descriptorsincludingimports→bufJSON; fieldnumbers/types/options/oneof/services unchanged afterauthorizednamecase normalization, sourceCodeInfo omittedonly. Raw/tmp/ngfw-contract-rename-internal-wire-proof.log. ALL17go.sum andrealPEMbase64blocks byteidenticalbase; upstreambinapi diffempty.

After30generation, actualfullGo race reachedrealtests(noinitpanic) butrenamefixturefailures: fixed16/20/32bytesyntheticsecrets grewonebyte, BFDexpected14→15,IKEv2fakeDataLen10→11, deterministicWireguardpublickeysortchanged, canonicalgoldenencodedmarkers andnftableslogrulecommenthashes stale. Preserved/tmp/ngfw-contract-rename-agent-final.log. Fixturecorrectionkeeps AES16/20 andX25519_32bytes byshorteningdocumentedNGFWplaceholder suffix (no realsecrets); fixedexpected15/11 explicitlengths; sortedpublickeytestvectors preserveexistingRetrieveorderedassertions. Existinggoldenupdateflags regenerate actualrendereroutput, no productassertionweakening. CapturednftablesJSON13commenthashes migrated fromcanonicalnewgolden withsame IDs; commentexplicitlydisclaimsnewkernelcapture. Initialtargeted9pkgs8PASS1FAIL becausedriftfixtureanchor oldhash; actualcanonicalanchor updatedfromsamegoldenmap, preserving strictdriftassertion; raw/tmp/ngfw-contract-rename-fixture-fixes.log. Finalfullrace running; no PASSclaimyet.

19testGo modules unitgo test-count1+go vetALLPASS withliveintegrationunset; /tmp/ngfw-contract-rename-go-modules.log. Sourcecanonicalcontractbuild/test/typecheck/lintpass above; wholefinalquick remainsintegrationrequired.

## Final frozen source verification
Frozenproduct75e8a38b25fa6cb743ab19dc22c14a8d4ba868aa/tree755399611a2e6fd974d5213f03ac6cb472a7d805; this finaldocumentation-only checkpoint follows. Prior e24scope,1bcontracts,fdeconsumer,f865internal,a096nft,75efixture history preserved. Root confirmed initiale24 remotedurable ebeffd67 (exacttree27b659); subsequent checkpoint publication handledcoordinator, no unverified remoteclaims.

Actualfinalcommands/results:
```text
pnpm --filter @ngfw/schema --filter @ngfw/proto --filter @ngfw/yang test
schema:76files/1600testsPASS; proto:2files/107testsPASS; Yang:1file/13testsPASS; exit0
allthreepackage typecheck andlint:exit0 (existingrootESLintmodule-typewarning retained)
go -C apps/agent test -race -count=1 ./...
exit0; 123testedpackagesPASS; agent73.873s,contracttest2.563s,desired28.873s,subsystems23.997s
go -C apps/agent vet ./...
PASS; actual log onlyresourcequeue10s, no veterrors
go -C apps/agent build ./...
PASS; emptybuildlog
all19testGo modules:go test-count1 ./... +go vet ./...
ALLPASS(unitmode; liveintegrationNOTRUN)
bash packages/proto/gen.sh (repeatafterfinaltests)
EXIT0; exactzero gitdiff againstfrozen75e generatedGo/TS/contracts
all17go.sum andupstreambinapi byteidenticalbase0d174
PASS
public+ALL30internaldescriptor fieldnumbers/types/options/oneof/services normalizednames
PASS
git diff --check
PASS; worktreecleanbeforethisdocumentationseal
```

Rawlogs /tmp/ngfw-contract-rename-{agent-green,agent-vet,agent-build,contract-tests,contract-types,contract-lint,go-modules,internal-wire-proof,proto-repeat}.log; initialfailures and actualcanonicalfixturemigrationrecord above remainhistoricalproof. No newstubs/skips/hostservices/DBmigrationexecution/upstreamhash mutation. Existing unitintegrationguards renamed NGFW and remainunset; no realdevice/VPPfullacceptanceclaimed.

Remaining: coordinatorpublishesfinalexacttree andconsumerrootintegration alignsAPI/web/CLI/deploy/tools/docs/protocolnames, assigns freshmandatoryindependent R1/R2/R3/R4/R7(/R6Yangdocsifapplicable), thenunchangedcompletequick onexactassembledcurrentmain tree. Ownisolatedbranch stillhasotheractors'legacyAPI/toolconsumers untilcomposition, so no wholeprojectquickpassclaimed. Explicitusertechnicalrename authorization recorded, no separateapprovalrequest needed. Developerdoesnotselfreview/merge. Allowntest/build/generatorprocesses ended; codefailure:none. Nextcommand: git rev-parse HEAD HEAD^{tree}; rootpublishesexactfinalcheckpoint anddispatchesindependentreview.


Independent review found actual VPP geometry BLOCKER in frozen13e: mechanical placeholderMask rename became17bytes while MatchNVectors remains1 (pinned API requires16). New strict request-boundary geometry+ownedcleanup regression actually RED; command `go -C apps/agent test -count=1 ./internal/vpp/ifsanitize -run "TestPlaceholder(PinnedGeometryAndOwnedCleanup|ChangedSignatureIsNeverDeleted)$"` EXIT1, log /tmp/ngfw-placeholder-geometry-red.log. Changed-signature cleanup refusal test passes and confirms foreign table never deleted. Prior123package race proof preserved but its existing fake did not enforce geometry. Next choose exact16-byte NGFW signature without changing vectors/ownership guard, rerun focused race/vet and fullagent suite, separate independent review required. This test-only RED checkpoint needs rootpublication.

Minimal productcorrection: placeholderMask now ngfw-td3-v19-hld, exactly16bytes; MatchNVectors=1, MaskLen derivedlength, ownership readback comparison and drop refusal unchanged. Full ifsanitize/subpackage uncachedrace executing, no PASS claim yet. RED parent40feb750 preserved; publication through root pending. Next actual command `go -C apps/agent vet ./internal/vpp/ifsanitize/...`, then fullagent race and independent reviewer. No hostVPP calls/install/contract changes.
