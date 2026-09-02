import React from 'react';
import { router } from 'expo-router';
import { useControllers } from '@/controllers/ControllerProvider';
import { useDevicesSummary } from '@/hooks/queries';
import { Group, Label, Muted, Pill, Row, Screen } from '@/ui/components';
import { formatCount } from '@/utils/format';

/**
 * Everything that is not one of the four things opened in a hurry.
 *
 * Switching has a row here from the first release rather than appearing one
 * day out of nowhere: it says what is coming, and it is where it will land.
 */
export default function MoreScreen() {
  const { activeProfile, profiles } = useControllers();
  const devices = useDevicesSummary();
  const switchCount = devices.data?.totalSwitches ?? 0;

  return (
    <Screen scroll>
      <Group header="Wireless">
        <Row
          title="Dynamic PSKs"
          subtitle="Issue, share and revoke per-device keys"
          onPress={() => router.push('/dpsk')}
        />
        <Row
          title="Zones and AP groups"
          subtitle="How this cluster is organised"
          onPress={() => router.push('/zones')}
        />
        <Row
          title="Alarms and events"
          subtitle="What the controller is complaining about"
          onPress={() => router.push('/alarms')}
        />
        <Row
          title="Diagnostics"
          subtitle="Ping and traceroute from an access point"
          onPress={() => router.push('/tools')}
        />
      </Group>

      <Group
        header="Wired"
        footer="Switching is the next thing this app grows into. The types and the client are in place; what is still missing is a switch API this controller answers on — every public path tried so far returns 404, even on a cluster that manages switches."
      >
        <Row
          title="Switches"
          subtitle={
            switchCount > 0
              ? `${formatCount(switchCount)} ICX switches registered on this cluster`
              : 'ICX switch management'
          }
          right={<Pill label="Coming soon" tone="accent" compact />}
          disabled
        />
      </Group>

      <Group header="This app">
        <Row
          title="Controllers"
          subtitle={
            profiles.length > 1
              ? `${profiles.length} saved · currently ${activeProfile?.label}`
              : activeProfile?.label
          }
          onPress={() => router.push('/connect')}
        />
        <Row
          title="Settings"
          subtitle="Certificates, session and sign-out"
          onPress={() => router.push('/settings')}
        />
      </Group>

      <Muted>
        <Label variant="footnote">
          Reading is always safe. Anything that changes the cluster asks first.
        </Label>
      </Muted>
    </Screen>
  );
}
