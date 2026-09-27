import { type ArgumentsHost, Catch, type ExceptionFilter, HttpException, Logger } from '@nestjs/common';
import type { FastifyReply } from 'fastify';
import { PROBLEM_TYPE_BASE, ProblemError, type ProblemIssue } from '../../common/problem.js';

/**
 * F-restconf-yang: render errors as RFC 8040 §7 `ietf-restconf:errors` instead of problem+json, so a RESTCONF client
 * gets the encoding it expects. The document's JSON pointers become `error-path`. Applied only to the RESTCONF
 * controller (`@UseFilters`), so the rest of the API keeps problem+json.
 */

const ERROR_TAG_BY_STATUS: Record<number, { tag: string; type: string }> = {
  400: { tag: 'invalid-value', type: 'application' },
  401: { tag: 'access-denied', type: 'protocol' },
  403: { tag: 'access-denied', type: 'protocol' },
  404: { tag: 'invalid-value', type: 'application' },
  405: { tag: 'operation-not-supported', type: 'protocol' },
  409: { tag: 'in-use', type: 'application' },
  415: { tag: 'invalid-value', type: 'protocol' },
  422: { tag: 'operation-failed', type: 'application' },
  501: { tag: 'operation-not-supported', type: 'protocol' },
  502: { tag: 'operation-failed', type: 'application' },
  503: { tag: 'operation-failed', type: 'application' },
};

interface RestconfError {
  'error-type': string;
  'error-tag': string;
  'error-path'?: string;
  'error-message': string;
}

function errorsFor(status: number, message: string, issues?: readonly ProblemIssue[]): RestconfError[] {
  const meta = ERROR_TAG_BY_STATUS[status] ?? { tag: 'operation-failed', type: 'application' };
  if (issues && issues.length > 0) {
    return issues.map((i) => ({
      'error-type': meta.type,
      'error-tag': meta.tag,
      'error-path': i.pointer,
      'error-message': i.message,
    }));
  }
  return [{ 'error-type': meta.type, 'error-tag': meta.tag, 'error-message': message }];
}

export function restconfErrorBody(
  status: number,
  message: string,
  issues?: readonly ProblemIssue[],
): { 'ietf-restconf:errors': { error: RestconfError[] } } {
  return { 'ietf-restconf:errors': { error: errorsFor(status, message, issues) } };
}

@Catch()
export class RestconfExceptionFilter implements ExceptionFilter {
  private readonly log = new Logger('restconf');

  catch(exception: unknown, host: ArgumentsHost): void {
    const reply = host.switchToHttp().getResponse<FastifyReply>();
    let status = 500;
    let message = 'internal error';
    let issues: readonly ProblemIssue[] | undefined;

    if (exception instanceof ProblemError) {
      status = exception.getStatus();
      message = exception.detail ?? exception.title;
      issues = exception.errors;
    } else if (exception instanceof HttpException) {
      status = exception.getStatus();
      const resp = exception.getResponse();
      message = typeof resp === 'string' ? resp : ((resp as { message?: string }).message ?? exception.message);
    } else {
      this.log.error(`unexpected RESTCONF error: ${String(exception)}`);
    }
    // Authentication/authorization stays problem+json, consistent with the rest of the API and the route guard —
    // the guard rejects before the handler and its answer must not depend on the endpoint's content type.
    if (status === 401 || status === 403) {
      const body =
        exception instanceof ProblemError
          ? exception.body()
          : {
              type: `${PROBLEM_TYPE_BASE}${status === 401 ? 'unauthorized' : 'forbidden'}`,
              title: status === 401 ? 'Unauthorized' : 'Forbidden',
              status,
              detail: message,
            };
      void reply.status(status).header('content-type', 'application/problem+json').send(body);
      return;
    }
    void reply
      .status(status)
      .header('content-type', 'application/yang-data+json')
      .send(restconfErrorBody(status, message, issues));
  }
}
