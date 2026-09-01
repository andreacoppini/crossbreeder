# What Aruba UXI does, and what this does

This is the map that decides whether Crossbreeder Sensor is worth pointing at a
site: every part of what a UXI deployment gives you, and what the equivalent
here is — including the parts where the answer is "it does not, and here is
what you would do instead".

UXI is two things: a sensor that behaves like a client and keeps testing, and a
cloud service that collects, judges and alerts. The sensor half is
reproducible on a Raspberry Pi almost completely. The cloud half is
reproducible in substance — scores, issues, root cause, history, alerting,
fleet management — but not as a hosted multi-tenant service with SSO, and this
does not pretend otherwise.

## The tests

| What UXI tests | Here | Notes |
|---|---|---|
| Wireless association, per SSID | **Yes** | Through wpa_supplicant, so it associates the way a client does. Scan, 802.11 authentication, EAP and the four-way handshake are timed separately. |
| Open, WPA2-PSK, WPA3-SAE, Enhanced Open | **Yes** | `security: open \| psk \| sae \| owe \| wep`, with protected management frames set as each requires. |
| 802.1X: PEAP, EAP-TTLS, EAP-TLS, EAP-PWD | **Yes** | Identity, anonymous identity, phase 2, CA certificate, client certificate and key, and `SubjectMatch` to pin the RADIUS server. |
| Hidden SSIDs, band and BSSID pinning | **Yes** | `hidden`, `freq`, `bssid` on the profile — which is how you test the 5 GHz radio of an SSID separately from its 2.4 GHz one, or one AP in particular. |
| Authentication (RADIUS) timing and failure | **Yes** | The EAP exchange is timed on its own, and a rejection is reported as an authentication failure rather than "wireless not working". |
| DHCP | **Yes** | A full DISCOVER/OFFER/REQUEST/ACK, timed at each step, with the lease released again so a five-minute cadence does not eat the scope. More than one server answering is reported. |
| DNS, internal and external | **Yes** | Its own resolver client over UDP, TCP, DoT and DoH. By default it asks the resolvers DHCP handed out — the ones the site's clients are using — rather than a public one. An expected answer can be pinned, which catches a resolver that answers quickly with the wrong address. |
| Gateway reachability | **Yes** | Against the gateway for that interface, read from the routing table rather than from whichever route the OS prefers, so a sensor with two networks up tests the right one. |
| Internet reachability | **Yes** | ICMP, in process, hundreds at a time if you list that many. |
| Web and SaaS application tests | **Yes** | Per-phase timing — DNS, connect, TLS, first byte, total — with a catalogue of seventeen applications (Microsoft 365, Google, Zoom, Webex, Teams, Slack, Salesforce, ServiceNow, Workday, Dropbox, Box, AWS, Azure, GitHub, Citrix, Cloudflare, and a plain internet check) plus any URL you name. |
| TLS certificate expiry | **Yes** | On every application tested, and on any `host:port` you name — including a RADIUS or portal certificate the sensor never browses. |
| Throughput | **Yes** | Three ways: an HTTP download or upload, a built-in peer (another sensor or the collector, so nothing else has to be installed), or iperf3 where the site already runs one. It runs on its own slower schedule, because it saturates the link it measures. |
| VoIP quality, MOS | **Yes** | A paced UDP stream against a reflector, with loss, jitter and round trip, scored through the ITU-T G.107 E-model for G.711, G.722 or G.729. |
| QoS / DSCP validation | **Yes** | The stream is marked (EF by default for voice) and the reflector reports what marking actually arrived, which is how you find out the network stripped it. |
| Video quality | **Partly** | The same UDP stream at a video-shaped rate and AF41 marking measures the loss, jitter and delay a video call would meet. There is no separate video MOS. |
| Traceroute / path | **Yes** | Its own ICMP traceroute where it has the privilege, falling back to the system `traceroute`. |
| Captive portal detection | **Yes** | A 204 probe, reported as interception with the portal's URL rather than as an outage. |
| Roaming | **Yes** | Forces a handover to the next-strongest radio of the same SSID and times it. A handover past a second is audible in a call, and that is where the line is drawn. |
| RF: signal, noise, SNR, channel, width | **Yes** | Read from the driver on every pass. |
| RF: neighbouring APs, co-channel and overlap | **Yes** | A scan per pass, counted relative to the channel the sensor is on. |
| RF: channel utilisation | **Yes** | Air-time busy against active, from the driver's own survey. |
| Wired tests | **Yes** | The same suite over the Ethernet port: DHCP, gateway, DNS, internet, applications. |
| Switch port identification (LLDP/CDP) | **Yes** | Both protocols parsed: switch name, port, VLAN, management address, capabilities. |
| External service tests (arbitrary TCP ports) | **Yes** | A TCP connect test with timing; name it as a web target for an HTTP service or use the port check for anything else. |
| Test cadence, per network | **Yes** | The rest between passes is configurable; five minutes by default, as UXI's is. |

