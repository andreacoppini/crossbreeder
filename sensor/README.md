# Crossbreeder Sensor

A network experience sensor on a Raspberry Pi: a small box that sits where the
users are, joins the networks they join, and keeps testing what they depend on
— association, DHCP, DNS, the gateway, the internet, the applications, and
whether a call would sound like anything.

It is the other end of the problem Crossbreeder Plus works on. That one drives
access points; this one reports what the network is actually like from the
floor, which is the argument you cannot win without evidence.

One static binary. No agent, no runtime, no controller, no account.

```
curl -fsSL https://raw.githubusercontent.com/andreacoppini/crossbreeder/master/pi/install.sh | sudo sh
```

See [`../pi/README.md`](../pi/README.md) for what to build it on, and
[`../docs/SENSOR-FEATURE-MAP.md`](../docs/SENSOR-FEATURE-MAP.md) for what it
does and does not do measured against Aruba UXI, which is the product this
sets out to replace.

## What a pass looks like

```
$ crossbreeder-sensor -once

Corporate (wifi) — health 78/100, fair, 6.204s
  b8:27:eb:aa:bb:02 on channel 36 (5 GHz), -58 dBm, SNR 37 dB
  10.20.30.55 from 10.20.30.1, gateway 10.20.30.1, resolvers 10.20.30.2, 10.20.30.3

  OK     association                    904ms  b8:27:eb:aa:bb:02 on channel 36, WPA2-Enterprise, EAP 250ms
  OK     802.1X authentication          250ms
  OK     signal                       -58dBm  noise -95 dBm, SNR 37 dB
  WARN   air time in use                 61%
  OK     DHCP                           412ms  10.20.30.55 from 10.20.30.1, offer 180ms, ack 232ms
  OK     gateway                         1.4ms
  WARN   DNS intranet.example.com      248ms   10.20.0.40
  OK     reach 1.1.1.1                   12ms
  OK     Microsoft 365 sign-in          268ms  HTTP 200, DNS 21ms, connect 12ms, TLS 48ms, first byte 186ms
  FAIL   Zoom                          5.01s   context deadline exceeded
  OK     call quality                    4.31  0.4% loss, 28ms round trip, 3ms jitter

  CRITICAL (root cause): Corporate — an application is unreachable: context deadline exceeded
  WARNING: Corporate — the wireless link is weak or busy: air time in use 61%
```

That is the whole idea: a number for every layer, in the order the layers
depend on each other, and a sentence at the end saying which one to go and
look at.

## How it decides what is wrong

Everything a test produces is one measurement with a judgement on it, and the
judgements are thresholds you can see and move — not a model. From those:

- **Scores**, per service and one overall, weighted so a DHCP failure outranks
  a slow Dropbox.
- **Issues**, one per service rather than one per measurement, with the failure
  furthest down the stack marked as the root cause and the ones above it told
  to say what they are a consequence of. Nine red rows that all come from one
  broken DHCP scope is one ticket, not nine.
- **Gating**: when a layer fails, the layers above it are recorded as *skipped*
  rather than run. A DNS test that never happened is not a DNS fault, and
  reporting it as one sends somebody to the wrong team.

Issues are tracked across passes, so a network that flaps produces one issue
that opens and closes rather than an alert every five minutes.

## Running it

```
crossbreeder-sensor                       # the loop, the dashboard, and the collector link
crossbreeder-sensor -once                 # one pass, printed; exit status 2 if anything failed
crossbreeder-sensor -once -json            # the same, for a script
crossbreeder-sensor -scan                 # what the radio can hear, and how busy the air is
crossbreeder-sensor -apps                 # the applications it knows how to test
crossbreeder-sensor -example > config.json # a fully worked configuration to edit
```

The dashboard binds to loopback (`127.0.0.1:52414`). It shows the latest pass
per network, the open issues, a history chart, and the three things somebody
standing next to the problem wants: a scan, a traceroute, and a packet capture
that downloads while it is still being taken.

Everything on it is drawn from the same JSON API anything else would read, so
the page and the integration cannot drift apart:

```
GET  /api/state         the sensor, its networks, where the loop is
GET  /api/latest        the most recent pass per network
GET  /api/results       history, ?network= &from=-24h &to=
GET  /api/issues        what is open now
GET  /api/series        one test's history, for a chart
GET  /api/events        server-sent events: one per pass, as it finishes
GET  /api/scan          scan now
GET  /api/traceroute    ?target=
GET  /api/capture       ?interface=&seconds=&host=&port= — a pcap, streamed
GET  /api/export        CSV, one row per measurement
GET  /metrics           Prometheus
GET  /api/config        the configuration, redacted
PUT  /api/config        replace it
POST /api/run           test now
```

