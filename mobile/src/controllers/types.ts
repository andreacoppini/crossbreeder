/**
 * A saved connection to one SmartZone cluster.
 *
 * Nothing secret lives in this object: it is kept in ordinary storage so the
 * controller list renders before any biometric prompt. The password sits in
 * the Keychain (iOS) or Keystore (Android) under `passwordKey`, and the
 * service ticket beside it.
 */
export interface ControllerProfile {
  id: string;
  /** What the operator calls this cluster. "HQ", "Site B", "Lab". */
  label: string;
  host: string;
  port: number;
  username: string;
  /** Only set on a multi-domain (MSP) cluster. */
  domainId?: string;
  /** Pinned API version; unset means negotiate on connect. */
  apiVersion?: string;
  /** Filled in after the first successful connection. */
  controllerVersion?: string;
  /** Accent colour, so two clusters are not confused at a glance. */
  tint?: string;
  createdAt: number;
  lastUsedAt?: number;
  /**
   * Set when the operator has chosen to trust this controller's self-signed
   * certificate. Recorded so the app can explain what it is relying on, and
   * so a later build can pin the fingerprint.
   */
  acceptedSelfSignedAt?: number;
}

/** What a QR code or a pasted link can carry. Never a password. */
export interface ControllerBootstrapPayload {
  host: string;
  port?: number;
  username?: string;
  label?: string;
  domainId?: string;
  apiVersion?: string;
}

export interface ConnectionProbe {
  reachable: boolean;
  /** Versions the controller offered, newest last. */
  apiSupportVersions?: string[];
  /** The version this app settled on. */
  negotiatedVersion?: string;
  /** Set when the failure was a rejected certificate rather than no route. */
  certificateRejected?: boolean;
  message?: string;
}
