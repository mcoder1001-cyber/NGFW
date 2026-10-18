import { Navigate, useLocation } from 'react-router';
import { domainPath } from '../../nav/nav';

/**
 * `/firewall/auto-block` → the Blocked sources tab of the Security page (F-security-page). The query string and hash of
 * the old address are kept (R1: a fixed target dropped them); only `tab` is forced to `blocked`.
 */
export function AutoBlockRedirect() {
  const { search, hash } = useLocation();
  const params = new URLSearchParams(search);
  params.set('tab', 'blocked');
  return <Navigate to={{ pathname: domainPath('security'), search: `?${params.toString()}`, hash }} replace />;
}
