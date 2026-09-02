import React from 'react';
import { RefreshControl } from 'react-native';
import { Stack, router } from 'expo-router';
import { SmartZoneError } from '@/api';
import { useZones } from '@/hooks/queries';
import {
  EmptyState,
  ErrorState,
  Group,
  Loading,
  Muted,
  Row,
  Screen,
} from '@/ui/components';

/**
 * Zones, the unit almost everything else in SmartZone hangs off: a WLAN, an
 * AP group, a DPSK and an AP all belong to exactly one. Being able to see the
 * shape of a cluster is what makes the filters on the other screens make
 * sense.
 */
export default function ZonesScreen() {
  const zones = useZones();

  if (zones.isLoading) {
    return (
      <Screen>
        <Stack.Screen options={{ title: 'Zones' }} />
        <Loading label="Reading zones" />
      </Screen>
    );
  }

  if (zones.isError) {
    return (
      <Screen scroll>
        <Stack.Screen options={{ title: 'Zones' }} />
        <ErrorState
          message={
            zones.error instanceof SmartZoneError
              ? zones.error.displayMessage
              : 'Could not read the zone list.'
          }
          onRetry={() => void zones.refetch()}
        />
      </Screen>
    );
  }

  const list = zones.data ?? [];

  return (
    <Screen
      scroll
      refreshControl={
        <RefreshControl refreshing={zones.isRefetching} onRefresh={() => void zones.refetch()} />
      }
    >
      <Stack.Screen options={{ title: 'Zones' }} />

      {list.length === 0 ? (
        <EmptyState title="No zones" message="This cluster has no zones you can see." />
      ) : (
        <Group header={`${list.length} zone${list.length === 1 ? '' : 's'}`}>
          {list.map((zone) => (
            <Row
              key={zone.id}
              title={zone.name}
              subtitle={zone.domainName}
              onPress={() =>
                router.push({ pathname: '/zones/[zoneId]', params: { zoneId: zone.id } })
              }
            />
          ))}
        </Group>
      )}

      <Muted>
        Tap a zone for its AP groups, WLAN groups and the firmware its access
        points are pinned to.
      </Muted>
    </Screen>
  );
}
