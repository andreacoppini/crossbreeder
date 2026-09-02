/** Formatting shared across screens, so the same value never reads two ways. */

/** Bytes into something a person reads, using the units network people use. */
export function formatBytes(bytes?: number): string {
  if (bytes == null || !Number.isFinite(bytes)) return '—';
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 100 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

/** Bits per second, for link and negotiated rates. */
export function formatRate(mbps?: number): string {
  if (mbps == null || !Number.isFinite(mbps)) return '—';
  if (mbps >= 1000) return `${(mbps / 1000).toFixed(1)} Gbps`;
  return `${mbps >= 10 ? Math.round(mbps) : mbps.toFixed(1)} Mbps`;
}

/**
 * A duration in seconds as an uptime reads on a device: the two largest
 * units, never more. "4d 6h", not "4d 6h 12m 5s".
 */
export function formatDuration(seconds?: number): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return '—';
  const s = Math.floor(seconds);
  if (s < 60) return `${s}s`;

  const days = Math.floor(s / 86400);
  const hours = Math.floor((s % 86400) / 3600);
  const minutes = Math.floor((s % 3600) / 60);

  if (days > 0) return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
  if (hours > 0) return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  return `${minutes}m`;
}

/**
 * "12 minutes ago" from an epoch. SmartZone is inconsistent about whether a
 * timestamp is seconds or milliseconds, so both are accepted: anything below
 * the year 2001 in milliseconds is treated as seconds.
 */
export function formatRelative(epoch?: number): string {
  const ms = normaliseEpoch(epoch);
  if (ms == null) return '—';

  const delta = Date.now() - ms;
  if (delta < 0) return 'just now';
  const seconds = Math.floor(delta / 1000);
  if (seconds < 45) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(ms).toLocaleDateString();
}

export function formatDateTime(epoch?: number): string {
  const ms = normaliseEpoch(epoch);
  if (ms == null) return '—';
  const d = new Date(ms);
  return `${d.toLocaleDateString()} ${d.toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })}`;
}

export function normaliseEpoch(epoch?: number): number | null {
  if (epoch == null || !Number.isFinite(epoch) || epoch <= 0) return null;
  // 1e12 ms is 2001; anything smaller arrived as seconds.
  return epoch < 1e12 ? epoch * 1000 : epoch;
}

/** Group a MAC into colon-separated pairs, whatever separator it arrived in. */
export function formatMac(mac?: string): string {
  if (!mac) return '—';
  const hex = mac.replace(/[^0-9a-fA-F]/g, '').toUpperCase();
  if (hex.length !== 12) return mac.toUpperCase();
  return (hex.match(/.{2}/g) ?? []).join(':');
}

/** Strip a MAC back to bare hex, for comparison and for URLs. */
export function normaliseMac(mac?: string): string {
  return (mac ?? '').replace(/[^0-9a-fA-F]/g, '').toLowerCase();
}

/** RSSI in dBm, always with its sign and unit. */
export function formatRssi(rssi?: number): string {
  if (rssi == null || !Number.isFinite(rssi)) return '—';
  return `${Math.round(rssi)} dBm`;
}

export function formatSnr(snr?: number): string {
  if (snr == null || !Number.isFinite(snr)) return '—';
  return `${Math.round(snr)} dB`;
}

/** A percentage, guarding the divide-by-zero the callers keep hitting. */
export function formatPercent(value?: number, total?: number): string {
  if (value == null) return '—';
  if (total == null) return `${Math.round(value)}%`;
  if (total === 0) return '—';
  return `${Math.round((value / total) * 100)}%`;
}

export function formatCount(value?: number): string {
  if (value == null || !Number.isFinite(value)) return '—';
  return value.toLocaleString();
}

/**
 * SmartZone's radio identifiers, shortened for a phone.
 *
 * Used for the AP endpoints, which name their radios properly. The client
 * endpoint does not: its `radioType` is a PHY string like "a/n/ac/ax/be", so
 * a client's band comes from `bandForClient` in the clients module instead.
 */
export function formatBand(radio?: string): string {
  switch (radio) {
    case '2.4G':
    case 'RADIO_24G':
      return '2.4 GHz';
    case '5G':
    case 'RADIO_5G':
      return '5 GHz';
    case '6G':
    case 'RADIO_6G':
      return '6 GHz';
    default:
      return radio ?? '—';
  }
}

/** The PHY a client negotiated, tidied up for display. */
export function formatPhy(radioType?: string): string {
  if (!radioType) return '—';
  // "a/n/ac/ax/be" reads better as "Wi-Fi 7 (be)" but the generation mapping
  // is not worth getting wrong; show the letters the controller reported.
  return `802.11${radioType}`;
}

/**
 * Fall back through a chain of maybe-empty strings.
 *
 * SmartZone fills some fields with the literal string "N/A" rather than
 * leaving them out — `userName` on a client with no identity is the common
 * one — so that counts as empty here. Showing "N/A" as if it were a username
 * is worse than showing nothing.
 */
export function firstNonEmpty(...values: (string | undefined | null)[]): string {
  for (const value of values) {
    if (!value) continue;
    const trimmed = value.trim();
    if (!trimmed) continue;
    if (/^(n\/a|none|null|undefined)$/i.test(trimmed)) continue;
    return trimmed;
  }
  return '—';
}
