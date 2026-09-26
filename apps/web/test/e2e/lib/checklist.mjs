// apps/web/test/e2e/lib/checklist.mjs — the ok()/check() pattern flow.e2e.mjs already used, shared so shots.mjs
// reports the same way: every check prints as it happens (`ok   <msg>`) and a failing `check()` throws immediately
// with the message that would have been printed, so the first failure is also the error that stops the run.
export function createChecklist() {
  const results = [];
  function ok(msg) {
    results.push(msg);
    console.log(`ok   ${msg}`);
    return true;
  }
  function check(cond, msg) {
    if (!cond) throw new Error(`FAILED: ${msg}`);
    return ok(msg);
  }
  return { ok, check, results };
}
