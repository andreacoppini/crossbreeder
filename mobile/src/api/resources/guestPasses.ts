import type { SmartZoneClient } from '../client';
import { withPath } from '../client';

/**
 * Guest passes: short-lived credentials handed to a visitor, distinct from
 * DPSKs in that they expire on a clock and are issued against a guest-access
 * WLAN rather than a PSK one.
 */
export interface GuestPass {
  id?: string;
  guestName?: string;
  userName?: string;
  password?: string;
  wlanId?: string;
  ssid?: string;
  zoneId?: string;
  /** ISO date. */
  expirationDate?: string;
  createdDate?: string;
  remarks?: string;
  /** How many devices may use the pass. */
  numberOfDevices?: number;
  status?: string;
}

export interface GenerateGuestPassRequest {
  zoneId: string;
  wlanId: string;
  numberOfPasses: number;
  guestName?: string;
  /** `Hours`, `Days`, `Weeks`. */
  durationUnit?: string;
  duration?: number;
  numberOfDevices?: number;
  remarks?: string;
}

export function guestPassesApi(client: SmartZoneClient) {
  return {
    list(criteria: Record<string, unknown> = {}, signal?: AbortSignal) {
      return client.post<{ list?: GuestPass[]; totalCount?: number }>(
        '/identity/guestpassList',
        { page: 1, limit: 50, ...criteria },
        { signal },
      );
    },

    generate(request: GenerateGuestPassRequest, signal?: AbortSignal) {
      return client.post<{ list?: GuestPass[] }>(
        '/identity/guestpass/generate',
        request,
        { signal },
      );
    },

    update(userId: string, patch: Partial<GuestPass>, signal?: AbortSignal) {
      return client.patch<void>(
        withPath('/identity/guestpass/{userId}', { userId }),
        patch,
        { signal },
      );
    },

    remove(userId: string, signal?: AbortSignal) {
      return client.delete<void>(
        withPath('/identity/guestpass/{userId}', { userId }),
        { signal },
      );
    },

    removeMany(userIds: string[], signal?: AbortSignal) {
      return client.delete<void>('/identity/guestpass', {
        body: { idList: userIds },
        signal,
      });
    },
  };
}
