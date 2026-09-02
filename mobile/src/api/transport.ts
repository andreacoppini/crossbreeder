import { SmartZoneError, classifyTransportError, kindForStatus } from './errors';
import type { SmartZoneApiFault } from './errors';

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

export interface TransportRequest {
  url: string;
  method: HttpMethod;
  body?: unknown;
  headers?: Record<string, string>;
  /** Milliseconds before the request is aborted. */
  timeoutMs?: number;
  /** Caller-supplied cancellation, composed with the timeout. */
  signal?: AbortSignal;
  /** Label used in errors and logs. Never include the service ticket. */
  label?: string;
}

export interface TransportResponse<T> {
  status: number;
  data: T;
  headers: Headers;
}

/** Strip the service ticket out of a URL so it is safe to log or display. */
export function redactUrl(url: string): string {
  return url.replace(/([?&]serviceTicket=)[^&]*/gi, '$1<redacted>');
}

/**
 * `AbortSignal.any` is not available on every React Native engine yet, so
 * the timeout and the caller's signal are composed by hand.
 */
function withTimeout(timeoutMs: number | undefined, outer?: AbortSignal) {
  const controller = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let timedOut = false;

  if (timeoutMs && timeoutMs > 0) {
    timer = setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, timeoutMs);
  }

  const onOuterAbort = () => controller.abort();
  if (outer) {
    if (outer.aborted) controller.abort();
    else outer.addEventListener('abort', onOuterAbort);
  }

  return {
    signal: controller.signal,
    didTimeOut: () => timedOut,
    dispose() {
      if (timer) clearTimeout(timer);
      outer?.removeEventListener('abort', onOuterAbort);
    },
  };
}

/**
 * One HTTP round trip against the controller, with SmartZone's response
 * conventions applied: a 204 or an empty body becomes `undefined`, and a
 * non-2xx becomes a SmartZoneError carrying the controller's own fault
 * message where it sent one.
 */
export async function send<T>(
  req: TransportRequest,
): Promise<TransportResponse<T>> {
  const label = req.label ?? `${req.method} ${redactUrl(req.url)}`;
  const timeout = withTimeout(req.timeoutMs ?? 20_000, req.signal);

  let res: Response;
  try {
    res = await fetch(req.url, {
      method: req.method,
      signal: timeout.signal,
      headers: {
        Accept: 'application/json',
        ...(req.body !== undefined
          ? { 'Content-Type': 'application/json;charset=UTF-8' }
          : {}),
        ...req.headers,
      },
      body: req.body !== undefined ? JSON.stringify(req.body) : undefined,
    });
  } catch (err) {
    if (timeout.didTimeOut()) {
      throw new SmartZoneError('timeout', 'The controller did not answer in time.', {
        request: label,
        cause: err,
      });
    }
    throw classifyTransportError(err, label);
  } finally {
    timeout.dispose();
  }

  const text = await res.text().catch(() => '');

  let parsed: unknown;
  if (text.length > 0) {
    try {
      parsed = JSON.parse(text);
    } catch {
      // SmartZone answers a few endpoints (CSV downloads, support logs) with
      // something other than JSON, and returns HTML from the web server when
      // the API path is wrong. Keep the raw text; the caller decides.
      parsed = text;
    }
  }

  if (!res.ok) {
    const fault = extractFault(parsed);
    throw new SmartZoneError(
      kindForStatus(res.status),
      fault?.message ?? `${req.method} failed with ${res.status}`,
      { status: res.status, fault, request: label },
    );
  }

  return { status: res.status, data: parsed as T, headers: res.headers };
}

/**
 * SmartZone is not consistent about its fault envelope: some endpoints send
 * `{message}`, some `{errors:[{message}]}`, some a bare string.
 */
function extractFault(body: unknown): SmartZoneApiFault | undefined {
  if (!body) return undefined;
  if (typeof body === 'string') {
    // An HTML error page is noise, not a message worth showing.
    if (/^\s*</.test(body)) return undefined;
    return { message: body.slice(0, 400) };
  }
  if (typeof body !== 'object') return undefined;

  const obj = body as Record<string, unknown>;

  const errors = obj.errors;
  if (Array.isArray(errors) && errors.length > 0) {
    const first = errors[0] as Record<string, unknown>;
    return {
      code: first.code as string | number | undefined,
      message: (first.message as string | undefined) ?? undefined,
      errorType: first.errorType as string | undefined,
      errorCode: first.errorCode as number | undefined,
    };
  }

  if (typeof obj.message === 'string' || obj.errorCode !== undefined) {
    return {
      code: obj.code as string | number | undefined,
      message: obj.message as string | undefined,
      errorType: obj.errorType as string | undefined,
      errorCode: obj.errorCode as number | undefined,
    };
  }
  return undefined;
}
