# Putting the sensor on a Raspberry Pi

The sensor is one static binary, so this comes down to three things: give it a
radio it can have to itself, give it the two capabilities it needs, and stop it
wearing out the SD card.

```
curl -fsSL https://raw.githubusercontent.com/andreacoppini/crossbreeder/master/pi/install.sh | sudo sh
```

That installs the binary, makes a user for it, writes the systemd units, takes
the test radio away from NetworkManager, and starts it. Run it again to
upgrade; it leaves the configuration alone.

## What to build it on

| Part | What to use | Why |
|---|---|---|
| Board | **Raspberry Pi 4 (2 GB)** or **Pi 5** | Gigabit Ethernet and USB 3, which is what a throughput test past a few hundred Mbps needs. A Pi 5 has the headroom for a gigabit line. |
| Test radio | **MediaTek MT7921AU** USB (Wi-Fi 6, 2.4/5 GHz, WPA3) | `mt7921u` has been in the mainline kernel since 5.16, so there is no out-of-tree driver to rebuild every time the kernel moves. Full nl80211 support, which is what wpa_supplicant needs for the timing this tool reports. |
| Test radio, cheaper | **MediaTek MT7612U** USB (Wi-Fi 5) | `mt76x2u`, also mainline. Fine where the site is 802.11ac. |
| Avoid | Realtek RTL88xxAU adapters | They need out-of-tree drivers that break on kernel updates, and their nl80211 support is partial. A sensor that stops working after `apt upgrade` is worse than no sensor. |
| 6 GHz | Pi 5 + an M.2 HAT with an **MT7925** card | There is still no USB 6 GHz adapter worth relying on. Without one, the sensor tests 2.4 and 5 GHz and says so. |
| Power | **PoE+ HAT** | One cable to a cupboard, and the switch port tells you the sensor's power state as well. |
| Storage | A2-rated SD card, or a USB SSD | See *The SD card* below. |
| Out-of-band | A USB LTE modem (Quectel EC25 and similar, through ModemManager) | So the sensor can still report when the site's own uplink is the thing that is broken. This is optional and off the sensor's own path — it is just another route to the collector. |

**One radio or two.** With two, the built-in `wlan0` keeps the Pi on a
management network and the USB adapter (`wlan1`) does nothing but test: it
associates, tests, disconnects, and scans between passes. That is the
arrangement to build. With one radio the sensor takes it, which means the Pi is
only reachable over Ethernet while a wireless pass is running — workable for a
wired-plus-wireless spot check, awkward to manage.

A dedicated scan radio is worth a third adapter on a site where the air matters:
set `monitor_interface` and the sensor scans on that one, so a scan never costs
the test radio air time in the middle of a measurement.

## What it needs to be allowed to do

The unit grants exactly two capabilities and runs as an unprivileged user for
everything else:

- **`CAP_NET_RAW`** — the ICMP socket for pings, the packet socket that a
  capture and LLDP/CDP discovery read from, and `SO_BINDTODEVICE` on the DHCP
  socket so a DHCP test goes out of the interface it is testing.
- **`CAP_NET_BIND_SERVICE`** — UDP port 68, where a DHCP client has to listen.

It also joins the `netdev` group, which is what owns wpa_supplicant's control
socket. Without that it cannot drive the radio; with it, it can do nothing else
privileged.

There is one subtlety worth knowing if you write your own unit:
wpa_supplicant replies to a control request at the address it came *from*, so
the sensor's own socket has to be somewhere both processes can see. The unit
sets `TMPDIR=/run/crossbreeder-sensor` for exactly this reason — a private
`/tmp` would leave the sensor talking to a socket wpa_supplicant cannot answer.

## The SD card

A sensor runs for years without anyone logging in, and the usual way that ends
is a worn-out card. Three things keep it off the flash:

