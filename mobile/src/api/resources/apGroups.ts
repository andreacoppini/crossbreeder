import type { SmartZoneClient } from '../client';
import { withPath } from '../client';

export interface ApGroupSummary {
  id: string;
  name: string;
  description?: string;
  /** Number of APs in the group, when the controller reports it. */
  apCount?: number;
}

/**
 * The AP group fields this app reads or edits. SmartZone's full object is
 * enormous (every radio, every model override); the rest is carried through
 * untouched on a PATCH by only sending the keys that changed.
 */
export interface ApGroup extends ApGroupSummary {
  zoneId?: string;
  location?: string;
  locationAdditionalInfo?: string;
  latitude?: number;
  longitude?: number;
  apMgmtVlan?: { mode?: string; vlanId?: number };
  radioConfig?: ApGroupRadioConfig;
  members?: { apMac?: string; name?: string }[];
  [key: string]: unknown;
}

export interface ApGroupRadioConfig {
  radio24g?: RadioSettings;
  radio5g?: RadioSettings;
  radio6g?: RadioSettings;
  radio5gLower?: RadioSettings;
  radio5gUpper?: RadioSettings;
}

export interface RadioSettings {
  channelWidth?: number | string;
  channel?: number | string;
  txPower?: string;
  wlanGroupId?: string;
  wlanGroupName?: string;
  autoChannelSelection?: { channelSelectMode?: string };
  protectionMode?: string;
  [key: string]: unknown;
}

export function apGroupsApi(client: SmartZoneClient) {
  return {
    list(zoneId: string, signal?: AbortSignal) {
      return client.listAll<ApGroupSummary>(
        withPath('/rkszones/{zoneId}/apgroups', { zoneId }),
        { signal },
      );
    },

    get(zoneId: string, id: string, signal?: AbortSignal) {
      return client.get<ApGroup>(
        withPath('/rkszones/{zoneId}/apgroups/{id}', { zoneId, id }),
        { signal },
      );
    },

    /** The zone's default AP group, which cannot be deleted. */
    getDefault(zoneId: string, signal?: AbortSignal) {
      return client.get<ApGroup>(
        withPath('/rkszones/{zoneId}/apgroups/default', { zoneId }),
        { signal },
      );
    },

    create(zoneId: string, body: Partial<ApGroup>, signal?: AbortSignal) {
      return client.post<{ id: string }>(
        withPath('/rkszones/{zoneId}/apgroups', { zoneId }),
        body,
        { signal },
      );
    },

    update(
      zoneId: string,
      id: string,
      patch: Partial<ApGroup>,
      signal?: AbortSignal,
    ) {
      return client.patch<void>(
        withPath('/rkszones/{zoneId}/apgroups/{id}', { zoneId, id }),
        patch,
        { signal },
      );
    },

    remove(zoneId: string, id: string, signal?: AbortSignal) {
      return client.delete<void>(
        withPath('/rkszones/{zoneId}/apgroups/{id}', { zoneId, id }),
        { signal },
      );
    },

    addMember(zoneId: string, id: string, apMac: string, signal?: AbortSignal) {
      return client.post<void>(
        withPath('/rkszones/{zoneId}/apgroups/{id}/members/{apMac}', {
          zoneId,
          id,
          apMac,
        }),
        undefined,
        { signal },
      );
    },

    removeMember(zoneId: string, id: string, apMac: string, signal?: AbortSignal) {
      return client.delete<void>(
        withPath('/rkszones/{zoneId}/apgroups/{id}/members/{apMac}', {
          zoneId,
          id,
          apMac,
        }),
        { signal },
      );
    },
  };
}
