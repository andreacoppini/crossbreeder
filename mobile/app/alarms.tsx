import React, { useCallback, useMemo, useState } from 'react';
import { Alert, FlatList, RefreshControl, View } from 'react-native';
import { Stack } from 'expo-router';
import { SmartZoneError, isAcknowledged, type Alarm, type SzEvent } from '@/api';
import { useAlarmActions, useAlarms, useEvents } from '@/hooks/queries';
import {
  Card,
  ChipBar,
  EmptyState,
  ErrorState,
  Loading,
  Muted,
  Pill,
  Row,
} from '@/ui/components';
import { severityTone, useTheme } from '@/ui/theme';
import { firstNonEmpty, formatCount, formatRelative } from '@/utils/format';

type Tab = 'alarms' | 'events';

/**
 * Alarms are conditions that persist until something clears them; events are
 * things that happened. They answer different questions and are kept apart
 * rather than merged into a single feed.
 */
export default function AlarmsScreen() {
  const t = useTheme();
  const [tab, setTab] = useState<Tab>('alarms');

  const alarms = useAlarms({});
  const events = useEvents({});
  const actions = useAlarmActions();

  const alarmRows = useMemo(
    () => alarms.data?.pages.flatMap((p) => p.list ?? []) ?? [],
    [alarms.data],
  );
  const eventRows = useMemo(
    () => events.data?.pages.flatMap((p) => p.list ?? []) ?? [],
    [events.data],
  );

  const active = tab === 'alarms' ? alarms : events;
  const rows: (Alarm | SzEvent)[] = tab === 'alarms' ? alarmRows : eventRows;

  const acknowledge = useCallback(
    (alarm: Alarm) => {
      const id = alarm.id;
      if (!id) return;
      Alert.alert(
        'Acknowledge this alarm?',
        'It stays on the cluster but stops counting as outstanding.',
        [
          { text: 'Cancel', style: 'cancel' },
          {
            text: 'Acknowledge',
            onPress: async () => {
              try {
                await actions.acknowledge.mutateAsync(id);
              } catch (err) {
                Alert.alert(
                  'Could not acknowledge',
                  err instanceof SmartZoneError ? err.displayMessage : 'The controller refused.',
                );
              }
            },
          },
        ],
      );
    },
    [actions.acknowledge],
  );

  return (
    <View style={{ flex: 1 }}>
      <Stack.Screen options={{ title: 'Alarms and events' }} />

      <View style={{ paddingVertical: t.space.md }}>
        <ChipBar<Tab>
          value={tab}
          onChange={setTab}
          options={[
            { value: 'alarms', label: 'Alarms' },
            { value: 'events', label: 'Events' },
          ]}
        />
      </View>

      {active.isError ? (
        <View style={{ padding: t.space.lg }}>
          <ErrorState
            message={
              active.error instanceof SmartZoneError
                ? active.error.displayMessage
                : 'Could not read from the controller.'
            }
            onRetry={() => void active.refetch()}
          />
        </View>
      ) : active.isLoading ? (
        <Loading />
      ) : (
        <FlatList
          data={rows}
          keyExtractor={(item, i) => item.id ?? String(i)}
          contentContainerStyle={{ padding: t.space.lg, paddingTop: 0, gap: t.space.sm }}
          refreshControl={
            <RefreshControl
              refreshing={active.isRefetching && !active.isFetchingNextPage}
              onRefresh={() => void active.refetch()}
            />
          }
          onEndReachedThreshold={0.4}
          onEndReached={() => {
            if (active.hasNextPage && !active.isFetchingNextPage) {
              void active.fetchNextPage();
            }
          }}
          ListHeaderComponent={
            <Muted>
              {formatCount(active.data?.pages[0]?.totalCount)}{' '}
              {tab === 'alarms' ? 'alarms' : 'events'}
            </Muted>
          }
          ListEmptyComponent={
            <EmptyState
              title={tab === 'alarms' ? 'Nothing outstanding' : 'Nothing recorded'}
              message={
                tab === 'alarms'
                  ? 'The controller has no open alarms.'
                  : 'No events in the window the controller keeps.'
              }
            />
          }
          ListFooterComponent={
            active.isFetchingNextPage ? <Loading /> : <View style={{ height: t.space.xl }} />
          }
          renderItem={({ item }) => {
            const tone = severityTone(item.severity);
            // `acknowledged` is the string "Yes"/"No", not a boolean; reading
            // it as truthy would mark every open alarm as acknowledged.
            const acknowledged = tab === 'alarms' && isAcknowledged(item as Alarm);
            const kind =
              'alarmType' in item ? item.alarmType : (item as SzEvent).eventType;
            return (
              <Card padded={false}>
                <Row
                  tone={tone}
                  title={firstNonEmpty(item.activity, kind, 'Event')}
                  subtitle={firstNonEmpty(kind, item.category)}
                  detail={<Muted>{formatRelative(item.insertionTime)}</Muted>}
                  right={
                    <Pill
                      label={acknowledged ? 'Acked' : (item.severity ?? '—')}
                      tone={acknowledged ? 'neutral' : tone}
                      compact
                    />
                  }
                  onPress={
                    tab === 'alarms' && !acknowledged
                      ? () => acknowledge(item as Alarm)
                      : undefined
                  }
                />
              </Card>
            );
          }}
        />
      )}
    </View>
  );
}
