import React, { useCallback, useMemo, useState } from 'react';
import { View } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { DEFAULT_API_PORT, SmartZoneError } from '@/api';
import { parseBootstrap, probeController } from '@/controllers/bootstrap';
import { useControllers } from '@/controllers/ControllerProvider';
import type { ConnectionProbe } from '@/controllers/types';
import {
  Button,
  Card,
  Field,
  Label,
  Muted,
  Pill,
  Screen,
} from '@/ui/components';
import { useTheme } from '@/ui/theme';

/**
 * Adding a controller, in two steps that are deliberately separate.
 *
 * First the address is tested on its own, with no credentials in play. That
 * is the step that distinguishes the three failures an operator actually
 * hits — wrong address, blocked port, untrusted certificate — and each one
 * needs a different answer. Only once the controller has answered does the
 * password get asked for, so a typo in the hostname never reads as "wrong
 * password".
 */
export default function AddControllerScreen() {
  const t = useTheme();
  const { addController } = useControllers();
  const params = useLocalSearchParams<{
    host?: string;
    port?: string;
    username?: string;
    label?: string;
    domainId?: string;
  }>();

  const [address, setAddress] = useState(
    params.host
      ? `${params.host}${params.port && params.port !== String(DEFAULT_API_PORT) ? `:${params.port}` : ''}`
      : '',
  );
  const [label, setLabel] = useState(params.label ?? '');
  const [username, setUsername] = useState(params.username ?? '');
  const [password, setPassword] = useState('');
  const [domainId, setDomainId] = useState(params.domainId ?? '');

  const [probe, setProbe] = useState<ConnectionProbe | null>(null);
  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  /** Whatever was typed, reduced to a host and a port. */
  const parsed = useMemo(() => parseBootstrap(address), [address]);
  const host = parsed?.host ?? '';
  const port = parsed?.port ?? DEFAULT_API_PORT;

  const test = useCallback(async () => {
    if (!parsed) {
      setError('That does not look like an address.');
      return;
    }
    setTesting(true);
    setError(null);
    setProbe(null);
    try {
      const result = await probeController(host, port);
      setProbe(result);
      if (result.reachable && !label) {
        // A sensible default the operator can overwrite.
        setLabel(host.split('.')[0] ?? host);
      }
    } finally {
      setTesting(false);
    }
  }, [host, label, parsed, port]);

  const save = useCallback(async () => {
    if (!parsed || !username || !password) return;
    setSaving(true);
    setError(null);
    try {
      await addController(
        {
          label: label.trim() || host,
          host,
          port,
          username: username.trim(),
          domainId: domainId.trim() || undefined,
          acceptedSelfSignedAt: probe?.certificateRejected
            ? Date.now()
            : undefined,
        },
        password,
      );
      router.replace('/');
    } catch (err) {
      setError(
        err instanceof SmartZoneError
          ? err.displayMessage
          : 'Could not sign in to the controller.',
      );
      setSaving(false);
    }
  }, [
    addController,
    domainId,
    host,
    label,
    parsed,
    password,
    port,
    probe?.certificateRejected,
    username,
  ]);

  const canSave = Boolean(parsed && username.trim() && password);

  return (
    <Screen scroll>
      <Card style={{ gap: t.space.lg }}>
        <Field
          label="Controller address"
          value={address}
          onChangeText={(next) => {
            setAddress(next);
            setProbe(null);
          }}
          placeholder="sz.example.com or 10.1.20.5:8443"
          keyboardType="url"
          hint={
            parsed
              ? `Will connect to https://${host}:${port}`
              : 'A hostname or address. Port 8443 unless you say otherwise.'
          }
          mono
          onSubmitEditing={() => void test()}
          returnKeyType="go"
        />

        <Button
          title={probe?.reachable ? 'Test again' : 'Test connection'}
          variant="secondary"
          loading={testing}
          disabled={!parsed || testing}
          onPress={() => void test()}
        />

        {probe ? <ProbeResult probe={probe} /> : null}
      </Card>

      <Card style={{ gap: t.space.lg }}>
        <Label variant="headline">Sign in</Label>
        <Field
          label="Name for this controller"
          value={label}
          onChangeText={setLabel}
          placeholder="HQ, Site B, Lab"
          autoCapitalize="words"
        />
        <Field
          label="Administrator"
          value={username}
          onChangeText={setUsername}
          placeholder="admin"
          hint="A SmartZone administrator account with API access."
        />
        <Field
          label="Password"
          value={password}
          onChangeText={setPassword}
          secure
          hint="Kept in this device's keychain, never in the QR code and never on our side."
        />
        <Field
          label="Domain (optional)"
          value={domainId}
          onChangeText={setDomainId}
          placeholder="Leave blank unless this is a multi-domain cluster"
        />

        {error ? (
          <Label variant="callout" tone="down">
            {error}
          </Label>
        ) : null}

        <Button
          title="Connect"
          loading={saving}
          disabled={!canSave || saving}
          onPress={() => void save()}
        />
      </Card>
    </Screen>
  );
}

/** What the probe found, and what to do about it. */
function ProbeResult({ probe }: { probe: ConnectionProbe }) {
  const t = useTheme();

  if (probe.reachable) {
    return (
      <View style={{ gap: t.space.sm }}>
        <Pill label="Controller answered" tone="up" />
        {probe.negotiatedVersion ? (
          <Muted>
            Speaking API {probe.negotiatedVersion.replace('_', '.')}
            {probe.apiSupportVersions?.length
              ? ` of ${probe.apiSupportVersions.length} offered.`
              : '.'}
          </Muted>
        ) : (
          <Muted>{probe.message}</Muted>
        )}
      </View>
    );
  }

  return (
    <View style={{ gap: t.space.sm }}>
      <Pill label={probe.certificateRejected ? 'Certificate refused' : 'No answer'} tone="down" />
      <Muted>{probe.message}</Muted>
      {probe.certificateRejected ? (
        <Muted>
          SmartZone ships with a self-signed certificate, which this device
          does not trust yet. Install the controller's certificate on the
          device — on iOS through a configuration profile, then enable it
          under Settings, General, About, Certificate Trust Settings; on
          Android under Settings, Security, Encryption &amp; credentials.
        </Muted>
      ) : (
        <Muted>
          Check the address and that this device can reach port {'8443'} on it.
          A cluster reachable only from a management network needs the VPN up
          first.
        </Muted>
      )}
    </View>
  );
}
