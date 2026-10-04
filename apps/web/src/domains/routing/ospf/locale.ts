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
    authBoundary:
      'RIPv2 MD5 uses a password reference and key ID (1–255). An unavailable key refuses commit. RIPng does not support this authentication.',
    live: {
      title: 'Live adjacencies and peers',
      observed: 'Observed FRR state; VPP forwarding verification is separate.',
      loading: 'Loading observations',
      defaultVrf: 'Peer status currently covers the default VRF only.',
      'frr-unavailable': 'FRR is unavailable',
      'reader-unavailable': 'Protocol reader is unavailable',
      'reader-invalid': 'Protocol response is invalid',
      'reader-limit-exceeded': 'Protocol response exceeds the observation limit',
      empty: 'No observed neighbors or peers',
      truncated: 'Showing the first 100 of {{count}} observations',
      vrf: 'VRF',
      systemId: 'System ID',
      interface: 'Interface',
      level: 'Level',
      state: 'State',
      address: 'Address',
      badPackets: 'Bad packets',
      badRoutes: 'Bad routes',
      distance: 'Distance',
      lastUpdate: 'Last update',
    },
    title: 'IS-IS and RIP',
    intro:
      'Configure link-state IS-IS and distance-vector RIP routing. Changes are staged in the candidate and committed from the pending-change bar.',
    tabs: { isis: 'IS-IS', rip: 'RIP', ripng: 'RIPng' },
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
    authBoundary:
      'برای MD5 در RIPv2 مرجع رمز و شناسه کلید (۱ تا ۲۵۵) وارد کنید. نبود کلید باعث رد کامیت می‌شود. RIPng از این احراز هویت پشتیبانی نمی‌کند.',
    live: {
      title: 'همسایه‌ها و همتایان زنده',
      observed: 'وضعیت مشاهده‌شده FRR؛ بررسی ارسال در VPP جداگانه است.',
      loading: 'بارگذاری مشاهدات',
      defaultVrf: 'وضعیت همتایان فعلاً فقط VRF پیش‌فرض را پوشش می‌دهد.',
      'frr-unavailable': 'FRR در دسترس نیست',
      'reader-unavailable': 'خواننده پروتکل در دسترس نیست',
      'reader-invalid': 'پاسخ پروتکل نامعتبر است',
      'reader-limit-exceeded': 'پاسخ از حد مشاهده فراتر است',
      empty: 'همسایه یا همتایی مشاهده نشده است',
      truncated: 'نمایش ۱۰۰ مورد نخست از {{count}} مشاهده',
      vrf: 'VRF',
      systemId: 'شناسه سیستم',
      interface: 'رابط',
      level: 'سطح',
      state: 'وضعیت',
      address: 'نشانی',
      badPackets: 'بسته نامعتبر',
      badRoutes: 'مسیر نامعتبر',
      distance: 'فاصله',
      lastUpdate: 'آخرین بروزرسانی',
    },
    title: 'IS-IS و RIP',
    intro:
      'مسیریابی حالت پیوند IS-IS و بردار فاصله RIP را تنظیم کنید. تغییرات در پیکربندی نامزد ذخیره و از نوار تغییرات معلق اعمال می‌شوند.',
    tabs: { isis: 'IS-IS', rip: 'RIP', ripng: 'RIPng' },
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
