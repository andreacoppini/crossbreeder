import React, { useCallback, useMemo, useState } from 'react';
import { Alert, RefreshControl, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import * as Clipboard from 'expo-clipboard';
import * as Haptics from 'expo-haptics';
import { SmartZoneError } from '@/api';
import { useAp, useApActions, useClientList } from '@/hooks/queries';
import {
  Button,
  Card,
  ErrorState,
  Group,
  Label,
  Loading,
  Muted,
  Pill,
  Row,
  Screen,
  Stat,
} from '@/ui/components';
import { apStatusTone, useTheme } from '@/ui/theme';
import {
  firstNonEmpty,
  formatBand,
  formatBytes,
  formatCount,
  formatDuration,
  formatMac,
  formatPercent,
  formatRelative,
  formatRssi,
} from '@/utils/format';

/**
 * One access point.
 *
 * Ordered by what someone standing under the AP wants: is it up and how long
 * has it been up, what is on it, then the identity and the radios. The
 * actions are at the bottom because two of them are disruptive and neither
 * should be the first thing a thumb lands on.
 */
export default function ApDetailScreen() {
  const t = useTheme();
  const { mac } = useLocalSearchParams<{ mac: string }>();
  const ap = useAp(mac);
  const actions = useApActions(mac ?? '');
  const clients = useClientList({ apMac: mac });

  const [busy, setBusy] = useState<string | null>(null);

  const clientRows = useMemo(
    () => clients.data?.pages.flatMap((p) => p.list ?? []) ?? [],
    [clients.data],
  );

  const copy = useCallback(async (value: string, what: string) => {
    await Clipboard.setStringAsync(value);
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    Alert.alert('Copied', `${what} copied to the clipboard.`);
  }, []);

  const confirmReboot = useCallback(() => {
    Alert.alert(
      'Reboot this access point?',
      `${firstNonEmpty(ap.data?.deviceName, mac)} will drop every client on it and take a minute or two to come back.`,
      [
        { text: 'Cancel', style: 'cancel' },
        {
          text: 'Reboot',
          style: 'destructive',
          onPress: async () => {
            setBusy('reboot');
            try {
              await actions.reboot.mutateAsync();
              void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
              Alert.alert('Rebooting', 'The controller has asked the AP to restart.');
            } catch (err) {
              Alert.alert(
                'Could not reboot',
                err instanceof SmartZoneError ? err.displayMessage : 'The controller refused.',
              );
            } finally {
              setBusy(null);
            }
          },
        },
      ],
    );
  }, [actions.reboot, ap.data?.deviceName, mac]);

  const blink = useCallback(async () => {
    setBusy('blink');
    try {
      await actions.blinkLed.mutateAsync();
      Alert.alert('Blinking', 'The AP’s LEDs are flashing so you can find it.');
    } catch (err) {
      Alert.alert(
        'Could not blink the LEDs',
        err instanceof SmartZoneError ? err.displayMessage : 'The controller refused.',
      );
    } finally {
      setBusy(null);
    }
  }, [actions.blinkLed]);

  if (ap.isLoading) {
    return (
      <Screen>
        <Stack.Screen options={{ title: 'Access point' }} />
        <Loading label="Reading the access point" />
      </Screen>
    );
  }

  if (ap.isError || !ap.data) {
    return (
      <Screen scroll>
        <Stack.Screen options={{ title: 'Access point' }} />
        <ErrorState
          message={
            ap.error instanceof SmartZoneError
              ? ap.error.displayMessage
              : 'That access point could not be read.'
          }
          onRetry={() => void ap.refetch()}
        />
      </Screen>
    );
  }

  const data = ap.data;
  const tone = apStatusTone(data.status);

  return (
    <Screen
      scroll
      refreshControl={
        <RefreshControl refreshing={ap.isRefetching} onRefresh={() => void ap.refetch()} />
      }
    >
      <Stack.Screen options={{ title: firstNonEmpty(data.deviceName, mac) }} />

      <Card style={{ gap: t.space.md }}>
        <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' }}>
          <View style={{ flex: 1, gap: 2 }}>
            <Label variant="title" numberOfLines={2}>
              {firstNonEmpty(data.deviceName, mac)}
            </Label>
            <Muted>{firstNonEmpty(data.model)}</Muted>
          </View>
          <Pill label={data.status ?? 'Unknown'} tone={tone} />
        </View>

        <View style={{ flexDirection: 'row', gap: t.space.xl }}>
          <View>
            <Label variant="title">{formatCount(data.numClients ?? 0)}</Label>
            <Muted>Clients</Muted>
          </View>
          <View>
            <Label variant="title">{formatDuration(data.uptime)}</Label>
            <Muted>Uptime</Muted>
          </View>
          <View>
            <Label variant="title">{formatBytes((data.tx ?? 0) + (data.rx ?? 0))}</Label>
            <Muted>Traffic</Muted>
          </View>
        </View>

        {data.status !== 'Online' ? (
          <Muted>Last seen {formatRelative(data.lastSeenTime)}.</Muted>
        ) : null}
      </Card>

      <Group header="Identity">
        <Stat label="MAC" value={formatMac(data.apMac ?? mac)} mono />
        <Stat label="Serial" value={firstNonEmpty(data.serial)} mono />
        <Stat label="Address" value={firstNonEmpty(data.ip, data.ipv6Address)} mono />
        <Stat label="Firmware" value={firstNonEmpty(data.firmwareVersion)} />
        <Stat label="Zone" value={firstNonEmpty(data.zoneName)} />
        <Stat label="AP group" value={firstNonEmpty(data.apGroupName)} />
        <Stat label="Location" value={firstNonEmpty(data.location)} />
      </Group>

      <Group header="Radios">
        <RadioStat
          band="2.4G"
          channel={data.channel24G}
          clients={data.numClients24G}
          airtime={data.airtime24G}
          txPower={data.txPower24G}
        />
        <RadioStat
          band="5G"
          channel={data.channel5G}
          clients={data.numClients5G}
          airtime={data.airtime5G}
          txPower={data.txPower5G}
        />
        {data.channel6G || data.numClients6G != null ? (
          <RadioStat band="6G" channel={data.channel6G} clients={data.numClients6G} />
        ) : null}
      </Group>

      {data.cpuPercentage != null || data.memoryPercentage != null ? (
        <Group header="Health">
          <Stat label="CPU" value={formatPercent(data.cpuPercentage)} />
          <Stat label="Memory" value={formatPercent(data.memoryPercentage)} />
        </Group>
      ) : null}

      <Group
        header="Clients on this AP"
        footer={
          clientRows.length > 0
            ? 'Tap a client to see its signal and session, or to disconnect it.'
            : undefined
        }
      >
        {clients.isLoading ? (
          <Row title="Reading clients…" />
        ) : clientRows.length === 0 ? (
          <Row title="No clients" subtitle="Nothing is associated to this AP" />
        ) : (
          clientRows.slice(0, 8).map((client, i) => (
            <Row
              key={client.clientMac ?? i}
              title={firstNonEmpty(client.hostname, client.userName, client.clientMac)}
              subtitle={`${firstNonEmpty(client.ssid)} · ${formatBand(client.radioType)} · ${formatRssi(client.rssi)}`}
              onPress={() =>
                client.clientMac
                  ? router.push({ pathname: '/client/[mac]', params: { mac: client.clientMac } })
                  : undefined
              }
            />
          ))
        )}
        {clientRows.length > 8 ? (
          <Row
            title={`See all ${formatCount(clients.data?.pages[0]?.totalCount)} clients`}
            onPress={() =>
              router.push({ pathname: '/(tabs)/clients', params: { apMac: mac } })
            }
          />
        ) : null}
      </Group>

      <View style={{ gap: t.space.sm }}>
        <Button
          title="Blink the LEDs"
          variant="secondary"
          loading={busy === 'blink'}
          disabled={busy !== null || data.status !== 'Online'}
          onPress={() => void blink()}
        />
        <Button
          title="Copy MAC"
          variant="secondary"
          onPress={() => void copy(formatMac(data.apMac ?? mac), 'The MAC address')}
        />
        <Button
          title="Reboot"
          variant="destructive"
          loading={busy === 'reboot'}
          disabled={busy !== null}
          onPress={confirmReboot}
        />
      </View>
    </Screen>
  );
}

function RadioStat({
  band,
  channel,
  clients,
  airtime,
  txPower,
}: {
  band: string;
  channel?: string;
  clients?: number;
  airtime?: number;
  txPower?: string;
}) {
  const parts = [
    channel ? `Ch ${channel}` : null,
    clients != null ? `${formatCount(clients)} clients` : null,
    airtime != null ? `${Math.round(airtime)}% airtime` : null,
    txPower ? `${txPower} power` : null,
  ].filter(Boolean);

  return (
    <Stat
      label={formatBand(band)}
      value={parts.length > 0 ? parts.join(' · ') : 'Not reported'}
    />
  );
}
