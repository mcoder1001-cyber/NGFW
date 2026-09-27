/**
 * F-dashboard-prom-alarms: POST an alarm as JSON to a webhook target, with a timeout and one retry. The bearer token
 * (from a `token/<name>` secret) is sent in the Authorization header and never logged. Only https/http URLs; the URL
 * is the admin-configured target (validated by the schema), so no redirect handling is needed beyond fetch's default.
 */
export interface DeliverOptions {
  token?: string;
  timeoutMs: number;
  retries?: number;
}

export class WebhookError extends Error {}

export async function deliver(url: string, body: unknown, opts: DeliverOptions): Promise<void> {
  const retries = opts.retries ?? 1;
  let lastErr: unknown;
  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      await postOnce(url, body, opts);
      return;
    } catch (e) {
      lastErr = e;
    }
  }
  throw lastErr instanceof Error ? lastErr : new WebhookError(String(lastErr));
}

async function postOnce(url: string, body: unknown, opts: DeliverOptions): Promise<void> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), opts.timeoutMs);
  try {
    const headers: Record<string, string> = { 'content-type': 'application/json' };
    if (opts.token) headers['authorization'] = `Bearer ${opts.token.trim()}`;
    const res = await fetch(url, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
      signal: controller.signal,
    });
    if (!res.ok) throw new WebhookError(`HTTP ${res.status}`);
  } catch (e) {
    throw e instanceof Error ? e : new WebhookError(String(e));
  } finally {
    clearTimeout(timer);
  }
}
