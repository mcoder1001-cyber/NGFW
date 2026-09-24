import { useQueryClient } from '@tanstack/react-query';
import { useWsClient } from '@ngfw/ui-kit/ws';
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { session as appSession, type Role, type Session, type SessionState } from './session';

interface AuthValue {
  session: Session;
  state: SessionState;
}

const AuthContext = createContext<AuthValue | null>(null);

/**
 * Exposes the session to React. On sign-out it drops every cached server response (the next user must not see the
 * previous user's data) and closes the shared WebSocket (its credential belonged to the old session).
 */
export function AuthProvider({ session = appSession, children }: { session?: Session; children: ReactNode }) {
  const [state, setState] = useState<SessionState>(session.state);
  const queryClient = useQueryClient();
  const ws = useWsClient();
  useEffect(() => {
    const off = session.subscribe((s) => {
      setState(s);
      if (s.status === 'anonymous') {
        queryClient.clear();
        ws.close();
      } else if (s.status === 'authenticated' && ws.topics.length > 0) {
        ws.connect(); // subscribers that mounted before the credential existed (the client stayed idle)
      }
    });
    if (session.state.status === 'unknown') void session.restore();
    else setState(session.state);
    return off;
  }, [session, queryClient, ws]);
  const value = useMemo(() => ({ session, state }), [session, state]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthValue {
  const v = useContext(AuthContext);
  if (!v) throw new Error('useAuth must be used inside <AuthProvider>');
  return v;
}

const RANK: Record<Role, number> = { readonly: 0, operator: 1, admin: 2 };

/**
 * What the signed-in user may do, mirroring P06's RBAC (D-091): readonly = read only; operator = edit, commit, confirm,
 * discard, rollback — but not users/AAA/secret references; admin = everything. The UI disables what the role cannot
 * do; the API remains the authority (a 403 is still shown if it disagrees).
 */
export interface Permissions {
  role: Role | null;
  editConfig: boolean;
  commit: boolean;
  manageUsers: boolean;
  breakLock: boolean;
}

export function permissionsFor(role: Role | null): Permissions {
  const r = role === null ? -1 : RANK[role];
  return { role, editConfig: r >= 1, commit: r >= 1, manageUsers: r >= 2, breakLock: r >= 2 };
}

export function usePermissions(): Permissions {
  const { state } = useAuth();
  return useMemo(() => permissionsFor(state.user?.role ?? null), [state.user?.role]);
}
