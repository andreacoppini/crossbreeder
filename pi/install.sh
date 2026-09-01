#!/bin/sh
# Install the Crossbreeder Sensor on a Raspberry Pi (or any systemd Linux).
#
#   curl -fsSL https://raw.githubusercontent.com/andreacoppini/crossbreeder/master/pi/install.sh | sudo sh
#
# or, from a clone:  sudo pi/install.sh
#
# It installs the binary, makes a user for it, writes the systemd units, hands
# the test radio to a wpa_supplicant of its own, and starts it. It is
# idempotent: running it again upgrades the binary and leaves the
# configuration alone.
#
# Options are environment variables, so this works through a pipe:
#   RADIO=wlan1        the radio the sensor tests with (default: wlan1 if the
#                      Pi has two, otherwise wlan0)
#   WIRED=eth0         the wired port to test
#   VERSION=1.2.0      a specific release (default: the latest)
#   BINARY=./sensor    install this file instead of downloading
#   COLLECTOR=1        install as a fleet collector rather than a sensor
#   TOKENS=...         collector: -tokens value, e.g. '*=a-long-shared-secret'
set -eu

REPO=andreacoppini/crossbreeder
PREFIX=/usr/local/bin
ETC=/etc/crossbreeder-sensor
RADIO=${RADIO:-}
WIRED=${WIRED:-eth0}
VERSION=${VERSION:-}
BINARY=${BINARY:-}
COLLECTOR=${COLLECTOR:-}
TOKENS=${TOKENS:-}

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "run this with sudo"
command -v systemctl >/dev/null 2>&1 || die "this installer expects systemd"

# ---- which build ----
case "$(uname -m)" in
  aarch64|arm64)  ARCH=arm64 ;;
  armv7l|armv6l)  ARCH=arm ;;
  x86_64|amd64)   ARCH=amd64 ;;
  *) die "no build for $(uname -m)" ;;
esac
ASSET="crossbreeder-sensor-linux-$ARCH"

# ---- which radio ----
if [ -z "$RADIO" ]; then
  # A second radio is much the better arrangement: one to test with, one left
  # to the Pi's own management network. With only one, the sensor takes it.
  if [ -d /sys/class/net/wlan1 ]; then RADIO=wlan1; else RADIO=wlan0; fi
fi

# ---- the binary ----
install_binary() {
  if [ -n "$BINARY" ]; then
    say "installing $BINARY"
    install -m 0755 "$BINARY" "$PREFIX/crossbreeder-sensor"
    return
  fi
  command -v curl >/dev/null 2>&1 || die "curl is needed to download the release"
  if [ -n "$VERSION" ]; then
    URL="https://github.com/$REPO/releases/download/v$VERSION/$ASSET"
  else
    URL="https://github.com/$REPO/releases/latest/download/$ASSET"
  fi
  say "downloading $URL"
  tmp=$(mktemp)
  curl -fsSL "$URL" -o "$tmp" || die "could not download $ASSET"
  # The release publishes a checksum for every file; check it before trusting
  # this one.
  sums=$(mktemp)
  if curl -fsSL "$(dirname "$URL")/SHA256SUMS.txt" -o "$sums" 2>/dev/null; then
    want=$(awk -v a="$ASSET" '$2 == a || $2 == "*" a { print $1 }' "$sums" | head -n1)
    got=$(sha256sum "$tmp" | cut -d' ' -f1)
    [ -n "$want" ] || die "the release does not publish a checksum for $ASSET"
    [ "$want" = "$got" ] || die "the download does not match its published checksum"
    say "checksum verified"
  else
    say "warning: no checksum file was published with this release"
  fi
  rm -f "$sums"
  install -m 0755 "$tmp" "$PREFIX/crossbreeder-sensor"
  rm -f "$tmp"
}

install_binary
say "installed $("$PREFIX/crossbreeder-sensor" -version)"

# ---- the collector is a different, simpler install ----
if [ -n "$COLLECTOR" ]; then
  [ -n "$TOKENS" ] || die "a collector needs TOKENS, e.g. TOKENS='*=$(head -c 24 /dev/urandom | base64 | tr -d /+=)'"
  id -u collector >/dev/null 2>&1 || useradd --system --home /var/lib/crossbreeder-collector --shell /usr/sbin/nologin collector
  install -d -o collector -g collector -m 0750 /var/lib/crossbreeder-collector
  cat > /etc/systemd/system/crossbreeder-collector.service <<UNIT
