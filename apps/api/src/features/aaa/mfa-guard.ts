import { Injectable } from '@nestjs/common';
import { ModuleRef } from '@nestjs/core';
import { problems } from '../../common/problem.js';
import type { Principal } from '../../common/principal.js';
import type { MfaRequired } from './aaa.service.js';
import { MfaService } from './mfa.service.js';

type Doc = Record<string, unknown>;

export const MFA_REQUIRED_POINTER = '/management/aaa/mfa/required';

const RANK: Record<MfaRequired, number> = { none: 0, admins: 1, all: 2 };

function obj(v: unknown): Doc {
  return typeof v === 'object' && v !== null && !Array.isArray(v) ? (v as Doc) : {};
}

/** `management.aaa.mfa.required` of a document (absent/unknown = `none`, as the login policy reads it). */
export function mfaRequiredOf(doc: Doc): MfaRequired {
  const req = obj(obj(obj(doc['management'])['aaa'])['mfa'])['required'];
  return req === 'admins' || req === 'all' ? req : 'none';
}

/** The policy is RAISED (covers more roles) from running to the document; lowering or keeping it is not. */
export function mfaRaised(running: Doc, doc: Doc): { from: MfaRequired; to: MfaRequired } | null {
  const from = mfaRequiredOf(running);
  const to = mfaRequiredOf(doc);
  return RANK[to] > RANK[from] ? { from, to } : null;
}

export interface MfaGuardDeps {
  /** The committer's own login session passed the second factor. */
  sessionPassedMfa(user: Principal): Promise<boolean>;
  /** At least one enabled admin has an active factor. */
  anyAdminEnrolled(): Promise<boolean>;
}

/**
 * F-aaa-mfa-lockout: raising `mfa.required` while no admin has a factor would end every admin session, and only an
 * admin session can issue enrolment tokens (D-159) — the console tool would be the only way back. A commit that
 * raises the policy is refused (400, pointer /management/aaa/mfa/required) unless the committer's session passed MFA
 * and at least one admin has an active factor. Lowering is never blocked.
 */
export async function assertMfaRaiseSafe(
  deps: MfaGuardDeps,
  user: Principal,
  running: Doc,
  doc: Doc,
): Promise<void> {
  const raise = mfaRaised(running, doc);
  if (raise === null) return;
  const reasons: string[] = [];
  if (!(await deps.anyAdminEnrolled())) reasons.push('no admin account has an active second factor');
  if (!(await deps.sessionPassedMfa(user)))
    reasons.push('your own login session has not passed the second factor');
  if (reasons.length === 0) return;
  const detail =
    `raising management.aaa.mfa.required from '${raise.from}' to '${raise.to}' would lock the admins out ` +
    `(${reasons.join('; ')}). Enrol an admin first: issue yourself an enrolment token (Users page), set up your ` +
    `second factor (My second factor), sign in again with the code, then commit`;
  throw problems.validation([{ pointer: MFA_REQUIRED_POINTER, message: detail }], detail, {
    tier: 'api',
  });
}

/** Nest wiring of the guard (AuthService resolved lazily: it depends on this feature). */
@Injectable()
export class MfaCommitGuard {
  constructor(
    private readonly mfa: MfaService,
    private readonly moduleRef: ModuleRef,
  ) {}

  async assert(user: Principal, running: Doc, doc: Doc): Promise<void> {
    if (mfaRaised(running, doc) === null) return;
    const { AuthService } = await import('../../auth/auth.service.js');
    const auth = this.moduleRef.get(AuthService, { strict: false });
    await assertMfaRaiseSafe(
      {
        sessionPassedMfa: (u) => auth.sessionPassedMfa(u),
        anyAdminEnrolled: () => this.mfa.anyAdminEnrolled(),
      },
      user,
      running,
      doc,
    );
  }
}
