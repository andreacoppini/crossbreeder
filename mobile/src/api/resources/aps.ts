import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import { queryPage, runQuery, type BuildCriteriaInput, type QueryResult } from '../query';

/**
 * An AP row as `POST /query/ap` returns it.
 *
 * Every field is optional: SmartZone omits what it has no value for, and
 * which fields appear at all varies with the controller version. Treating
 * them as optional here is what keeps a 5.2 cluster from crashing a screen
 * built against a 7.x one.
 */
export interface ApRow {
  apMac?: string;
  deviceName?: string;
  description?: string;
  status?: ApStatus;
  administrativeState?: string;
  zoneId?: string;
  zoneName?: string;
  apGroupId?: string;
  apGroupName?: string;
  domainId?: string;
  domainName?: string;
  model?: string;
  serial?: string;
  ip?: string;
  ipv6Address?: string;
  externalIp?: string;
  firmwareVersion?: string;
  location?: string;
  latitude?: number;
  longitude?: number;
  /** Seconds. */
  uptime?: number;
  lastSeenTime?: number;
  numClients?: number;
  numClients24G?: number;
  numClients5G?: number;
  numClients6G?: number;
  channel24G?: string;
  channel5G?: string;
  channel6G?: string;
  txPower24G?: string;
  txPower5G?: string;
  meshRole?: string;
  connectionState?: string;
  registrationState?: string;
  configurationStatus?: string;
  alerts?: number;
  /** Bytes since last reset. */
  tx?: number;
  rx?: number;
  airtime24G?: number;
  airtime5G?: number;
  cpuPercentage?: number;
  memoryPercentage?: number;
  poePortStatus?: string;
}

export type ApStatus =
  | 'Online'
  | 'Offline'
  | 'Flagged'
  | 'Discovery'
  | 'Provisioned'
  | 'RebootRequired'
  | string;

/** The configuration object behind `GET /aps/{apMac}`. */
export interface ApConfig {
  mac?: string;
  name?: string;
  description?: string;
  zoneId?: string;
  apGroupId?: string;
  serial?: string;
  model?: string;
  location?: string;
  locationAdditionalInfo?: string;
  latitude?: number;
  longitude?: number;
  administrativeState?: 'Unlocked' | 'Locked' | string;
  /** Static or DHCP addressing for the AP's management interface. */
  network?: Record<string, unknown>;
  wifi24?: Record<string, unknown>;
  wifi50?: Record<string, unknown>;
  wifi6?: Record<string, unknown>;
  [key: string]: unknown;
}

/** `GET /aps/{apMac}/operational/summary` — the detail screen's source. */
export interface ApOperationalSummary extends ApRow {
  apMac?: string;
  /** Present on mesh members. */
  meshRole?: string;
  wlanGroup24Name?: string;
  wlanGroup50Name?: string;
  wlanGroup6Name?: string;
  eth0?: Record<string, unknown>;
  eth1?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface ApNeighbour {
  apMac?: string;
  deviceName?: string;
  rssi?: number;
  channel?: string;
  band?: string;
}

/** Sort columns the AP list screen offers. */
export const AP_SORT_COLUMNS = [
  'deviceName',
  'status',
  'numClients',
  'model',
  'zoneName',
  'apGroupName',
  'lastSeenTime',
] as const;

export function apsApi(client: SmartZoneClient) {
  return {
    /** One page of APs, filtered and sorted server-side. */
    query(input: BuildCriteriaInput, signal?: AbortSignal) {
      return queryPage<ApRow>(client, '/query/ap', input, signal);
    },

    /** BSSIDs an AP is broadcasting, per radio. Used when troubleshooting. */
    wlans(apMac: string, signal?: AbortSignal) {
      return runQuery<Record<string, unknown>>(
        client,
        '/query/ap/wlan',
        { page: 1, limit: 100, filters: [{ type: 'AP', value: apMac }] },
        signal,
      );
    },

    config(apMac: string, signal?: AbortSignal) {
      return client.get<ApConfig>(withPath('/aps/{apMac}', { apMac }), { signal });
    },

    operational(apMac: string, signal?: AbortSignal) {
      return client.get<ApOperationalSummary>(
        withPath('/aps/{apMac}/operational/summary', { apMac }),
        { signal },
      );
    },

    neighbours(apMac: string, signal?: AbortSignal) {
      return client.get<{ list?: ApNeighbour[] }>(
        withPath('/aps/{apMac}/operational/neighbor', { apMac }),
        { signal },
      );
    },

    clientCount(apMac: string, signal?: AbortSignal) {
      return client.get<{ totalCount?: number }>(
        withPath('/aps/{apMac}/operational/client/totalCount', { apMac }),
        { signal },
      );
    },

    update(apMac: string, patch: Partial<ApConfig>, signal?: AbortSignal) {
      return client.patch<void>(withPath('/aps/{apMac}', { apMac }), patch, {
        signal,
      });
    },

    /** Register an AP the controller has not seen before. */
    create(body: Partial<ApConfig>, signal?: AbortSignal) {
      return client.post<void>('/aps', body, { signal });
    },

    remove(apMac: string, signal?: AbortSignal) {
      return client.delete<void>(withPath('/aps/{apMac}', { apMac }), { signal });
    },

    reboot(apMac: string, signal?: AbortSignal) {
      return client.put<void>(withPath('/aps/{apMac}/reboot', { apMac }), undefined, {
        signal,
      });
    },

    /**
     * Flash the AP's LEDs so someone on a ladder can find it.
     * The controller stops on its own after a minute or so.
     */
    blinkLed(apMac: string, signal?: AbortSignal) {
      return client.post<void>(
        withPath('/aps/{apMac}/operational/blinkLed', { apMac }),
        undefined,
        { signal },
      );
    },

    /** Move APs between zones or AP groups in one call. */
    move(
      apMacs: string[],
      target: { zoneId?: string; apGroupId?: string },
      signal?: AbortSignal,
    ) {
      return client.post<void>(
        '/aps/move',
        { apList: apMacs.map((mac) => ({ mac })), ...target },
        { signal },
      );
    },

    /** Assign one AP to an AP group inside its zone. */
    setGroup(
      zoneId: string,
      apGroupId: string,
      apMac: string,
      signal?: AbortSignal,
    ) {
      return client.post<void>(
        withPath('/rkszones/{zoneId}/apgroups/{id}/members/{apMac}', {
          zoneId,
          id: apGroupId,
          apMac,
        }),
        undefined,
        { signal },
      );
    },
  };
}

export type ApQueryResult = QueryResult<ApRow>;
