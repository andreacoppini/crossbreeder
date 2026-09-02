# SmartZone Console

A native iOS and Android app for running a Ruckus SmartZone cluster from a
phone: access points, WLANs, their groups, dynamic PSKs, and client
troubleshooting.

> Part of the [Crossbreeder](../README.md) repository. Crossbreeder itself
> works *standalone* APs over SSH with no controller; this app is the other
> half of the estate, and talks to SmartZone over its public API.

## Why React Native and Expo

The brief allowed React Native/Expo or Swift/SwiftUI. It asked for iOS **and**
Android, and SwiftUI only delivers one of them, so this is Expo (SDK 57, React
Native 0.86) in TypeScript.

That is not a compromise on feel. The app uses the platform's own navigation
stacks and tab bars through `expo-router`, the real Keychain and Keystore
through `expo-secure-store`, system light and dark schemes, and haptics on
every action that changes something. Nothing here is a web view.

## Getting started

```sh
cd mobile
npm install
npx expo start            # then open in Expo Go, or press i / a
```

Expo Go covers most of the app. The camera and keychain modules need a
development build, which is the point at which you also get the certificate
handling described below:

```sh
npx expo prebuild         # generates ios/ and android/
npx expo run:ios          # or run:android
```

Checks:

```sh
npm test                  # jest
npm run typecheck         # tsc --noEmit
```

## Connecting to a controller

Three ways in, in the order of how much typing they cost:

1. **Scan a QR code.** `Connect → Scan`. The code carries the address, port,
   username and a label.
2. **Paste a link or a URL.** Anything of the shape
   `szconsole://connect?host=…`, a JSON blob, or the `https://sz.example.com:8443/wsg/…`
   URL out of a browser's address bar. It is parsed down to a host and a port.
3. **Type the address.** `sz.example.com`, `10.1.20.5:8443` or an IPv6
   literal, with or without a scheme, port or path.

Whichever route, the address is tested on its own before any credentials are
asked for. That step separates the three failures that actually happen —
wrong address, blocked port, untrusted certificate — and each gets its own
answer instead of one "could not connect".

**A bootstrap payload never carries a password.** A QR code ends up
photographed, screenshotted and pasted into a group chat, and an administrator
password on a SmartZone cluster is the whole estate. The payload gets you to a
pre-filled sign-in form; the password is typed once and goes to the device
keychain.

You need an administrator account on the cluster and its public API reachable
on port 8443.

### Certificates

Almost every SmartZone cluster answers on a self-signed certificate, and that
is the single most common reason a controller app fails on first run. This app
**will not accept a certificate it cannot verify** — an app holding
administrator credentials for an entire wireless estate is exactly the one
that should not — so the certificate has to be installed on the device:

- **iOS**: mail or AirDrop the certificate to yourself, install the profile,
  then enable it under *Settings → General → About → Certificate Trust
  Settings*. `Info.plist` cannot do this for you: App Transport Security
  exceptions relax the cipher and protocol policy, not the trust evaluation.
- **Android**: install it under *Settings → Security → Encryption &
  credentials*. Since API 24 an app ignores user-installed CAs unless it opts
  in, so [`plugins/withControllerTrust.js`](plugins/withControllerTrust.js)
  writes a network security config that trusts the system store *and* the user
  store, and nothing else. Cleartext HTTP stays off.

A controller with a certificate from your own internal CA, or a public one,
needs none of this.

### Where credentials live

The administrator password goes to the iOS Keychain or the Android Keystore,
marked `WHEN_UNLOCKED_THIS_DEVICE_ONLY`, so it is reachable only while the
device is unlocked and never travels in a backup to another device. The
service ticket is kept beside it and discarded after 24 hours, which is when
SmartZone expires it anyway. Returning to a controller offers Face ID or the
fingerprint reader before the saved password is used — a gate on using it, not
a second factor, and what stops a handed-over unlocked phone from rebooting an
estate.

Nothing is sent anywhere except to the controller you named. There is no
backend.

## What it does

**Overview** — APs online, flagged and offline, each tapping through to that
filter; client count; outstanding alarms with severity.

**Access points** — server-side search, filter by status and zone, paged as
you scroll. Per AP: status, uptime, traffic, clients, identity, per-radio
channel, client count, airtime and power, and CPU/memory where reported.
Blink the LEDs to find it on a ceiling; reboot it, behind a confirmation.

**WLANs** — every SSID across every zone, with security, VLAN, client count
and traffic. Editable from a phone: name, SSID, broadcast, VLAN and the
passphrase. Deliberately *not* editable: authentication model, portals,
tunnelling — changes whose consequences should be visible on a real screen.

