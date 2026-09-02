import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query';
import type { BuildCriteriaInput, QueryFilter } from '@/api';
import { DEFAULT_PAGE_SIZE, SmartZoneError } from '@/api';
import { useControllers } from '@/controllers/ControllerProvider';

/**
 * React Query bindings.
 *
 * Two rules run through all of this. First, every key is namespaced by the
 * controller id, so switching cluster cannot show cluster A's APs under
 * cluster B's name while the request is in flight. Second, live data
 * (APs, clients, alarms) is short-lived and configuration data (zones,
 * groups, WLAN definitions) is not, because a phone on a cellular link
 * should not re-fetch a zone list to draw a filter menu.
 */

const LIVE_MS = 15_000;
const CONFIG_MS = 5 * 60_000;

/** Namespaced key root for the connected controller. */
function useScope() {
  const { activeProfile } = useControllers();
  return activeProfile?.id ?? 'none';
}

function useReadyApi() {
  const { api, state } = useControllers();
  return { api, enabled: state === 'connected' && api != null };
}

/** Do not keep retrying what will not get better by being asked again. */
function retryPolicy(failureCount: number, error: unknown): boolean {
  if (error instanceof SmartZoneError && !error.retryable) return false;
  return failureCount < 2;
}

const shared = {
  retry: retryPolicy,
  refetchOnWindowFocus: false,
} satisfies Partial<UseQueryOptions>;

/* ---------------------------------------------------------------- overview */

export function useDevicesSummary() {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'devicesSummary'],
    enabled,
    staleTime: LIVE_MS,
    refetchInterval: 30_000,
    queryFn: ({ signal }) => api!.system.devicesSummary(signal),
  });
}

export function useAlarmSummary() {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'alarmSummary'],
    enabled,
    staleTime: LIVE_MS,
    queryFn: ({ signal }) => api!.alarms.summary({}, signal),
  });
}

/* ------------------------------------------------------------------- zones */

export function useZones() {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'zones'],
    enabled,
    staleTime: CONFIG_MS,
    queryFn: ({ signal }) => api!.zones.list(signal),
  });
}

export function useZone(zoneId?: string) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'zone', zoneId],
    enabled: enabled && !!zoneId,
    staleTime: CONFIG_MS,
    queryFn: ({ signal }) => api!.zones.get(zoneId!, signal),
  });
}

/* --------------------------------------------------------------------- APs */

export interface ApListInput {
  search?: string;
  zoneId?: string;
  apGroupId?: string;
  status?: string;
  sortColumn?: string;
  sortDir?: 'ASC' | 'DESC';
}

function apFilters(input: ApListInput): QueryFilter[] {
  const filters: QueryFilter[] = [];
  if (input.zoneId) filters.push({ type: 'ZONE', value: input.zoneId });
  if (input.apGroupId) filters.push({ type: 'APGROUP', value: input.apGroupId });
  return filters;
}

/**
 * The AP list, paged from the controller.
 *
 * Infinite rather than offset-paged because the screen is a scroll: the list
 * asks for the next page when the operator reaches the bottom, and nothing
 * larger than one page is ever held for a cluster that may have thousands.
 */
export function useApList(input: ApListInput) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();

  return useInfiniteQuery({
    ...shared,
    queryKey: [scope, 'aps', input],
    enabled,
    staleTime: LIVE_MS,
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) =>
      api!.aps.query(
        {
          page: pageParam,
          pageSize: DEFAULT_PAGE_SIZE,
          search: input.search,
          filters: apFilters(input),
          extraFilters: input.status
            ? [{ type: 'STATUS', value: input.status }]
            : undefined,
          sort: input.sortColumn
            ? { sortColumn: input.sortColumn, dir: input.sortDir ?? 'ASC' }
            : undefined,
        },
        signal,
      ),
    getNextPageParam: (last, pages) => (last.hasMore ? pages.length + 1 : undefined),
  });
}

export function useAp(apMac?: string) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'ap', apMac],
    enabled: enabled && !!apMac,
    staleTime: LIVE_MS,
    refetchInterval: 30_000,
    queryFn: ({ signal }) => api!.aps.operational(apMac!, signal),
  });
}

export function useApConfig(apMac?: string) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'apConfig', apMac],
    enabled: enabled && !!apMac,
    staleTime: CONFIG_MS,
    queryFn: ({ signal }) => api!.aps.config(apMac!, signal),
  });
}

