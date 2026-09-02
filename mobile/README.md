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

**Overview** — APs online, flagged and offline, counted from the AP query
because the controller has no summary that carries them; cluster inventory and
licensed capacity; outstanding alarms with severity.

**Access points** — server-side search and sort, filter by zone, paged as
you scroll. Per AP: status, uptime, traffic, clients, identity, per-radio
channel, client count, airtime and power, and the controller's own health
rollup. Blink the LEDs to find it on a ceiling; reboot it, behind a
confirmation.

**WLANs** — every SSID across every zone, with security, VLAN, client count
and traffic. Editable from a phone: name, SSID, broadcast, VLAN and the
passphrase. Deliberately *not* editable: authentication model, portals,
tunnelling — changes whose consequences should be visible on a real screen.

**Zones and groups** — the shape of the cluster: zones, their AP groups, their
WLAN groups, and the firmware their APs are pinned to. Read-only, because a
zone-level change reaches every AP in the zone at once.

**Dynamic PSKs** — search across the cluster, generate against any
DPSK-enabled WLAN with a name, shared-or-per-device, expiry and VLAN override,
revoke, and export the current filter as CSV through the share sheet.

Because SmartZone will not read a passphrase back (see below), a generated key
is shown once, at creation, and the generate form lets you set the passphrase
yourself — the only way to have it on record for the tenant who rings up in
six months.

**Client troubleshooting** — the screen this app exists for. It answers the
questions in the order an engineer asks them:

1. *Is the radio link any good?* RSSI and SNR with a plain-language verdict,
   because a number in dBm means nothing to whoever raised the ticket.
2. *Did it authenticate?* An associated-but-unauthorised client looks
   identical to a working one on every summary screen, and is the most common
   "it's connected but nothing works".
3. *What is it attached to, and on what?* AP, band, channel, BSSID, VLAN.
4. *Has it been dropping?* Past sessions, with how long each one lasted. The
   controller records no disconnect reason, so the screen does not invent one
   — but a column of two-minute sessions is a roaming or authentication
   problem, and says so more reliably than a reason code would.

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

- [`src/api/resources/switches.ts`](src/api/resources/switches.ts) hangs off
  the same client, session and query builder as Wi-Fi, with typed `SwitchRow`,
  `SwitchPort` and `SwitchGroup`.
- What is missing is not screens but an endpoint. Every public switch path
  probed against a 7.1.1 cluster managing 43 switches returned 404, so
  `probe()` asks the controller at runtime which path answers rather than
  shipping one that is known not to. That question — where the switch API
  actually lives on this release — is the first thing to settle before any
  switch screen is worth building.
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

## Verified against a real controller

Every read path in this app has been replayed against a live **SmartZone
7.1.1** cluster — 563 access points, ~490 clients, 16 WLANs, 649 DPSKs — and
the request shapes it sends are the ones that came back 200. That exercise
changed a lot, because the published schema and the controller disagree in
places that matter. The findings are worth knowing whoever works on this next:

- **The identifier on a WLAN row is `wlanId`, not `id`.** Getting that wrong
  makes every row in the list silently un-tappable.
- **DPSK passphrases are write-only.** Neither the WLAN's DPSK endpoint nor
  `POST /query/dpsk` returns one; a key row carries a `key` UUID and no
  passphrase at all. There is therefore no "reveal" in the key list and no
  passphrase column in the export, and the generate screen lets you set the
  passphrase yourself, because that is the only way to have it on record.
- **`acknowledged` on an alarm is the string `"Yes"`/`"No"`, not a boolean.**
  Reading it as truthy marks every open alarm acknowledged.
- **A client's `rssi`/`snr` of 0 means "no reading", not a perfect signal.**
  Scoring it as a number reports a dead client as excellent, which is the
  worst possible failure on the troubleshooting screen.
- **There is no `sessionDuration` field on a client**, only
  `sessionStartTime` — though `sessionDuration` *is* accepted as a sort
  column. And `radioType` is a PHY string (`"a/n/ac/ax/be"`), not a band, so
  the band comes from the channel number.
- **`filters` and `extraFilters` accept different type enums, and the
  controller enforces it.** `SSID` or `CLIENT` in `filters` is a 400. Both
  enums are transcribed in `src/api/query.ts`.
- **AP status cannot be filtered server-side.** A `STATUS` extraFilter is
  accepted and then matches nothing, and an `attributes` projection returns
  rows missing the projected field. So the overview counts statuses by paging
  the AP query, and the AP list sorts by status and narrows what it has
  loaded — and says which, rather than showing an empty list and implying
  there are no offline APs.
- **A DPSK `WLAN` filter matches nothing** in either slot, so narrowing to one
  WLAN happens locally.
- **`devicesSummary` carries no health breakdown.** On this cluster it reports
  `aps: 287` while the AP query finds 549 online: the two count different
  things and neither is a health figure. It is used for inventory and licensed
  capacity only.
- **The AP query row and `/aps/{mac}/operational/summary` use different
  names** for the same things (`apMac`/`mac`, `deviceName`/`name`,
  `numClients`/`clientCount`, `lastSeen`/`lastSeenTime`,
  `firmwareVersion`/`version`). The detail screen reads the query row, which
  is far richer; there is no CPU or memory figure on either.
- **A WLAN's own passphrase *is* returned in cleartext** by the WLAN endpoint,
  unlike a DPSK, so the editor shows it masked behind a tap.
- **API version negotiation needs a ceiling, not a list.** This controller
  offers up to `v13_1`, a point release no hand-maintained list happened to
  name; an exact-match list quietly dropped back to `v13_0`.

## Known limits

- Verified against one 7.1.1 cluster. Older controllers are handled by version
  negotiation and by treating every response field as optional, so a missing
  one degrades a row rather than crashing a screen — but they have not been
  exercised.
- **Nothing that writes has been run against a live controller.** Reboot,
  client disconnect and deauthentication, WLAN edits, DPSK generation and
  revocation are built from the endpoint definitions and from a working
  production integration against this same cluster, but running them would
  have changed a production estate. The request shapes are right; the
  behaviour on success is the part still to confirm.
- **Switch management has no working endpoint yet.** Every public path tried
  returns 404 on 7.1.1 — `/query/switch`, `/query/switches`,
  `/query/switchport`, `/switches`, `/switchgroups`, `/switchm/*` — on a
  cluster that manages 43 switches. So the switch API is either off the
  `/wsg/api/public` tree on this release or needs a scope this admin account
  lacks. `switchesApi.probe()` finds out at runtime rather than asserting a
  path that is known not to work.
- WLAN creation is wired in the API layer (`wlans.create`, one endpoint per
  authentication style) but has no screen: the form that does it justice is
  larger than the editor, and a half-built WLAN is worse than none.
- Guest passes, block lists, packet capture and SpeedFlex are implemented in
  the API layer without screens.
- No push notifications for alarms. That needs somewhere to run, which this
  app deliberately does not have.
- Web builds run, but `expo-secure-store` has no browser equivalent; the
  fallback warns loudly and should not be used with a production controller.
