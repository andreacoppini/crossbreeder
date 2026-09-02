import React, { useCallback, useEffect, useState } from 'react';
import { Alert, RefreshControl, Switch, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import * as Clipboard from 'expo-clipboard';
import * as Haptics from 'expo-haptics';
import {
  SmartZoneError,
  accessVlan,
  isDpskWlan,
  isExternalDpskWlan,
  isOpenWlan,
  isSsidBroadcast,
  wlanPatch,
} from '@/api';
import { useWlan, useWlanMutations } from '@/hooks/queries';
import {
  Button,
  Card,
  ErrorState,
  Field,
  Group,
  Label,
  Loading,
  Muted,
  Pill,
  Row,
  Screen,
  Stat,
} from '@/ui/components';
import { useTheme } from '@/ui/theme';
import { firstNonEmpty } from '@/utils/format';

/**
 * One WLAN, readable and editable.
 *
 * What can be changed from a phone is deliberately narrow: name, SSID,
 * broadcast, VLAN and the passphrase. Those are the changes that get made
 * under pressure and are safe to make in a hurry. Anything that reshapes a
 * WLAN — its authentication model, its portal, its tunnelling — is left to
 * the controller's own UI, where the consequences are visible.
 *
 * The configuration object nests where the list endpoint flattens: the VLAN
 * is `vlan.accessVlan`, DPSK is `dpsk.dpskEnabled`, and broadcast is the
 * *inverse* of `advancedOptions.hideSsidEnabled`. `wlanPatch` builds an
 * update that keeps each nested object whole, because sending a partial one
 * clears the keys left out of it.
 */
export default function WlanDetailScreen() {
  const t = useTheme();
  const { zoneId, id } = useLocalSearchParams<{ zoneId: string; id: string }>();
  const wlan = useWlan(zoneId, id);
  const { update, remove } = useWlanMutations(zoneId ?? '', id ?? '');

  const [name, setName] = useState('');
  const [ssid, setSsid] = useState('');
  const [broadcast, setBroadcast] = useState(true);
  const [vlan, setVlan] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const [revealed, setRevealed] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const data = wlan.data;

  // Seed the form once the WLAN arrives, and again after a refetch.
  useEffect(() => {
    if (!data) return;
    setName(data.name ?? '');
    setSsid(data.ssid ?? '');
    setBroadcast(isSsidBroadcast(data));
    const av = accessVlan(data);
    setVlan(av != null ? String(av) : '');
    setPassphrase('');
    setRevealed(false);
  }, [data]);

  const dirty = Boolean(
    data &&
      (name !== (data.name ?? '') ||
        ssid !== (data.ssid ?? '') ||
        broadcast !== isSsidBroadcast(data) ||
        vlan !== (accessVlan(data) != null ? String(accessVlan(data)) : '') ||
        passphrase.length > 0),
  );

  const save = useCallback(async () => {
    if (!data) return;
    setError(null);

    let parsedVlan: number | undefined;
    if (vlan !== (accessVlan(data) != null ? String(accessVlan(data)) : '')) {
      parsedVlan = Number(vlan);
      if (!Number.isInteger(parsedVlan) || parsedVlan < 1 || parsedVlan > 4094) {
        setError('A VLAN id is a whole number between 1 and 4094.');
        return;
      }
    }
    if (passphrase && passphrase.length < 8) {
      setError('A WPA passphrase is at least 8 characters.');
      return;
    }

    const patch = wlanPatch(data, {
      name,
      ssid,
      broadcast,
      accessVlan: parsedVlan,
      passphrase: passphrase || undefined,
    });
    if (Object.keys(patch).length === 0) return;

    setSaving(true);
    try {
      await update.mutateAsync(patch);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      setPassphrase('');
      Alert.alert('Saved', 'The controller has taken the change.');
    } catch (err) {
      setError(
        err instanceof SmartZoneError
          ? err.displayMessage
          : 'The controller refused the change.',
      );
    } finally {
      setSaving(false);
    }
  }, [broadcast, data, name, passphrase, ssid, update, vlan]);

  const confirmDelete = useCallback(() => {
    Alert.alert(
      'Delete this WLAN?',
      `${firstNonEmpty(data?.ssid, data?.name)} will stop being broadcast everywhere it is deployed. This cannot be undone.`,
      [
        { text: 'Cancel', style: 'cancel' },
        {
          text: 'Delete',
          style: 'destructive',
          onPress: async () => {
            try {
              await remove.mutateAsync();
              router.back();
            } catch (err) {
              Alert.alert(
                'Could not delete',
                err instanceof SmartZoneError ? err.displayMessage : 'The controller refused.',
              );
            }
          },
        },
      ],
    );
  }, [data?.name, data?.ssid, remove]);

  if (wlan.isLoading) {
    return (
      <Screen>
        <Stack.Screen options={{ title: 'WLAN' }} />
        <Loading label="Reading the WLAN" />
      </Screen>
    );
  }

  if (wlan.isError || !data) {
    return (
      <Screen scroll>
        <Stack.Screen options={{ title: 'WLAN' }} />
        <ErrorState
          message={
            wlan.error instanceof SmartZoneError
              ? wlan.error.displayMessage
              : 'That WLAN could not be read.'
          }
          onRetry={() => void wlan.refetch()}
        />
      </Screen>
    );
  }

  const security = firstNonEmpty(data.encryption?.method);
  const open = isOpenWlan(data);
  const currentPassphrase = data.encryption?.passphrase ?? undefined;

  return (
    <Screen
      scroll
      refreshControl={
        <RefreshControl refreshing={wlan.isRefetching} onRefresh={() => void wlan.refetch()} />
      }
    >
      <Stack.Screen options={{ title: firstNonEmpty(data.ssid, data.name) }} />

      <Card style={{ gap: t.space.md }}>
        <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' }}>
          <Label variant="title">{firstNonEmpty(data.ssid, data.name)}</Label>
          <Pill label={security} tone={open ? 'warn' : 'up'} />
        </View>
        {data.description ? <Muted>{data.description}</Muted> : null}
      </Card>

      <Group header="Settings">
        <Field label="WLAN name" value={name} onChangeText={setName} autoCapitalize="none" />
        <Field
          label="SSID"
          value={ssid}
          onChangeText={setSsid}
          hint="What clients see. Changing it disconnects everything on this WLAN."
        />
        <Row
          title="Broadcast the SSID"
          subtitle={broadcast ? 'Visible when scanning' : 'Hidden'}
          right={<Switch value={broadcast} onValueChange={setBroadcast} />}
        />
        <Field
          label="VLAN"
          value={vlan}
          onChangeText={setVlan}
          keyboardType="number-pad"
          hint="The access VLAN, 1 to 4094."
        />
      </Group>

      {!open ? (
        <Group
          header="Passphrase"
          footer="Saving a new passphrase disconnects every client on this WLAN."
        >
          {currentPassphrase ? (
            <Row
              title={revealed ? currentPassphrase : '••••••••••••'}
              subtitle={revealed ? 'The current key' : 'Tap to reveal the current key'}
              onPress={() => setRevealed((v) => !v)}
              right={
                revealed ? (
                  <Button
                    title="Copy"
                    variant="plain"
                    onPress={async () => {
                      await Clipboard.setStringAsync(currentPassphrase);
                      void Haptics.selectionAsync();
                      Alert.alert('Copied', 'The passphrase is on the clipboard.');
                    }}
                  />
                ) : null
              }
            />
          ) : (
            <Row
              title="Not readable"
              subtitle="This WLAN's key is not one the controller returns"
            />
          )}
          <Field
            label="New passphrase"
            value={passphrase}
            onChangeText={setPassphrase}
            secure
            placeholder="Leave blank to keep the current one"
            mono
          />
        </Group>
      ) : null}

      <Group header="Read only">
        <Stat label="Type" value={firstNonEmpty(data.type)} />
        <Stat label="Encryption" value={firstNonEmpty(data.encryption?.method)} />
        <Stat label="Algorithm" value={firstNonEmpty(data.encryption?.algorithm)} />
        <Stat label="Management frame protection" value={firstNonEmpty(data.encryption?.mfp)} />
        <Stat
          label="Client isolation"
          value={data.advancedOptions?.clientIsolationEnabled ? 'On' : 'Off'}
        />
        <Stat
          label="Per-device keys"
          value={
            isExternalDpskWlan(data)
              ? 'External DPSK'
              : isDpskWlan(data)
                ? `Enabled · ${firstNonEmpty(data.dpsk?.dpskType)}`
                : 'No'
          }
        />
      </Group>

      {isDpskWlan(data) && !isExternalDpskWlan(data) ? (
        <Group header="Dynamic PSKs">
          <Row
            title="Manage keys on this WLAN"
            subtitle="Issue and revoke per-device passphrases"
            onPress={() => router.push({ pathname: '/dpsk', params: { zoneId, wlanId: id } })}
          />
        </Group>
      ) : isExternalDpskWlan(data) ? (
        <Card style={{ gap: t.space.xs }}>
          <Label variant="subhead">External DPSK</Label>
          <Muted>
            This WLAN&apos;s keys live on an external server rather than the
            controller, so they cannot be managed from here.
          </Muted>
        </Card>
      ) : null}

      {error ? (
        <Label variant="callout" tone="down">
          {error}
        </Label>
      ) : null}

      <View style={{ gap: t.space.sm }}>
        <Button
          title="Save changes"
          disabled={!dirty || saving}
          loading={saving}
          onPress={() => void save()}
        />
        <Button title="Delete WLAN" variant="destructive" onPress={confirmDelete} />
      </View>
    </Screen>
  );
}
