import React, { useCallback, useMemo, useState } from 'react';
import { Alert, FlatList, RefreshControl, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import * as Sharing from 'expo-sharing';
import { File, Paths } from 'expo-file-system';
import { SmartZoneError, dpskToCsv, expiryDate, type Dpsk } from '@/api';
import { useApi } from '@/controllers/ControllerProvider';
import { useDpskList, useDpskMutations, useZones } from '@/hooks/queries';
import {
  Button,
  Card,
  ChipBar,
  EmptyState,
  ErrorState,
  Field,
  Group,
  Label,
  Loading,
  Muted,
  Pill,
  Row,
  Stat,
} from '@/ui/components';
import { useTheme } from '@/ui/theme';
import {
  firstNonEmpty,
  formatCount,
  formatDateTime,
  formatMac,
  formatRelative,
} from '@/utils/format';

/**
 * Dynamic PSKs.
 *
 * The shape of this screen is dictated by one fact about SmartZone:
 * **a DPSK passphrase is write-only.** Neither the WLAN's DPSK endpoint nor
 * `POST /query/dpsk` returns one — verified against a 7.1.1 cluster, where a
 * key row carries a `key` UUID and no passphrase at all.
 *
 * So there is no "reveal" here, and no passphrase column in the export. The
 * only moment a passphrase can be known is when it is created, which is why
 * the generate screen shows them once and offers to copy them, and why it
 * lets an operator choose the passphrase rather than have the controller
 * invent one they can never read back.
 */
export default function DpskScreen() {
  const t = useTheme();
  const params = useLocalSearchParams<{ zoneId?: string; wlanId?: string }>();
  const api = useApi();

  const [search, setSearch] = useState('');
  const [zoneId, setZoneId] = useState<string | undefined>(params.zoneId || undefined);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);

  const zones = useZones();
  const query = useDpskList({ search, zoneId, wlanId: params.wlanId });
  const { revoke } = useDpskMutations();

  const keys = useMemo(
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

  const confirmRevoke = useCallback(
    (dpsk: Dpsk) => {
      if (!dpsk.key || !dpsk.wlanId || !dpsk.zoneId) {
        Alert.alert(
          'Cannot revoke this key',
          'The controller did not say which WLAN it belongs to.',
        );
        return;
      }
      Alert.alert(
        'Revoke this key?',
        `${firstNonEmpty(dpsk.userName)} will be disconnected and cannot rejoin with this passphrase. It cannot be undone: the passphrase is not readable, so the key cannot be recreated as it was.`,
        [
          { text: 'Cancel', style: 'cancel' },
          {
            text: 'Revoke',
            style: 'destructive',
            onPress: async () => {
              try {
                await revoke.mutateAsync({
                  zoneId: dpsk.zoneId!,
                  wlanId: dpsk.wlanId!,
                  ids: [dpsk.key!],
                });
              } catch (err) {
                Alert.alert(
                  'Could not revoke',
                  err instanceof SmartZoneError ? err.displayMessage : 'The controller refused.',
                );
              }
            },
          },
        ],
      );
    },
    [revoke],
  );

  /**
   * Export what the current filter matches. No passphrases, because the
   * controller does not have them to give; a column of blanks would read as
   * "these keys have no passphrase", which is worse than no column.
   */
  const exportCsv = useCallback(async () => {
    setExporting(true);
    try {
      const all = await api.dpsk.queryAll({
        search,
        filters: zoneId ? [{ type: 'ZONE', value: zoneId }] : undefined,
      });
      const csv = dpskToCsv(all);
      const file = new File(Paths.cache, `dpsk-export-${Date.now()}.csv`);
      file.create({ overwrite: true });
      file.write(csv);
      if (await Sharing.isAvailableAsync()) {
        await Sharing.shareAsync(file.uri, {
          mimeType: 'text/csv',
          dialogTitle: 'Export dynamic PSKs',
        });
      } else {
        Alert.alert('Export ready', `Written to ${file.uri}`);
      }
    } catch (err) {
      Alert.alert(
        'Could not export',
        err instanceof SmartZoneError ? err.displayMessage : 'The export failed.',
      );
    } finally {
      setExporting(false);
    }
  }, [api.dpsk, search, zoneId]);

  return (
    <View style={{ flex: 1 }}>
      <Stack.Screen options={{ title: 'Dynamic PSKs' }} />

      <View style={{ paddingHorizontal: t.space.lg, paddingTop: t.space.sm }}>
        <Field
          label=""
          value={search}
          onChangeText={setSearch}
          placeholder="Search by user name"
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
                : 'Could not read the keys.'
            }
            onRetry={() => void query.refetch()}
          />
        </View>
      ) : query.isLoading ? (
        <Loading label="Reading keys" />
      ) : (
        <FlatList
          data={keys}
          keyExtractor={(item, i) => item.key ?? String(i)}
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
            <View style={{ gap: t.space.sm }}>
              <Muted>
                {formatCount(total)} key{total === 1 ? '' : 's'} in this scope
              </Muted>
              <Button
                title="Generate keys"
                onPress={() =>
                  router.push({ pathname: '/dpsk/generate', params: { zoneId: zoneId ?? '' } })
                }
              />
              <Card style={{ gap: 4 }}>
                <Label variant="subhead">Passphrases are write-only</Label>
                <Muted>
                  SmartZone will not read a DPSK passphrase back, so it can be
                  seen only at the moment it is created. Set your own when you
                  generate a key if you will need it again.
                </Muted>
              </Card>
            </View>
          }
          ListEmptyComponent={
            <EmptyState
              title="No keys"
              message={
                search
                  ? 'Nothing matches that search.'
                  : 'No dynamic PSKs have been issued in this scope.'
              }
            />
          }
          ListFooterComponent={
            query.isFetchingNextPage ? (
              <Loading />
            ) : keys.length > 0 ? (
              <View style={{ paddingTop: t.space.md, gap: t.space.sm }}>
                <Button
                  title="Export this list as CSV"
                  variant="secondary"
                  loading={exporting}
                  onPress={() => void exportCsv()}
                />
                <Muted>
                  Names, WLANs, VLANs and expiry. No passphrases, because the
                  controller does not have them to give.
                </Muted>
              </View>
            ) : null
          }
          renderItem={({ item }) => (
            <DpskCard
              dpsk={item}
              expanded={!!item.key && expanded === item.key}
              onToggle={() => setExpanded((prev) => (prev === item.key ? null : item.key ?? null))}
              onRevoke={() => confirmRevoke(item)}
            />
          )}
        />
      )}
    </View>
  );
}

