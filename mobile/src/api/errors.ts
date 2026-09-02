/**
 * Error taxonomy for SmartZone calls.
 *
 * The point of this file is that every failure the app can show a human
 * reaches the UI as one of these, with a `kind` the UI can switch on. A
 * screen should never have to parse a message string to decide whether to
 * offer "sign in again", "trust this certificate" or "retry".
 */

export type SmartZoneErrorKind =
  | 'network' // the controller was not reachable at all
  | 'tls' // reachable, but the certificate was rejected
  | 'timeout'
  | 'auth' // 401: no ticket, or the ticket expired and re-login failed
  | 'forbidden' // 403: authenticated, but this admin lacks the privilege
  | 'notFound'
  | 'conflict' // 422 business-rule violation, the controller's usual "no"
  | 'server' // 5xx
  | 'parse' // a 2xx whose body was not the JSON we expected
  | 'cancelled'
  | 'unknown';

/** The error envelope SmartZone returns on a 4xx/5xx. */
export interface SmartZoneApiFault {
  code?: string | number;
  message?: string;
  /** Present on validation failures; names the field the controller rejected. */
  errorType?: string;
  errorCode?: number;
}

export class SmartZoneError extends Error {
  readonly kind: SmartZoneErrorKind;
  readonly status?: number;
  /** The parsed body of the failing response, when there was one. */
  readonly fault?: SmartZoneApiFault;
  /** Method and path, for logs and bug reports. Never carries the ticket. */
  readonly request?: string;
  readonly cause?: unknown;

  constructor(
    kind: SmartZoneErrorKind,
    message: string,
    opts: {
      status?: number;
      fault?: SmartZoneApiFault;
      request?: string;
      cause?: unknown;
    } = {},
  ) {
    super(message);
    this.name = 'SmartZoneError';
    this.kind = kind;
    this.status = opts.status;
    this.fault = opts.fault;
    this.request = opts.request;
    this.cause = opts.cause;
  }

  /** True when retrying the same call unchanged could plausibly succeed. */
  get retryable(): boolean {
    return (
      this.kind === 'network' ||
      this.kind === 'timeout' ||
      this.kind === 'server'
    );
  }

  /**
   * A sentence to put in front of an operator. The controller's own message
   * is preferred when it has one, because SmartZone's validation text is
   * usually more specific than anything we could write.
   */
  get displayMessage(): string {
    if (this.fault?.message) return this.fault.message;
    switch (this.kind) {
      case 'network':
        return 'Could not reach the controller. Check the address, the port, and whether you are on a network that can see it.';
      case 'tls':
        return 'The controller’s certificate was rejected. SmartZone ships with a self-signed certificate; see Settings → Certificates.';
      case 'timeout':
        return 'The controller did not answer in time.';
      case 'auth':
        return 'Sign-in failed. The username, password, or the ticket has expired.';
      case 'forbidden':
        return 'This administrator account is not allowed to do that.';
      case 'notFound':
        return 'That object no longer exists on the controller.';
      case 'conflict':
        return 'The controller refused the change.';
      case 'server':
        return 'The controller reported an internal error.';
      case 'parse':
        return 'The controller sent a response this app could not read.';
      case 'cancelled':
        return 'Cancelled.';
      default:
        return this.message || 'Something went wrong.';
    }
  }
}

/** Map an HTTP status onto the kind the UI switches on. */
export function kindForStatus(status: number): SmartZoneErrorKind {
  if (status === 401) return 'auth';
  if (status === 403) return 'forbidden';
  if (status === 404) return 'notFound';
  if (status === 422 || status === 400 || status === 409) return 'conflict';
  if (status >= 500) return 'server';
  return 'unknown';
}

/**
 * Classify a thrown fetch failure. React Native's networking layer collapses
 * nearly everything into `TypeError: Network request failed`, so the message
 * is all we have to separate "no route to host" from "certificate rejected" —
 * and the certificate case is by far the most common first-run problem with
 * SmartZone, which ships self-signed.
 */
export function classifyTransportError(
  err: unknown,
  request?: string,
): SmartZoneError {
  if (err instanceof SmartZoneError) return err;

  const name = (err as { name?: string })?.name ?? '';
  const message = String((err as { message?: string })?.message ?? err ?? '');

  if (name === 'AbortError') {
    return new SmartZoneError('cancelled', 'Request cancelled', {
      request,
      cause: err,
    });
  }

  const tlsMarkers = [
    'certificate',
    'ssl',
    'trust',
    'cert_',
    'ERR_CERT',
    'CertPathValidator',
    'self signed',
    'self-signed',
    'unable to verify',
    'hostname mismatch',
  ];
  if (tlsMarkers.some((m) => message.toLowerCase().includes(m.toLowerCase()))) {
    return new SmartZoneError('tls', message, { request, cause: err });
  }

  return new SmartZoneError(
    'network',
    message || 'Network request failed',
    { request, cause: err },
  );
}