## The platform

| What the UXI dashboard gives you | Here | Notes |
|---|---|---|
| Per-service health scores | **Yes** | Per service and one overall, weighted so a DHCP failure outranks a slow Dropbox. The arithmetic is deliberately simple enough to argue with. |
| Issue detection with a root cause | **Yes** | One issue per service rather than one per measurement, and the failure furthest down the dependency order is marked as the cause; the rest say what they are a consequence of. |
| Issue history: opened, cleared, duration | **Yes** | Issues are tracked across passes, so the dashboard shows "failing for forty minutes" rather than a fresh alert every five. |
| Time series and charts | **Yes** | Every measurement is kept; the dashboard charts the score, and any test's history is one API call. |
| Alerting | **Yes** | Webhook, Slack, syslog (RFC 5424) and email, with a minimum severity and a repeat interval. |
| Ticketing and chat integrations | **Partly** | Through the webhook, which is how most sites wire ServiceNow, Teams or PagerDuty anyway. There is no first-party integration. |
| Fleet view across sites | **Yes** | The collector: one page, worst first, with each sensor's networks and open issues. |
| Groups and sites | **Yes** | `site` and `group` on each sensor, carried through to the fleet view and the metrics. |
| Sensor health and "sensor offline" | **Yes** | A sensor that stops reporting shows as offline — which is usually the site's power or uplink, and is itself the finding. |
| Configuration pushed from the dashboard | **Yes** | The collector queues a configuration; the sensor takes it on its next report, if it has been set to accept one. |
| Remote packet capture | **Yes** | Bounded by packets, bytes and time, filtered by host, port or protocol, streamed straight into the browser as a pcap while it is still being taken. |
| Remote troubleshooting: scan, traceroute, test now | **Yes** | From the dashboard, the API, or as a command from the collector. |
| Sensor firmware updates | **Yes** | Pulled by the sensor, checked against the release's published SHA-256, and applied on request or on a collector's command. Never automatic without being asked. |
| API access | **Yes** | Everything the dashboard shows, as JSON, plus CSV export and a Prometheus endpoint. |
| Aruba Central / ClearPass integration | **No** | Nothing here reads a controller's own view. The sensor reports what a client experiences, and that is all it claims to know. |
| Multi-tenant service, SSO, role-based access | **No** | The collector is single-tenant and takes a bearer token. Put it behind whatever your organisation already authenticates with. |
| Anomaly detection and baselining | **No** | Thresholds, in the configuration, where you can see them and move them. A score whose reasoning nobody can follow gets ignored the first time it disagrees with somebody's experience. |
| Scheduled PDF reports | **No** | CSV, JSON and Prometheus. Everything else is somebody else's report generator. |
| Zero-touch provisioning over Bluetooth | **No** | A sensor is provisioned by writing one file, or by giving it a collector URL and a token and letting the rest arrive. |

## Hardware

| UXI | Here |
|---|---|
| Purpose-built sensor with two radios, Ethernet, BLE and an LTE option | A Raspberry Pi 4 or 5 with a mainline-supported USB radio, PoE if you want one cable, and any USB modem the OS supports. See [`../pi/README.md`](../pi/README.md). |
| Tri-band, 6 GHz | Only with a Pi 5 and an M.2 6 GHz card. There is still no USB adapter for it worth relying on, and where the radio cannot reach a band the sensor says so rather than guessing. |
| Cellular out-of-band management | Any modem the OS brings up is another route to the collector. Nothing in the sensor knows or cares which route it took. |
| Hardware support contract | You have a spare Pi in a drawer. |

## What has actually been exercised

Being straight about this matters more than the table above.

- Everything in `netprobe`, `wifi` and `l2` is covered by tests that stand up
  the real thing: a DNS server, a DHCP scope, a throughput peer and a voice
  reflector on loopback, and a fake wpa_supplicant on a real control socket
  speaking the real control protocol. The full pass — layer ordering, the
  gating when a layer fails, scoring and issue detection — runs against those.
- The suite has been run for real against an ordinary Linux host: DNS, the
  gateway, web and application tests, the dashboard, the collector and the
  fleet round trip all work end to end.
- **What has not met real hardware yet is the radio.** The association path
  has been exercised against a fake supplicant, not against a real adapter and
  a real AP. The first contact with a real 802.1X network is where this will
  need adjusting, and it is the first thing to try on a bench before trusting
  it at a site.
- The DHCP client, the packet socket and LLDP/CDP discovery need Linux and the
  two capabilities the unit grants; they have been tested at the protocol level
  but not yet against a live scope and a live switch.
