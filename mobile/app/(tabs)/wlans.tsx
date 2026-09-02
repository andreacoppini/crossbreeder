import React, { useMemo, useState } from 'react';
import { FlatList, RefreshControl, View } from 'react-native';
import { router } from 'expo-router';
import { SmartZoneError, type WlanRow } from '@/api';
import { useWlanList, useZones } from '@/hooks/queries';
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
import { useTheme } from '@/ui/theme';
import { firstNonEmpty, formatBytes, formatCount } from '@/utils/format';

/**
 * Every WLAN on the cluster, across zones.
 *
 * The SSID is the thing an operator is told on the phone ("the guest one is
 * down"), so it leads; the WLAN's configuration name and zone sit under it,
 * because on a multi-zone cluster the same SSID legitimately exists several
 * times over and picking the wrong one is the classic mistake.
 */
export default function WlansScreen() {
  const t = useTheme();
  const [search, setSearch] = useState('');
  const [zoneId, setZoneId] = useState<string | undefined>();

  const zones = useZones();
  const query = useWlanList({ search, zoneId });

  const wlans = useMemo(
    () => query.data?.pages.flatMap((p) => p.list ?? []) ?? [],
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
      <View style={{ paddingHorizontal: t.space.lg, paddingTop: t.space.sm }}>
        <Field
          label=""
          value={search}
          onChangeText={setSearch}
          placeholder="Search SSID or WLAN name"
        />
      </View>

      {zoneOptions.length > 2 ? (
        <View style={{ paddingVertical: t.space.sm }}>
          <ChipBar
            value={zoneId ?? 'all'}
            onChange={(next) => setZoneId(next === 'all' ? undefined : next)}
            options={zoneOptions}
          />
        </View>
      ) : null}

      {query.isError ? (
        <View style={{ padding: t.space.lg }}>
          <ErrorState
            message={
              query.error instanceof SmartZoneError
                ? query.error.displayMessage
                : 'Could not read the WLAN list.'
            }
            onRetry={() => void query.refetch()}
          />
        </View>
      ) : query.isLoading ? (
        <Loading label="Reading WLANs" />
      ) : (
        <FlatList
          data={wlans}
          keyExtractor={(item, i) => `${item.zoneId}-${item.id ?? i}`}
          contentContainerStyle={{ padding: t.space.lg, paddingTop: t.space.sm, gap: t.space.sm }}
          refreshControl={
            <RefreshControl
              refreshing={query.isRefetching && !query.isFetchingNextPage}
              onRefresh={() => void query.refetch()}
            />
          }
          onEndReachedThreshold={0.4}
          onEndReached={() => {
            if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage();
          }}
          ListHeaderComponent={
            <Muted>
              {formatCount(total)} WLAN{total === 1 ? '' : 's'}
            </Muted>
          }
          ListEmptyComponent={
            <EmptyState
              title="No WLANs"
              message={
                search
                  ? 'Nothing matches that search.'
                  : 'This scope has no WLANs configured.'
              }
            />
          }
          ListFooterComponent={
            query.isFetchingNextPage ? <Loading /> : <View style={{ height: t.space.xl }} />
          }
          renderItem={({ item }) => <WlanCard wlan={item} />}
        />
      )}
    </View>
  );
}

function WlanCard({ wlan }: { wlan: WlanRow }) {
  const security = firstNonEmpty(wlan.encryptionMethod, wlan.authMethod);
  const open = /none|open/i.test(security);

  return (
    <Card padded={false}>
      <Row
        title={firstNonEmpty(wlan.ssid, wlan.name)}
        subtitle={`${firstNonEmpty(wlan.name)} · ${firstNonEmpty(wlan.zoneName)}`}
        detail={
          <Muted>
            {formatCount(wlan.clients ?? 0)} clients
            {wlan.traffic != null ? ` · ${formatBytes(wlan.traffic)}` : ''}
            {wlan.vlanId != null ? ` · VLAN ${wlan.vlanId}` : ''}
          </Muted>
        }
        right={<Pill label={security} tone={open ? 'warn' : 'up'} compact />}
        onPress={() =>
          wlan.id && wlan.zoneId
            ? router.push({
                pathname: '/wlan/[zoneId]/[id]',
                params: { zoneId: wlan.zoneId, id: wlan.id },
              })
            : undefined
        }
      />
    </Card>
  );
}
