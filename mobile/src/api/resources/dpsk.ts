import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import { queryPage, runQueryAll, buildCriteria, type BuildCriteriaInput } from '../query';

/**
 * A dynamic pre-shared key.
 *
 * A DPSK binds one passphrase to one user (and optionally one device MAC) on
 * a WLAN, which is how a site gives every tenant, room or student their own
 * key on a single SSID. The passphrase is returned by the controller on
 * creation and on read, so anything that displays it has to be treated the
 * way a password is.
 */
export interface Dpsk {
  id?: string;
  /** The label the key was issued under. */
  userName?: string;
  passphrase?: string;
  /** Bound device, when the key is not roaming. */
  mac?: string;
  wlanId?: string;
  wlanName?: string;
  ssid?: string;
  zoneId?: string;
  zoneName?: string;
  vlanId?: number;
  userRoleId?: string;
  userRoleName?: string;
  /** ISO date, or absent when the key does not expire. */
  expirationDate?: string;
  createdDate?: string;
  /** How many devices may share the key. */
  deviceCountLimit?: number;
  numberOfDevicesUsed?: number;
  /** `Expired`, `Active`, `Unbound`. */
  status?: string;
}

export interface DpskBatchRequest {
  /** How many keys to make. Names get a `-1`, `-2` suffix past one. */
  numberOfDpsks: number;
  userName?: string;
  /** Leave unset to let the controller generate passphrases. */
  passphrases?: string[];
  vlanId?: number;
  userRoleId?: string;
  /** ISO date. Omit for a key with no expiry. */
  expirationDate?: string;
  deviceCountLimit?: number;
  /** Only for keys pinned to one device. */
  macAddresses?: string[];
}

export interface DpskEnabledWlan {
  id: string;
  name: string;
  ssid?: string;
}

export function dpskApi(client: SmartZoneClient) {
  return {
    /** Cluster-wide DPSK search. The only endpoint that pages server-side. */
    query(input: BuildCriteriaInput, signal?: AbortSignal) {
      return queryPage<Dpsk>(client, '/query/dpsk', input, signal);
    },

    /** Every key matching a search, for export. Capped at 20 pages. */
    queryAll(input: BuildCriteriaInput, signal?: AbortSignal) {
      return runQueryAll<Dpsk>(
        client,
        '/query/dpsk',
        buildCriteria({ ...input, pageSize: 250 }),
        { signal },
      );
    },

    /** Keys on one WLAN. */
    listForWlan(zoneId: string, wlanId: string, signal?: AbortSignal) {
      return client.get<{ list?: Dpsk[]; totalCount?: number }>(
        withPath('/rkszones/{zoneId}/wlans/{id}/dpsk', { zoneId, id: wlanId }),
        { signal },
      );
    },

    /** Every key in a zone, across its DPSK WLANs. */
    listForZone(zoneId: string, signal?: AbortSignal) {
      return client.get<{ list?: Dpsk[]; totalCount?: number }>(
        withPath('/rkszones/{zoneId}/dpsk', { zoneId }),
        { signal },
      );
    },

    get(zoneId: string, wlanId: string, dpskId: string, signal?: AbortSignal) {
      return client.get<Dpsk>(
        withPath('/rkszones/{zoneId}/wlans/{id}/dpsk/{dpskId}', {
          zoneId,
          id: wlanId,
          dpskId,
        }),
        { signal },
      );
    },

    /** Which WLANs in a zone can issue keys. Drives the generate form. */
    enabledWlans(zoneId: string, signal?: AbortSignal) {
      return client.get<{ list?: DpskEnabledWlan[] }>(
        withPath('/rkszones/{zoneId}/dpskEnabledWlans', { zoneId }),
        { signal },
      );
    },

    /**
     * Generate keys.
     *
     * "Unbound" means not tied to a device MAC: the key works for whatever
     * device presents it first, up to `deviceCountLimit`. That is what almost
     * every site wants, and it is the only batch endpoint SmartZone exposes.
     */
    generate(
      zoneId: string,
      wlanId: string,
      request: DpskBatchRequest,
      signal?: AbortSignal,
    ) {
      return client.post<{ list?: Dpsk[] }>(
        withPath('/rkszones/{zoneId}/wlans/{id}/dpsk/batchGenUnbound', {
          zoneId,
          id: wlanId,
        }),
        request,
        { signal },
      );
    },

    update(
      zoneId: string,
      wlanId: string,
      dpskId: string,
      patch: Partial<Dpsk>,
      signal?: AbortSignal,
    ) {
      return client.patch<void>(
        withPath('/rkszones/{zoneId}/wlans/{id}/dpsk/{dpskId}', {
          zoneId,
          id: wlanId,
          dpskId,
        }),
        patch,
        { signal },
      );
    },

    /**
     * Revoke keys.
     *
     * SmartZone spells deletion as a POST carrying the ids, not a DELETE —
     * a wart of the public API, not a mistake here.
     */
    revoke(
      zoneId: string,
      wlanId: string,
      dpskIds: string[],
      signal?: AbortSignal,
    ) {
      return client.post<void>(
        withPath('/rkszones/{zoneId}/wlans/{id}/dpsk', { zoneId, id: wlanId }),
        { idList: dpskIds },
        { signal },
      );
    },
  };
}

/** Render keys as CSV for sharing out of the app. */
export function dpskToCsv(keys: Dpsk[]): string {
  const header = [
    'User name',
    'Passphrase',
    'SSID',
    'Zone',
    'VLAN',
    'Device limit',
    'Devices used',
    'Expires',
    'Status',
  ];
  const rows = keys.map((k) => [
    k.userName ?? '',
    k.passphrase ?? '',
    k.ssid ?? k.wlanName ?? '',
    k.zoneName ?? '',
    k.vlanId != null ? String(k.vlanId) : '',
    k.deviceCountLimit != null ? String(k.deviceCountLimit) : '',
    k.numberOfDevicesUsed != null ? String(k.numberOfDevicesUsed) : '',
    k.expirationDate ?? 'Never',
    k.status ?? '',
  ]);
  return [header, ...rows].map((row) => row.map(csvCell).join(',')).join('\n');
}

function csvCell(value: string): string {
  return /[",\n]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
}
