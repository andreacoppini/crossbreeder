import React, { useCallback, useMemo, useState } from 'react';
import { Alert, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import * as Clipboard from 'expo-clipboard';
import * as Haptics from 'expo-haptics';
import { SmartZoneError, type Dpsk, type DpskBatchRequest } from '@/api';
import { useDpskEnabledWlans, useDpskMutations, useZones } from '@/hooks/queries';
import {
  Button,
  Card,
  ChipBar,
  Field,
  Group,
  Label,
  Muted,
  Row,
  Screen,
} from '@/ui/components';
import { useTheme } from '@/ui/theme';
import { firstNonEmpty } from '@/utils/format';

/**
 * Issue new keys.
 *
 * The passphrase box is the important part. SmartZone will never read a DPSK
 * passphrase back, so a key the controller invents is one nobody can look up
 * again — fine for a batch you hand out on the spot, useless for a tenant who
 * rings up in six months. Setting it explicitly is the only way to have it on
 * record, and an explicit passphrase also overrides the WLAN's configured
 * DPSK length, so it is accepted even when shorter than that setting.
 */
export default function GenerateDpskScreen() {
  const t = useTheme();
  const params = useLocalSearchParams<{ zoneId?: string; wlanId?: string }>();

  const zones = useZones();
  const [zoneId, setZoneId] = useState<string | undefined>(params.zoneId || undefined);
  const wlans = useDpskEnabledWlans(zoneId);
  const [wlanId, setWlanId] = useState<string | undefined>(params.wlanId);

  const [userName, setUserName] = useState('');
  const [count, setCount] = useState('1');
  const [passphrase, setPassphrase] = useState('');
  const [shared, setShared] = useState(false);
  const [expiresInDays, setExpiresInDays] = useState('');
  const [vlan, setVlan] = useState('');

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [issued, setIssued] = useState<(Dpsk & { passphrase?: string })[] | null>(null);
  const [issuedPassphrase, setIssuedPassphrase] = useState<string | null>(null);

  const { generate } = useDpskMutations();

  const zoneOptions = useMemo(
    () => (zones.data ?? []).map((z) => ({ value: z.id, label: z.name })),
    [zones.data],
  );
  const wlanOptions = useMemo(
    () =>
      (wlans.data?.list ?? []).map((w) => ({
        value: w.wlanId,
        label: firstNonEmpty(w.ssid, w.wlanName, w.wlanId),
      })),
    [wlans.data],
  );

  const numberOfKeys = Number(count);

  const submit = useCallback(async () => {
    setError(null);

    if (!zoneId || !wlanId) {
      setError('Choose a zone and a WLAN that issues keys.');
      return;
    }
    if (!Number.isInteger(numberOfKeys) || numberOfKeys < 1 || numberOfKeys > 500) {
      setError('Between 1 and 500 keys at a time.');
      return;
    }
    if (!userName.trim()) {
      setError('Give the keys a name; a batch gets numbered from it.');
      return;
    }
    if (passphrase && numberOfKeys > 1) {
      setError('One passphrase cannot be shared across a batch. Ask for a single key, or leave it blank.');
      return;
    }
    if (passphrase && passphrase.length < 8) {
      setError('A DPSK passphrase is at least 8 characters.');
      return;
    }

    const request: DpskBatchRequest = {
      amount: numberOfKeys,
      userName: userName.trim(),
      groupDpsk: shared,
    };
    if (passphrase) request.passphraseList = [passphrase];

    if (expiresInDays) {
      const days = Number(expiresInDays);
      if (!Number.isInteger(days) || days < 1) {
        setError('An expiry is a whole number of days.');
        return;
      }
      request.expirationDate = new Date(Date.now() + days * 86_400_000).toISOString();
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
      setIssuedPassphrase(passphrase || null);
      setIssued(result?.list ?? []);
    } catch (err) {
      setError(err instanceof SmartZoneError ? err.displayMessage : 'The controller refused.');
    } finally {
      setBusy(false);
    }
  }, [
    expiresInDays,
    generate,
    numberOfKeys,
    passphrase,
    shared,
    userName,
    vlan,
    wlanId,
    zoneId,
  ]);

  if (issued) {
    return <IssuedKeys keys={issued} chosenPassphrase={issuedPassphrase} />;
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
          label="Passphrase"
          value={passphrase}
          onChangeText={setPassphrase}
          placeholder="Leave blank to let the controller choose"
          mono
          autoCapitalize="none"
          hint={
            numberOfKeys > 1
              ? 'Only for a single key. A batch always gets controller-generated passphrases.'
              : 'The controller will never show you a passphrase again. Set your own if you will need to look it up.'
          }
        />
        <Row
          title="Shared key"
          subtitle={
            shared
              ? 'One passphrase many devices can use'
              : 'Binds to the first device that uses it'
          }
          right={
            <Button
              title={shared ? 'Shared' : 'Per device'}
              variant="secondary"
              onPress={() => setShared((v) => !v)}
            />
          }
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
 * The keys, once.
 *
 * This is genuinely the only time a controller-generated passphrase can be
 * read, so the screen says so rather than letting somebody walk away assuming
 * they can come back for it.
 */
function IssuedKeys({
  keys,
  chosenPassphrase,
}: {
  keys: (Dpsk & { passphrase?: string })[];
  chosenPassphrase: string | null;
}) {
  const t = useTheme();
  const withPassphrases = keys.filter((k) => k.passphrase);

  const copyAll = useCallback(async () => {
    const text = withPassphrases
      .map((k) => `${firstNonEmpty(k.userName)}: ${k.passphrase}`)
      .join('\n');
    if (!text) return;
    await Clipboard.setStringAsync(text);
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    Alert.alert('Copied', `${withPassphrases.length} key(s) on the clipboard.`);
  }, [withPassphrases]);

  return (
    <Screen scroll>
      <Stack.Screen options={{ title: 'Keys issued' }} />

      <Card style={{ gap: t.space.sm }}>
        <Label variant="headline" tone="up">
          {keys.length || 1} key{keys.length === 1 ? '' : 's'} issued
        </Label>
        {chosenPassphrase ? (
          <Muted>
            Issued with the passphrase you chose. Keep it somewhere: the
            controller will not show it again.
          </Muted>
        ) : withPassphrases.length > 0 ? (
          <Muted>
            This is the only time these passphrases can be read. The controller
            will not return them again.
          </Muted>
        ) : (
          <Muted>
            The controller did not return the passphrases it generated, and it
            will not read them back later. To have a key on record, issue it
            with a passphrase you choose.
          </Muted>
        )}
      </Card>

      {chosenPassphrase ? (
        <Group header="Passphrase">
          <Row title={chosenPassphrase} subtitle="As you set it" />
        </Group>
      ) : null}

      {withPassphrases.length > 0 ? (
        <Group header="Passphrases">
          {withPassphrases.map((key, i) => (
            <Row
              key={key.key ?? i}
              title={firstNonEmpty(key.userName)}
              subtitle={key.passphrase}
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
      ) : null}

      <View style={{ gap: t.space.sm }}>
        {withPassphrases.length > 1 ? (
          <Button title="Copy them all" variant="secondary" onPress={() => void copyAll()} />
        ) : null}
        <Button title="Done" onPress={() => router.back()} />
      </View>
    </Screen>
  );
}
