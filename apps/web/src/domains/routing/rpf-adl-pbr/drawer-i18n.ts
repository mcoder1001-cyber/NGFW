import enInterfaces from '../../../locales/en/interfaces.json';
import faInterfaces from '../../../locales/fa/interfaces.json';
import en from '../../../locales/en/rpf-adl-pbr.json';
import fa from '../../../locales/fa/rpf-adl-pbr.json';

type Drawer = typeof en.drawer;

/**
 * The interface drawer (P08) titles its fields and groups from the `interfaces` namespace (`field.<name>.title|help`,
 * `group.<group>`), a file this task does not own. The strings of the uRPF/ADL fields and of their `rpf-adl-pbr` group
 * ("Security") live in this feature's namespace (`drawer.*`) and are merged into `interfaces` here — before i18next
 * initialises (i18n.ts imports this module under the F-rpf-adl-pbr anchor) and never over an existing key.
 */
function merge(target: Record<string, unknown>, src: Drawer): void {
  const field = (target['field'] ??= {}) as Record<string, unknown>;
  field['urpf'] ??= src.urpf;
  field['adl'] ??= src.adl;
  const group = (target['group'] ??= {}) as Record<string, unknown>;
  group['rpf-adl-pbr'] ??= src.group;
}

merge(enInterfaces as Record<string, unknown>, en.drawer);
merge(faInterfaces as Record<string, unknown>, fa.drawer);
