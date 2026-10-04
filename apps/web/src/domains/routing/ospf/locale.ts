import i18n from '../../../i18n';

/**
 * WEB-4a: the `routingIgp` namespace (OSPF, IS-IS/RIP, BFD + redistribution screens) lives next to the unrouted
 * screens so nothing shared is touched; F-ospf / F-isis-rip / F-bfd-redistribution may move it to locales/.
 */
export const NS = 'routingIgp';

export const igpEn = {
  save: 'Save to candidate',
  remove: 'Remove from configuration',
  notConfigured: '{{proto}} is not configured — fill in the form to enable it',
  committed: 'Committed',
  pending: 'Not committed',
  none: '—',
  yes: 'yes',
  no: 'no',
  ospf: {
    title: 'OSPF',
    v2: 'OSPFv2',
    v3: 'OSPFv3',
    authBoundary:
      'MD5 requires an approved agent secret resolver. Without one, commit is refused; authentication is never silently disabled.',
    versions: 'OSPF versions',
    neighbors: 'Live neighbors',
    noNeighbors: 'No observed neighbors',
    observationWarning:
      'Observation unavailable, partial or truncated; configuration is not runtime state.',
    vrf: 'VRF',
    routerId: 'Router ID',
    state: 'State',
    intro:
      'Configure the OSPFv2 process, areas and interfaces. Changes are staged in the candidate and committed from the pending-change bar.',
    areas: 'Areas',
    area: 'Area',
    type: 'Type',
    noSummary: 'No summary',
    interfaces: 'Interfaces',
    interface: 'Interface',
    cost: 'Cost',
    passive: 'Passive',
    bfd: 'BFD',
    empty: 'None configured',
  },
  isisRip: {
    title: 'IS-IS and RIP',
    intro:
      'Configure link-state IS-IS and distance-vector RIP routing. Changes are staged in the candidate and committed from the pending-change bar.',
    tabs: { isis: 'IS-IS', rip: 'RIP' },
    interfaces: 'Interfaces',
    interface: 'Interface',
    metric: 'Metric',
    circuit: 'Circuit type',
    passive: 'Passive',
    networks: 'Networks',
    empty: 'None configured',
  },
  bfd: {
    title: 'BFD and redistribution',
    intro:
      'Configure BFD sessions and the routes each dynamic protocol redistributes. Changes are staged in the candidate.',
    tabs: { sessions: 'BFD sessions', redistribution: 'Redistribution' },
    sessions: 'Sessions',
    interface: 'Interface',
    local: 'Local',
    peer: 'Peer',
    timers: 'TX / RX (µs) × mult',
    enabled: 'Enabled',
    empty: 'No BFD sessions configured',
    matrix: 'Redistribution matrix',
    into: 'Into ↓ / from →',
    notRunning: 'not configured',
  },
};

export const igpFa: typeof igpEn = {
  save: 'ذخیره در نامزد',
  remove: 'حذف از پیکربندی',
  notConfigured: '{{proto}} پیکربندی نشده است — برای فعال‌سازی فرم را پر کنید',
  committed: 'اعمال‌شده',
  pending: 'اعمال‌نشده',
  none: '—',
  yes: 'بله',
  no: 'خیر',
  ospf: {
    title: 'OSPF',
    v2: 'OSPFv2',
    v3: 'OSPFv3',
    authBoundary:
      'MD5 نیازمند حل‌کنندهٔ راز مجاز در ایجنت است. بدون آن ثبت تنظیمات رد می‌شود؛ احراز هویت هرگز خودکار غیرفعال نمی‌شود.',
    versions: 'نسخه‌های OSPF',
    neighbors: 'همسایه‌های زنده',
    noNeighbors: 'همسایه‌ای مشاهده نشد',
    observationWarning: 'مشاهده در دسترس نیست، ناقص است یا محدود شده؛ پیکربندی وضعیت زنده نیست.',
    vrf: 'VRF',
    routerId: 'شناسهٔ روتر',
    state: 'وضعیت',
    intro:
      'فرایند OSPFv2، ناحیه‌ها و رابط‌ها را تنظیم کنید. تغییرات در پیکربندی نامزد ذخیره و از نوار تغییرات معلق اعمال می‌شوند.',
    areas: 'ناحیه‌ها',
    area: 'ناحیه',
    type: 'نوع',
    noSummary: 'بدون خلاصه',
    interfaces: 'رابط‌ها',
    interface: 'رابط',
    cost: 'هزینه',
    passive: 'غیرفعال (passive)',
    bfd: 'BFD',
    empty: 'موردی پیکربندی نشده است',
  },
  isisRip: {
    title: 'IS-IS و RIP',
    intro:
      'مسیریابی حالت پیوند IS-IS و بردار فاصله RIP را تنظیم کنید. تغییرات در پیکربندی نامزد ذخیره و از نوار تغییرات معلق اعمال می‌شوند.',
    tabs: { isis: 'IS-IS', rip: 'RIP' },
    interfaces: 'رابط‌ها',
    interface: 'رابط',
    metric: 'متریک',
    circuit: 'نوع مدار',
    passive: 'غیرفعال (passive)',
    networks: 'شبکه‌ها',
    empty: 'موردی پیکربندی نشده است',
  },
  bfd: {
    title: 'BFD و بازتوزیع',
    intro:
      'نشست‌های BFD و مسیرهایی را که هر پروتکل پویا بازتوزیع می‌کند تنظیم کنید. تغییرات در پیکربندی نامزد ذخیره می‌شوند.',
    tabs: { sessions: 'نشست‌های BFD', redistribution: 'بازتوزیع' },
    sessions: 'نشست‌ها',
    interface: 'رابط',
    local: 'محلی',
    peer: 'همتا',
    timers: 'ارسال / دریافت (µs) × ضریب',
    enabled: 'فعال',
    empty: 'هیچ نشست BFD پیکربندی نشده است',
    matrix: 'ماتریس بازتوزیع',
    into: 'به ↓ / از →',
    notRunning: 'پیکربندی نشده',
  },
};

let registered = false;
/** Idempotent; called by each page module on import. */
export function registerIgpLocale() {
  if (registered) return;
  i18n.addResourceBundle('en', NS, igpEn, true, true);
  i18n.addResourceBundle('fa', NS, igpFa, true, true);
  registered = true;
}
