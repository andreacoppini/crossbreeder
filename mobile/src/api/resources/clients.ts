import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import { queryPage, type BuildCriteriaInput } from '../query';

/**
 * A connected client as `POST /query/client` returns it.
 *
 * This is the object the whole troubleshooting flow hangs off: it carries the
 * radio measurements (RSSI, SNR, MCS), the association facts (AP, WLAN, VLAN)
 * and the authentication result in one row, which is most of what an engineer
 * standing in the room needs before they start guessing.
 */
export interface ClientRow {
  clientMac?: string;
  hostname?: string;
  userName?: string;
  ipAddress?: string;
  ipv6Address?: string;
  osType?: string;
  /** The AP the client is on. */
  apMac?: string;
  apName?: string;
  zoneId?: string;
  zoneName?: string;
  apGroupId?: string;
  apGroupName?: string;
  ssid?: string;
  wlanId?: string;
  bssid?: string;
  vlan?: number;
  /** `2.4G`, `5G`, `6G`. */
  radioType?: string;
  channel?: number;
  /** dBm. Negative; closer to zero is stronger. */
  rssi?: number;
  /** dB. Under about 20 is where trouble starts. */
  snr?: number;
  noiseFloor?: number;
  /** Negotiated rates in Mbps, not throughput. */
  rxMcsRate?: number;
  txMcsRate?: number;
  receiveSignalStrength?: number;
  /** Bytes. */
  rxBytes?: number;
  txBytes?: number;
  rxFrames?: number;
  txFrames?: number;
  rxDrops?: number;
  txDrops?: number;
  /** Seconds since association. */
  sessionDuration?: number;
  connectedSince?: number;
  authMethod?: string;
  encryptionMethod?: string;
  /** `Authorized`, `Unauthorized`. The first thing to check on a failure. */
  authStatus?: string;
  status?: string;
  /** Set when the key came from a DPSK. */
  dpskId?: string;
  /** Roaming and steering history the controller keeps per client. */
  isRoamed?: boolean;
  traffic?: number;
  vlanId?: number;
  nasId?: string;
  callingStationId?: string;
}

/** A row from `POST /query/historicalclient`: sessions that have ended. */
export interface HistoricalClientRow extends ClientRow {
  disconnectTime?: number;
  sessionStartTime?: number;
  sessionEndTime?: number;
  disconnectReason?: string;
}

/** A blocked client, from the controller's block list. */
export interface BlockedClient {
  id?: string;
  mac?: string;
  description?: string;
  zoneId?: string;
  zoneName?: string;
}

/** Sort columns offered on the client list. */
export const CLIENT_SORT_COLUMNS = [
  'hostname',
  'clientMac',
  'rssi',
  'snr',
  'ssid',
  'apName',
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
     * This is the half of troubleshooting that matters when the complaint is
     * "it dropped me an hour ago": the live table will not have the client at
     * all, but the historical one carries the disconnect reason.
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
        { pageSize: 1, filters: [{ type: 'CLIENT', value: mac }] },
        signal,
      );
      return res.list?.[0];
    },

    /** Clients on one AP. */
    onAp(apMac: string, input: BuildCriteriaInput = {}, signal?: AbortSignal) {
      return queryPage<ClientRow>(
        client,
        '/query/client',
        { ...input, filters: [...(input.filters ?? []), { type: 'AP', value: apMac }] },
        signal,
      );
    },

    /** Clients on one SSID. */
    onSsid(ssid: string, input: BuildCriteriaInput = {}, signal?: AbortSignal) {
      return queryPage<ClientRow>(
        client,
        '/query/client',
        {
          ...input,
          filters: [...(input.filters ?? []), { type: 'SSID', value: ssid }],
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

/**
 * Turn a client's radio numbers into a verdict.
 *
 * The thresholds are the ones a Wi-Fi engineer would use out loud: RSSI below
 * -75 dBm is a coverage problem whatever else is true, and SNR under 20 dB is
 * a noise problem even when RSSI looks fine. Keeping the rule here means the
 * AP screen, the client screen and the troubleshooting sheet all say the same
 * thing about the same client.
 */
export type SignalVerdict = 'good' | 'fair' | 'poor' | 'unknown';

export function signalVerdict(client: {
  rssi?: number;
  snr?: number;
}): SignalVerdict {
  const { rssi, snr } = client;
  if (rssi == null && snr == null) return 'unknown';
  if ((rssi != null && rssi < -75) || (snr != null && snr < 15)) return 'poor';
  if ((rssi != null && rssi < -67) || (snr != null && snr < 25)) return 'fair';
  return 'good';
}

/** A short reason to sit under the verdict. */
export function signalReason(client: { rssi?: number; snr?: number }): string {
  const { rssi, snr } = client;
  if (rssi != null && rssi < -75) return 'Too far from the AP, or through too much wall.';
  if (snr != null && snr < 15) return 'Noise is close to the signal. Look for interference on this channel.';
  if (rssi != null && rssi < -67) return 'Usable, but a closer AP would do better.';
  if (snr != null && snr < 25) return 'Some noise on this channel.';
  return 'Signal and noise are both healthy.';
}
