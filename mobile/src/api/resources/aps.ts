import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import { queryPage, runQuery, type BuildCriteriaInput, type QueryResult } from '../query';

/**
 * An AP row as `POST /query/ap` returns it.
 *
 * Field names verified against a SmartZone 7.1.1 cluster, which returns 193
 * of them per AP. Only the ones this app uses are named here; the rest come
 * through untyped and unharmed. Every field is optional because SmartZone
 * omits what it has no value for, and several (`tx`, `rx`, `location`) come
 * back as an explicit null on a perfectly healthy AP.
 *
 * Mind the near-misses against `/aps/{mac}/operational/summary`, which spells
 * several of the same things differently: this row has `apMac`, `deviceName`,
 * `numClients` and `lastSeen`, where that endpoint has `mac`, `name`,
 * `clientCount` and `lastSeenTime`.
 */
export interface ApRow {
  apMac?: string;
  deviceName?: string;
  description?: string | null;
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
  /** The address the controller sees the AP from. Not `externalIp`. */
  extIp?: string;
  extPort?: number;
  firmwareVersion?: string;
  zoneFirmwareVersion?: string;
  location?: string | null;
  /** Seconds. */
  uptime?: number;
  /** Milliseconds since the epoch. Not `lastSeenTime` on this endpoint. */
  lastSeen?: number;
  registrationTime?: number;
  numClients?: number;
  numClients24G?: number;
  numClients5G?: number;
  numClients6G?: number;
  /** A rendered string, e.g. "52 (40MHz)". */
  channel24G?: string;
  channel5G?: string;
  channel6G?: string;
  channel24gValue?: number;
  channel50gValue?: number;
  channel6gValue?: number;
  txPower24G?: string;
  txPower5G?: string;
  txPower6G?: string;
  /** Percentages. */
  airtime24G?: number;
  airtime5G?: number;
  airtime6G?: number;
  noise24G?: number;
  noise5G?: number;
  noise6G?: number;
  latency24G?: number;
  latency50G?: number;
  latency6G?: number;
  retry24G?: number;
  retry5G?: number;
  retry6G?: number;
  capacity?: number;
  meshRole?: string;
  meshMode?: string;
  /** "Connect" or "Disconnect". Not `connectionState`. */
  connectionStatus?: string;
  registrationState?: string;
  configurationStatus?: string;
  alerts?: number;
  /** SmartZone's own rollup of its per-metric health flags. */
  isOverallHealthStatusFlagged?: boolean;
  isCriticalAp?: boolean;
  crashDump?: string | null;
  /** Bytes since last reset. Null on an AP that has passed nothing. */
  tx?: number | null;
  rx?: number | null;
  txRx?: number | null;
  wlanGroup24Name?: string;
  wlanGroup50Name?: string;
  wlanGroup6gName?: string;
  indoorMapName?: string;
  poePortStatus?: string;
  powerMode?: string;
  managementVlan?: number;
}

/** The status values a 7.1.1 cluster actually returns. */
export type ApStatus = 'Online' | 'Offline' | 'Flagged' | string;

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
  administrativeState?: string;
  apMgmtVlan?: Record<string, unknown>;
  network?: Record<string, unknown>;
  radioConfig?: Record<string, unknown>;
  [key: string]: unknown;
}

/**
 * `GET /aps/{apMac}/operational/summary`.
 *
 * A different vocabulary from the query row, and a thinner one: no status, no
 * traffic, no per-radio client counts, no airtime. The detail screen reads
 * from `POST /query/ap` filtered to one AP instead, and this is kept for the
 * few fields the query row does not carry (country code, mesh hop, approval
 * time).
 */
