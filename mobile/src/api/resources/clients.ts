import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import { queryPage, type BuildCriteriaInput } from '../query';

/**
 * A connected client as `POST /query/client` returns it.
 *
 * Field names verified against a SmartZone 7.1.1 cluster. Two things about
 * this endpoint drive most of the client-facing code:
 *
 *   - There is no `sessionDuration` field, only `sessionStartTime`. The
 *     duration is computed here. (`sessionDuration` *is* accepted as a sort
 *     column, which is a nice inconsistency to know about.)
 *   - `radioType` is the PHY string — "a/n/ac/ax/be" — not a band. The band
 *     has to come from the channel number.
 */
export interface ClientRow {
  clientMac?: string;
  hostname?: string;
  userName?: string;
  ipAddress?: string;
  ipv6Address?: string;
  osType?: string;
  osVendorType?: string;
  deviceType?: string;
  modelName?: string;
  /** The AP the client is on. */
  apMac?: string;
  apName?: string;
  zoneId?: string;
  ssid?: string;
  bssid?: string;
  wlanType?: string;
  vlan?: number;
  vni?: number;
  /** The PHY, e.g. "a/n/ac/ax/be". Use `bandForClient` for a band. */
  radioType?: string;
  channel?: number;
  /** dBm, negative. Zero means the controller has no reading. */
  rssi?: number;
  /** dB. Zero means no reading. */
  snr?: number;
  /** Negotiated rates in Mbps, not achieved throughput. */
  medianRxMCSRate?: number;
  medianTxMCSRate?: number;
  txRatebps?: number;
  uplinkRate?: number;
  downlinkRate?: number;
  /** Bytes. */
  rxBytes?: number;
  txBytes?: number;
  txRxBytes?: number;
  traffic?: number;
  rxFrames?: number;
  txFrames?: number;
  txDropDataFrames?: number;
  /** Milliseconds since the epoch. There is no duration field. */
  sessionStartTime?: number;
  authMethod?: string;
  encryptionMethod?: string;
  /** "AUTHORIZED" or "UNAUTHORIZED". */
  authStatus?: string;
  status?: string;
  userRoleId?: string;
  userRoleName?: string;
  controlPlaneName?: string;
  dataPlaneName?: string;
  alerts?: number;
  mloCapability?: unknown;
  mloLinks?: unknown;
}

/**
 * A row from `POST /query/historicalclient`: sessions that have ended.
 *
 * Note there is no disconnect reason on 7.1.1 — the useful signal is the
 * shape of the session times, not a stated cause.
 */
export interface HistoricalClientRow {
  clientMac?: string;
  hostname?: string;
  apMac?: string;
  ssid?: string;
  ipAddress?: string;
  ipv6Address?: string;
  modelName?: string;
  mvnoName?: string;
  coreNetworkType?: string;
  sessionStartTime?: number;
  sessionEndTime?: number;
  rxBytes?: number;
  txBytes?: number;
  rxFrames?: number;
  txFrames?: number;
  rxDrops?: number;
  txDrops?: number;
}

export interface BlockedClient {
  id?: string;
  mac?: string;
  description?: string;
  zoneId?: string;
  zoneName?: string;
}

/** Sort columns confirmed to work on `POST /query/client`. */
export const CLIENT_SORT_COLUMNS = [
  'hostname',
  'rssi',
  'snr',
  'ssid',
  'apName',
  'sessionStartTime',
  'sessionDuration',
] as const;

export function clientsApi(client: SmartZoneClient) {
  return {
    /** One page of connected clients. */
    query(input: BuildCriteriaInput, signal?: AbortSignal) {
      return queryPage<ClientRow>(client, '/query/client', input, signal);
    },

    /**
     * Sessions that have already ended.
     *
     * The half of troubleshooting that matters when the complaint is "it
     * dropped me an hour ago": the live table will not have the client at all.
     * `CLIENT` belongs in `extraFilters` here — this endpoint's `filters`
     * enum is a different set entirely and rejects it.
     */
    history(input: BuildCriteriaInput, signal?: AbortSignal) {
      return queryPage<HistoricalClientRow>(
        client,
        '/query/historicalclient',
        input,
        signal,
      );
    },

    /** Everything the controller knows about one MAC, live. */
    async byMac(mac: string, signal?: AbortSignal) {
      const res = await queryPage<ClientRow>(
        client,
        '/query/client',
        // CLIENT is not valid in `filters` on this endpoint; it is a 400.
        { pageSize: 1, extraFilters: [{ type: 'CLIENT', value: mac }] },
        signal,
      );
      return res.list?.[0];
    },

    /** Clients on one AP. `AP` is a scope filter, so it goes in `filters`. */
    onAp(apMac: string, input: BuildCriteriaInput = {}, signal?: AbortSignal) {
      return queryPage<ClientRow>(
        client,
        '/query/client',
        { ...input, filters: [...(input.filters ?? []), { type: 'AP', value: apMac }] },
        signal,
      );
    },

    /** Clients on one SSID. `SSID` is an attribute, so it goes in `extraFilters`. */
    onSsid(ssid: string, input: BuildCriteriaInput = {}, signal?: AbortSignal) {
      return queryPage<ClientRow>(
        client,
        '/query/client',
        {
          ...input,
          extraFilters: [...(input.extraFilters ?? []), { type: 'SSID', value: ssid }],
        },
        signal,
      );
    },

    /**
     * Kick a client off. It will normally come straight back, which is the
     * point: it forces a fresh association and a fresh authentication.
     */
    disconnect(macs: string[], signal?: AbortSignal) {
      return client.post<void>(
        macs.length > 1 ? '/clients/bulkDisconnect' : '/clients/disconnect',
        macs.length > 1 ? { macList: macs.map((mac) => ({ mac })) } : { mac: macs[0] },
        { signal },
      );
    },

    /** Deauthenticate: drops the session and the authorisation with it. */
    deauth(macs: string[], signal?: AbortSignal) {
      return client.post<void>(
        macs.length > 1 ? '/clients/bulkDeauth' : '/clients/deauth',
        macs.length > 1 ? { macList: macs.map((mac) => ({ mac })) } : { mac: macs[0] },
        { signal },
      );
    },

    /** Clients the controller is refusing, per zone. */
    blocked(zoneId: string, signal?: AbortSignal) {
      return client.get<{ list?: BlockedClient[] }>(
        withPath('/blockClient/byZone/{zoneId}', { zoneId }),
        { signal },
      );
    },

    block(zoneId: string, mac: string, description?: string, signal?: AbortSignal) {
      return client.post<{ id: string }>(
        withPath('/blockClient/byZoneId/{zoneId}', { zoneId }),
        { mac, description },
        { signal },
      );
    },

    unblock(id: string, signal?: AbortSignal) {
      return client.delete<void>(withPath('/blockClient/{id}', { id }), { signal });
    },
  };
}