- The history is bounded by both age and size (`storage.keep` and
  `storage.max_mib`), and pruned every six hours. The defaults — a fortnight
  and 512 MiB — hold a five-minute cadence comfortably.
- One append per pass. Nothing is rewritten, and no database compacts itself
  behind your back.
- Set `journalctl` to volatile storage, or install `log2ram`, so the log does
  not outwrite the sensor.

```
sudo mkdir -p /etc/systemd/journald.conf.d
printf '[Journal]\nStorage=volatile\nRuntimeMaxUse=32M\n' | sudo tee /etc/systemd/journald.conf.d/volatile.conf
sudo systemctl restart systemd-journald
```

Where the history matters more than the card — a sensor kept as a reference for
a site — put `storage.dir` on a USB SSD.

## Getting at it

The dashboard binds to loopback. A sensor that offers a web interface to a
guest network is a sensor somebody will find, so reach it over SSH:

```
ssh -L 52414:127.0.0.1:52414 pi@sensor-lobby
```

then open <http://127.0.0.1:52414>. For a fleet, point the sensors at a
collector instead — they connect out to it, so nothing has to be forwarded to
the sensor at all.

## Adding the wireless networks

Edit `/etc/crossbreeder-sensor/config.json` — there is an annotated example
beside it — and add what the site's clients use:

```json
{
  "name": "Corporate",
  "kind": "wifi",
  "profile": {
    "SSID": "Campus-Secure",
    "EAP": "PEAP",
    "Identity": "sensor@example.com",
    "Password": "...",
    "Phase2": "auth=MSCHAPV2",
    "CACert": "/etc/crossbreeder-sensor/radius-ca.pem",
    "SubjectMatch": "CN=radius.example.com"
  },
  "tests": {"dhcp": true, "gateway": true, "dns": [{"query": "intranet.example.com"}]}
}
```

Give the sensor its own RADIUS account rather than borrowing somebody's — the
point is to know when authentication breaks, and a shared account that gets
locked out tells you nothing.

`SubjectMatch` is worth setting. Without it the sensor will authenticate
against any server that answers, which is not the test anyone wanted.

Then:

```
sudo systemctl restart crossbreeder-sensor
sudo -u sensor crossbreeder-sensor -config /etc/crossbreeder-sensor/config.json -once
```

The second command runs one pass and prints it, which is the quickest way to
find a typo in a passphrase.

## When it will not associate

```
sudo -u sensor crossbreeder-sensor -scan            # what the radio can hear
sudo wpa_cli -i wlan1 -p /run/wpa_supplicant status # what the supplicant thinks
journalctl -u wpa_supplicant-sensor@wlan1 -f        # the supplicant's own view
```

The three things that account for most of it:

- **NetworkManager took the radio back.** Check
  `/etc/NetworkManager/conf.d/99-crossbreeder-sensor.conf` names the right
  interface. Two supplicants on one radio produce an association that works and
  then drops about thirty seconds later.
- **The user is not in `netdev`**, so the control socket is unreachable. The
  error names the socket path.
- **The adapter needs firmware.** `dmesg | grep -i firmware` after plugging it
  in; MediaTek adapters want `firmware-misc-nonfree` on Debian-derived images.

## Building an image for several sensors

For more than two or three, build one card and clone it:

1. Install on a Pi as above and configure it fully.
2. `sudo rm -f /etc/crossbreeder-sensor/config.json` — leave the example.
3. Clear the machine identity so the clones do not all claim the same one:
   `sudo truncate -s0 /etc/machine-id && sudo rm -f /etc/ssh/ssh_host_*`.
4. Image the card.
5. On each sensor, write `/etc/crossbreeder-sensor/config.json` with its own
   `name`, `site` and `group` — or write only the `upstream` block and let the
   collector push the rest down on first contact.

The last of those is the one to use for a fleet: a sensor with nothing but a
collector URL and a token gets its whole configuration on its first report, and
changing what a site tests is then one push rather than twenty SSH sessions.
