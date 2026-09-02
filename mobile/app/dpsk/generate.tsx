import React, { useCallback, useMemo, useState } from 'react';
import { Alert, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import * as Clipboard from 'expo-clipboard';
import * as Haptics from 'expo-haptics';
import { SmartZoneError, type Dpsk } from '@/api';
import { useDpskEnabledWlans, useDpskMutations, useZones } from '@/hooks/queries';
import {
  Button,
  Card,
  ChipBar,
  Field,
  Group,
  Label,
  Loading,
  Muted,
  Row,
  Screen,
} from '@/ui/components';
import { useTheme } from '@/ui/theme';
import { firstNonEmpty } from '@/utils/format';

/**
 * Issue new keys.
 *
 * Deliberately a short form. A batch, a name, an optional expiry and a device
 * limit covers the cases that come up — one key for a new tenant, thirty for
 * a conference — and everything else is a controller job. The generated keys
 * are shown once, on this screen, with copy on each, because that is the
 * moment they are needed.
 */
export default function GenerateDpskScreen() {
  const t = useTheme();
  const params = useLocalSearchParams<{ zoneId?: string; wlanId?: string }>();

  const zones = useZones();
  const [zoneId, setZoneId] = useState<string | undefined>(
    params.zoneId || undefined,
  );
  const wlans = useDpskEnabledWlans(zoneId);
  const [wlanId, setWlanId] = useState<string | undefined>(params.wlanId);

  const [userName, setUserName] = useState('');
  const [count, setCount] = useState('1');
  const [deviceLimit, setDeviceLimit] = useState('1');
  const [expiresInDays, setExpiresInDays] = useState('');
  const [vlan, setVlan] = useState('');

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [issued, setIssued] = useState<Dpsk[] | null>(null);

  const { generate } = useDpskMutations();

  const zoneOptions = useMemo(
    () => (zones.data ?? []).map((z) => ({ value: z.id, label: z.name })),
    [zones.data],
  );
  const wlanOptions = useMemo(
    () => (wlans.data?.list ?? []).map((w) => ({ value: w.id, label: w.name })),
    [wlans.data],
  );

  const submit = useCallback(async () => {
    setError(null);

    if (!zoneId || !wlanId) {
      setError('Choose a zone and a WLAN that issues keys.');
      return;
    }
    const numberOfDpsks = Number(count);
    if (!Number.isInteger(numberOfDpsks) || numberOfDpsks < 1 || numberOfDpsks > 500) {
      setError('Between 1 and 500 keys at a time.');
      return;
    }
    if (!userName.trim()) {
      setError('Give the keys a name; a batch gets numbered from it.');
      return;
    }

    const request: Parameters<typeof generate.mutateAsync>[0]['request'] = {
      numberOfDpsks,
      userName: userName.trim(),
    };

    if (deviceLimit) {
      const limit = Number(deviceLimit);
      if (!Number.isInteger(limit) || limit < 1) {
        setError('The device limit is a whole number of 1 or more.');
        return;
      }
      request.deviceCountLimit = limit;
    }

    if (expiresInDays) {
      const days = Number(expiresInDays);
      if (!Number.isInteger(days) || days < 1) {
        setError('An expiry is a whole number of days.');
        return;
      }
      request.expirationDate = new Date(
        Date.now() + days * 86_400_000,
      ).toISOString();
    }

    if (vlan) {
      const parsed = Number(vlan);
      if (!Number.isInteger(parsed) || parsed < 1 || parsed > 4094) {
        setError('A VLAN id is a whole number between 1 and 4094.');
        return;
      }
      request.vlanId = parsed;
    }

    setBusy(true);
    try {
      const result = await generate.mutateAsync({ zoneId, wlanId, request });
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      setIssued(result?.list ?? []);
    } catch (err) {
      setError(
        err instanceof SmartZoneError ? err.displayMessage : 'The controller refused.',
      );
    } finally {
      setBusy(false);
    }
  }, [count, deviceLimit, expiresInDays, generate, userName, vlan, wlanId, zoneId]);

  if (issued) {
    return <IssuedKeys keys={issued} />;
  }

  return (
    <Screen scroll>
      <Stack.Screen options={{ title: 'Generate keys' }} />

      <Group header="Where">
        {zones.isLoading ? (
          <Row title="Reading zones…" />
        ) : (
          <View style={{ paddingVertical: t.space.sm }}>
            <ChipBar
              value={zoneId ?? ''}
              onChange={(next) => {
                setZoneId(next);
                setWlanId(undefined);
              }}
              options={zoneOptions}
            />
          </View>
        )}
        {!zoneId ? (
          <Row title="Choose a zone" subtitle="Keys belong to a WLAN inside a zone" />
        ) : wlans.isLoading ? (
          <Row title="Reading WLANs…" />
        ) : wlanOptions.length === 0 ? (
          <Row
            title="No WLAN in this zone issues keys"
            subtitle="Turn on dynamic PSKs on a WLAN in the controller first"
          />
        ) : (
          <View style={{ paddingVertical: t.space.sm }}>
            <ChipBar value={wlanId ?? ''} onChange={setWlanId} options={wlanOptions} />
          </View>
        )}
      </Group>

      <Group header="Keys">
        <Field
          label="Name"
          value={userName}
          onChangeText={setUserName}
          placeholder="Flat 3B, or Conference"
          autoCapitalize="words"
          hint="A batch of more than one gets numbered: Conference-1, Conference-2."
        />
        <Field
          label="How many"
          value={count}
          onChangeText={setCount}
          keyboardType="number-pad"
        />
        <Field
          label="Devices per key"
          value={deviceLimit}
          onChangeText={setDeviceLimit}
          keyboardType="number-pad"
          hint="How many devices may share one passphrase."
        />
        <Field
          label="Expires in (days)"
          value={expiresInDays}
          onChangeText={setExpiresInDays}
          keyboardType="number-pad"
          placeholder="Leave blank for no expiry"
        />
        <Field
          label="VLAN (optional)"
          value={vlan}
          onChangeText={setVlan}
          keyboardType="number-pad"
          placeholder="Overrides the WLAN's VLAN for these keys"
        />
      </Group>

      {error ? (
        <Label variant="callout" tone="down">
          {error}
        </Label>
      ) : null}

      <Button
        title={`Generate ${count || '1'} key${count === '1' ? '' : 's'}`}
        loading={busy}
        disabled={busy || !zoneId || !wlanId}
        onPress={() => void submit()}
      />
    </Screen>
  );
}

/**
 * The keys, once. The controller will not show a passphrase this plainly
 * again on some builds, so the screen says so rather than letting somebody
 * walk away assuming they can come back for it.
 */
function IssuedKeys({ keys }: { keys: Dpsk[] }) {
  const t = useTheme();

  const copyAll = useCallback(async () => {
    const text = keys
      .map((k) => `${firstNonEmpty(k.userName)}: ${k.passphrase ?? ''}`)
      .join('\n');
    await Clipboard.setStringAsync(text);
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    Alert.alert('Copied', `${keys.length} key${keys.length === 1 ? '' : 's'} on the clipboard.`);
  }, [keys]);

  return (
    <Screen scroll>
      <Stack.Screen options={{ title: 'Keys issued' }} />

      <Card style={{ gap: t.space.sm }}>
        <Label variant="headline" tone="up">
          {keys.length} key{keys.length === 1 ? '' : 's'} issued
        </Label>
        <Muted>
          Hand these out now. They are live on the WLAN, and the passphrases
          are easiest to read here.
        </Muted>
      </Card>

      <Group header="Passphrases">
        {keys.map((key, i) => (
          <Row
            key={key.id ?? i}
            title={firstNonEmpty(key.userName)}
            subtitle={key.passphrase ?? 'Not returned'}
            right={
              <Button
                title="Copy"
                variant="plain"
                onPress={async () => {
                  if (!key.passphrase) return;
                  await Clipboard.setStringAsync(key.passphrase);
                  void Haptics.selectionAsync();
                }}
              />
            }
          />
        ))}
      </Group>

      <View style={{ gap: t.space.sm }}>
        <Button title="Copy them all" variant="secondary" onPress={() => void copyAll()} />
        <Button title="Done" onPress={() => router.back()} />
      </View>
    </Screen>
  );
}
