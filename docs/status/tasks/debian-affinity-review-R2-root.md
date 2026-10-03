# Independent R2 bounded build-affinity portability fix

Root reviewer/coordinator committedexactdeveloperwrittenfiles, no productcodeauthorship. Frozenccc0d2e630250aeb609326ce698882c9cecb7ac2/treedf38e6a5f6417f1d7d45475e5dfb4230f9fbe1f2, publishedce77e7ba41a50743576ecd50a053f43a5d7bf17e. R2 APPROVE, no findings.

Actual processaffinity via standardPython sched_getaffinity, explicitintegerCPUlist, jobs1..8 validation and existingtaskset/nice/jobcap retained. No uservaluesshellcode, privilege, inputprovenance, checksum/dependency/installguard or compilerflags weakened. Tests changeaffinity of onlyspawnedchildren and verifyrealchildmask, sparse/singleton andinvalidcounts. Rootactualcompiler ownsprivateworktree paths; sourceinputs hashverified, no hostpackage/service/configmutation. New helper Linuxonlyconsistentexistingtasksetrecipe.

Actualrootcheck:
```text
gitleaks git --redact --config .github/gitleaks.toml --log-opts=a2378278..HEAD
2 commits scanned; no leaks found; exit0
git diff --check ae23d2dc HEAD
exit0
```
Developerstatic72PASS/packaging30PASS1signingskip; freshR1/R7/R8reviewinprogress. Fullcurrentmainintegrationgate remainsrequiredbeforemerge; no real.deb yet.
