import type { SmartZoneClient } from '../client';
import { withPath } from '../client';

export interface WlanGroupSummary {
  id: string;
  name: string;
  description?: string;
}

export interface WlanGroupMember {
  id: string;
  name?: string;
  /** Overrides the WLAN's own VLAN for this group only. */
  vlanOverride?: { accessVlan?: number; vlanType?: string };
  nasIdType?: string;
  nasId?: string;
}

export interface WlanGroup extends WlanGroupSummary {
  zoneId?: string;
  members?: WlanGroupMember[];
  [key: string]: unknown;
}

export function wlanGroupsApi(client: SmartZoneClient) {
  return {
    list(zoneId: string, signal?: AbortSignal) {
      return client.listAll<WlanGroupSummary>(
        withPath('/rkszones/{zoneId}/wlangroups', { zoneId }),
        { signal },
      );
    },

    get(zoneId: string, id: string, signal?: AbortSignal) {
      return client.get<WlanGroup>(
        withPath('/rkszones/{zoneId}/wlangroups/{id}', { zoneId, id }),
        { signal },
      );
    },

    create(zoneId: string, body: Partial<WlanGroup>, signal?: AbortSignal) {
      return client.post<{ id: string }>(
        withPath('/rkszones/{zoneId}/wlangroups', { zoneId }),
        body,
        { signal },
      );
    },

    update(
      zoneId: string,
      id: string,
      patch: Partial<WlanGroup>,
      signal?: AbortSignal,
    ) {
      return client.patch<void>(
        withPath('/rkszones/{zoneId}/wlangroups/{id}', { zoneId, id }),
        patch,
        { signal },
      );
    },

    remove(zoneId: string, id: string, signal?: AbortSignal) {
      return client.delete<void>(
        withPath('/rkszones/{zoneId}/wlangroups/{id}', { zoneId, id }),
        { signal },
      );
    },

    /** Put a WLAN into the group. `id` is the WLAN's id. */
    addMember(
      zoneId: string,
      groupId: string,
      member: { id: string } & Partial<WlanGroupMember>,
      signal?: AbortSignal,
    ) {
      return client.post<void>(
        withPath('/rkszones/{zoneId}/wlangroups/{id}/members', {
          zoneId,
          id: groupId,
        }),
        member,
        { signal },
      );
    },

    updateMember(
      zoneId: string,
      groupId: string,
      memberId: string,
      patch: Partial<WlanGroupMember>,
      signal?: AbortSignal,
    ) {
      return client.patch<void>(
        withPath('/rkszones/{zoneId}/wlangroups/{id}/members/{memberId}', {
          zoneId,
          id: groupId,
          memberId,
        }),
        patch,
        { signal },
      );
    },

    removeMember(
      zoneId: string,
      groupId: string,
      memberId: string,
      signal?: AbortSignal,
    ) {
      return client.delete<void>(
        withPath('/rkszones/{zoneId}/wlangroups/{id}/members/{memberId}', {
          zoneId,
          id: groupId,
          memberId,
        }),
        { signal },
      );
    },

    /** Drop a member's VLAN override, putting it back on the WLAN's own. */
    clearMemberVlanOverride(
      zoneId: string,
      groupId: string,
      memberId: string,
      signal?: AbortSignal,
    ) {
      return client.delete<void>(
        withPath(
          '/rkszones/{zoneId}/wlangroups/{id}/members/{memberId}/vlanOverride',
          { zoneId, id: groupId, memberId },
        ),
        { signal },
      );
    },
  };
}
