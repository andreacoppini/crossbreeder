import React, { useCallback } from 'react';
import { Alert, View } from 'react-native';
import { Stack, router } from 'expo-router';
import * as Clipboard from 'expo-clipboard';
import { useControllers } from '@/controllers/ControllerProvider';
import { toBootstrapLink } from '@/controllers/bootstrap';
import {
  Button,
  Card,
  Group,
  Label,
  Muted,
  Row,
  Screen,
  Stat,
} from '@/ui/components';
import { useTheme } from '@/ui/theme';
import { firstNonEmpty, formatDateTime } from '@/utils/format';

export default function SettingsScreen() {
  const t = useTheme();
  const { activeProfile, session, signOut, removeController } = useControllers();

  const shareLink = useCallback(async () => {
    if (!activeProfile) return;
    const link = toBootstrapLink({
      host: activeProfile.host,
      port: activeProfile.port,
      username: activeProfile.username,
      label: activeProfile.label,
      domainId: activeProfile.domainId,
    });
    await Clipboard.setStringAsync(link);
    Alert.alert(
      'Link copied',
      'It carries the address and the username, and no password. Whoever you send it to still needs their own credentials.',
    );
  }, [activeProfile]);

  const confirmForget = useCallback(() => {
    if (!activeProfile) return;
    Alert.alert(
      `Remove ${activeProfile.label}?`,
      'The saved password and session are deleted from this device. Nothing changes on the controller.',
      [
        { text: 'Cancel', style: 'cancel' },
        {
          text: 'Remove',
          style: 'destructive',
          onPress: async () => {
            await removeController(activeProfile.id);
            router.replace('/');
          },
        },
      ],
    );
  }, [activeProfile, removeController]);

  return (
    <Screen scroll>
      <Stack.Screen options={{ title: 'Settings' }} />

      <Group header="Connected controller">
        <Stat label="Name" value={firstNonEmpty(activeProfile?.label)} />
        <Stat
          label="Address"
          value={`${activeProfile?.host}:${activeProfile?.port}`}
          mono
        />
        <Stat label="Administrator" value={firstNonEmpty(activeProfile?.username)} />
        <Stat
          label="SmartZone"
          value={firstNonEmpty(session?.controllerVersion, activeProfile?.controllerVersion)}
        />
        <Stat
          label="API version"
          value={firstNonEmpty(session?.apiVersion?.replace('_', '.'))}
        />
        <Stat label="Signed in" value={formatDateTime(session?.issuedAt)} />
      </Group>

      <Card style={{ gap: t.space.sm }}>
        <Label variant="headline">Certificates</Label>
        <Muted>
          SmartZone ships with a self-signed certificate, which no phone trusts
          by default. This app will not accept a certificate it cannot verify:
          an app holding administrator credentials for a whole wireless estate
          is exactly the one that should not.
        </Muted>
        <Muted>
          The way through is to install the controller&apos;s certificate on
          this device. On iOS, mail or AirDrop the certificate to yourself,
          install the profile, then turn it on under Settings, General, About,
          Certificate Trust Settings. On Android, install it under Settings,
          Security, Encryption &amp; credentials, and this app is built to
          trust what you have installed.
        </Muted>
        <Muted>
          A controller with a certificate from your own internal CA, or a
          public one, needs none of this.
        </Muted>
      </Card>

      <Card style={{ gap: t.space.sm }}>
        <Label variant="headline">Where your credentials live</Label>
        <Muted>
          The administrator password is held in this device&apos;s keychain
          (iOS) or keystore (Android), reachable only while the device is
          unlocked and never included in a backup to another device. The
          service ticket is kept beside it and discarded after 24 hours,
          because that is when SmartZone expires it.
        </Muted>
        <Muted>
          Nothing is sent anywhere except to the controller you named.
        </Muted>
      </Card>

      <Group header="Sharing">
        <Row
          title="Copy a setup link for this controller"
          subtitle="Address and username only, never a password"
          onPress={() => void shareLink()}
        />
      </Group>

      <View style={{ gap: t.space.sm }}>
        <Button
          title="Sign out"
          variant="secondary"
          onPress={async () => {
            await signOut();
            router.replace('/');
          }}
        />
        <Button
          title="Sign out and forget the password"
          variant="secondary"
          onPress={async () => {
            await signOut({ forget: true });
            router.replace('/');
          }}
        />
        <Button title="Remove this controller" variant="destructive" onPress={confirmForget} />
      </View>
    </Screen>
  );
}