function DpskCard({
  dpsk,
  expanded,
  onToggle,
  onRevoke,
}: {
  dpsk: Dpsk;
  expanded: boolean;
  onToggle: () => void;
  onRevoke: () => void;
}) {
  const t = useTheme();
  const expiry = expiryDate(dpsk);

  return (
    <Card padded={false}>
      <Row
        tone={dpsk.expired ? 'down' : 'up'}
        title={firstNonEmpty(dpsk.userName)}
        subtitle={
          dpsk.ueMac ? `Bound to ${formatMac(dpsk.ueMac)}` : 'Not bound to a device'
        }
        detail={
          <Muted>
            {dpsk.vlanId != null ? `VLAN ${dpsk.vlanId} · ` : ''}
            {expiry ? `expires ${formatRelative(expiry.getTime())}` : 'no expiry'}
            {dpsk.group ? ' · shared key' : ''}
          </Muted>
        }
        right={
          dpsk.expired ? (
            <Pill label="Expired" tone="down" compact />
          ) : dpsk.group ? (
            <Pill label="Shared" tone="neutral" compact />
          ) : null
        }
        onPress={onToggle}
      />
      {expanded ? (
        <View style={{ paddingBottom: t.space.md }}>
          <Group>
            <Stat label="Created" value={formatDateTime(dpsk.createDateTime)} />
            <Stat label="Expires" value={expiry ? formatDateTime(expiry.getTime()) : 'Never'} />
            <Stat label="VLAN" value={dpsk.vlanId ?? '—'} />
            <Stat label="Device" value={dpsk.ueMac ? formatMac(dpsk.ueMac) : 'Any'} mono />
            <Stat label="WLAN id" value={firstNonEmpty(dpsk.wlanId)} mono />
          </Group>
          <View style={{ paddingHorizontal: t.space.lg, paddingTop: t.space.md }}>
            <Button title="Revoke this key" variant="destructive" onPress={onRevoke} />
          </View>
        </View>
      ) : null}
    </Card>
  );
}
