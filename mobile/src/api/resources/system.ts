import type { SmartZoneClient } from '../client';
import { runQuery } from '../query';

/**
 * `GET /system/devicesSummary`, as a 7.1.1 cluster actually returns it.
 *
 * Worth being explicit about what this endpoint is *not*: it carries no
 * online/offline/flagged breakdown. On the cluster this was verified against
 * it reported `aps: 287` while `POST /query/ap` found 549 APs online, so the
 * two are counting different things and neither is a health figure. Anything
 * that wants a status breakdown has to count rows — see `apsApi.statusCounts`.
 *
 * What it is good for is inventory and licensing: how many devices exist, and
 * how much of the cluster's capacity is spent.
 */
export interface DevicesSummary {
  /** Registered APs and switches on the cluster. */
  totalAps?: number;
  totalSwitches?: number;
  /** A narrower count whose meaning varies by build; do not read as health. */
  aps?: number;
  switches?: number;
  dualRadioAps?: number;
  triRadioAps?: number;
  /** Licensed capacity and what is left of it. */
  apCapacity?: number;
  switchCapacity?: number;
  totalApCapacity?: number;
  totalSwitchCapacity?: number;
  totalRemainingApCapacity?: number;
  totalRemainingSwitchCapacity?: number;
  maxApOfCluster?: number;
  maxSwitchOfCluster?: number;
  /** Data planes. */
  totalConnectedDps?: number;
  totalRemainingDps?: number;
  totalDpCapacity?: number;
}

export interface SystemSummary {
  clusterName?: string;
  version?: string;
  [key: string]: unknown;
}

export interface ControlPlane {
  id?: string;
  name?: string;
  model?: string;
  serialNumber?: string;
  version?: string;
  managementIp?: string;
  uptime?: number;
}

export function systemApi(client: SmartZoneClient) {
  return {
    devicesSummary(signal?: AbortSignal) {
      return client.get<DevicesSummary>('/system/devicesSummary', { signal });
    },

    system(signal?: AbortSignal) {
      return client.get<SystemSummary>('/system', { signal });
    },

    inventory(signal?: AbortSignal) {
      return client.get<Record<string, unknown>>('/system/inventory', { signal });
    },

    controlPlanes(signal?: AbortSignal) {
      return client.get<{ list?: ControlPlane[] }>('/controller', { signal });
    },

    /** Free-form system query. */
    query<T>(criteria: Parameters<typeof runQuery>[2], signal?: AbortSignal) {
      return runQuery<T>(client, '/system/query', criteria, signal);
    },
  };
}

/** How much AP capacity is spoken for, as a percentage. */
export function apCapacityUsed(summary: DevicesSummary | undefined): number | undefined {
  const capacity = summary?.totalApCapacity;
  const remaining = summary?.totalRemainingApCapacity;
  if (!capacity || remaining == null) return undefined;
  return Math.round(((capacity - remaining) / capacity) * 100);
}
