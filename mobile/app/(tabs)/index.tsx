import React, { useCallback, useState } from 'react';
import { RefreshControl, View } from 'react-native';
import { router } from 'expo-router';
import { useControllers } from '@/controllers/ControllerProvider';
import {
  useAlarmSummary,
  useAlarms,
  useDevicesSummary,
} from '@/hooks/queries';
import {
  Card,
  ErrorState,
  Group,
  Label,
  Metric,
  Muted,
  Pill,
  Row,
  Screen,
} from '@/ui/components';
import { severityTone, useTheme } from '@/ui/theme';
import { formatCount, formatRelative } from '@/utils/format';
import { SmartZoneError } from '@/api';

/**
 * The screen the app opens on: is anything wrong, and how wrong.
 *
 * Offline APs and outstanding alarms come first because they are the two
 * questions that get asked; the totals underneath are context, not the point.
 */
export default function OverviewScreen() {
  const t = useTheme();
  const { activeProfile, session } = useControllers();
  const devices = useDevicesSummary();
  const alarmSummary = useAlarmSummary();
  const alarms = useAlarms({ pageSize: 5 });

  const [refreshing, setRefreshing] = useState(false);
  const refresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.allSettled([
      devices.refetch(),
      alarmSummary.refetch(),
      alarms.refetch(),
    ]);
    setRefreshing(false);
  }, [alarmSummary, alarms, devices]);

  const summary = devices.data;
  const online = summary?.apOnlineCount ?? 0;
  const offline = summary?.apOfflineCount ?? 0;
  const flagged = summary?.apFlaggedCount ?? 0;
  const total = summary?.apTotalCount ?? online + offline + flagged;

  const recentAlarms = alarms.data?.pages.flatMap((p) => p.list ?? []) ?? [];

  return (
    <Screen
      scroll
      refreshControl={
        <RefreshControl refreshing={refreshing} onRefresh={() => void refresh()} />
      }
    >
      <View style={{ gap: 2 }}>
        <Label variant="largeTitle">{activeProfile?.label}</Label>
        <Muted>
          {activeProfile?.host}
          {session?.controllerVersion ? ` · SmartZone ${session.controllerVersion}` : ''}
        </Muted>
      </View>

      {devices.isError ? (
        <ErrorState
          message={
            devices.error instanceof SmartZoneError
              ? devices.error.displayMessage
              : 'Could not read the cluster summary.'
          }
          onRetry={() => void devices.refetch()}
        />
      ) : (
        <Card>
          <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
            <Metric
              value={formatCount(online)}
              caption="APs online"
              tone="up"
              onPress={() => router.push({ pathname: '/(tabs)/aps', params: { status: 'Online' } })}
            />
            <Metric
              value={formatCount(flagged)}
              caption="Flagged"
              tone={flagged > 0 ? 'warn' : 'neutral'}
              onPress={() => router.push({ pathname: '/(tabs)/aps', params: { status: 'Flagged' } })}
            />
            <Metric
              value={formatCount(offline)}
              caption="Offline"
              tone={offline > 0 ? 'down' : 'neutral'}
              onPress={() => router.push({ pathname: '/(tabs)/aps', params: { status: 'Offline' } })}
            />
          </View>
          <Muted>
            {formatCount(total)} access points ·{' '}
            {formatCount(summary?.clientCount)} clients connected
          </Muted>
        </Card>
      )}

      {summary?.switchTotalCount ? (
        <Card style={{ gap: t.space.sm }}>
          <View
            style={{
              flexDirection: 'row',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}
          >
            <Label variant="headline">Switches</Label>
            <Pill label="Coming soon" tone="accent" compact />
          </View>
          <Muted>
            This cluster manages {formatCount(summary.switchTotalCount)} ICX
            switches. Switch management is the next thing this app grows into;
            for now they are visible in the controller only.
          </Muted>
        </Card>
      ) : null}

      <Group
        header="Alarms"
        footer={
          alarmSummary.data
            ? `${formatCount(alarmSummary.data.criticalCount)} critical · ${formatCount(
                alarmSummary.data.majorCount,
              )} major · ${formatCount(alarmSummary.data.minorCount)} minor`
            : undefined
        }
      >
        {recentAlarms.length === 0 ? (
          <Row title="Nothing outstanding" subtitle="No alarms on this cluster" tone="up" />
        ) : (
          recentAlarms.slice(0, 5).map((alarm, i) => (
            <Row
              key={alarm.id ?? alarm.alarmId ?? i}
              title={alarm.activity ?? alarm.description ?? 'Alarm'}
              subtitle={`${alarm.entityName ?? alarm.zoneName ?? ''} · ${formatRelative(
                alarm.datetime ?? alarm.firstAppearTime,
              )}`}
              tone={severityTone(alarm.severity)}
              right={<Pill label={alarm.severity ?? '—'} tone={severityTone(alarm.severity)} compact />}
              onPress={() => router.push('/alarms')}
            />
          ))
        )}
        <Row title="All alarms and events" onPress={() => router.push('/alarms')} />
      </Group>
    </Screen>
  );
}
