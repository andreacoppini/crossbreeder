import { SmartZoneError } from './errors';
import { redactUrl, send } from './transport';
import type { HttpMethod } from './transport';
import type {
  ApiInfo,
  ListPage,
  ServiceTicket,
  Session,
  SmartZoneList,
} from './types';

/**
 * API versions this app knows how to talk. The client negotiates downwards
 * from the newest the controller offers, so a 5.2 cluster and a 7.x cluster
 * both work without the operator choosing anything.
 *
 * Ordered oldest to newest; `pickApiVersion` walks it backwards.
 */
export const SUPPORTED_API_VERSIONS = [
  'v9_0',
  'v9_1',
  'v10_0',
  'v11_0',
  'v11_1',
  'v12_0',
  'v13_0',
  'v14_0',
] as const;

export const DEFAULT_API_PORT = 8443;

/** Fallback when a controller will not tell us what it supports. */
export const FALLBACK_API_VERSION = 'v11_0';

export interface ControllerEndpoint {
  /** Hostname or address. No scheme, no port. */
  host: string;
  port?: number;
  /**
   * Pinned API version. Left unset, the client negotiates one and caches it
   * on the session.
   */
  apiVersion?: string;
}

export interface Credentials {
  username: string;
  password: string;
  /** Only needed on a multi-domain (MSP) cluster. */
  domainId?: string;
}

export interface SmartZoneClientOptions {
  endpoint: ControllerEndpoint;
  credentials: Credentials;
  timeoutMs?: number;
  /** Called whenever a new ticket is obtained, so it can be persisted. */
  onSession?: (session: Session) => void;
  /** A ticket recovered from storage, to skip a login on cold start. */
  initialSession?: Session;
}

interface RequestOptions {
  query?: Record<string, string | number | boolean | undefined | null>;
  body?: unknown;
  signal?: AbortSignal;
  timeoutMs?: number;
  /** Set for the login call itself, which must not try to re-authenticate. */
  skipAuth?: boolean;
}

/**
 * Choose the newest version both sides understand.
 *
 * SmartZone reports versions as `v11_0`, which does not sort as a string
 * (`v9_0` > `v11_0` lexically), so they are compared numerically.
 */
export function pickApiVersion(offered: string[] | undefined): string {
  if (!offered || offered.length === 0) return FALLBACK_API_VERSION;
  const known = new Set<string>(SUPPORTED_API_VERSIONS);
  const usable = offered.filter((v) => known.has(v));
  if (usable.length === 0) {
    // The controller is newer or older than anything we were built against.
    // Prefer the newest it offers over failing outright: the endpoints this
    // app uses have been stable across versions.
    const sorted = [...offered].sort(compareApiVersions);
    return sorted[sorted.length - 1] ?? FALLBACK_API_VERSION;
  }
  usable.sort(compareApiVersions);
  return usable[usable.length - 1] ?? FALLBACK_API_VERSION;
}

export function compareApiVersions(a: string, b: string): number {
  const parse = (v: string) => {
    const m = /^v?(\d+)[._](\d+)$/.exec(v.trim());
    return m ? [Number(m[1]), Number(m[2])] : [0, 0];
  };
  const [aMaj = 0, aMin = 0] = parse(a);
  const [bMaj = 0, bMin = 0] = parse(b);
  return aMaj !== bMaj ? aMaj - bMaj : aMin - bMin;
}

/** `https://host:8443` with no trailing slash. */
export function originFor(endpoint: ControllerEndpoint): string {
  const port = endpoint.port ?? DEFAULT_API_PORT;
  const host = endpoint.host.trim().replace(/\/+$/, '');
  // An IPv6 literal has to be bracketed before a port can be appended.
  const bracketed = host.includes(':') && !host.startsWith('[') ? `[${host}]` : host;
  return `https://${bracketed}:${port}`;
}

/**
 * A connection to one SmartZone cluster.
 *
 * Everything about SmartZone's session model is contained here: the ticket
 * travels as a `serviceTicket` query parameter, it expires after 24 hours or
 * whenever the cluster is failed over, and the recovery is always the same —
 * log in again and replay the call once. Concurrent callers that all hit a
 * dead ticket at once share a single re-login rather than stampeding the
 * controller, which will otherwise start rejecting logins.
 */
