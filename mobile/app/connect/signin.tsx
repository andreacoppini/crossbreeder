import React, { useCallback, useEffect, useState } from 'react';
import { View } from 'react-native';
import { router } from 'expo-router';
import * as LocalAuthentication from 'expo-local-authentication';
import { useControllers } from '@/controllers/ControllerProvider';
import { Button, Card, Field, Label, Muted, Screen } from '@/ui/components';
import { useTheme } from '@/ui/theme';

/**
 * Re-entry for a controller whose password is saved but whose session has
 * gone — the app was killed, the ticket aged out, or the operator signed out.
 *
 * The biometric prompt is a gate on using the saved password, not a second
 * factor: the password is already in the keychain, and this is what stops a
 * handed-over unlocked phone from rebooting an estate.
 */
export default function SignInScreen() {
  const t = useTheme();
  const { activeProfile, state, error, reconnect, profiles } = useControllers();
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [biometricTried, setBiometricTried] = useState(false);

  useEffect(() => {
    if (state === 'connected') router.replace('/(tabs)');
  }, [state]);

  /** Try the saved password behind a biometric check, once, on arrival. */
  const unlockWithBiometrics = useCallback(async () => {
    setBiometricTried(true);
    const hasHardware = await LocalAuthentication.hasHardwareAsync();
    const enrolled = await LocalAuthentication.isEnrolledAsync();
    if (!hasHardware || !enrolled) return;

    const result = await LocalAuthentication.authenticateAsync({
      promptMessage: `Unlock ${activeProfile?.label ?? 'this controller'}`,
      fallbackLabel: 'Use password',
    });
    if (!result.success) return;

    setBusy(true);
    try {
      await reconnect();
    } finally {
      setBusy(false);
    }
  }, [activeProfile?.label, reconnect]);

  useEffect(() => {
    if (state === 'locked' && !biometricTried) void unlockWithBiometrics();
  }, [biometricTried, state, unlockWithBiometrics]);

  const submit = useCallback(async () => {
    if (!password) return;
    setBusy(true);
    try {
      await reconnect(password);
      setPassword('');
    } finally {
      setBusy(false);
    }
  }, [password, reconnect]);

  if (!activeProfile) {
    return (
      <Screen scroll>
        <Card style={{ gap: t.space.md }}>
          <Label variant="headline">No controller selected</Label>
          <Button
            title="Choose a controller"
            onPress={() => router.replace('/connect')}
          />
        </Card>
      </Screen>
    );
  }

  return (
    <Screen scroll>
      <Card style={{ gap: t.space.lg }}>
        <View style={{ gap: t.space.xs }}>
          <Label variant="title">{activeProfile.label}</Label>
          <Muted>
            {activeProfile.username} at {activeProfile.host}:{activeProfile.port}
          </Muted>
        </View>

        {state === 'connecting' || busy ? (
          <Muted>Connecting…</Muted>
        ) : error ? (
          <Label variant="callout" tone="down">
            {error}
          </Label>
        ) : null}

        <Field
          label="Password"
          value={password}
          onChangeText={setPassword}
          secure
          onSubmitEditing={() => void submit()}
          returnKeyType="go"
          hint="Only needed if the saved one has stopped working."
        />

        <Button
          title="Sign in"
          loading={busy}
          disabled={busy || !password}
          onPress={() => void submit()}
        />
        <Button
          title="Use saved password"
          variant="secondary"
          disabled={busy}
          onPress={() => void unlockWithBiometrics()}
        />
        {profiles.length > 1 ? (
          <Button
            title="Switch controller"
            variant="plain"
            onPress={() => router.replace('/connect')}
          />
        ) : null}
      </Card>
    </Screen>
  );
}
