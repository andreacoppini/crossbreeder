import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import type { QueryCriteria, QueryResult } from '../query';
import { buildCriteria, type BuildCriteriaInput } from '../query';

export type AlarmSeverity =
  | 'Critical'
  | 'Major'
  | 'Minor'
  | 'Warning'
  | 'Informational'
  | string;

/**
 * An alarm from `POST /alert/alarm/list`.
 *
 * Two traps here, both verified against 7.1.1. The timestamp is
 * `insertionTime`, not `datetime`. And `acknowledged` is the *string* "Yes"
 * or "No", not a boolean — reading it as truthy marks every open alarm as
 * acknowledged, which is precisely backwards.
 */
export interface Alarm {
  id?: string;
  severity?: AlarmSeverity;
  /** The human sentence the controller composed. */
  activity?: string;
  category?: string;
  alarmType?: string;
  alarmCode?: number;
  /** "Outstanding" or "Cleared". */
  alarmState?: string;
  /** "Yes" or "No". A string. */
  acknowledged?: string;
  ackTime?: number | null;
  ackUser?: string | null;
  clearTime?: number | null;
  clearUser?: string | null;
  clearComment?: string | null;
  /** Milliseconds since the epoch. */
  insertionTime?: number;
}

/** An event from `POST /alert/event/list`. */
export interface SzEvent {
  id?: string;
  severity?: AlarmSeverity;
  activity?: string;
  category?: string;
  eventType?: string;
  eventCode?: number;
  insertionTime?: number;
}

/** `POST /alert/alarmSummary`. There is no total; add the parts. */
export interface AlarmSummary {
  criticalCount?: number;
  majorCount?: number;
  minorCount?: number;
  warningCount?: number;
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

/** The controller sends "Yes"/"No"; anything else means not acknowledged. */
export function isAcknowledged(alarm: Alarm): boolean {
  return /^yes$/i.test(alarm.acknowledged ?? '');
}

export function alarmTotal(summary: AlarmSummary | undefined): number | undefined {
  if (!summary) return undefined;
  return (
    (summary.criticalCount ?? 0) +
    (summary.majorCount ?? 0) +
    (summary.minorCount ?? 0) +
    (summary.warningCount ?? 0)
  );
}

/** Order used everywhere alarms are shown, worst first. */
export const SEVERITY_ORDER: Record<string, number> = {
  Critical: 0,
  Major: 1,
  Minor: 2,
  Warning: 3,
  Informational: 4,
};

export function compareSeverity(a?: string, b?: string): number {
  const av = SEVERITY_ORDER[a ?? ''] ?? 9;
  const bv = SEVERITY_ORDER[b ?? ''] ?? 9;
  return av - bv;
}