**Zones and groups** — the shape of the cluster: zones, their AP groups, their
WLAN groups, and the firmware their APs are pinned to. Read-only, because a
zone-level change reaches every AP in the zone at once.

**Dynamic PSKs** — search across the cluster, generate a batch against any
DPSK-enabled WLAN with a name, device limit, expiry and VLAN override, reveal
and copy a passphrase one tap at a time, revoke, and export the current filter
as CSV through the share sheet.

**Client troubleshooting** — the screen this app exists for. It answers the
questions in the order an engineer asks them:

1. *Is the radio link any good?* RSSI and SNR with a plain-language verdict,
   because a number in dBm means nothing to whoever raised the ticket.
2. *Did it authenticate?* An associated-but-unauthorised client looks
   identical to a working one on every summary screen, and is the most common
   "it's connected but nothing works".
3. *What is it attached to, and on what?* AP, band, channel, BSSID, VLAN.
4. *Has it been dropping?* Past sessions with the controller's own disconnect
   reasons — where a client keeps reconnecting, that pattern says more than
   any single live reading.

Then disconnect (forces a re-association) or deauthenticate (forces a fresh
authentication too).

**Alarms and events**, kept apart because they answer different questions, with
acknowledgement.

**Diagnostics** — ping and traceroute *from an access point*. The value is in
where it runs: a ping from your handset proves your handset's path; one from
the AP's uplink proves the AP's, which is the one in question when a site says
the wireless is broken and the wireless is fine.

## Room for switches

Switching is the next thing this app grows into, and the shape of that growth
is settled now so it disturbs nothing later:

- [`src/api/resources/switches.ts`](src/api/resources/switches.ts) already
  hangs off the same client, session and query builder as Wi-Fi, with typed
  `SwitchRow`, `SwitchPort` and `SwitchGroup`. What is missing is screens, not
  plumbing.
- The **More** tab carries a Switches row from this first release rather than
  having one appear out of nowhere, and the overview surfaces the cluster's
  switch count when it has one.
- Ports carry `neighbourMacAddress`, which is the join that makes the
  interesting screen possible: the AP you are looking at, and the switch port
  it is powered from.

## How it is put together

```
src/api/          The SmartZone client. Framework-free and unit-tested.
  transport.ts      One HTTP round trip; SmartZone's response conventions.
  client.ts         Version negotiation, service tickets, pagination.
  query.ts          Criteria builder for the POST /query/* endpoints.
  errors.ts         The error taxonomy the UI switches on.
  resources/        One module per area of the API.
src/controllers/  Profiles, secure storage, QR/URL bootstrap, React context.
src/hooks/        React Query bindings, keyed per controller.
src/ui/           Theme and the shared component vocabulary.
src/utils/        Formatting, so a value never reads two ways.
app/              Routes (expo-router, file-based).
plugins/          Config plugin for certificate trust.
```

Four decisions are worth knowing about:

**Version negotiation.** SmartZone's API is versioned in the path
(`/wsg/api/public/v11_0/…`). The client asks `apiInfo` what the controller
speaks and takes the newest version both sides know, so a 5.2 cluster and a
7.x cluster both work with nothing to configure. Versions are compared
numerically — `v9_0` sorts *after* `v11_0` as a string, and that bug has a
test.

**One re-login, shared.** The ticket travels as a `serviceTicket` query
parameter and dies after 24 hours or a cluster failover. A 401 triggers one
re-login and one replay. Concurrent callers that all hit a dead ticket share a
single login rather than stampeding a controller that will start refusing
them.

**Server-side everything.** Search, filter, sort and paging all happen on the
controller through the `POST /query/*` endpoints. A cluster with three thousand
APs costs one request per screenful, not a download.

**Errors have kinds.** Every failure reaches the UI as a `SmartZoneError` with
a `kind` a screen can switch on, so nothing has to parse a message string to
decide whether to offer "sign in again", "trust this certificate" or "retry".

## Known limits

- Written against the SmartZone public API schema. The live controller this
  was to be validated against was unreachable during development
  (`http_526` at its edge), so the endpoint paths, request shapes and response
  envelopes come from the API specification rather than from observed traffic.
  Field-level differences between controller versions are the most likely
  place to need a fix; every response model treats every field as optional so
  a missing one degrades a row rather than crashing a screen.
- WLAN creation is wired in the API layer (`wlans.create`, one endpoint per
  authentication style) but has no screen: the form that does it justice is
  larger than the editor, and a half-built WLAN is worse than none.
- Guest passes, block lists, packet capture and SpeedFlex are implemented in
  the API layer without screens.
- No push notifications for alarms. That needs somewhere to run, which this
  app deliberately does not have.
- Web builds run, but `expo-secure-store` has no browser equivalent; the
  fallback warns loudly and should not be used with a production controller.
