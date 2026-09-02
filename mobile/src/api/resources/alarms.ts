import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import type { QueryCriteria, QueryResult } from '../query';
import { buildCriteria, type BuildCriteriaInput } from '../query';

export type AlarmSeverity = 'Critical' | 'Major' | 'Minor' | 'Warning' | 'Info' | string;

export interface Alarm {
  id?: string;
  alarmId?: string;
  severity?: AlarmSeverity;
  /** Human text the controller composed. */
  activity?: string;
  description?: string;
  category?: string;
  code?: number;
  /** `Outstanding`, `Acknowledged`, `Cleared`. */
  status?: string;
  acknowledged?: boolean;
  entityType?: string;
  entityId?: string;
  entityName?: string;
  apMac?: string;
  zoneName?: string;
  datetime?: number;
  firstAppearTime?: number;
  clearedTime?: number;
  ackTime?: number;
}

export interface SzEvent {
  id?: string;
  eventType?: string;
  category?: string;
  severity?: AlarmSeverity;
  activity?: string;
  description?: string;
  entityName?: string;
  apMac?: string;
  clientMac?: string;
  zoneName?: string;
  datetime?: number;
}

export interface AlarmSummary {
  criticalCount?: number;
  majorCount?: number;
  minorCount?: number;
  warningCount?: number;
  totalCount?: number;
}

export function alarmsApi(client: SmartZoneClient) {
  return {
    list(input: BuildCriteriaInput, signal?: AbortSignal) {
      return client.post<QueryResult<Alarm>>('/alert/alarm/list', buildCriteria(input), {
        signal,
      });
    },

    summary(criteria: QueryCriteria = {}, signal?: AbortSignal) {
      return client.post<AlarmSummary>('/alert/alarmSummary', criteria, { signal });
    },

    events(input: BuildCriteriaInput, signal?: AbortSignal) {
      return client.post<QueryResult<SzEvent>>('/alert/event/list', buildCriteria(input), {
        signal,
      });
    },

    eventSummary(criteria: QueryCriteria = {}, signal?: AbortSignal) {
      return client.post<Record<string, unknown>>('/alert/eventSummary', criteria, {
        signal,
      });
    },

    acknowledge(alarmId: string, signal?: AbortSignal) {
      return client.put<void>(
        withPath('/alert/alarm/{alarmID}/ack', { alarmID: alarmId }),
        undefined,
        { signal },
      );
    },

    acknowledgeMany(alarmIds: string[], signal?: AbortSignal) {
      return client.put<void>('/alert/alarm/ack', { idList: alarmIds }, { signal });
    },

    clear(alarmId: string, signal?: AbortSignal) {
      return client.put<void>(
        withPath('/alert/alarm/{alarmID}/clear', { alarmID: alarmId }),
        undefined,
        { signal },
      );
    },

    clearMany(alarmIds: string[], signal?: AbortSignal) {
      return client.put<void>('/alert/alarm/clear', { idList: alarmIds }, { signal });
    },
  };
}

/** Order used everywhere alarms are shown, worst first. */
export const SEVERITY_ORDER: Record<string, number> = {
  Critical: 0,
  Major: 1,
  Minor: 2,
  Warning: 3,
  Info: 4,
};

export function compareSeverity(a?: string, b?: string): number {
  const av = SEVERITY_ORDER[a ?? ''] ?? 9;
  const bv = SEVERITY_ORDER[b ?? ''] ?? 9;
  return av - bv;
}
