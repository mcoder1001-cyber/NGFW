# PR title
fix(management): close HTTPS stream sockets during API shutdown

# PR description
An authenticated WebSocket upgraded through the management HTTPS listener survives the listener's close operation, leaving active stream connections after API shutdown. Track only this listener's upgraded sockets, remove them on close, and destroy remaining sockets during shutdown. Refuse newly arriving upgrade callbacks once shutdown starts, including while a previous certificate reload is still waiting on a secret read.

The production TLS/Fastify stream fixture reproduced the old failure, then passed with the correction. Final focused suite:14/14PASS7.86s; APItypecheck and focusedESLint PASS; unchanged repository staticcheck PASS9s. The new test holds an active authenticated TLS101 stream, blocks a reload, rejects a new upgrade before subscription attach, then observes client closure and disabled listener state. Certificates/authentication/route shape remain unchanged. English/Persian user docs explain restart disconnection versus live certificate reload.

This branch depends on frozen lifecyclePR100 and streamPR99. Independent applicable reviews and unchanged complete hostedquick remain mandatory; no localfullquick or realDB/browser/appliance pass is claimed. The separate inherited Go fixture failure is under its own correction task. Whole management and certificate removal policy acceptance remain incomplete; no host services, privileges, VPP, contracts or packages are changed.
