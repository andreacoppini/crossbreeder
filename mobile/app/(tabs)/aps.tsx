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
 * Search, filtering and paging all happen on the controller: a cluster with
 * three thousand APs costs one request per screenful, not a download. What
 * the row shows is chosen for a phone held at arm's length — name, status,
 * client count — with the identifying detail (MAC, model, zone) underneath.
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
    status: status === 'all' ? undefined : status,
    sortColumn: 'deviceName',
  });

  const aps = useMemo(
    () => query.data?.pages.flatMap((page) => page.list ?? []) ?? [],
    [query.data],
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
      <View style={{ paddingHorizontal: t.space.lg, paddingTop: t.space.sm, gap: t.space.sm }}>
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
              {formatCount(total)} access point{total === 1 ? '' : 's'}
            </Muted>
          }
          ListEmptyComponent={
            <EmptyState
              title="Nothing matches"
              message={
                search
                  ? 'No access point matches that search in this scope.'
                  : 'This scope has no access points.'
              }
            />
          }
          ListFooterComponent={
            query.isFetchingNextPage ? <Loading /> : <View style={{ height: t.space.xl }} />
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
              : ` · last seen ${formatRelative(ap.lastSeenTime)}`}
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
