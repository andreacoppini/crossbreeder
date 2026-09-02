/**
 * ICX switch management.
 *
 * Deliberately a stub, and now an honest one. Switching is the next thing
 * this app grows into, and the shape of that growth is settled here so it
 * disturbs nothing later: the switch surface hangs off the same client, the
 * same session and the same query builder as Wi-Fi.
 *
 * **The endpoint paths below are unverified.** Every candidate was probed
 * against a SmartZone 7.1.1 cluster that manages 43 switches, and all of them
 * answered 404: `/query/switch`, `/query/switches`, `/query/switchport`,
 * `/query/switch/port`, `/switches`, `/switchgroups`, `/switchm/*`. So the
 * switch API is either not on the `/wsg/api/public` tree at all on this
 * release, or it needs a scope this admin account does not carry.
 *
 * Rather than ship paths that are known not to work, the calls below are
 * marked and gated: `probe()` finds out what the controller answers before
 * any screen is built on top of it. The types are still worth having — they
 * are what the screens will render — and `SwitchPort.neighbourMacAddress` is
 * the join that makes the interesting screen possible: the AP you are looking
 * at, and the switch port it is powered from.
 */

import type { SmartZoneClient } from '../client';
import { SmartZoneError } from '../errors';
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
  /** Set when an AP hangs off this port. The join worth building on. */
  neighbourMacAddress?: string;
}

export interface SwitchGroup {
  id?: string;
  name?: string;
  description?: string;
  domainId?: string;
}

/** Candidate paths, in the order worth trying. None confirmed yet. */
export const SWITCH_QUERY_CANDIDATES = [
  '/query/switch',
  '/query/switches',
  '/switchm/switch/query',
] as const;

export interface SwitchSupport {
  available: boolean;
  /** The path that answered, once one does. */
  path?: string;
  detail: string;
}

export function switchesApi(client: SmartZoneClient) {
  return {
    /**
     * Find out whether this controller exposes a switch query at all, and
     * where. Cheap, and the honest way to decide whether to show the tab.
     */
    async probe(signal?: AbortSignal): Promise<SwitchSupport> {
      for (const path of SWITCH_QUERY_CANDIDATES) {
        try {
          await queryPage<SwitchRow>(client, path, { pageSize: 1 }, signal);
          return { available: true, path, detail: `Switch data is at ${path}.` };
        } catch (err) {
          if (err instanceof SmartZoneError && err.kind === 'notFound') continue;
          // Anything other than "no such endpoint" — a permission problem,
          // say — is worth reporting rather than swallowing.
          if (err instanceof SmartZoneError) {
            return { available: false, detail: err.displayMessage };
          }
          throw err;
        }
      }
      return {
        available: false,
        detail:
          'This controller does not expose switch management on the public API paths this app knows.',
      };
    },

    /** One page of switches, once `probe` has found a working path. */
    query(path: string, input: BuildCriteriaInput, signal?: AbortSignal) {
      return queryPage<SwitchRow>(client, path, input, signal);
    },
  };
}