/* -------------------------------------------------------- reading a client */

/**
 * The band a client is on, from its channel number.
 *
 * `radioType` is the PHY ("a/n/ac/ax/be"), not a band, so the channel is what
 * there is to go on. 6 GHz and 5 GHz overlap in channel numbering — channel 52
 * is valid in both — and the PHY narrows it: only 6 GHz clients report `be`
 * on a low channel number. Where it is genuinely ambiguous this says 5 GHz,
 * which is the overwhelmingly more likely answer on deployed hardware.
 */
export function bandForClient(client: {
  channel?: number;
  radioType?: string;
}): '2.4 GHz' | '5 GHz' | '6 GHz' | null {
  const { channel } = client;
  if (channel == null || !Number.isFinite(channel) || channel <= 0) return null;
  if (channel <= 14) return '2.4 GHz';
  // 6 GHz runs 1..233; the channels above 177 have no 5 GHz counterpart.
  if (channel > 177) return '6 GHz';
  return '5 GHz';
}

/** Seconds connected, from the only time field the controller sends. */
export function sessionDuration(client: {
  sessionStartTime?: number;
}): number | undefined {
  const start = client.sessionStartTime;
  if (!start || start <= 0) return undefined;
  const ms = start < 1e12 ? start * 1000 : start;
  const seconds = Math.floor((Date.now() - ms) / 1000);
  return seconds >= 0 ? seconds : undefined;
}

/**
 * Turn a client's radio numbers into a verdict.
 *
 * The thresholds are the ones a Wi-Fi engineer would use out loud: RSSI below
 * -75 dBm is a coverage problem whatever else is true, and SNR under 20 dB is
 * a noise problem even when RSSI looks fine.
 *
 * A reading of exactly 0 is the controller saying it has no measurement, not
 * a very strong signal, and it is common on a client that has just associated.
 * Treating 0 as a number would report a dead client as excellent, which is
 * the worst possible failure for this particular screen.
 */
export type SignalVerdict = 'good' | 'fair' | 'poor' | 'unknown';

function reading(value?: number): number | undefined {
  return value == null || !Number.isFinite(value) || value === 0 ? undefined : value;
}

export function signalVerdict(client: {
  rssi?: number;
  snr?: number;
}): SignalVerdict {
  const rssi = reading(client.rssi);
  const snr = reading(client.snr);
  if (rssi === undefined && snr === undefined) return 'unknown';
  if ((rssi !== undefined && rssi < -75) || (snr !== undefined && snr < 15)) {
    return 'poor';
  }
  if ((rssi !== undefined && rssi < -67) || (snr !== undefined && snr < 25)) {
    return 'fair';
  }
  return 'good';
}

/** A short reason to sit under the verdict. */
export function signalReason(client: { rssi?: number; snr?: number }): string {
  const rssi = reading(client.rssi);
  const snr = reading(client.snr);
  if (rssi === undefined && snr === undefined) {
    return 'The controller has no signal reading for this client yet. That is normal for the first few seconds after it associates.';
  }
  if (rssi !== undefined && rssi < -75) {
    return 'Too far from the AP, or through too much wall.';
  }
  if (snr !== undefined && snr < 15) {
    return 'Noise is close to the signal. Look for interference on this channel.';
  }
  if (rssi !== undefined && rssi < -67) return 'Usable, but a closer AP would do better.';
  if (snr !== undefined && snr < 25) return 'Some noise on this channel.';
  return 'Signal and noise are both healthy.';
}

/** True when the controller says this client got past authentication. */
export function isAuthorised(client: { authStatus?: string; status?: string }): boolean {
  const value = client.authStatus ?? client.status;
  if (!value) return true; // Nothing reported; do not cry wolf.
  return /^authorized$/i.test(value);
}
