import type { SmartZoneClient } from '../client';
import { alarmsApi } from './alarms';
import { apGroupsApi } from './apGroups';
import { apsApi } from './aps';
import { clientsApi } from './clients';
import { dpskApi } from './dpsk';
import { guestPassesApi } from './guestPasses';
import { switchesApi } from './switches';
import { systemApi } from './system';
import { toolsApi } from './tools';
import { wlanGroupsApi } from './wlanGroups';
import { wlansApi } from './wlans';
import { zonesApi } from './zones';

/**
 * Everything the app can ask a controller, bound to one client.
 *
 * Built per client rather than imported as free functions so that a screen
 * never has to remember which controller it is talking to: it takes the api
 * out of context and calls it.
 */
export function createApi(client: SmartZoneClient) {
  return {
    client,
    system: systemApi(client),
    zones: zonesApi(client),
    aps: apsApi(client),
    apGroups: apGroupsApi(client),
    wlans: wlansApi(client),
    wlanGroups: wlanGroupsApi(client),
    dpsk: dpskApi(client),
    clients: clientsApi(client),
    alarms: alarmsApi(client),
    tools: toolsApi(client),
    guestPasses: guestPassesApi(client),
    /** Reserved for the switching release; see the module's note. */
    switches: switchesApi(client),
  };
}

export type SmartZoneApi = ReturnType<typeof createApi>;

export * from './alarms';
export * from './apGroups';
export * from './aps';
export * from './clients';
export * from './dpsk';
export * from './guestPasses';
export * from './switches';
export * from './system';
export * from './tools';
export * from './wlanGroups';
export * from './wlans';
export * from './zones';
