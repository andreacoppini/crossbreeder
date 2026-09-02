import React, { useCallback, useState } from 'react';
import { Alert, RefreshControl, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import * as Clipboard from 'expo-clipboard';
import * as Haptics from 'expo-haptics';
import { SmartZoneError, signalReason, signalVerdict } from '@/api';
import {
  useClient,
  useClientActions,
  useClientHistory,
} from '@/hooks/queries';
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
import { useTheme, type StatusTone } from '@/ui/theme';
import {
  firstNonEmpty,
  formatBand,
  formatBytes,
  formatDateTime,
  formatDuration,
  formatMac,
  formatRate,
  formatRelative,
  formatRssi,
  formatSnr,
} from '@/utils/format';

const VERDICT_TONE: Record<string, StatusTone> = {
  good: 'up',
  fair: 'warn',
  poor: 'down',
  unknown: 'neutral',
};

/**
 * Client troubleshooting.
 *
 * The screen answers the questions in the order an engineer asks them:
 *
 *   1. Is the radio link any good? RSSI and SNR, with a verdict, because a
 *      number in dBm means nothing to whoever raised the ticket.
 *   2. Did it authenticate? An associated-but-unauthorised client looks
 *      identical to a working one on every summary screen, and is the single
 *      most common "it's connected but nothing works".
 *   3. What is it attached to, and on what? AP, band, channel, VLAN.
 *   4. Has it been dropping? The past sessions, with the controller's own
 *      disconnect reasons.
 *
 * Then the two things worth doing from a phone: kick it so it re-associates,
 * or deauthenticate it so it re-authenticates as well.
 */
export default function ClientDetailScreen() {
  const t = useTheme();
  const { mac } = useLocalSearchParams<{ mac: string }>();
  const client = useClient(mac);
  const history = useClientHistory(mac);
  const actions = useClientActions();
  const [busy, setBusy] = useState<string | null>(null);

  const copy = useCallback(async (value: string, what: string) => {
    await Clipboard.setStringAsync(value);
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    Alert.alert('Copied', `${what} copied to the clipboard.`);
  }, []);

  const run = useCallback(
    (
      kind: 'disconnect' | 'deauth',
      title: string,
      message: string,
      mutate: (macs: string[]) => Promise<unknown>,
    ) => {
      Alert.alert(title, message, [
        { text: 'Cancel', style: 'cancel' },
        {
          text: title.split(' ')[0] ?? 'Confirm',
          style: 'destructive',
          onPress: async () => {
            if (!mac) return;
            setBusy(kind);
            try {
              await mutate([mac]);
              void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
              Alert.alert('Done', 'The controller has actioned it.');
            } catch (err) {
              Alert.alert(
                'Could not do that',
                err instanceof SmartZoneError ? err.displayMessage : 'The controller refused.',
              );
            } finally {
              setBusy(null);
            }
          },
        },
      ]);
    },
    [mac],
  );

  if (client.isLoading) {
    return (
      <Screen>
        <Stack.Screen options={{ title: 'Client' }} />
        <Loading label="Reading the client" />
      </Screen>
    );
  }

  if (client.isError) {
    return (
      <Screen scroll>
        <Stack.Screen options={{ title: 'Client' }} />
        <ErrorState
          message={
            client.error instanceof SmartZoneError
              ? client.error.displayMessage
              : 'That client could not be read.'
          }
          onRetry={() => void client.refetch()}
        />
      </Screen>
    );
  }

  const data = client.data;
  const pastSessions = history.data?.list ?? [];

  // A client that has gone away between the list and this screen is a real
  // and common case, and it has its own useful answer: the history.
  if (!data) {
    return (
      <Screen scroll>
        <Stack.Screen options={{ title: formatMac(mac) }} />
        <Card style={{ gap: t.space.md }}>
          <Label variant="headline">Not connected right now</Label>
          <Muted>
            {formatMac(mac)} is not associated to this cluster at the moment.
            Its recent sessions are below, if the controller still has them.
          </Muted>
        </Card>
        <PastSessions sessions={pastSessions} loading={history.isLoading} />
      </Screen>
    );
  }

  const verdict = signalVerdict(data);
  const tone = VERDICT_TONE[verdict] ?? 'neutral';
  const authorised = !data.authStatus || /^authorized$/i.test(data.authStatus);

  return (
    <Screen
      scroll
      refreshControl={
        <RefreshControl refreshing={client.isRefetching} onRefresh={() => void client.refetch()} />
      }
    >
      <Stack.Screen
        options={{
          title: firstNonEmpty(data.hostname, data.userName, formatMac(mac)),
        }}
      />

      <Card style={{ gap: t.space.md }}>
        <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' }}>
          <View style={{ flex: 1, gap: 2 }}>
            <Label variant="title" numberOfLines={1}>
              {firstNonEmpty(data.hostname, data.userName, formatMac(mac))}
            </Label>
            <Muted>{firstNonEmpty(data.osType, 'Unknown device')}</Muted>
          </View>
          <Pill label={`Signal ${verdict}`} tone={tone} />
        </View>

        <View style={{ flexDirection: 'row', gap: t.space.xl }}>
          <View>
            <Label variant="title" tone={tone}>
              {formatRssi(data.rssi)}
            </Label>
            <Muted>Signal</Muted>
          </View>
          <View>
            <Label variant="title" tone={tone}>
              {formatSnr(data.snr)}
            </Label>
            <Muted>Noise margin</Muted>
          </View>
          <View>
            <Label variant="title">{formatDuration(data.sessionDuration)}</Label>
            <Muted>Connected</Muted>
          </View>
        </View>

        <Muted>{signalReason(data)}</Muted>
      </Card>

      {!authorised ? (
        <Card style={{ gap: t.space.sm }}>
          <Pill label="Not authorised" tone="down" />
          <Label variant="callout">
            This client has associated to the radio but has not passed
            authentication, so it has a Wi-Fi link and no network. On an 802.1X
            WLAN look at the RADIUS server; on a PSK or DPSK WLAN the key is
            wrong or has expired; on a portal WLAN nobody has completed the
            login page.
          </Label>
        </Card>
      ) : null}

      <Group header="Connection">
        <Stat label="MAC" value={formatMac(data.clientMac ?? mac)} mono />
        <Stat label="IP" value={firstNonEmpty(data.ipAddress, data.ipv6Address)} mono />
        <Stat label="SSID" value={firstNonEmpty(data.ssid)} />
        <Stat label="VLAN" value={data.vlan ?? data.vlanId ?? '—'} />
        <Row
          title="Access point"
          subtitle={`${firstNonEmpty(data.apName)} · ${formatMac(data.apMac)}`}
          onPress={() =>
            data.apMac
              ? router.push({ pathname: '/ap/[mac]', params: { mac: data.apMac } })
              : undefined
          }
        />
        <Stat label="Band" value={formatBand(data.radioType)} />
        <Stat label="Channel" value={data.channel ?? '—'} />
        <Stat label="BSSID" value={formatMac(data.bssid)} mono />
      </Group>

      <Group header="Authentication">
        <Stat
          label="Status"
          value={firstNonEmpty(data.authStatus, 'Not reported')}
          tone={authorised ? 'up' : 'down'}
        />
        <Stat label="Method" value={firstNonEmpty(data.authMethod)} />
        <Stat label="Encryption" value={firstNonEmpty(data.encryptionMethod)} />
        <Stat label="Username" value={firstNonEmpty(data.userName)} />
        {data.dpskId ? <Stat label="Keyed by" value="Dynamic PSK" /> : null}
      </Group>

      <Group header="Throughput" footer="Rates are what the radios negotiated, not what the client is achieving.">
        <Stat label="Received" value={formatBytes(data.rxBytes)} />
        <Stat label="Sent" value={formatBytes(data.txBytes)} />
        <Stat label="Rx rate" value={formatRate(data.rxMcsRate)} />
        <Stat label="Tx rate" value={formatRate(data.txMcsRate)} />
        {data.noiseFloor != null ? (
          <Stat label="Noise floor" value={formatRssi(data.noiseFloor)} />
        ) : null}
      </Group>

      <PastSessions sessions={pastSessions} loading={history.isLoading} />

      <View style={{ gap: t.space.sm }}>
        <Button
          title="Copy MAC"
          variant="secondary"
          onPress={() => void copy(formatMac(data.clientMac ?? mac), 'The MAC address')}
        />
        <Button
          title="Disconnect"
          variant="secondary"
          loading={busy === 'disconnect'}
          disabled={busy !== null}
          onPress={() =>
            run(
              'disconnect',
              'Disconnect this client?',
              'It will re-associate within a few seconds. Use this to force a fresh connection.',
              actions.disconnect.mutateAsync,
            )
          }
        />
        <Button
          title="Deauthenticate"
          variant="destructive"
          loading={busy === 'deauth'}
          disabled={busy !== null}
          onPress={() =>
            run(
              'deauth',
              'Deauthenticate this client?',
              'It will have to authenticate again, not just re-associate. On a portal WLAN it will see the login page again.',
              actions.deauth.mutateAsync,
            )
          }
        />
      </View>
    </Screen>
  );
}

/** Past sessions, with whatever reason the controller recorded for the drop. */
function PastSessions({
  sessions,
  loading,
}: {
  sessions: { sessionEndTime?: number; disconnectTime?: number; disconnectReason?: string; apName?: string; ssid?: string; sessionDuration?: number }[];
  loading: boolean;
}) {
  return (
    <Group
      header="Recent sessions"
      footer="Where a client keeps reconnecting, the pattern here says more than any single reading above."
    >
      {loading ? (
        <Row title="Reading history…" />
      ) : sessions.length === 0 ? (
        <Row title="No earlier sessions" subtitle="The controller has none recorded" />
      ) : (
        sessions.slice(0, 10).map((session, i) => (
          <Row
            key={i}
            title={firstNonEmpty(session.disconnectReason, 'Disconnected')}
            subtitle={`${firstNonEmpty(session.ssid)} · ${firstNonEmpty(session.apName)}`}
            detail={
              <Muted>
                {formatDateTime(session.sessionEndTime ?? session.disconnectTime)}
                {session.sessionDuration != null
                  ? ` · lasted ${formatDuration(session.sessionDuration)}`
                  : ''}
                {' · '}
                {formatRelative(session.sessionEndTime ?? session.disconnectTime)}
              </Muted>
            }
          />
        ))
      )}
    </Group>
  );
}
