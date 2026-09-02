import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import { queryPage, type BuildCriteriaInput } from '../query';

/** A WLAN row from `POST /query/wlan`, which spans every zone at once. */
export interface WlanRow {
  id?: string;
  name?: string;
  ssid?: string;
  zoneId?: string;
  zoneName?: string;
  domainId?: string;
  domainName?: string;
  apGroupCount?: number;
  vlanId?: number;
  authMethod?: string;
  encryptionMethod?: string;
  wlanType?: string;
  /** Live counters, where the controller supplies them. */
  clients?: number;
  traffic?: number;
  trafficUplink?: number;
  trafficDownlink?: number;
  status?: string;
}

/**
 * How a WLAN authenticates. This is the field that decides which creation
 * endpoint applies and which panels the editor shows.
 */
export type WlanAuthType =
  | 'standard' // open or PSK, no portal
  | 'standard8021X'
  | 'standardmac'
  | 'standard8021Xmac'
  | 'guest'
  | 'webauth'
  | 'wispr'
  | 'wisprmac'
  | 'wispr8021X'
  | 'wechat'
  | 'hotspot20'
  | 'hotspot20open'
  | 'hotspot20osen';

/** The WLAN configuration fields the editor touches. */
export interface Wlan {
  id?: string;
  name?: string;
  ssid?: string;
  description?: string;
  zoneId?: string;
  /** Broadcast or hidden. */
  ssidBroadcastEnabled?: boolean;
  vlanId?: number;
  encryption?: WlanEncryption;
  /** `AAA`, `HOTSPOT`, `GUEST`, `WEBAUTH`, `SELF_SIGNIN` and friends. */
  authServiceOrProfile?: { throughController?: boolean; id?: string; name?: string };
  accountingServiceOrProfile?: { id?: string; name?: string };
  /** Present when the WLAN issues per-device keys. */
  dpskEnabled?: boolean;
  dpsk?: DpskSettings;
  /** Rate limiting and QoS. */
  userTrafficProfile?: { id?: string; name?: string };
  /** Client isolation, band steering and the rest of the advanced panel. */
  advancedOptions?: Record<string, unknown>;
  clientIsolationEnabled?: boolean;
  bypassCNA?: boolean;
  priority?: 'High' | 'Low' | string;
  wlanScheduler?: { id?: string; name?: string; type?: string };
  [key: string]: unknown;
}

export interface WlanEncryption {
  /** `None`, `WPA2`, `WPA3`, `WPA23Mixed`, `WPA_Mixed`, `WEP_64`, `WEP_128`. */
  method?: string;
  /** `AES`, `TKIP_AES`, `AES_GCMP_256`. */
  algorithm?: string;
  /** The pre-shared key. Write-only in practice: SmartZone masks it on read. */
  passphrase?: string;
  saePassphrase?: string;
  mfp?: string;
  keyIndex?: number;
  keyInHex?: string;
  supportH2E?: boolean;
}

export interface DpskSettings {
  /** `Secure`, `KeyboardFriendly`, `NumbersOnly`. */
  dpskType?: string;
  dpskLength?: number;
  /** `Unlimited` or an ISO date. */
  expiration?: string;
  deviceCountLimit?: number;
}

/** WLAN scheduler profiles, which switch an SSID on and off by timetable. */
export interface WlanScheduler {
  id: string;
  name: string;
  description?: string;
}

export function wlansApi(client: SmartZoneClient) {
  return {
    /** Cluster-wide WLAN search, the list screen's source. */
    query(input: BuildCriteriaInput, signal?: AbortSignal) {
      return queryPage<WlanRow>(client, '/query/wlan', input, signal);
    },

    /** WLANs configured in one zone. */
    listInZone(zoneId: string, signal?: AbortSignal) {
      return client.listAll<{ id: string; name: string }>(
        withPath('/rkszones/{zoneId}/wlans', { zoneId }),
        { signal },
      );
    },

    get(zoneId: string, id: string, signal?: AbortSignal) {
      return client.get<Wlan>(
        withPath('/rkszones/{zoneId}/wlans/{id}', { zoneId, id }),
        { signal },
      );
    },

    /**
     * Partial update. SmartZone rejects a PATCH that carries keys it did not
     * send on the GET, so the editor sends only the fields it changed.
     */
    update(zoneId: string, id: string, patch: Partial<Wlan>, signal?: AbortSignal) {
      return client.patch<void>(
        withPath('/rkszones/{zoneId}/wlans/{id}', { zoneId, id }),
        patch,
        { signal },
      );
    },

    remove(zoneId: string, id: string, signal?: AbortSignal) {
      return client.delete<void>(
        withPath('/rkszones/{zoneId}/wlans/{id}', { zoneId, id }),
        { signal },
      );
    },

    /**
     * Create a WLAN. Each authentication style has its own endpoint on
     * SmartZone rather than a discriminated body, so the type picks the path.
     */
    create(
      zoneId: string,
      type: WlanAuthType,
      body: Partial<Wlan>,
      signal?: AbortSignal,
    ) {
      const suffix = type === 'standard' ? '' : `/${type}`;
      return client.post<{ id: string }>(
        `${withPath('/rkszones/{zoneId}/wlans', { zoneId })}${suffix}`,
        body,
        { signal },
      );
    },

    schedulers(zoneId: string, signal?: AbortSignal) {
      return client.listAll<WlanScheduler>(
        withPath('/rkszones/{zoneId}/wlanschedulers', { zoneId }),
        { signal },
      );
    },

    /** Rate-limit and QoS profiles a WLAN can point at. */
    userTrafficProfiles(zoneId: string, signal?: AbortSignal) {
      return client.listAll<{ id: string; name: string }>(
        withPath('/rkszones/{zoneId}/portals/utp', { zoneId }),
        { signal },
      );
    },
  };
}
