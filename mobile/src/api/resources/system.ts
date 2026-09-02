import type { SmartZoneClient } from '../client';
import { runQuery } from '../query';

/** `GET /system/devicesSummary` — the numbers the dashboard opens with. */
export interface DevicesSummary {
  apTotalCount?: number;
  apOnlineCount?: number;
  apOfflineCount?: number;
  apFlaggedCount?: number;
  apDiscoveryCount?: number;
  switchTotalCount?: number;
  switchOnlineCount?: number;
  switchOfflineCount?: number;
  clientCount?: number;
}

export interface SystemSummary {
  clusterName?: string;
  version?: string;
  controlPlanes?: unknown[];
}

export interface SystemInventory {
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
      return client.get<SystemInventory>('/system/inventory', { signal });
    },

    controlPlanes(signal?: AbortSignal) {
      return client.get<{ list?: ControlPlane[] }>('/controller', { signal });
    },

    /** Who is signed in, and against which domain. */
    currentSession(signal?: AbortSignal) {
      return client.get<Record<string, unknown>>('/session', { signal });
    },

    /** Free-form system query, used by the cluster health card. */
    query<T>(criteria: Parameters<typeof runQuery>[2], signal?: AbortSignal) {
      return runQuery<T>(client, '/system/query', criteria, signal);
    },
  };
}
