import {
  DEFAULT_API_PORT,
  SmartZoneClient,
  SmartZoneError,
  pickApiVersion,
} from '@/api';
import type { ConnectionProbe, ControllerBootstrapPayload } from './types';

/**
 * Getting connected is the part of a controller app people give up on, so
 * this module takes anything an operator plausibly has to hand and turns it
 * into a connection: a QR code from the deployment runbook, the URL in their
 * browser's address bar, or a hostname typed with a scheme and a path glued
 * on by autocorrect.
 *
 * A bootstrap payload never carries a password. A QR code is photographed,
 * screenshotted and pasted into chat far too easily, and an admin password on
 * a SmartZone cluster is the whole estate. The payload gets the operator to a
 * pre-filled sign-in form; they type the password once and it goes to the
 * Keychain.
 */

/** The scheme the app registers, so a deep link can carry a controller. */
export const BOOTSTRAP_SCHEME = 'szconsole';

/**
 * Parse whatever was scanned, pasted or typed.
 *
 * Accepts:
 *   szconsole://connect?host=sz.example.com&port=8443&user=admin&label=HQ
 *   {"host":"sz.example.com","port":8443,"username":"admin","label":"HQ"}
 *   https://sz.example.com:8443/wsg/api/public/v11_0
 *   sz.example.com:8443
 *   10.1.20.5
 */
export function parseBootstrap(
  input: string,
): ControllerBootstrapPayload | null {
  const text = input.trim();
  if (!text) return null;

  if (text.startsWith('{')) return fromJson(text);
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(text)) return fromUrl(text);
  return fromHostPort(text);
}

function fromJson(text: string): ControllerBootstrapPayload | null {
  try {
    const raw = JSON.parse(text) as Record<string, unknown>;
    const host = str(raw.host ?? raw.hostname ?? raw.address);
    if (!host) return null;
    return clean({
      host,
      port: num(raw.port),
      username: str(raw.username ?? raw.user),
      label: str(raw.label ?? raw.name),
      domainId: str(raw.domainId),
      apiVersion: str(raw.apiVersion),
    });
  } catch {
    return null;
  }
}

function fromUrl(text: string): ControllerBootstrapPayload | null {
  let url: URL;
  try {
    url = new URL(text);
  } catch {
    return null;
  }

  const params = url.searchParams;

  // szconsole://connect?host=... puts the controller in the query, because a
  // custom scheme has no meaningful authority component to rely on.
  const hostParam = params.get('host');
  const host = hostParam || stripBrackets(url.hostname);
  if (!host) return null;

  const port =
    Number(params.get('port')) ||
    (url.port ? Number(url.port) : undefined) ||
    undefined;

  return clean({
    host,
    port,
    username: params.get('user') ?? params.get('username') ?? undefined,
    label: params.get('label') ?? params.get('name') ?? undefined,
    domainId: params.get('domainId') ?? undefined,
    apiVersion: params.get('apiVersion') ?? undefined,
  });
}

function fromHostPort(text: string): ControllerBootstrapPayload | null {
  // Drop any path or query someone pasted along with the address.
  const bare = text.split(/[/?#]/)[0] ?? '';
  if (!bare) return null;

  // Bracketed IPv6, with or without a port.
  const v6 = /^\[([^\]]+)\](?::(\d+))?$/.exec(bare);
  if (v6) {
    return clean({ host: v6[1] ?? '', port: v6[2] ? Number(v6[2]) : undefined });
  }

  // A bare IPv6 literal has colons of its own, so only split a trailing port
  // when there is exactly one colon.
  const colons = (bare.match(/:/g) ?? []).length;
  if (colons === 1) {
    const [host, port] = bare.split(':');
    if (host) return clean({ host, port: port ? Number(port) : undefined });
  }
  return clean({ host: bare });
}

function clean(
  payload: ControllerBootstrapPayload,
): ControllerBootstrapPayload | null {
  const host = payload.host?.trim();
  if (!host || !isPlausibleHost(host)) return null;
  const port =
    payload.port && payload.port > 0 && payload.port <= 65535
      ? payload.port
      : undefined;
  return {
    host,
    ...(port ? { port } : {}),
    ...(payload.username?.trim() ? { username: payload.username.trim() } : {}),
    ...(payload.label?.trim() ? { label: payload.label.trim() } : {}),
    ...(payload.domainId?.trim() ? { domainId: payload.domainId.trim() } : {}),
    ...(payload.apiVersion?.trim() ? { apiVersion: payload.apiVersion.trim() } : {}),
  };
}

function isPlausibleHost(host: string): boolean {
  if (host.length > 253) return false;
  // A hostname, an IPv4 literal, or an IPv6 literal. Deliberately loose: the
  // connection attempt is the real validation, and rejecting a legitimate
  // internal name here would be worse than trying and failing.
  return /^[A-Za-z0-9._:-]+$/.test(host);
}

function stripBrackets(host: string): string {
  return host.replace(/^\[|\]$/g, '');
}

function str(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

function num(value: unknown): number | undefined {
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? n : undefined;
}

/** Build the QR payload for sharing a controller with a colleague. */
export function toBootstrapLink(payload: ControllerBootstrapPayload): string {
  const params = new URLSearchParams();
  params.set('host', payload.host);
  if (payload.port && payload.port !== DEFAULT_API_PORT) {
    params.set('port', String(payload.port));
  }
  if (payload.username) params.set('user', payload.username);
  if (payload.label) params.set('label', payload.label);
  if (payload.domainId) params.set('domainId', payload.domainId);
  return `${BOOTSTRAP_SCHEME}://connect?${params.toString()}`;
}

/**
 * Ask a controller whether it is there and what it speaks, before any
 * credentials are involved.
 *
 * `apiInfo` is unauthenticated, which makes it the right probe: a wrong
 * address, a blocked port and a rejected certificate are all distinguishable
 * here, and each one has a different thing to tell the operator.
 */
export async function probeController(
  host: string,
  port: number = DEFAULT_API_PORT,
  signal?: AbortSignal,
): Promise<ConnectionProbe> {
  const client = new SmartZoneClient({
    endpoint: { host, port },
    credentials: { username: '', password: '' },
    timeoutMs: 12_000,
  });

  try {
    const info = await client.apiInfo(signal);
    const versions = info?.apiSupportVersions ?? [];
    return {
      reachable: true,
      apiSupportVersions: versions,
      negotiatedVersion: pickApiVersion(versions),
    };
  } catch (err) {
    if (err instanceof SmartZoneError) {
      // A 401 or 403 from apiInfo still proves a controller is answering:
      // some builds put the whole /wsg tree behind auth.
      if (err.kind === 'auth' || err.kind === 'forbidden') {
        return { reachable: true, message: 'Reachable; sign in to continue.' };
      }
      return {
        reachable: false,
        certificateRejected: err.kind === 'tls',
        message: err.displayMessage,
      };
    }
    return { reachable: false, message: 'Could not reach the controller.' };
  }
}
