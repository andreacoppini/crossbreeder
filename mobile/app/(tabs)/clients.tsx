import React, { useMemo, useState } from 'react';
import { FlatList, RefreshControl, View } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import {
  SmartZoneError,
  bandForClient,
  sessionDuration,
  signalVerdict,
  type ClientRow,
} from '@/api';
import { useClientList, useZones } from '@/hooks/queries';
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
import { useTheme, type StatusTone } from '@/ui/theme';
import {
  firstNonEmpty,
  formatBand,
  formatCount,
  formatDuration,
  formatMac,
  formatRssi,
} from '@/utils/format';

type SortKey = 'hostname' | 'rssi' | 'sessionDuration';

const VERDICT_TONE: Record<string, StatusTone> = {
  good: 'up',
  fair: 'warn',
  poor: 'down',
  unknown: 'neutral',
};

/**
 * Every connected client, searchable by whatever the person on the phone
 * actually has: a hostname, a MAC, a username, an IP.
 *
 * Sorting by signal, worst first, is the one that gets used: it turns "the
 * wifi is bad in the east wing" into a list of the clients that agree.
 */
export default function ClientsScreen() {
  const t = useTheme();
  const params = useLocalSearchParams<{ apMac?: string; ssid?: string }>();

  const [search, setSearch] = useState('');
  const [zoneId, setZoneId] = useState<string | undefined>();
  const [sort, setSort] = useState<SortKey>('hostname');

  const zones = useZones();
  const query = useClientList({
    search,
    zoneId,
    apMac: params.apMac,
    ssid: params.ssid,
    sortColumn: sort,
    // Weakest signal first is the useful direction; everything else reads
    // better ascending. `sessionDuration` is accepted as a sort column even
    // though the controller does not return it as a field.
    sortDir: sort === 'rssi' ? 'ASC' : sort === 'sessionDuration' ? 'DESC' : 'ASC',
  });

  const clients = useMemo(
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
          placeholder="Search hostname, MAC, user or IP"
        />
      </View>

      <View style={{ gap: t.space.sm, paddingVertical: t.space.sm }}>
        <ChipBar<SortKey>
          value={sort}
          onChange={setSort}
          options={[
            { value: 'hostname', label: 'By name' },
            { value: 'rssi', label: 'Weakest signal' },
            { value: 'sessionDuration', label: 'Longest session' },
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
                : 'Could not read the client list.'
            }
            onRetry={() => void query.refetch()}
          />
        </View>
      ) : query.isLoading ? (
        <Loading label="Reading clients" />
      ) : (
        <FlatList
          data={clients}
          keyExtractor={(item, i) => item.clientMac ?? String(i)}
          contentContainerStyle={{ padding: t.space.lg, paddingTop: 0, gap: t.space.sm }}
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
              {formatCount(total)} client{total === 1 ? '' : 's'}
              {params.apMac ? ` on ${formatMac(params.apMac)}` : ''}
            </Muted>
          }
          ListEmptyComponent={
            <EmptyState
              title="No clients"
              message={
                search ? 'Nothing matches that search.' : 'Nothing is associated in this scope.'
              }
            />
          }
          ListFooterComponent={
            query.isFetchingNextPage ? <Loading /> : <View style={{ height: t.space.xl }} />
          }
          renderItem={({ item }) => <ClientCard client={item} />}
        />
      )}
    </View>
  );
}

function ClientCard({ client }: { client: ClientRow }) {
  const verdict = signalVerdict(client);
  const tone = VERDICT_TONE[verdict] ?? 'neutral';
  const duration = sessionDuration(client);

  return (
    <Card padded={false}>
      <Row
        tone={tone}
        title={firstNonEmpty(client.hostname, client.userName, formatMac(client.clientMac))}
        subtitle={`${firstNonEmpty(client.ssid)} · ${firstNonEmpty(client.apName)}`}
        detail={
          <Muted>
            {bandForClient(client) ?? 'Band unknown'} · {formatRssi(client.rssi)}
            {duration != null ? ` · ${formatDuration(duration)}` : ''}
          </Muted>
        }
        right={<Pill label={verdict} tone={tone} compact />}
        onPress={() =>
          client.clientMac
            ? router.push({ pathname: '/client/[mac]', params: { mac: client.clientMac } })
            : undefined
        }
      />
    </Card>
  );
}
