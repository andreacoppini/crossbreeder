import React from 'react';
import { View } from 'react-native';
import { router } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useControllers } from '@/controllers/ControllerProvider';
import {
  Button,
  EmptyState,
  Group,
  Label,
  Muted,
  Pill,
  Row,
  Screen,
} from '@/ui/components';
import { useTheme } from '@/ui/theme';
import { formatRelative } from '@/utils/format';

/**
 * The controller list, and the way in for a new one.
 *
 * Sites are rarely one cluster, and the operator who needs this app is
 * usually holding a phone in a plant room with three of them. Switching is a
 * single tap, and each controller carries the address underneath so two
 * similarly-named clusters cannot be confused.
 */
export default function ControllersScreen() {
  const { profiles, activeProfile, switchTo, state } = useControllers();
  const t = useTheme();
  const insets = useSafeAreaInsets();

  return (
    <Screen scroll contentStyle={{ paddingBottom: insets.bottom + t.space.xl }}>
      {profiles.length === 0 ? (
        <View style={{ paddingTop: t.space.xxl, gap: t.space.lg }}>
          <EmptyState
            title="No controllers yet"
            message="Add a SmartZone cluster to manage its access points, WLANs and clients. Scanning a QR code fills in the address for you."
          />
        </View>
      ) : (
        <Group header="Saved controllers">
          {profiles.map((profile) => {
            const isActive = profile.id === activeProfile?.id;
            return (
              <Row
                key={profile.id}
                title={profile.label}
                subtitle={`${profile.username} at ${profile.host}:${profile.port}`}
                tone={isActive && state === 'connected' ? 'up' : 'neutral'}
                right={
                  isActive ? (
                    <Pill
                      label={state === 'connected' ? 'Connected' : state}
                      tone={state === 'connected' ? 'up' : 'warn'}
                      compact
                    />
                  ) : profile.lastUsedAt ? (
                    <Muted>{formatRelative(profile.lastUsedAt)}</Muted>
                  ) : null
                }
                onPress={async () => {
                  await switchTo(profile.id);
                  router.replace('/');
                }}
              />
            );
          })}
        </Group>
      )}

      <View style={{ gap: t.space.md, marginTop: t.space.md }}>
        <Button
          title="Scan a controller QR code"
          onPress={() => router.push('/connect/scan')}
        />
        <Button
          title="Enter an address"
          variant="secondary"
          onPress={() => router.push('/connect/add')}
        />
      </View>

      <View style={{ marginTop: t.space.lg, gap: t.space.xs }}>
        <Label variant="subhead">Before you start</Label>
        <Muted>
          You need an administrator account on the cluster and its public API
          reachable on port 8443. SmartZone ships with a self-signed
          certificate; install it on this device first, or the connection will
          be refused. Settings explains how.
        </Muted>
      </View>
    </Screen>
  );
}