[Unit]
Description=Crossbreeder Sensor fleet collector
After=network.target

[Service]
ExecStart=$PREFIX/crossbreeder-sensor -collector -tokens '$TOKENS' -listen 127.0.0.1:52415 -collector-dir /var/lib/crossbreeder-collector
Restart=always
RestartSec=5s
User=collector
Group=collector
StateDirectory=crossbreeder-collector
ProtectSystem=strict
ProtectHome=yes
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable --now crossbreeder-collector
  say ""
  say "The collector is listening on 127.0.0.1:52415."
  say "Put a reverse proxy with a certificate in front of it before pointing"
  say "any sensor at it, and set -admin-token if the fleet views will be reachable."
  exit 0
fi

# ---- user, directories, configuration ----
id -u sensor >/dev/null 2>&1 || useradd --system --home /var/lib/crossbreeder-sensor --shell /usr/sbin/nologin sensor
# netdev is what lets it reach wpa_supplicant's control socket.
getent group netdev >/dev/null 2>&1 && usermod -aG netdev sensor
install -d -o sensor -g sensor -m 0750 /var/lib/crossbreeder-sensor
install -d -m 0755 "$ETC"

here=$(cd "$(dirname "$0")" 2>/dev/null && pwd || echo /tmp)
fetch_support() { # file
  if [ -f "$here/$1" ]; then
    cp "$here/$1" "$2"
  else
    curl -fsSL "https://raw.githubusercontent.com/$REPO/master/pi/$1" -o "$2"
  fi
}

fetch_support wpa_supplicant.conf "$ETC/wpa_supplicant.conf"
fetch_support crossbreeder-sensor.service /etc/systemd/system/crossbreeder-sensor.service
fetch_support wpa_supplicant-sensor@.service /etc/systemd/system/wpa_supplicant-sensor@.service

if [ ! -f "$ETC/config.json" ]; then
  say "writing a starting configuration to $ETC/config.json"
  "$PREFIX/crossbreeder-sensor" -example > "$ETC/config.json.example"
  cat > "$ETC/config.json" <<CONF
{
  "sensor": {
    "name": "$(hostname)",
    "site": "",
    "wireless_interface": "$RADIO",
    "wired_interface": "$WIRED",
    "interval": "5m",
    "listen": "127.0.0.1:52414"
  },
  "networks": [
    {
      "name": "Wired",
      "kind": "wired",
      "tests": {
        "dhcp": true,
        "gateway": true,
        "captive_portal": true,
        "discovery": true,
        "dns": [{"query": "www.google.com"}],
        "internet": ["1.1.1.1", "8.8.8.8"],
        "apps": ["Microsoft 365", "Google"]
      }
    }
  ],
  "storage": {"dir": "/var/lib/crossbreeder-sensor", "keep": "336h", "max_mib": 512}
}
CONF
  # The file holds passphrases once somebody adds an SSID to it.
  chown root:sensor "$ETC/config.json"
  chmod 0640 "$ETC/config.json"
fi

# ---- hand the radio over ----
if [ -d /sys/class/net/"$RADIO" ] && [ -d /etc/NetworkManager ]; then
  install -d /etc/NetworkManager/conf.d
  cat > /etc/NetworkManager/conf.d/99-crossbreeder-sensor.conf <<NM
# Two supplicants on one radio fight; the symptom is an association that works
# and then drops. NetworkManager keeps its hands off the sensor's test radio.
[keyfile]
unmanaged-devices=interface-name:$RADIO
NM
  systemctl reload NetworkManager 2>/dev/null || systemctl restart NetworkManager 2>/dev/null || true
fi

systemctl daemon-reload
if [ -d /sys/class/net/"$RADIO" ]; then
  systemctl enable --now "wpa_supplicant-sensor@$RADIO"
  say "wpa_supplicant is driving $RADIO for the sensor"
else
  say "no radio called $RADIO — wireless tests are off until one is fitted"
fi
systemctl enable --now crossbreeder-sensor

say ""
say "Crossbreeder Sensor is running."
say "  configuration   $ETC/config.json  (an annotated example is beside it)"
say "  dashboard       http://127.0.0.1:52414  — over SSH:"
say "                  ssh -L 52414:127.0.0.1:52414 $(logname 2>/dev/null || echo pi)@$(hostname)"
say "  log             journalctl -u crossbreeder-sensor -f"
say "  one pass now    sudo -u sensor crossbreeder-sensor -config $ETC/config.json -once"
