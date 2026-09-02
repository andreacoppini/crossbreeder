import React, { useCallback, useMemo, useState } from 'react';
import { Alert, FlatList, RefreshControl, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import * as Clipboard from 'expo-clipboard';
import * as Haptics from 'expo-haptics';
import * as Sharing from 'expo-sharing';
import { File, Paths } from 'expo-file-system';
import { SmartZoneError, dpskToCsv, type Dpsk } from '@/api';
import { useApi } from '@/controllers/ControllerProvider';
import { useDpskList, useDpskMutations, useZones } from '@/hooks/queries';
import {
  Button,
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
import { firstNonEmpty, formatCount, formatRelative } from '@/utils/format';

/**
 * Dynamic PSKs.
 *
 * The two things this screen exists for are issuing a key and handing it to
 * somebody. A passphrase is masked until tapped, because this list gets held
 * up in a room with other people in it, and copying one is a single tap
 * because the alternative is reading sixteen random characters aloud.
 */
export default function DpskScreen() {
  const t = useTheme();
  const params = useLocalSearchParams<{ zoneId?: string; wlanId?: string }>();
  const api = useApi();

  const [search, setSearch] = useState('');
  const [zoneId, setZoneId] = useState<string | undefined>(params.zoneId);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
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

  const copyKey = useCallback(async (dpsk: Dpsk) => {
    if (!dpsk.passphrase) return;
    await Clipboard.setStringAsync(dpsk.passphrase);
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    Alert.alert('Copied', `The passphrase for ${firstNonEmpty(dpsk.userName)} is on the clipboard.`);
  }, []);

  const confirmRevoke = useCallback(
    (dpsk: Dpsk) => {
      if (!dpsk.id || !dpsk.wlanId || !dpsk.zoneId) {
        Alert.alert(
          'Cannot revoke from here',
          'The controller did not say which WLAN this key belongs to. Open it from its WLAN instead.',
        );
        return;
      }
      Alert.alert(
        'Revoke this key?',
        `${firstNonEmpty(dpsk.userName)} will be disconnected and will not be able to rejoin with this passphrase.`,
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
                  ids: [dpsk.id!],
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
   * Export what the current filter matches, as a CSV.
   *
   * Written to the app's own cache and handed to the share sheet rather than
   * saved anywhere: these are live credentials, and they should leave through
   * a deliberate choice about where they go.
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
          placeholder="Search by user, SSID or MAC"
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
          keyExtractor={(item, i) => item.id ?? String(i)}
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
                {formatCount(total)} key{total === 1 ? '' : 's'}
              </Muted>
              <Button
                title="Generate keys"
                onPress={() =>
                  router.push({ pathname: '/dpsk/generate', params: { zoneId: zoneId ?? '' } })
                }
              />
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
              <View style={{ paddingTop: t.space.md }}>
                <Button
                  title="Export these keys as CSV"
                  variant="secondary"
                  loading={exporting}
                  onPress={() => void exportCsv()}
                />
                <View style={{ paddingTop: t.space.sm }}>
                  <Muted>
                    The export carries live passphrases. It goes to the share
                    sheet, not to storage, so you choose where it lands.
                  </Muted>
                </View>
              </View>
            ) : null
          }
          renderItem={({ item }) => (
            <DpskCard
              dpsk={item}
              revealed={!!(item.id && revealed[item.id])}
              onToggle={() =>
                item.id &&
                setRevealed((prev) => ({ ...prev, [item.id!]: !prev[item.id!] }))
              }
              onCopy={() => void copyKey(item)}
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
  revealed,
  onToggle,
  onCopy,
  onRevoke,
}: {
  dpsk: Dpsk;
  revealed: boolean;
  onToggle: () => void;
  onCopy: () => void;
  onRevoke: () => void;
}) {
  const t = useTheme();
  const expired = /expired/i.test(dpsk.status ?? '');
  const used = dpsk.numberOfDevicesUsed ?? 0;
  const limit = dpsk.deviceCountLimit;

  return (
    <Card padded={false}>
      <Row
        tone={expired ? 'down' : 'up'}
        title={firstNonEmpty(dpsk.userName)}
        subtitle={`${firstNonEmpty(dpsk.ssid, dpsk.wlanName)}${dpsk.vlanId != null ? ` · VLAN ${dpsk.vlanId}` : ''}`}
        detail={
          <Muted>
            {limit ? `${used} of ${limit} devices` : `${used} devices`}
            {dpsk.expirationDate ? ` · expires ${formatRelative(Date.parse(dpsk.expirationDate))}` : ' · no expiry'}
          </Muted>
        }
        right={expired ? <Pill label="Expired" tone="down" compact /> : null}
        onPress={onToggle}
      />
      {revealed ? (
        <View style={{ paddingHorizontal: t.space.lg, paddingBottom: t.space.lg, gap: t.space.sm }}>
          <Card style={{ backgroundColor: t.colors.background }}>
            <Muted>Passphrase</Muted>
            <Row title={dpsk.passphrase ?? 'Not returned by the controller'} />
          </Card>
          <View style={{ flexDirection: 'row', gap: t.space.sm }}>
            <Button
              title="Copy"
              variant="secondary"
              style={{ flex: 1 }}
              disabled={!dpsk.passphrase}
              onPress={onCopy}
            />
            <Button
              title="Revoke"
              variant="destructive"
              style={{ flex: 1 }}
              onPress={onRevoke}
            />
          </View>
        </View>
      ) : null}
    </Card>
  );
}
