import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import { queryPage, runQueryAll, buildCriteria, type BuildCriteriaInput } from '../query';

/**
 * Dynamic PSKs.
 *
 * A DPSK binds one passphrase to one user (and optionally one device MAC) on
 * a WLAN, which is how a site gives every tenant, room or student their own
 * key on a single SSID.
 *
 * **Passphrases are write-only.** Neither `GET /rkszones/{z}/wlans/{w}/dpsk`
 * nor `POST /query/dpsk` returns one — verified against a 7.1.1 cluster,
 * where the row carries a `key` (a UUID, the record's id) and no passphrase
 * at all. The only moment a passphrase is knowable is when it is created, and
 * the only way to know it later is to have chosen it yourself. Everything in
 * this app's DPSK flow follows from that.
 *
 * A `PATCH` can change `userName` and little else; a passphrase change means
 * revoke and reissue.
 */
export interface Dpsk {
  /** The record's id, a UUID. This is what `revoke` takes. Not a passphrase. */
  key?: string;
  userName?: string;
  /** Bound device, or null for a roaming key. */
  ueMac?: string | null;
  wlanId?: string;
  zoneId?: string;
  domainId?: string;
  tenantId?: string;
  vlanId?: number;
  userRoleId?: string | null;
  /** Milliseconds since the epoch. */
  createDateTime?: number;
  /** Milliseconds, or 0 when the key does not expire. */
  expirationTime?: number;
  expirationStartTime?: number;
  /** Seconds, or 0 for unlimited. */
  ttl?: number;
  /** True for a group (shared) key rather than a per-device one. */
  group?: boolean;
  expired?: boolean;
}

/**
 * The body `batchGenUnbound` actually wants.
 *
 * Not the field names the published schema suggests: it is `amount` and
 * `passphraseList`, confirmed against a working production integration
 * against this controller.
 */
export interface DpskBatchRequest {
  /** How many keys to make. Names get a `-1`, `-2` suffix past one. */
  amount: number;
  userName?: string;
  /**
   * Explicit passphrases. Supplying them is the only way to know the key
   * afterwards, since the controller will not read one back. An explicit
   * passphrase also overrides the WLAN's configured DPSK length.
   */
  passphraseList?: string[];
  /** A shared key rather than one binding per device. */
  groupDpsk?: boolean;
  vlanId?: number;
  userRoleId?: string;
  /** ISO date. Omit for a key with no expiry. */
  expirationDate?: string;
}

/** `GET /rkszones/{zoneId}/dpskEnabledWlans` returns these. */
export interface DpskEnabledWlan {
  wlanId: string;
  wlanName?: string;
  ssid?: string;
}

export function dpskApi(client: SmartZoneClient) {
  return {
    /**
     * DPSK search.
     *
     * Only `ZONE` filters this endpoint. A `WLAN` filter is accepted in both
     * slots and matches nothing on 7.1.1, so narrowing to a WLAN has to
     * happen locally — see `filterByWlan`.
     */
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

    /** Keys on one WLAN, straight from the WLAN's own endpoint. */
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
     * device presents it first. It is the only batch endpoint SmartZone
     * exposes, and what almost every site wants.
     *
     * The response is the one and only chance to see the passphrases.
     */
    generate(
      zoneId: string,
      wlanId: string,
      request: DpskBatchRequest,
      signal?: AbortSignal,
    ) {
      return client.post<{ list?: (Dpsk & { passphrase?: string })[] }>(
        withPath('/rkszones/{zoneId}/wlans/{id}/dpsk/batchGenUnbound', {
          zoneId,
          id: wlanId,
        }),
        request,
        { signal },
      );
    },

    /** Rename a key. The only field a PATCH will move. */
    rename(
      zoneId: string,
      wlanId: string,
      dpskId: string,
      userName: string,
      signal?: AbortSignal,
    ) {
      return client.patch<void>(
        withPath('/rkszones/{zoneId}/wlans/{id}/dpsk/{dpskId}', {
          zoneId,
          id: wlanId,
          dpskId,
        }),
        { userName },
        { signal },
      );
    },

    /**
     * Revoke keys, by their `key` (the record id).
     *
     * SmartZone spells deletion as a POST carrying the ids, not a DELETE — a
     * wart of the public API, not a mistake here. It answers
     * `{resultCount: n}` and is a no-op for ids it does not recognise.
     */
    revoke(zoneId: string, wlanId: string, dpskIds: string[], signal?: AbortSignal) {
      return client.post<{ resultCount?: number }>(
        withPath('/rkszones/{zoneId}/wlans/{id}/dpsk', { zoneId, id: wlanId }),
        { idList: dpskIds },
        { signal },
      );
    },
  };
}

/** Narrow a zone-wide key list to one WLAN, which the API will not do. */
export function filterByWlan(keys: Dpsk[], wlanId?: string): Dpsk[] {
  if (!wlanId) return keys;
  return keys.filter((k) => String(k.wlanId) === String(wlanId));
}

/** When a key expires, or null when it never does. */
export function expiryDate(dpsk: Dpsk): Date | null {
  const t = dpsk.expirationTime;
  if (!t || t <= 0) return null;
  return new Date(t < 1e12 ? t * 1000 : t);
}

/**
 * Render keys as CSV.
 *
 * Without passphrases, because the controller does not have them to give.
 * A column of blanks would read as "these keys have no passphrase", which is
 * worse than not offering the column at all.
 */
export function dpskToCsv(
  keys: Dpsk[],
  lookup: { wlanName?: (wlanId?: string) => string | undefined } = {},
): string {
  const header = [
    'User name',
    'WLAN',
    'VLAN',
    'Bound device',
    'Shared key',
    'Created',
    'Expires',
    'Expired',
  ];
  const rows = keys.map((k) => {
    const expiry = expiryDate(k);
    return [
      k.userName ?? '',
      lookup.wlanName?.(k.wlanId) ?? k.wlanId ?? '',
      k.vlanId != null ? String(k.vlanId) : '',
      k.ueMac ?? '',
      k.group ? 'yes' : 'no',
      k.createDateTime ? new Date(k.createDateTime).toISOString() : '',
      expiry ? expiry.toISOString() : 'Never',
      k.expired ? 'yes' : 'no',
    ];
  });
  return [header, ...rows].map((row) => row.map(csvCell).join(',')).join('\n');
}

function csvCell(value: string): string {
  return /[",\n]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
}