export function useApGroups(zoneId?: string) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'apGroups', zoneId],
    enabled: enabled && !!zoneId,
    staleTime: CONFIG_MS,
    queryFn: ({ signal }) => api!.apGroups.list(zoneId!, signal),
  });
}

/** AP actions, with the lists they invalidate. */
export function useApActions(apMac: string) {
  const scope = useScope();
  const { api } = useReadyApi();
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: [scope, 'ap', apMac] });
    void qc.invalidateQueries({ queryKey: [scope, 'aps'] });
  };

  return {
    reboot: useMutation({
      mutationFn: () => api!.aps.reboot(apMac),
      onSuccess: invalidate,
    }),
    blinkLed: useMutation({
      mutationFn: () => api!.aps.blinkLed(apMac),
    }),
    rename: useMutation({
      mutationFn: (name: string) => api!.aps.update(apMac, { name }),
      onSuccess: invalidate,
    }),
    setDescription: useMutation({
      mutationFn: (description: string) => api!.aps.update(apMac, { description }),
      onSuccess: invalidate,
    }),
  };
}

/* ------------------------------------------------------------------- WLANs */

export function useWlanList(input: { search?: string; zoneId?: string }) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useInfiniteQuery({
    ...shared,
    queryKey: [scope, 'wlans', input],
    enabled,
    staleTime: LIVE_MS,
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) =>
      api!.wlans.query(
        {
          page: pageParam,
          pageSize: DEFAULT_PAGE_SIZE,
          search: input.search,
          filters: input.zoneId ? [{ type: 'ZONE', value: input.zoneId }] : undefined,
        },
        signal,
      ),
    getNextPageParam: (last, pages) => (last.hasMore ? pages.length + 1 : undefined),
  });
}

export function useWlan(zoneId?: string, wlanId?: string) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'wlan', zoneId, wlanId],
    enabled: enabled && !!zoneId && !!wlanId,
    staleTime: CONFIG_MS,
    queryFn: ({ signal }) => api!.wlans.get(zoneId!, wlanId!, signal),
  });
}

export function useWlanMutations(zoneId: string, wlanId: string) {
  const scope = useScope();
  const { api } = useReadyApi();
  const qc = useQueryClient();

  return {
    update: useMutation({
      mutationFn: (patch: Record<string, unknown>) =>
        api!.wlans.update(zoneId, wlanId, patch),
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: [scope, 'wlan', zoneId, wlanId] });
        void qc.invalidateQueries({ queryKey: [scope, 'wlans'] });
      },
    }),
    remove: useMutation({
      mutationFn: () => api!.wlans.remove(zoneId, wlanId),
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: [scope, 'wlans'] });
      },
    }),
  };
}

export function useWlanGroups(zoneId?: string) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'wlanGroups', zoneId],
    enabled: enabled && !!zoneId,
    staleTime: CONFIG_MS,
    queryFn: ({ signal }) => api!.wlanGroups.list(zoneId!, signal),
  });
}

/* ------------------------------------------------------------------ DPSKs */

export function useDpskList(input: { search?: string; zoneId?: string; wlanId?: string }) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useInfiniteQuery({
    ...shared,
    queryKey: [scope, 'dpsks', input],
    enabled,
    staleTime: LIVE_MS,
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) =>
      api!.dpsk.query(
        {
          page: pageParam,
          pageSize: DEFAULT_PAGE_SIZE,
          search: input.search,
          filters: [
            ...(input.zoneId ? [{ type: 'ZONE' as const, value: input.zoneId }] : []),
            ...(input.wlanId ? [{ type: 'WLAN' as const, value: input.wlanId }] : []),
          ],
        },
        signal,
      ),
    getNextPageParam: (last, pages) => (last.hasMore ? pages.length + 1 : undefined),
  });
}

export function useDpskEnabledWlans(zoneId?: string) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'dpskWlans', zoneId],
    enabled: enabled && !!zoneId,
    staleTime: CONFIG_MS,
    queryFn: ({ signal }) => api!.dpsk.enabledWlans(zoneId!, signal),
  });
}

export function useDpskMutations() {
  const scope = useScope();
  const { api } = useReadyApi();
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: [scope, 'dpsks'] });

  return {
    generate: useMutation({
      mutationFn: (args: {
        zoneId: string;
        wlanId: string;
        request: Parameters<NonNullable<typeof api>['dpsk']['generate']>[2];
      }) => api!.dpsk.generate(args.zoneId, args.wlanId, args.request),
      onSuccess: () => void invalidate(),
    }),
    revoke: useMutation({
      mutationFn: (args: { zoneId: string; wlanId: string; ids: string[] }) =>
        api!.dpsk.revoke(args.zoneId, args.wlanId, args.ids),
      onSuccess: () => void invalidate(),
    }),
  };
}

