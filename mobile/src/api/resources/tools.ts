import type { SmartZoneClient } from '../client';
import { withPath } from '../client';

/**
 * The controller's own diagnostic tools, run from the AP rather than the
 * phone. That distinction is the whole value: a ping from an AP's uplink
 * proves something a ping from the engineer's handset does not.
 */

export interface PingResult {
  /** Raw output as the controller returns it. */
  result?: string;
  status?: string;
  minRtt?: number;
  maxRtt?: number;
  avgRtt?: number;
  packetLoss?: number;
}

export interface TraceRouteResult {
  result?: string;
  status?: string;
}

/** SpeedFlex measures AP-to-client throughput without a server in the path. */
export interface SpeedFlexRequest {
  apMac?: string;
  clientMac?: string;
  /** `Downlink`, `Uplink`, `Both`. */
  direction?: string;
  protocol?: 'TCP' | 'UDP';
  duration?: number;
}

export interface SpeedFlexResult {
  wcid?: string;
  status?: string;
  downlink?: number;
  uplink?: number;
  loss?: number;
  latency?: number;
  result?: Record<string, unknown>;
}

export function toolsApi(client: SmartZoneClient) {
  return {
    ping(
      params: { apMac: string; target: string; packetSize?: number },
      signal?: AbortSignal,
    ) {
      return client.get<PingResult>('/tool/ping', {
        query: {
          apMac: params.apMac,
          host: params.target,
          packetSize: params.packetSize,
        },
        // The AP answers when it has finished pinging, not before.
        timeoutMs: 45_000,
        signal,
      });
    },

    traceRoute(
      params: { apMac: string; target: string },
      signal?: AbortSignal,
    ) {
      return client.get<TraceRouteResult>('/tool/traceRoute', {
        query: { apMac: params.apMac, host: params.target },
        timeoutMs: 90_000,
        signal,
      });
    },

    /** Start a SpeedFlex run; poll `speedFlexResult` with the returned id. */
    startSpeedFlex(request: SpeedFlexRequest, signal?: AbortSignal) {
      return client.post<{ wcid?: string }>('/tool/speedflex', request, { signal });
    },

    speedFlexResult(wcid: string, signal?: AbortSignal) {
      return client.get<SpeedFlexResult>(
        withPath('/tool/speedflex/{wcid}', { wcid }),
        { signal },
      );
    },

    /** AP-to-controller throughput test, for uplink complaints. */
    startApSpeedTest(apMac: string, signal?: AbortSignal) {
      return client.post<Record<string, unknown>>(
        '/tool/speedTestC',
        { apMac },
        { signal },
      );
    },

    apSpeedTestResult(apMac: string, signal?: AbortSignal) {
      return client.get<Record<string, unknown>>(
        withPath('/tool/speedTestC/{apMac}', { apMac }),
        { signal },
      );
    },

    /** Start capturing packets on an AP, to be downloaded afterwards. */
    startPacketCapture(
      apMac: string,
      options: Record<string, unknown>,
      signal?: AbortSignal,
    ) {
      return client.post<void>(
        withPath('/aps/{apMac}/apPacketCapture/startFileCapture', { apMac }),
        options,
        { signal },
      );
    },

    packetCaptureStatus(apMac: string, signal?: AbortSignal) {
      return client.get<Record<string, unknown>>(
        withPath('/aps/{apMac}/apPacketCapture', { apMac }),
        { signal },
      );
    },

    stopPacketCapture(apMac: string, signal?: AbortSignal) {
      return client.post<void>(
        withPath('/aps/{apMac}/apPacketCapture/stop', { apMac }),
        undefined,
        { signal },
      );
    },
  };
}
