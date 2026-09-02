/**
 * ICX switch management.
 *
 * Deliberately a stub. Switching is the next thing this app grows into, and
 * the shape of that growth is settled here so it does not disturb anything
 * later: the switch surface hangs off the same client, the same session and
 * the same query builder as Wi-Fi, and appears in the UI behind
 * `features.switching`.
 *
 * The endpoints below are the ones SmartZone actually exposes for switches;
 * they are typed and callable now, and what is missing is the screens, not
 * the plumbing.
 */

import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import { queryPage, type BuildCriteriaInput } from '../query';

export interface SwitchRow {
  id?: string;
  serialNumber?: string;
  name?: string;
  model?: string;
  status?: string;
  ipAddress?: string;
  macAddress?: string;
  firmwareVersion?: string;
  switchGroupId?: string;
  switchGroupName?: string;
  domainId?: string;
  uptime?: number;
  numOfUnits?: number;
  /** Port counts the summary card would show. */
  portStatusUp?: number;
  portStatusDown?: number;
  portStatusWarning?: number;
  clientCount?: number;
  poeUtilization?: number;
  registrationState?: string;
  lastSeenTime?: number;
}

export interface SwitchPort {
  portIdentifier?: string;
  portName?: string;
  status?: string;
  adminStatus?: string;
  speed?: string;
  vlan?: string;
  poeEnabled?: boolean;
  poeUsage?: number;
  neighbourName?: string;
  /** Set when an AP is hanging off this port, which is the join we want. */
  neighbourMacAddress?: string;
}

export interface SwitchGroup {
  id?: string;
  name?: string;
  description?: string;
  domainId?: string;
}

export function switchesApi(client: SmartZoneClient) {
  return {
    /** One page of switches. Mirrors `apsApi.query`. */
    query(input: BuildCriteriaInput, signal?: AbortSignal) {
      return queryPage<SwitchRow>(client, '/query/switch', input, signal);
    },

    ports(input: BuildCriteriaInput, signal?: AbortSignal) {
      return queryPage<SwitchPort>(client, '/query/switch/port', input, signal);
    },

    get(id: string, signal?: AbortSignal) {
      return client.get<SwitchRow>(withPath('/switches/{id}', { id }), { signal });
    },

    groups(signal?: AbortSignal) {
      return client.listAll<SwitchGroup>('/switchgroups', { signal });
    },
  };
}