/* ---------------------------------------------------------------- clients */

export interface ClientListInput {
  search?: string;
  zoneId?: string;
  apMac?: string;
  ssid?: string;
  sortColumn?: string;
  sortDir?: 'ASC' | 'DESC';
}

export function useClientList(input: ClientListInput) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useInfiniteQuery({
    ...shared,
    queryKey: [scope, 'clients', input],
    enabled,
    staleTime: LIVE_MS,
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) =>
      api!.clients.query(
        {
          page: pageParam,
          pageSize: DEFAULT_PAGE_SIZE,
          search: input.search,
          filters: [
            ...(input.zoneId ? [{ type: 'ZONE' as const, value: input.zoneId }] : []),
            ...(input.apMac ? [{ type: 'AP' as const, value: input.apMac }] : []),
            ...(input.ssid ? [{ type: 'SSID' as const, value: input.ssid }] : []),
          ],
          sort: input.sortColumn
            ? { sortColumn: input.sortColumn, dir: input.sortDir ?? 'ASC' }
            : undefined,
        },
        signal,
      ),
    getNextPageParam: (last, pages) => (last.hasMore ? pages.length + 1 : undefined),
  });
}

export function useClient(mac?: string) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'client', mac],
    enabled: enabled && !!mac,
    // A client's radio numbers move constantly; this screen is watched live.
    staleTime: 5_000,
    refetchInterval: 15_000,
    queryFn: ({ signal }) => api!.clients.byMac(mac!, signal),
  });
}

/** Past sessions for one MAC, the other half of troubleshooting. */
export function useClientHistory(mac?: string) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useQuery({
    ...shared,
    queryKey: [scope, 'clientHistory', mac],
    enabled: enabled && !!mac,
    staleTime: 60_000,
    queryFn: ({ signal }) =>
      api!.clients.history(
        { pageSize: 25, filters: [{ type: 'CLIENT', value: mac! }] },
        signal,
      ),
  });
}

export function useClientActions() {
  const scope = useScope();
  const { api } = useReadyApi();
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: [scope, 'clients'] });

  return {
    disconnect: useMutation({
      mutationFn: (macs: string[]) => api!.clients.disconnect(macs),
      onSuccess: () => void invalidate(),
    }),
    deauth: useMutation({
      mutationFn: (macs: string[]) => api!.clients.deauth(macs),
      onSuccess: () => void invalidate(),
    }),
    block: useMutation({
      mutationFn: (args: { zoneId: string; mac: string; description?: string }) =>
        api!.clients.block(args.zoneId, args.mac, args.description),
      onSuccess: () => void invalidate(),
    }),
  };
}

/* ----------------------------------------------------------------- alarms */

export function useAlarms(input: BuildCriteriaInput = {}) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useInfiniteQuery({
    ...shared,
    queryKey: [scope, 'alarms', input],
    enabled,
    staleTime: LIVE_MS,
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) =>
      api!.alarms.list({ ...input, page: pageParam, pageSize: 30 }, signal),
    getNextPageParam: (last, pages) => (last.hasMore ? pages.length + 1 : undefined),
  });
}

export function useEvents(input: BuildCriteriaInput = {}) {
  const scope = useScope();
  const { api, enabled } = useReadyApi();
  return useInfiniteQuery({
    ...shared,
    queryKey: [scope, 'events', input],
    enabled,
    staleTime: LIVE_MS,
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) =>
      api!.alarms.events({ ...input, page: pageParam, pageSize: 30 }, signal),
    getNextPageParam: (last, pages) => (last.hasMore ? pages.length + 1 : undefined),
  });
}

export function useAlarmActions() {
  const scope = useScope();
  const { api } = useReadyApi();
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: [scope, 'alarms'] });
    void qc.invalidateQueries({ queryKey: [scope, 'alarmSummary'] });
  };

  return {
    acknowledge: useMutation({
      mutationFn: (id: string) => api!.alarms.acknowledge(id),
      onSuccess: invalidate,
    }),
    clear: useMutation({
      mutationFn: (id: string) => api!.alarms.clear(id),
      onSuccess: invalidate,
    }),
  };
}
