import React, { useCallback, useRef, useState } from 'react';
import { StyleSheet, View } from 'react-native';
import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { router } from 'expo-router';
import { parseBootstrap } from '@/controllers/bootstrap';
import { Button, EmptyState, Label, Muted, Screen } from '@/ui/components';
import { useTheme } from '@/ui/theme';

/**
 * QR bootstrap.
 *
 * The code carries an address, a port, a username and a label. It never
 * carries a password: a QR code ends up photographed, screenshotted and
 * pasted into a group chat, and an administrator password on a SmartZone
 * cluster is the whole estate. The scan gets you to a filled-in sign-in
 * form, and the password is typed once and kept in the device keychain.
 */
export default function ScanScreen() {
  const t = useTheme();
  const [permission, requestPermission] = useCameraPermissions();
  const [error, setError] = useState<string | null>(null);
  /** A camera fires the same code many times a second. */
  const handled = useRef(false);

  const onScanned = useCallback(({ data }: { data: string }) => {
    if (handled.current) return;

    const payload = parseBootstrap(data);
    if (!payload) {
      setError('That code does not carry a controller address.');
      return;
    }

    handled.current = true;
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    router.replace({
      pathname: '/connect/add',
      params: {
        host: payload.host,
        port: payload.port ? String(payload.port) : '',
        username: payload.username ?? '',
        label: payload.label ?? '',
        domainId: payload.domainId ?? '',
      },
    });
  }, []);

  if (!permission) {
    return (
      <Screen>
        <EmptyState title="Checking camera access" />
      </Screen>
    );
  }

  if (!permission.granted) {
    return (
      <Screen scroll>
        <EmptyState
          title="Camera access is off"
          message="Scanning a QR code needs the camera. You can also type the controller's address instead."
          action={
            <View style={{ gap: t.space.sm }}>
              <Button title="Allow camera" onPress={() => void requestPermission()} />
              <Button
                title="Type it instead"
                variant="secondary"
                onPress={() => router.replace('/connect/add')}
              />
            </View>
          }
        />
      </Screen>
    );
  }

  return (
    <View style={{ flex: 1, backgroundColor: '#000' }}>
      <CameraView
        style={StyleSheet.absoluteFill}
        facing="back"
        barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
        onBarcodeScanned={onScanned}
      />
      <View
        style={{
          flex: 1,
          alignItems: 'center',
          justifyContent: 'flex-end',
          padding: t.space.xl,
          gap: t.space.md,
        }}
      >
        <View
          style={{
            backgroundColor: 'rgba(0,0,0,0.6)',
            padding: t.space.lg,
            borderRadius: t.radius.lg,
            gap: t.space.xs,
          }}
        >
          <Label variant="headline" color="#FFFFFF">
            {error ?? 'Point the camera at the controller QR code'}
          </Label>
          <Muted>
            {error
              ? 'Try again, or type the address instead.'
              : 'The code carries the address and username only. You will type the password next.'}
          </Muted>
        </View>
        <Button
          title="Type the address instead"
          variant="secondary"
          onPress={() => router.replace('/connect/add')}
        />
      </View>
    </View>
  );
}
