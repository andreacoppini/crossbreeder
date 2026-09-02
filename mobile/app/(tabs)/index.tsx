import React, { useCallback, useState } from 'react';
import { RefreshControl, View } from 'react-native';
import { router } from 'expo-router';
import { SmartZoneError, alarmTotal, apCapacityUsed, isAcknowledged } from '@/api';
import { useControllers } from '@/controllers/ControllerProvider';
import {
  useAlarmSummary,
  useAlarms,
  useApStatusCounts,
  useDevicesSummary,
} from '@/hooks/queries';
import {
  Card,
  ErrorState,
  Group,
  Label,
  Loading,
  Metric,
  Muted,
  Pill,
  Row,
  Screen,
} from '@/ui/components';
import { severityTone, useTheme } from '@/ui/theme';
import { firstNonEmpty, formatCount, formatRelative } from '@/utils/format';

/**
 * The screen the app opens on: is anything wrong, and how wrong.
 *
 * The AP status figures are counted from the AP query rather than read from
 * `devicesSummary`, which has no health breakdown at all — on the cluster
 * this was built against it reports 287 APs while the AP query finds 549
 * online, so the two count different things and neither is a health number.
 * Counting costs a few requests, which is why this is refreshed on a pull
 * rather than on a timer.
 */
export default function OverviewScreen() {
  const t = useTheme();
  const { activeProfile, session } = useControllers();
  const devices = useDevicesSummary();
  const counts = useApStatusCounts();
  const alarmSummary = useAlarmSummary();
  const alarms = useAlarms({ pageSize: 5 });

  const [refreshing, setRefreshing] = useState(false);
  const refresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.allSettled([
      devices.refetch(),
      counts.refetch(),
      alarmSummary.refetch(),
      alarms.refetch(),
    ]);
    setRefreshing(false);
  }, [alarmSummary, alarms, counts, devices]);

  const recentAlarms = alarms.data?.pages.flatMap((p) => p.list ?? []) ?? [];
  const outstanding = alarmTotal(alarmSummary.data);
  const capacityUsed = apCapacityUsed(devices.data);

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

      {counts.isError ? (
        <ErrorState
          message={
            counts.error instanceof SmartZoneError
              ? counts.error.displayMessage
              : 'Could not count the access points.'
          }
          onRetry={() => void counts.refetch()}
        />
      ) : counts.isLoading ? (
        <Card>
          <Loading label="Counting access points" />
        </Card>
      ) : (
        <Card style={{ gap: t.space.sm }}>
          <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
            <Metric
              value={formatCount(counts.data?.online)}
              caption="Online"
              tone="up"
              onPress={() => router.push('/(tabs)/aps')}
            />
            <Metric
              value={formatCount(counts.data?.flagged)}
              caption="Flagged"
              tone={(counts.data?.flagged ?? 0) > 0 ? 'warn' : 'neutral'}
              onPress={() => router.push('/(tabs)/aps')}
            />
            <Metric
              value={formatCount(counts.data?.offline)}
              caption="Offline"
              tone={(counts.data?.offline ?? 0) > 0 ? 'down' : 'neutral'}
              onPress={() => router.push('/(tabs)/aps')}
            />
          </View>
          <Muted>
            {formatCount(counts.data?.total)} access points
            {counts.data?.truncated
              ? ' counted so far — this cluster is larger than one sweep'
              : ' on this cluster'}
          </Muted>
        </Card>
      )}

      {devices.data ? (
        <Group header="Inventory">
          <Row
            title="Access points"
            subtitle={`${formatCount(devices.data.totalAps)} registered${
              capacityUsed != null ? ` · ${capacityUsed}% of licensed capacity` : ''
            }`}
          />
          <Row
            title="Switches"
            subtitle={
              devices.data.totalSwitches
                ? `${formatCount(devices.data.totalSwitches)} registered`
                : 'None on this cluster'
            }
            right={
              devices.data.totalSwitches ? (
                <Pill label="Coming soon" tone="accent" compact />
              ) : null
            }
          />
        </Group>
      ) : null}

      <Group
        header="Alarms"
        footer={
          alarmSummary.data
            ? `${formatCount(alarmSummary.data.criticalCount)} critical · ${formatCount(
                alarmSummary.data.majorCount,
              )} major · ${formatCount(alarmSummary.data.minorCount)} minor · ${formatCount(
                alarmSummary.data.warningCount,
              )} warning`
            : undefined
        }
      >
        {alarms.isLoading ? (
          <Row title="Reading alarms…" />
        ) : recentAlarms.length === 0 ? (
          <Row title="Nothing outstanding" subtitle="No alarms on this cluster" tone="up" />
        ) : (
          recentAlarms.slice(0, 5).map((alarm, i) => (
            <Row
              key={alarm.id ?? i}
              title={firstNonEmpty(alarm.alarmType, alarm.activity, 'Alarm')}
              subtitle={`${firstNonEmpty(alarm.category)} · ${formatRelative(
                alarm.insertionTime,
              )}`}
              tone={severityTone(alarm.severity)}
              right={
                <Pill
                  label={isAcknowledged(alarm) ? 'Acked' : (alarm.severity ?? '—')}
                  tone={isAcknowledged(alarm) ? 'neutral' : severityTone(alarm.severity)}
                  compact
                />
              }
              onPress={() => router.push('/alarms')}
            />
          ))
        )}
        <Row
          title="All alarms and events"
          subtitle={
            outstanding != null ? `${formatCount(outstanding)} outstanding` : undefined
          }
          onPress={() => router.push('/alarms')}
        />
      </Group>
    </Screen>
  );
}
