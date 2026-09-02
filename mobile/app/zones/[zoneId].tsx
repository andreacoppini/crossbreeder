import React from 'react';
import { RefreshControl } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { SmartZoneError } from '@/api';
import {
  useApGroups,
  useWlanGroups,
  useZone,
} from '@/hooks/queries';
import {
  ErrorState,
  Group,
  Loading,
  Muted,
  Row,
  Screen,
  Stat,
} from '@/ui/components';
import { firstNonEmpty } from '@/utils/format';

/**
 * One zone: its settings, the AP groups inside it and the WLAN groups that
 * decide which SSIDs each radio broadcasts.
 */
export default function ZoneDetailScreen() {
  const { zoneId } = useLocalSearchParams<{ zoneId: string }>();
  const zone = useZone(zoneId);
  const apGroups = useApGroups(zoneId);
  const wlanGroups = useWlanGroups(zoneId);

  if (zone.isLoading) {
    return (
      <Screen>
        <Stack.Screen options={{ title: 'Zone' }} />
        <Loading label="Reading the zone" />
      </Screen>
    );
  }

  if (zone.isError || !zone.data) {
    return (
      <Screen scroll>
        <Stack.Screen options={{ title: 'Zone' }} />
        <ErrorState
          message={
            zone.error instanceof SmartZoneError
              ? zone.error.displayMessage
              : 'That zone could not be read.'
          }
          onRetry={() => void zone.refetch()}
        />
      </Screen>
    );
  }

  const data = zone.data;
  const timezone =
    typeof data.timezone === 'string' ? data.timezone : data.timezone?.systemTimezone;

  return (
    <Screen
      scroll
      refreshControl={
        <RefreshControl refreshing={zone.isRefetching} onRefresh={() => void zone.refetch()} />
      }
    >
      <Stack.Screen options={{ title: data.name }} />

      <Group header="Zone">
        <Stat label="Name" value={data.name} />
        <Stat label="Description" value={firstNonEmpty(data.description)} />
        <Stat label="Country" value={firstNonEmpty(data.countryCode)} />
        <Stat label="Time zone" value={firstNonEmpty(timezone)} />
        <Stat label="AP firmware" value={firstNonEmpty(data.version, data.apFirmware)} />
        <Stat label="Mesh" value={data.mesh?.enabled ? 'Enabled' : 'Disabled'} />
      </Group>

      <Group
        header="AP groups"
        footer="An AP group is where radio settings and WLAN groups are set for a set of access points."
      >
        {apGroups.isLoading ? (
          <Row title="Reading AP groups…" />
        ) : (apGroups.data ?? []).length === 0 ? (
          <Row title="No AP groups" subtitle="Only the zone default" />
        ) : (
          (apGroups.data ?? []).map((group) => (
            <Row
              key={group.id}
              title={group.name}
              subtitle={firstNonEmpty(group.description)}
              onPress={() =>
                router.push({
                  pathname: '/(tabs)/aps',
                  params: { zoneId, apGroupId: group.id },
                })
              }
            />
          ))
        )}
      </Group>

      <Group
        header="WLAN groups"
        footer="A WLAN group is the set of SSIDs a radio broadcasts. Changing which group a radio uses is how an SSID reaches, or stops reaching, part of a site."
      >
        {wlanGroups.isLoading ? (
          <Row title="Reading WLAN groups…" />
        ) : (wlanGroups.data ?? []).length === 0 ? (
          <Row title="No WLAN groups" />
        ) : (
          (wlanGroups.data ?? []).map((group) => (
            <Row key={group.id} title={group.name} subtitle={firstNonEmpty(group.description)} />
          ))
        )}
      </Group>

      <Muted>
        Zone-level changes are deliberately not editable from a phone: they
        reach every access point in the zone at once.
      </Muted>
    </Screen>
  );
}
