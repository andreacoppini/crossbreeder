import type { SmartZoneClient } from '../client';
import { withPath } from '../client';
import type { NamedRef, SmartZoneList } from '../types';

/** A zone as the list endpoint returns it. */
export interface ZoneSummary {
  id: string;
  name: string;
  domainId?: string;
  domainName?: string;
}

/** The parts of a zone this app reads or edits. */
export interface Zone extends ZoneSummary {
  description?: string;
  countryCode?: string;
  timezone?: { systemTimezone?: string } | string;
  version?: string;
  apLoginName?: string;
  mesh?: { enabled?: boolean };
  login?: { apLoginName?: string };
  /** AP firmware the zone pins its members to. */
  apFirmware?: string;
}

export interface ZoneFirmware {
  firmwareVersion?: string;
}

export interface AvailableFirmware {
  list?: { version?: string; supportedApModels?: string[] }[];
}

export function zonesApi(client: SmartZoneClient) {
  return {
    list(signal?: AbortSignal) {
      return client.listAll<ZoneSummary>('/rkszones', { signal, pageSize: 250 });
    },

    page(index: number, listSize: number, signal?: AbortSignal) {
      return client.list<ZoneSummary>('/rkszones', { index, listSize }, { signal });
    },

    get(zoneId: string, signal?: AbortSignal) {
      return client.get<Zone>(withPath('/rkszones/{id}', { id: zoneId }), {
        signal,
      });
    },

    update(zoneId: string, patch: Partial<Zone>, signal?: AbortSignal) {
      return client.patch<void>(
        withPath('/rkszones/{id}', { id: zoneId }),
        patch,
        { signal },
      );
    },

    /** The AP firmware the zone pins to, and what it could be moved to. */
    firmware(zoneId: string, signal?: AbortSignal) {
      return client.get<ZoneFirmware>(
        withPath('/rkszones/{zoneId}/apFirmware', { zoneId }),
        { signal },
      );
    },

    setFirmware(zoneId: string, firmwareVersion: string, signal?: AbortSignal) {
      return client.put<void>(
        withPath('/rkszones/{zoneId}/apFirmware', { zoneId }),
        { firmwareVersion },
        { signal },
      );
    },

    /** WLAN groups defined in a zone, used when assigning them to radios. */
    wlanGroups(zoneId: string, signal?: AbortSignal) {
      return client.listAll<NamedRef>(
        withPath('/rkszones/{zoneId}/wlangroups', { zoneId }),
        { signal },
      );
    },
  };
}

export type ZoneList = SmartZoneList<ZoneSummary>;