export interface ApOperationalSummary {
  mac?: string;
  name?: string;
  description?: string;
  model?: string;
  serial?: string;
  /** Firmware. Spelled `version` here, `firmwareVersion` on the query row. */
  version?: string;
  ip?: string;
  ipv6?: string;
  externalIp?: string;
  externalPort?: number;
  zoneId?: string;
  apGroupId?: string;
  countryCode?: string;
  clientCount?: number;
  connectionState?: string;
  configState?: string;
  registrationState?: string;
  administrativeState?: string;
  lastSeenTime?: number;
  approvedTime?: number;
  uptime?: number;
  location?: string;
  locationAdditionalInfo?: string;
  latitude?: number;
  longitude?: number;
  managementVlan?: number;
  meshRole?: string;
  meshHop?: number;
  isCriticalAP?: boolean;
  wifi24Channel?: string;
  wifi50Channel?: string;
  wifi6gChannel?: string;
  provisionMethod?: string;
  provisionStage?: string;
  [key: string]: unknown;
}

export interface ApNeighbour {
  apMac?: string;
  deviceName?: string;
  rssi?: number;
  channel?: string;
  band?: string;
}

/** Sort columns confirmed to work on `POST /query/ap`. */
export const AP_SORT_COLUMNS = [
  'deviceName',
  'status',
  'numClients',
  'model',
  'zoneName',
  'apGroupName',
  'lastSeen',
] as const;

/** How many APs of each status, and whether we managed to see all of them. */
export interface ApStatusCounts {
  online: number;
  flagged: number;
  offline: number;
  other: number;
  total: number;
  /** True when the cluster was larger than the page budget allowed. */
  truncated: boolean;
}

export function apsApi(client: SmartZoneClient) {
  return {
    /** One page of APs, searched and sorted server-side. */
    query(input: BuildCriteriaInput, signal?: AbortSignal) {
      return queryPage<ApRow>(client, '/query/ap', input, signal);
    },

    /**
     * Everything the controller knows about one AP.
     *
     * The query endpoint rather than `/operational/summary`, because it is far
     * richer — status, traffic, per-radio clients, airtime, zone and group
     * names — and filtering it to a single MAC costs exactly one request.
     */
    async byMac(apMac: string, signal?: AbortSignal) {
      const res = await queryPage<ApRow>(
        client,
        '/query/ap',
        { pageSize: 1, filters: [{ type: 'AP', value: apMac }] },
        signal,
      );
      return res.list?.[0];
    },

    /**
     * Count APs by status.
     *
     * There is no server-side way to do this on 7.1.1: a `STATUS` extraFilter
     * is accepted and then matches nothing, and an `attributes` projection
     * comes back without the projected field. So the rows are counted here,
     * in pages of 1000, against a budget. `truncated` reports a cluster
     * bigger than that budget rather than quietly under-counting.
     */
    async statusCounts(
      opts: { zoneId?: string; maxPages?: number; signal?: AbortSignal } = {},
    ): Promise<ApStatusCounts> {
      const maxPages = opts.maxPages ?? 6;
      const counts: ApStatusCounts = {
        online: 0,
        flagged: 0,
        offline: 0,
        other: 0,
        total: 0,
        truncated: false,
      };

      for (let page = 1; page <= maxPages; page += 1) {
        const res = await queryPage<ApRow>(
          client,
          '/query/ap',
          {
            page,
            pageSize: 1000,
            filters: opts.zoneId ? [{ type: 'ZONE', value: opts.zoneId }] : undefined,
          },
          opts.signal,
        );
        for (const ap of res.list ?? []) {
          counts.total += 1;
          switch (ap.status) {
            case 'Online':
              counts.online += 1;
              break;
            case 'Flagged':
              counts.flagged += 1;
              break;
            case 'Offline':
              counts.offline += 1;
              break;
            default:
              counts.other += 1;
          }
        }
        if (!res.hasMore || (res.list?.length ?? 0) === 0) return counts;
        if (page === maxPages) counts.truncated = true;
      }
      return counts;
    },

    /** BSSIDs an AP is broadcasting, per radio. */
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
     * Flash the AP's LEDs so someone on a ladder can find it. The controller
     * stops on its own after a minute or so.
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
    setGroup(zoneId: string, apGroupId: string, apMac: string, signal?: AbortSignal) {
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
