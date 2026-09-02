import React, { useCallback, useEffect, useState } from 'react';
import { Alert, RefreshControl, Switch, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import * as Haptics from 'expo-haptics';
import { SmartZoneError } from '@/api';
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
 * Every save sends only the fields that actually changed: SmartZone rejects a
 * PATCH carrying keys it did not itself return, and a WLAN object carries
 * dozens of them.
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
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Seed the form once the WLAN arrives, and again after a refetch.
  useEffect(() => {
    const data = wlan.data;
    if (!data) return;
    setName(data.name ?? '');
    setSsid(data.ssid ?? '');
    setBroadcast(data.ssidBroadcastEnabled !== false);
    setVlan(data.vlanId != null ? String(data.vlanId) : '');
    setPassphrase('');
  }, [wlan.data]);

  const data = wlan.data;

  const dirty = Boolean(
    data &&
      (name !== (data.name ?? '') ||
        ssid !== (data.ssid ?? '') ||
        broadcast !== (data.ssidBroadcastEnabled !== false) ||
        vlan !== (data.vlanId != null ? String(data.vlanId) : '') ||
        passphrase.length > 0),
  );

  const save = useCallback(async () => {
    if (!data) return;

    const patch: Record<string, unknown> = {};
    if (name !== (data.name ?? '')) patch.name = name;
    if (ssid !== (data.ssid ?? '')) patch.ssid = ssid;
    if (broadcast !== (data.ssidBroadcastEnabled !== false)) {
      patch.ssidBroadcastEnabled = broadcast;
    }
    if (vlan !== (data.vlanId != null ? String(data.vlanId) : '')) {
      const parsed = Number(vlan);
      if (!Number.isInteger(parsed) || parsed < 1 || parsed > 4094) {
        setError('A VLAN id is a whole number between 1 and 4094.');
        return;
      }
      patch.vlanId = parsed;
    }
    if (passphrase) {
      if (passphrase.length < 8) {
        setError('A WPA passphrase is at least 8 characters.');
        return;
      }
      // Carry the existing encryption settings through: SmartZone validates
      // the whole encryption object, not the passphrase on its own.
      patch.encryption = { ...(data.encryption ?? {}), passphrase };
    }

    if (Object.keys(patch).length === 0) return;

    setSaving(true);
    setError(null);
    try {
      await update.mutateAsync(patch);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      setPassphrase('');
      Alert.alert('Saved', 'The controller has taken the change.');
    } catch (err) {
      setError(
        err instanceof SmartZoneError ? err.displayMessage : 'The controller refused the change.',
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
          <Pill label={security} tone={/none/i.test(security) ? 'warn' : 'up'} />
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
          hint="1 to 4094."
        />
      </Group>

      {data.encryption?.method && !/none/i.test(data.encryption.method) ? (
        <Group
          header="Passphrase"
          footer="The controller never returns the current key, so this box is blank until you set a new one. Saving a new passphrase disconnects every client on this WLAN."
        >
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
        <Stat label="Zone" value={firstNonEmpty(zoneId)} mono />
        <Stat label="Encryption" value={firstNonEmpty(data.encryption?.method)} />
        <Stat label="Algorithm" value={firstNonEmpty(data.encryption?.algorithm)} />
        <Stat label="Per-device keys" value={data.dpskEnabled ? 'Enabled (DPSK)' : 'No'} />
        <Stat
          label="Client isolation"
          value={data.clientIsolationEnabled ? 'On' : 'Off'}
        />
      </Group>

      {data.dpskEnabled ? (
        <Group header="Dynamic PSKs">
          <Row
            title="Manage keys on this WLAN"
            subtitle="Issue, share and revoke per-device passphrases"
            onPress={() =>
              router.push({ pathname: '/dpsk', params: { zoneId, wlanId: id } })
            }
          />
        </Group>
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
