import i18n from 'i18next';
import en from '../../../locales/en/neighbors-ra.json';
import fa from '../../../locales/fa/neighbors-ra.json';

/**
 * The per-interface fields of F-neighbors-ra (`ipv6Ra`, `proxyArp`, `proxyNd`, group `neighbors-ra`) are rendered by
 * P08's interface drawer, which localises top-level field titles and group names from the `interfaces` namespace
 * (`field.<name>.title|help`, `group.<group>`). That namespace's files are not this feature's (wave-A-hotspots W4), so the
 * strings live in `neighbors-ra.json` (`interfaceDrawer`) and are merged into `interfaces` here, without overwriting
 * any existing key. Imported for its side effect by `i18n.ts` (the one import under the feature's W3 anchor).
 */
export function mergeInterfaceDrawerStrings(instance: typeof i18n = i18n): void {
  instance.addResourceBundle('en', 'interfaces', en.interfaceDrawer, true, false);
  instance.addResourceBundle('fa', 'interfaces', fa.interfaceDrawer, true, false);
}

if (i18n.isInitialized) mergeInterfaceDrawerStrings();
else i18n.on('initialized', () => mergeInterfaceDrawerStrings());
