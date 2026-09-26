// apps/web/test/e2e/lib/nav.mjs — the left navigation, including collapsed groups (ui-nav-collapse, review D-117):
// groups start collapsed except the one holding the current page, so reaching Users/Revisions (or any future entry
// in a multi-item group) from a fresh load needs the owning group opened first, exactly like a person would.

/** Resolves once `link` is the current route (`aria-current="page"`) — the router navigates asynchronously. */
export async function waitForCurrentPage(page, link) {
  await link.and(page.locator('[aria-current="page"]')).waitFor();
}

/**
 * The nav label for entry `key` (`apps/web/src/nav/nav.ts`, `buildNav`): the fixed screens (dashboard, users,
 * revisions, tools) use `nav:<key>` directly; every schema-domain screen (interfaces, vrfs, nat, ...) uses
 * `nav:domains.<key>` instead. Tries the fixed form first, falls back to the domain form — so a screen never has to
 * know which kind of entry it is.
 */
function navLabel(tr, lang, key) {
  try {
    return tr(lang, `nav:${key}`);
  } catch {
    return tr(lang, `nav:domains.${key}`);
  }
}

const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/**
 * Clicks the nav entry `key`, opening its collapsed group first if needed. No per-screen "which group is this in"
 * map: the target link's own enclosing `<ul id>` (rendered by `AppShell`'s `NavList`) names its header through
 * `aria-controls`, so this generalizes to every current and future group without a lookup table to keep in sync
 * (`apps/web/src/nav/nav.ts` stays the only place that assigns groups).
 *
 * The initial "is it already reachable" probe deliberately does NOT use `getByRole('link', ...)`: MUI's `Collapse`
 * leaves a collapsed group's links in the DOM but excluded from the accessibility tree (zero-height, effectively
 * hidden), so a role query finds nothing at all for them — not "found but hidden". A plain locator (`menu.locator('a')`,
 * CSS-engine, not accessibility-tree-filtered) finds the anchor either way; only once its group is confirmed open do
 * we switch to the accessible, role-based locator to click and to wait for `aria-current`.
 *
 * `check(cond, msg)` is optional — pass a checklist's `check` (see `lib/checklist.mjs`) to record the "opened the
 * collapsed group" step as a named check (as `flow.e2e.mjs` does); omitted, this just throws if the group failed to
 * open.
 */
export async function nav(page, tr, lang, key, { check } = {}) {
  const assert = check ?? ((cond, msg) => {
    if (!cond) throw new Error(`nav: ${msg}`);
  });
  const menu = page.getByRole('navigation', { name: tr(lang, 'common:menu.navigation') });
  const label = navLabel(tr, lang, key);
  const anyLink = menu.locator('a').filter({ hasText: new RegExp(`^${escapeRe(label)}$`) }).first();
  await anyLink.waitFor({ state: 'attached', timeout: 15_000 });
  if (!(await anyLink.isVisible())) {
    const listId = await anyLink.evaluate((el) => el.closest('ul[id]')?.id);
    if (!listId) throw new Error(`nav: "${key}" is not visible and is not inside a collapsible group`);
    const header = menu.locator(`[aria-controls="${listId}"]`);
    const headerLabel = (await header.innerText()).trim();
    await header.click();
    assert((await header.getAttribute('aria-expanded')) === 'true', `[${lang}] nav: opened the collapsed "${headerLabel}" group to reach ${key}`);
  }
  const link = menu.getByRole('link', { name: label });
  await link.click();
  await waitForCurrentPage(page, link);
}
