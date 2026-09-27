import i18n from '../../../i18n';

/**
 * WEB-4b: the `ha` namespace lives next to the (unrouted) screens so nothing shared is touched; F-vrrp-config-sync
 * may move it to locales/{en,fa}/ha.json when it routes the page.
 */
export const NS = 'ha';

export const haEn = {
  title: 'High availability',
  intro:
    'VRRPv3 virtual routers and cluster membership. Changes are staged in the candidate and committed from the pending-change bar.',
  tabs: { vrrp: 'VRRP', cluster: 'Cluster' },
  save: 'Save to candidate',
  none: '—',
  vrrp: {
    title: 'Virtual routers',
    empty: 'No virtual routers configured',
    name: 'Name',
    interface: 'Interface',
    vrId: 'VRID',
    family: 'Family',
    priority: 'Priority',
    addresses: 'Virtual addresses',
    engine: 'Engine',
    status: 'Status',
    pending: 'Not committed',
    committed: 'Committed',
    disabled: 'Disabled',
  },
  cluster: {
    title: 'Cluster membership',
    notConfigured: 'Clustering is not configured',
    node: 'This node',
    peers: 'Peers',
    port: 'Cluster port',
    configSync: 'Configuration synchronisation',
    stateSync: 'State synchronisation',
    on: 'on',
    off: 'off',
    pending: 'Not committed',
  },
};

export const haFa: typeof haEn = {
  title: 'دسترس‌پذیری بالا',
  intro:
    'مسیریاب‌های مجازی VRRPv3 و عضویت در خوشه. تغییرات در پیکربندی نامزد ذخیره و از نوار تغییرات معلق اعمال می‌شوند.',
  tabs: { vrrp: 'VRRP', cluster: 'خوشه' },
  save: 'ذخیره در نامزد',
  none: '—',
  vrrp: {
    title: 'مسیریاب‌های مجازی',
    empty: 'هیچ مسیریاب مجازی پیکربندی نشده است',
    name: 'نام',
    interface: 'رابط',
    vrId: 'VRID',
    family: 'خانواده',
    priority: 'اولویت',
    addresses: 'نشانی‌های مجازی',
    engine: 'موتور',
    status: 'وضعیت',
    pending: 'اعمال‌نشده',
    committed: 'اعمال‌شده',
    disabled: 'غیرفعال',
  },
  cluster: {
    title: 'عضویت در خوشه',
    notConfigured: 'خوشه پیکربندی نشده است',
    node: 'این گره',
    peers: 'همتاها',
    port: 'درگاه خوشه',
    configSync: 'همگام‌سازی پیکربندی',
    stateSync: 'همگام‌سازی وضعیت',
    on: 'روشن',
    off: 'خاموش',
    pending: 'اعمال‌نشده',
  },
};

let registered = false;
/** Idempotent; called by the page module on import. */
export function registerHaLocale() {
  if (registered) return;
  i18n.addResourceBundle('en', NS, haEn, true, true);
  i18n.addResourceBundle('fa', NS, haFa, true, true);
  registered = true;
}