export class SmartZoneClient {
  readonly endpoint: ControllerEndpoint;
  private credentials: Credentials;
  private readonly timeoutMs: number;
  private readonly onSession?: (session: Session) => void;

  private session: Session | null;
  private loginInFlight: Promise<Session> | null = null;

  constructor(opts: SmartZoneClientOptions) {
    this.endpoint = opts.endpoint;
    this.credentials = opts.credentials;
    this.timeoutMs = opts.timeoutMs ?? 20_000;
    this.onSession = opts.onSession;
    this.session = opts.initialSession ?? null;
  }

  get origin(): string {
    return originFor(this.endpoint);
  }

  get currentSession(): Session | null {
    return this.session;
  }

  /** Replace the stored credentials, e.g. after the operator re-types them. */
  setCredentials(credentials: Credentials) {
    this.credentials = credentials;
    this.session = null;
  }

  /**
   * Ask the controller which API versions it speaks. Unauthenticated, so it
   * doubles as the reachability probe during onboarding.
   */
  async apiInfo(signal?: AbortSignal): Promise<ApiInfo> {
    const url = `${this.origin}/wsg/api/public/apiInfo`;
    const res = await send<ApiInfo>({
      url,
      method: 'GET',
      timeoutMs: this.timeoutMs,
      signal,
      label: 'GET /wsg/api/public/apiInfo',
    });
    return res.data;
  }

  /** The negotiated version, resolving it against the controller if needed. */
  async resolveApiVersion(signal?: AbortSignal): Promise<string> {
    if (this.endpoint.apiVersion) return this.endpoint.apiVersion;
    if (this.session?.apiVersion) return this.session.apiVersion;
    try {
      const info = await this.apiInfo(signal);
      return pickApiVersion(info.apiSupportVersions);
    } catch (err) {
      // A controller with the public API disabled, or an appliance behind a
      // proxy that hides apiInfo, still answers /serviceTicket on a known
      // version. Fall back rather than block onboarding here.
      if (err instanceof SmartZoneError && err.kind === 'notFound') {
        return FALLBACK_API_VERSION;
      }
      throw err;
    }
  }

  private base(apiVersion: string): string {
    return `${this.origin}/wsg/api/public/${apiVersion}`;
  }

  /** Log in and cache the ticket. Safe to call concurrently. */
  async login(signal?: AbortSignal): Promise<Session> {
    if (this.loginInFlight) return this.loginInFlight;

    this.loginInFlight = (async () => {
      const apiVersion = await this.resolveApiVersion(signal);
      const url = `${this.base(apiVersion)}/serviceTicket`;
      const res = await send<ServiceTicket>({
        url,
        method: 'POST',
        body: {
          username: this.credentials.username,
          password: this.credentials.password,
          ...(this.credentials.domainId
            ? { domainId: this.credentials.domainId }
            : {}),
        },
        timeoutMs: this.timeoutMs,
        signal,
        label: 'POST /serviceTicket',
      });

      if (!res.data?.serviceTicket) {
        throw new SmartZoneError('auth', 'The controller issued no service ticket.', {
          status: res.status,
          request: 'POST /serviceTicket',
        });
      }

      const session: Session = {
        serviceTicket: res.data.serviceTicket,
        controllerVersion: res.data.controllerVersion,
        apiVersion,
        issuedAt: Date.now(),
      };
      this.session = session;
      this.onSession?.(session);
      return session;
    })();

    try {
      return await this.loginInFlight;
    } finally {
      this.loginInFlight = null;
    }
  }

  /** Release the ticket. Failures are ignored: the ticket expires anyway. */
  async logout(): Promise<void> {
    const session = this.session;
    this.session = null;
    if (!session) return;
    try {
      await send({
        url: `${this.base(session.apiVersion)}/serviceTicket?serviceTicket=${encodeURIComponent(
          session.serviceTicket,
        )}`,
        method: 'DELETE',
        timeoutMs: 5_000,
        label: 'DELETE /serviceTicket',
      });
    } catch {
      // Nothing useful to do; the operator has already been signed out here.
    }
  }

  private async ensureSession(signal?: AbortSignal): Promise<Session> {
    if (this.session) return this.session;
    return this.login(signal);
  }

