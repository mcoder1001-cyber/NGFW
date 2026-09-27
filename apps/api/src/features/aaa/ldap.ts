import { Client, Filter, InvalidCredentialsError } from 'ldapts';

/**
 * F-aaa-login: LDAP authentication (bind + search, `ldapts`). The box binds as a read-only service DN, searches the
 * user under `baseDn` with `userFilter` (the login name is RFC 4515-escaped into `%s`), requires exactly ONE entry,
 * then binds as that entry's DN with the user's password. Groups come from `groupAttr` (e.g. memberOf DNs).
 *
 * Security: never a clear-text bind — `ldaps://` or StartTLS (the schema's semantic rule refuses the config, and this
 * module refuses again as defence in depth); an empty password is refused before any network I/O (an LDAP "simple
 * bind" with an empty password is an unauthenticated bind that many servers accept, RFC 4513 §5.1.2). The bind
 * password is passed in by the caller (resolved from the secret store at use time) and never logged.
 */

export interface LdapServer {
  url: string;
  bindDn: string;
  bindPassword: string;
  baseDn: string;
  userFilter: string;
  groupAttr: string;
  startTls: boolean;
  timeoutMs: number;
}

export type LdapResult =
  | { status: 'accept'; groups: string[]; dn: string }
  | { status: 'reject'; message: string }
  | { status: 'unreachable'; error: string };

/** The subset of the ldapts Client used here (a seam for unit tests). */
export interface LdapClientLike {
  startTLS(options?: object): Promise<void>;
  bind(dn: string, password?: string): Promise<void>;
  search(
    base: string,
    options: { scope: 'sub'; filter: string; attributes: string[]; sizeLimit: number },
  ): Promise<{ searchEntries: { dn: string; [k: string]: unknown }[] }>;
  unbind(): Promise<void>;
}

export type LdapClientFactory = (url: string, timeoutMs: number) => LdapClientLike;

const defaultFactory: LdapClientFactory = (url, timeoutMs) =>
  new Client({ url, timeout: timeoutMs, connectTimeout: timeoutMs, strictDN: true });

/** `userFilter` with every `%s` replaced by the escaped login name (RFC 4515) — no filter injection. */
export function userFilterFor(template: string, username: string): string {
  const esc = Filter.escape(username);
  return template.split('%s').join(esc);
}

function values(v: unknown): string[] {
  if (v === undefined || v === null) return [];
  const arr = Array.isArray(v) ? v : [v];
  return arr.map((x) => (Buffer.isBuffer(x) ? x.toString('utf8') : String(x)));
}

export async function ldapAuthenticate(
  server: LdapServer,
  username: string,
  password: string,
  factory: LdapClientFactory = defaultFactory,
): Promise<LdapResult> {
  if (password.length === 0) return { status: 'reject', message: 'empty password' };
  if (!server.url.startsWith('ldaps://') && !server.startTls) {
    return {
      status: 'unreachable',
      error: 'clear-text LDAP bind refused (use ldaps:// or StartTLS)',
    };
  }
  const client = factory(server.url, server.timeoutMs);
  // service phase: any failure here is the server's or the configuration's, never the user's → unreachable
  let dn: string;
  let groups: string[];
  try {
    if (server.startTls && server.url.startsWith('ldap://')) await client.startTLS();
    await client.bind(server.bindDn, server.bindPassword);
    const { searchEntries } = await client.search(server.baseDn, {
      scope: 'sub',
      filter: userFilterFor(server.userFilter, username),
      attributes: [server.groupAttr],
      sizeLimit: 2,
    });
    if (searchEntries.length !== 1) {
      await client.unbind().catch(() => undefined);
      return {
        status: 'reject',
        message: searchEntries.length === 0 ? 'user not found' : 'user filter is ambiguous',
      };
    }
    const entry = searchEntries[0]!;
    dn = entry.dn;
    groups = values(entry[server.groupAttr]);
  } catch (e) {
    await client.unbind().catch(() => undefined);
    const why =
      e instanceof InvalidCredentialsError ? 'service bind refused' : (e as Error).message;
    return { status: 'unreachable', error: `ldap: ${why}` };
  }
  // user phase: the user's own bind
  try {
    await client.bind(dn, password);
    return { status: 'accept', groups, dn };
  } catch (e) {
    if (e instanceof InvalidCredentialsError)
      return { status: 'reject', message: 'invalid credentials' };
    return { status: 'unreachable', error: `ldap: ${(e as Error).message}` };
  } finally {
    await client.unbind().catch(() => undefined);
  }
}
