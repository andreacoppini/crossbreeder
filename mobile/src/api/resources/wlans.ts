import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import { queryPage, type BuildCriteriaInput } from '../query';

/**
 * A WLAN row from `POST /query/wlan`, which spans every zone at once.
 *
 * The identifier is `wlanId`, not `id` — getting that wrong makes every row
 * un-tappable, which is exactly what it did before this was checked against a
 * real controller.
 */
export interface WlanRow {
  wlanId?: string;
  name?: string;
  ssid?: string;
  description?: string;
  zoneId?: string;
  zoneName?: string;
  domainName?: string;
  tenantDomainName?: string;
  /** The access VLAN, flattened to a number on this endpoint. */
  vlan?: number;
  authMethod?: string;
  authType?: string;
  encryptionMethod?: string;
  wpaVersion?: string;
  wepEncryptionStrength?: string;
  /** "APBridged", "TunnelSCG" and friends. */
  tunneled?: string;
  clients?: number;
  traffic?: number;
  trafficUplink?: number;
  trafficDownlink?: number;
  availability?: number;
  alerts?: number | null;
  status?: string | null;
  applicationVisibility?: boolean;
  zeroITEnabled?: boolean;
  firewallProfile?: string;
}

/**
 * How a WLAN authenticates. Picks the creation endpoint and, on a read, which
 * panels the editor shows.
 */
export type WlanAuthType =
  | 'standard'
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

/**
 * A WLAN as `GET /rkszones/{zoneId}/wlans/{id}` returns it.
 *
 * The configuration object nests where the query row flattens: the VLAN is
 * `vlan.accessVlan`, DPSK is `dpsk.dpskEnabled`, and everything to do with
 * broadcast and isolation lives under `advancedOptions`. The 7.1.1 object has
 * 54 top-level keys; the ones this app reads or writes are named.
 */
export interface Wlan {
  id?: string;
  name?: string;
  ssid?: string;
  description?: string;
  zoneId?: string;
  /** e.g. "Standard_Open", "Standard_8021X". */
  type?: string;
  vlan?: WlanVlan;
  encryption?: WlanEncryption;
  dpsk?: DpskSettings;
  externalDpsk?: { enabled?: boolean; encryption?: WlanEncryption };
  advancedOptions?: WlanAdvancedOptions;
  authServiceOrProfile?: { throughController?: boolean; id?: string; name?: string };
  accountingServiceOrProfile?: { id?: string; name?: string };
  defaultUserTrafficProfile?: { id?: string; name?: string };
  schedule?: { type?: string; id?: string; name?: string };
  macAuth?: Record<string, unknown>;
  bypassCNA?: boolean;
  [key: string]: unknown;
}

export interface WlanVlan {
  accessVlan?: number;
  aaaVlanOverride?: boolean | null;
  coreQinQEnabled?: boolean | null;
  coreSVlan?: number | null;
  vlanPooling?: string | null;
}

export interface WlanEncryption {
  /** "None", "WPA2", "WPA3", "WPA23Mixed", "OWE", "WEP_64", "WEP_128". */
  method?: string;
  /** "AES", "TKIP_AES", "AES_GCMP_256". */
  algorithm?: string;
  /**
   * The pre-shared key.
   *
   * Unlike a DPSK, this *is* returned in cleartext by the WLAN endpoint, so
   * anything that displays it has to treat it the way a password is treated.
   */
  passphrase?: string | null;
  saePassphrase?: string | null;
  mfp?: string;
  keyIndex?: number | null;
  keyInHex?: string | null;
  support80211rEnabled?: boolean;
  transitionDisable?: boolean;
}

export interface DpskSettings {
  dpskEnabled?: boolean;
  /** Governs auto-generated passphrases only; an explicit one overrides it. */
  length?: number;
  /** "Secure", "KeyboardFriendly", "NumbersOnly". */
  dpskType?: string;
  /** "Unlimited", or a period. */
  expiration?: string;
  dpskFromType?: string;
}

/** The `advancedOptions` fields this app reads. There are ninety-odd others. */
export interface WlanAdvancedOptions {
  /** Inverted: the UI concept is "broadcast the SSID". */
  hideSsidEnabled?: boolean;
  clientIsolationEnabled?: boolean;
  clientIsolationUnicastEnabled?: boolean;
  clientIsolationMulticastEnabled?: boolean;
  clientLoadBalancingEnabled?: boolean;
  maxClientsPerRadio?: number;
  clientIdleTimeoutSec?: number;
  userSessionTimeout?: number;
  priority?: string;
  bssMinRateMbps?: number;
  mgmtTxRateMbps?: number;
  proxyARPEnabled?: boolean;
  okcEnabled?: boolean;
  pmkCachingEnabled?: boolean;
  support80211kEnabled?: boolean;
  wifi6Enabled?: boolean;
  multiLinkOperationEnabled?: boolean;
  [key: string]: unknown;
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
     * Partial update. SmartZone validates whole nested objects, so a change
     * to one field inside `vlan`, `encryption` or `advancedOptions` has to
     * carry that object's existing contents with it — see `wlanPatch`.
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
  };
}

/* ------------------------------------------------------------ reading them */

/** Does this WLAN issue per-device keys? */
export function isDpskWlan(wlan: Wlan | undefined): boolean {
  return Boolean(wlan?.dpsk?.dpskEnabled);
}

/** Keys held on an external server, which this app cannot manage. */
export function isExternalDpskWlan(wlan: Wlan | undefined): boolean {
  return Boolean(wlan?.externalDpsk?.enabled);
}

/** The UI concept, from the inverted field the controller stores. */
export function isSsidBroadcast(wlan: Wlan | undefined): boolean {
  return wlan?.advancedOptions?.hideSsidEnabled !== true;
}

export function accessVlan(wlan: Wlan | undefined): number | undefined {
  return wlan?.vlan?.accessVlan;
}

/** True when the WLAN has no encryption at all. */
export function isOpenWlan(wlan: { encryption?: WlanEncryption } | undefined): boolean {
  const method = wlan?.encryption?.method;
  return !method || /^none$/i.test(method);
}

/**
 * Build a PATCH that changes only what the operator touched, while keeping
 * each nested object whole. Sending `{vlan: {accessVlan: 5}}` alone drops the
 * other four keys inside `vlan`, which the controller reads as clearing them.
 */
export function wlanPatch(
  current: Wlan,
  changes: {
    name?: string;
    ssid?: string;
    broadcast?: boolean;
    accessVlan?: number;
    passphrase?: string;
  },
): Partial<Wlan> {
  const patch: Partial<Wlan> = {};

  if (changes.name !== undefined && changes.name !== current.name) {
    patch.name = changes.name;
  }
  if (changes.ssid !== undefined && changes.ssid !== current.ssid) {
    patch.ssid = changes.ssid;
  }
  if (
    changes.broadcast !== undefined &&
    changes.broadcast !== isSsidBroadcast(current)
  ) {
    patch.advancedOptions = {
      ...(current.advancedOptions ?? {}),
      hideSsidEnabled: !changes.broadcast,
    };
  }
  if (
    changes.accessVlan !== undefined &&
    changes.accessVlan !== current.vlan?.accessVlan
  ) {
    patch.vlan = { ...(current.vlan ?? {}), accessVlan: changes.accessVlan };
  }
  if (changes.passphrase) {
    patch.encryption = { ...(current.encryption ?? {}), passphrase: changes.passphrase };
  }
  return patch;
}
