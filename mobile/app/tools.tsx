import React, { useCallback, useMemo, useState } from 'react';
import { View } from 'react-native';
import { Stack } from 'expo-router';
import { SmartZoneError } from '@/api';
import { ConnectedOnly } from '@/controllers/ConnectedOnly';
import { useApi } from '@/controllers/ControllerProvider';
import { useApList } from '@/hooks/queries';
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
import { firstNonEmpty, formatMac } from '@/utils/format';

type Tool = 'ping' | 'traceroute';

/**
 * Ping and traceroute, run from an access point.
 *
 * The value is in where it runs. A ping from the engineer's phone proves the
 * phone's path; a ping from the AP's own uplink proves the AP's, which is the
 * one in question when a site says the wireless is broken and the wireless is
 * fine.
 */
export default function ToolsScreen() {
  // Outside the tab layout, so this route carries its own gate: it calls
  // useApi() directly, which throws until a controller is connected.
  return (
    <ConnectedOnly>
      <Diagnostics />
    </ConnectedOnly>
  );
}

function Diagnostics() {
  const t = useTheme();
  const api = useApi();

  const [tool, setTool] = useState<Tool>('ping');
  const [apSearch, setApSearch] = useState('');
  const [apMac, setApMac] = useState<string | undefined>();
  const [target, setTarget] = useState('8.8.8.8');
  const [running, setRunning] = useState(false);
  const [output, setOutput] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Only an online AP can run anything, and the controller will not filter by
  // status, so offline ones are dropped here instead.
  const aps = useApList({ search: apSearch, sortColumn: 'deviceName' });
  const apOptions = useMemo(
    () =>
      (aps.data?.pages.flatMap((p) => p.list ?? []) ?? [])
        .filter((ap) => ap.apMac && ap.status === 'Online')
        .slice(0, 25)
        .map((ap) => ({
          value: ap.apMac!,
          label: firstNonEmpty(ap.deviceName, ap.apMac),
        })),
    [aps.data],
  );

  const run = useCallback(async () => {
    if (!apMac || !target.trim()) return;
    setRunning(true);
    setOutput(null);
    setError(null);
    try {
      const result =
        tool === 'ping'
          ? await api.tools.ping({ apMac, target: target.trim() })
          : await api.tools.traceRoute({ apMac, target: target.trim() });
      setOutput(result?.result ?? JSON.stringify(result, null, 2));
    } catch (err) {
      setError(
        err instanceof SmartZoneError
          ? err.displayMessage
          : 'The controller could not run that.',
      );
    } finally {
      setRunning(false);
    }
  }, [api.tools, apMac, target, tool]);

  return (
    <Screen scroll>
      <Stack.Screen options={{ title: 'Diagnostics' }} />

      <Card style={{ gap: t.space.sm }}>
        <Label variant="headline">Run from an access point</Label>
        <Muted>
          The controller asks the AP to do this, so what comes back is the
          AP&apos;s view of the network, not this phone&apos;s.
        </Muted>
      </Card>

      <View style={{ marginHorizontal: -t.space.lg }}>
        <ChipBar<Tool>
          value={tool}
          onChange={setTool}
          options={[
            { value: 'ping', label: 'Ping' },
            { value: 'traceroute', label: 'Traceroute' },
          ]}
        />
      </View>

      <Group header="Access point">
        <Field
          label="Find an AP"
          value={apSearch}
          onChangeText={setApSearch}
          placeholder="Search online access points"
        />
        {apOptions.length === 0 ? (
          <Row
            title={aps.isLoading ? 'Reading access points…' : 'No online APs match'}
          />
        ) : (
          <View style={{ paddingVertical: t.space.sm, marginHorizontal: -t.space.lg }}>
            <ChipBar value={apMac ?? ''} onChange={setApMac} options={apOptions} />
          </View>
        )}
        {apMac ? <Row title="Selected" subtitle={formatMac(apMac)} /> : null}
      </Group>

      <Group header="Target">
        <Field
          label="Address or hostname"
          value={target}
          onChangeText={setTarget}
          placeholder="8.8.8.8 or gateway.example.com"
          keyboardType="url"
          mono
        />
      </Group>

      <Button
        title={tool === 'ping' ? 'Ping' : 'Trace the route'}
        loading={running}
        disabled={running || !apMac || !target.trim()}
        onPress={() => void run()}
      />

      {running ? (
        <Muted>
          {tool === 'ping'
            ? 'The AP answers when it has finished pinging.'
            : 'A traceroute can take a minute or more.'}
        </Muted>
      ) : null}

      {error ? (
        <Label variant="callout" tone="down">
          {error}
        </Label>
      ) : null}

      {output ? (
        <Card style={{ gap: t.space.sm }}>
          <Label variant="subhead">Result</Label>
          <Label variant="footnote" mono>
            {output}
          </Label>
        </Card>
      ) : null}
    </Screen>
  );
}