## Configuring it

One JSON file. `-example` prints a worked one; the short version is a list of
networks, each with a profile and a list of tests:

```json
{
  "sensor": {"name": "lobby-1", "site": "Head office", "wireless_interface": "wlan1", "interval": "5m"},
  "networks": [{
    "name": "Corporate",
    "kind": "wifi",
    "profile": {"SSID": "Campus-Secure", "EAP": "PEAP", "Identity": "sensor@example.com",
                "Password": "...", "Phase2": "auth=MSCHAPV2",
                "CACert": "/etc/crossbreeder-sensor/radius-ca.pem",
                "SubjectMatch": "CN=radius.example.com"},
    "tests": {
      "dhcp": true, "gateway": true, "captive_portal": true, "roaming": true,
      "dns": [{"query": "intranet.example.com", "expect": "10.20.0.40"}],
      "internet": ["1.1.1.1"],
      "apps": ["Microsoft 365", "Zoom"],
      "voip": {"reflector": "collector.example.com:52416", "dscp": 46},
      "throughput": {"mode": "peer", "peer": "collector.example.com:52415",
                     "every": "6h", "expect_mbps": 100}
    }
  }]
}
```

Notes worth having before you write one:

- **`kind`** is `wifi` or `wired`. A wired network needs no profile; it is the
  same suite over the Ethernet port, plus LLDP/CDP so you know which switch
  port the sensor is on.
- **DNS with no `server`** asks whatever DHCP handed out — the resolvers the
  site's clients are actually using. A sensor that always asks 8.8.8.8 is
  monitoring Google's uptime.
- **`expect`** on a DNS test pins the answer, which catches a resolver that
  answers quickly with the wrong address — a captive portal, or a filtering
  resolver handing back its own.
- **`freq` and `bssid`** on a profile pin the association, which is how you
  test the 5 GHz radio of an SSID separately from its 2.4 GHz one, or one AP in
  particular.
- **Throughput** moves real traffic, so it has its own `every` and never runs
  on a normal pass.
- **Thresholds** are in the file. "Slow" at a hospital and "slow" at a
  warehouse are different numbers, and an operator who cannot move the line
  will filter the alerts instead.

Secrets — passphrases, 802.1X passwords, tokens — are never shown by the API,
never written to a log, and never sent to the collector. The dashboard is shown
the redacted form and a save from it puts the real values back.

## A fleet

The same binary is the collector:

```
crossbreeder-sensor -collector -tokens '*=a-long-shared-secret' -listen 127.0.0.1:52415
```

Sensors connect **out** to it and ask for work, so a sensor can sit on a
customer's network behind NAT with nothing forwarded to it. The collector keeps
each sensor's history, shows one page for the fleet — worst first — and hands
back configuration and commands on the next report: test now, take this
configuration, update yourself, restart.

A token can be tied to one sensor (`lobby-1=token`) or shared across a fleet
(`*=token`). A tied token may only report as that sensor, so one compromised
box cannot rewrite everyone else's history.

It also answers the fleet's voice and throughput tests, so a site can measure
the path to it without another server. Put a reverse proxy with a certificate
in front of it, and set `-admin-token` if the fleet views will be reachable
from anywhere but localhost.

## Alerts

Webhook, Slack, syslog and email, with a minimum severity and a repeat
interval so a flapping network does not alert every five minutes:

```json
"alerts": {
  "enabled": true, "min_severity": "warning", "repeat": "1h",
  "webhooks": ["https://example.com/hooks/network"],
  "slack_webhook": "https://hooks.slack.com/services/...",
  "syslog": "10.20.0.30:514"
}
```

The webhook payload is flat on purpose: sensor, network, service, severity,
state (opened or cleared), title, detail, evidence, and whether it is the root
cause.

## Building it

```
cd sensor && go build -o crossbreeder-sensor .
GOOS=linux GOARCH=arm64 go build -o crossbreeder-sensor-linux-arm64 .   # Pi 4 and 5, 64-bit
GOOS=linux GOARCH=arm   go build -o crossbreeder-sensor-linux-arm .     # 32-bit Pi OS
```

Only Go is needed. The dashboard is embedded in the binary.

## Tests

```
go test -race ./...
```

The tests stand up the real things rather than mocking them: a DNS server, a
DHCP scope, a throughput peer and a voice reflector on loopback, a fake
wpa_supplicant on a real control socket speaking the real control protocol, and
a real collector for the fleet round trip. The whole pass — the ordering, the
gating, the scoring, the issue detection — runs against those.

What that does not cover is the radio itself: see the last section of the
[feature map](../docs/SENSOR-FEATURE-MAP.md).