  /**
   * Issue an authenticated request, replaying it once if the ticket turns
   * out to be dead.
   */
  async request<T>(
    method: HttpMethod,
    path: string,
    opts: RequestOptions = {},
  ): Promise<T> {
    if (opts.skipAuth) {
      const apiVersion = await this.resolveApiVersion(opts.signal);
      return this.dispatch<T>(method, path, apiVersion, undefined, opts);
    }

    const session = await this.ensureSession(opts.signal);
    try {
      return await this.dispatch<T>(
        method,
        path,
        session.apiVersion,
        session.serviceTicket,
        opts,
      );
    } catch (err) {
      if (!(err instanceof SmartZoneError) || err.kind !== 'auth') throw err;
      // The ticket expired, or the cluster failed over under us.
      if (this.session === session) this.session = null;
      const fresh = await this.login(opts.signal);
      return this.dispatch<T>(
        method,
        path,
        fresh.apiVersion,
        fresh.serviceTicket,
        opts,
      );
    }
  }

  private async dispatch<T>(
    method: HttpMethod,
    path: string,
    apiVersion: string,
    ticket: string | undefined,
    opts: RequestOptions,
  ): Promise<T> {
    const url = buildUrl(this.base(apiVersion), path, opts.query, ticket);
    const res = await send<T>({
      url,
      method,
      body: opts.body,
      timeoutMs: opts.timeoutMs ?? this.timeoutMs,
      signal: opts.signal,
      label: `${method} ${redactUrl(path)}`,
    });
    return res.data;
  }

  get<T>(path: string, opts: RequestOptions = {}) {
    return this.request<T>('GET', path, opts);
  }
  post<T>(path: string, body?: unknown, opts: RequestOptions = {}) {
    return this.request<T>('POST', path, { ...opts, body });
  }
  put<T>(path: string, body?: unknown, opts: RequestOptions = {}) {
    return this.request<T>('PUT', path, { ...opts, body });
  }
  patch<T>(path: string, body?: unknown, opts: RequestOptions = {}) {
    return this.request<T>('PATCH', path, { ...opts, body });
  }
  delete<T>(path: string, opts: RequestOptions = {}) {
    return this.request<T>('DELETE', path, opts);
  }

  /** One page of a `GET` collection endpoint. */
  list<T>(
    path: string,
    page: ListPage = {},
    opts: RequestOptions = {},
  ): Promise<SmartZoneList<T>> {
    return this.get<SmartZoneList<T>>(path, {
      ...opts,
      query: {
        ...opts.query,
        index: page.index ?? 0,
        listSize: page.listSize ?? 100,
      },
    });
  }

  /**
   * Every page of a `GET` collection endpoint.
   *
   * `maxPages` is a guard, not a preference: a zone with tens of thousands of
   * DPSKs would otherwise pull the lot onto a phone.
   */
  async listAll<T>(
    path: string,
    opts: RequestOptions & { pageSize?: number; maxPages?: number } = {},
  ): Promise<T[]> {
    const pageSize = opts.pageSize ?? 250;
    const maxPages = opts.maxPages ?? 20;
    const out: T[] = [];
    for (let page = 0; page < maxPages; page += 1) {
      const res = await this.list<T>(
        path,
        { index: page * pageSize, listSize: pageSize },
        opts,
      );
      out.push(...(res.list ?? []));
      if (!res.hasMore || (res.list?.length ?? 0) === 0) break;
    }
    return out;
  }
}

/**
 * Assemble a request URL. The ticket is appended last so that a `path` which
 * already carries query parameters still works.
 */
export function buildUrl(
  base: string,
  path: string,
  query?: Record<string, string | number | boolean | undefined | null>,
  ticket?: string,
): string {
  const parts: string[] = [];
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null) continue;
      parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`);
    }
  }
  if (ticket) parts.push(`serviceTicket=${encodeURIComponent(ticket)}`);

  const sep = path.includes('?') ? '&' : '?';
  const suffix = parts.length > 0 ? `${sep}${parts.join('&')}` : '';
  return `${base}${path.startsWith('/') ? path : `/${path}`}${suffix}`;
}

/** Interpolate `{name}` placeholders, escaping each value. */
export function withPath(
  template: string,
  params: Record<string, string | number>,
): string {
  return template.replace(/\{(\w+)\}/g, (_, key: string) => {
    const value = params[key];
    if (value === undefined) {
      throw new Error(`Missing path parameter "${key}" for ${template}`);
    }
    return encodeURIComponent(String(value));
  });
}
