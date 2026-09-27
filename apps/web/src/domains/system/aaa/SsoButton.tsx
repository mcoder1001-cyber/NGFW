import Button from '@mui/material/Button';
import Divider from '@mui/material/Divider';
import { useTranslation } from 'react-i18next';
import { useLoginMethods } from './queries';

/** F-aaa-login: "Sign in with single sign-on" on the login page, only when OpenID Connect is configured. */
export function SsoButton() {
  const { t } = useTranslation('aaa');
  const m = useLoginMethods();
  if (m.data?.oidc !== true) return null;
  return (
    <>
      <Divider>{t('sso.or')}</Divider>
      <Button variant="outlined" href="/api/v1/auth/oidc/start" data-testid="sso-button">
        {t('sso.button')}
      </Button>
    </>
  );
}

/** The localized text of an OIDC handover error slug (unknown slugs fall back to the generic one). */
export function ssoErrorKey(slug: string): string {
  const known = [
    'no-role-mapping',
    'rate-limited',
    'oidc-not-configured',
    'oidc-unavailable',
    'oidc-failed',
  ];
  return `sso.error.${known.includes(slug) ? slug : 'oidc-failed'}`;
}
