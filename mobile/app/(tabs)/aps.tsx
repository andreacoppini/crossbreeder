import React, { useMemo, useState } from 'react';
import { FlatList, RefreshControl, View } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { SmartZoneError, type ApRow } from '@/api';
import { useApList, useZones } from '@/hooks/queries';
import {
  Card,
  ChipBar,
  EmptyState,
  ErrorState,
  Field,
  Loading,
  Muted,
  Pill,
  Row,
} from '@/ui/components';
import { apStatusTone, useTheme } from '@/ui/theme';
import { firstNonEmpty, formatCount, formatMac, formatRelative } from '@/utils/format';

type StatusFilter = 'all' | 'Online' | 'Flagged' | 'Offline';

/**
 * The AP list.
 *
 * Search, sorting and paging happen on the controller: a cluster with three
 * thousand APs costs one request per screenful, not a download.
 *
 * Status is the exception, and it is worth being explicit about why. A
 * `STATUS` filter is accepted by a 7.1.1 controller and then matches nothing,
 * so filtering server-side would show an empty list and imply there are no
 * offline APs — the worst possible wrong answer here. Instead, picking a
 * status sorts the controller-side query by status so the ones you want load
 * first, and narrows what has been loaded. The header says how many of the
 * cluster that is, so nobody mistakes a screenful for the whole estate.
 */
export default function ApsScreen() {
  const t = useTheme();
  const params = useLocalSearchParams<{ status?: string; zoneId?: string }>();

  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<StatusFilter>(
    (params.status as StatusFilter) ?? 'all',
  );
  const [zoneId, setZoneId] = useState<string | undefined>(params.zoneId);

  const zones = useZones();
  const query = useApList({
    search,
    zoneId,
    sortColumn: status === 'all' ? 'deviceName' : 'status',
    // "Offline" sorts before "Online" ascending; "Online" needs the reverse.
    sortDir: status === 'Online' ? 'DESC' : 'ASC',
  });

  const loaded = useMemo(
    () => query.data?.pages.flatMap((page) => page.list ?? []) ?? [],
    [query.data],
  );
  const aps = useMemo(
    () => (status === 'all' ? loaded : loaded.filter((ap) => ap.status === status)),
    [loaded, status],
  );
  const total = query.data?.pages[0]?.totalCount ?? 0;

  const zoneOptions = useMemo(
    () => [
      { value: 'all', label: 'All zones' },
      ...(zones.data ?? []).map((z) => ({ value: z.id, label: z.name })),
    ],
    [zones.data],
  );

  return (
    <View style={{ flex: 1 }}>
      <View style={{ paddingHorizontal: t.space.lg, paddingTop: t.space.sm }}>
        <Field
          label=""
          value={search}
          onChangeText={setSearch}
          placeholder="Search name, MAC, model or address"
        />
      </View>

      <View style={{ gap: t.space.sm, paddingVertical: t.space.sm }}>
        <ChipBar<StatusFilter>
          value={status}
          onChange={setStatus}
          options={[
            { value: 'all', label: 'All' },
            { value: 'Online', label: 'Online', tone: 'up' },
            { value: 'Flagged', label: 'Flagged', tone: 'warn' },
            { value: 'Offline', label: 'Offline', tone: 'down' },
          ]}
        />
        {zoneOptions.length > 2 ? (
          <ChipBar
            value={zoneId ?? 'all'}
            onChange={(next) => setZoneId(next === 'all' ? undefined : next)}
            options={zoneOptions}
          />
        ) : null}
      </View>

      {query.isError ? (
        <View style={{ padding: t.space.lg }}>
          <ErrorState
            message={
              query.error instanceof SmartZoneError
                ? query.error.displayMessage
                : 'Could not read the AP list.'
            }
            onRetry={() => void query.refetch()}
          />
        </View>
      ) : query.isLoading ? (
        <Loading label="Reading access points" />
      ) : (
        <FlatList
          data={aps}
          keyExtractor={(item, i) => item.apMac ?? String(i)}
          contentContainerStyle={{ padding: t.space.lg, paddingTop: 0, gap: t.space.sm }}
          refreshControl={
            <RefreshControl
              refreshing={query.isRefetching && !query.isFetchingNextPage}
              onRefresh={() => void query.refetch()}
            />
          }
          onEndReachedThreshold={0.4}
          onEndReached={() => {
            if (query.hasNextPage && !query.isFetchingNextPage) {
              void query.fetchNextPage();
            }
          }}
          ListHeaderComponent={
            <Muted>
              {status === 'all'
                ? `${formatCount(total)} access point${total === 1 ? '' : 's'}`
                : `${formatCount(aps.length)} ${status.toLowerCase()} of ${formatCount(
                    loaded.length,
                  )} loaded · ${formatCount(total)} in this scope`}
            </Muted>
          }
          ListEmptyComponent={
            <EmptyState
              title={status === 'all' ? 'Nothing matches' : `No ${status.toLowerCase()} APs loaded`}
              message={
                status === 'all'
                  ? search
                    ? 'No access point matches that search in this scope.'
                    : 'This scope has no access points.'
                  : 'Scroll to load more of the cluster, or use the Overview for exact counts.'
              }
            />
          }
          ListFooterComponent={
            query.isFetchingNextPage ? (
              <Loading />
            ) : query.hasNextPage ? (
              <View style={{ padding: t.space.md, alignItems: 'center' }}>
                <Muted>Scroll for more</Muted>
              </View>
            ) : (
              <View style={{ height: t.space.xl }} />
            )
          }
          renderItem={({ item }) => <ApCard ap={item} />}
        />
      )}
    </View>
  );
}

function ApCard({ ap }: { ap: ApRow }) {
  const tone = apStatusTone(ap.status);
  return (
    <Card padded={false}>
      <Row
        tone={tone}
        title={firstNonEmpty(ap.deviceName, ap.apMac)}
        subtitle={`${firstNonEmpty(ap.model)} · ${formatMac(ap.apMac)}`}
        detail={
          <Muted>
            {firstNonEmpty(ap.zoneName)}
            {ap.apGroupName ? ` · ${ap.apGroupName}` : ''}
            {ap.status === 'Online'
              ? ` · ${formatCount(ap.numClients ?? 0)} clients`
              : ` · last seen ${formatRelative(ap.lastSeen)}`}
          </Muted>
        }
        right={<Pill label={ap.status ?? 'Unknown'} tone={tone} compact />}
        onPress={() =>
          ap.apMac
            ? router.push({ pathname: '/ap/[mac]', params: { mac: ap.apMac } })
            : undefined
        }
      />
    </Card>
  );
}
