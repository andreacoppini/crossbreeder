# crossbreeder-engine (proof of concept)

A Go implementation of Crossbreeder's SSH core, written to answer one question:
what does it take to stop doing one AP at a time? See
[`../docs/ARCHITECTURE-REVIEW.md`](../docs/ARCHITECTURE-REVIEW.md) for the
analysis this belongs to.

It does the same job as `ChangeFW.Run` in the Xojo project — walk a CSV of
standalone Ruckus APs, collect inventory, optionally push firmware, reset,
run a command or reboot — with the APs worked in parallel.

It runs in two phases:

1. **Ping sweep.** Every address gets an ICMP echo request, hundreds at a time,
   with a 1.5s timeout and one retry. On a site list where most addresses are
   dead — the normal case — this is where the run is won: a dead address costs
   one unanswered packet, not an SSH handshake against a timeout.
2. **SSH**, over the addresses that answered, `-c` at a time.

Measured on a 500-address list holding 40 live APs:

```
ping sweep + SSH   6.6s      (460 skipped in the sweep, 40 contacted)
no gate (-probe none)  1m36s  (SSH attempted against all 500)
```

## Build

```
go build -o crossbreeder-engine .            # this platform
GOOS=windows GOARCH=amd64 go build -o crossbreeder-engine.exe .
GOOS=darwin  GOARCH=arm64 go build -o crossbreeder-engine-macos .
```

No runtime, no installer, no licence key: one static binary per platform, built
from any one machine.

## Use

Inventory only — the default when no action is selected, same as the GUI:

```
crossbreeder-engine -csv aps.csv -user admin -pass Ruckus123 -c 50 -out results.csv
```

Firmware push, 20 at a time (sized to what the firmware server can serve):

```
crossbreeder-engine -csv aps.csv -user admin -pass Ruckus123 -default -c 20 \
  -fw -fw-proto http -fw-host 10.0.0.9 -fw-port 8080 -fw-file "%M_110.0.0.0.1347.bl7" \
  -reboot -out results.json
```

`-csv` reads the first column of each row and keeps anything that parses as an
IP address, so the CSVs the GUI already accepts work unchanged. `%M` in
`-fw-file` is replaced with the model detected on each AP. `-out` writes CSV or
JSON depending on the extension. `-v` dumps the full session transcript.

Run `crossbreeder-engine -h` for the rest.

### Flags worth knowing

- `-c` — how many APs at once in the SSH phase. Default 25. For inventory and
  CLI work 50–100 is fine; for firmware pushes keep it to what your image server
  and uplink can carry, since every AP downloads at once.
- `-probe` — how an address is judged alive before it costs an SSH session:
  - `icmp` (default) — ping, which is the cheapest way to drop a dead list.
  - `tcp` — connect to the SSH port instead. Use where ICMP is filtered.
  - `both` — alive if either answers. Slower on dead addresses (it waits for the
    ping, then the connect), but it will not skip an AP that is up with ICMP
    blocked by an ACL.
  - `none` — no gate; try SSH on everything.
- `-ping-timeout` (default 1.5s), `-ping-retries` (default 1, so two attempts
  before an address is written off), `-pc` (default 256 probes in flight).
- `-default` — also try the factory-default `super`/`sp-admin` login, as the
  GUI's "also try default" checkbox does.
- `-legacy` — on by default. Re-enables the SHA-1 KEX and CBC ciphers that
  pre-2015 ZoneFlex firmware negotiates and that modern SSH stacks refuse.
  Turn it off on a fleet that is entirely modern.
- `-timeout` — per-step timeout, default 8s. The whole-AP deadline is 12× this.

## Differences from the Xojo version

- The ping sweep is in-process ICMP, not a shell-out to `ping.exe`. No process
  per address, no parsing of localised console text, and hundreds in flight
  instead of one at a time. On Windows it goes through `iphlpapi`'s
  `IcmpSendEcho` — the same unprivileged path `ping.exe` uses, so no
  administrator rights are needed; on macOS and Linux it uses an ICMP datagram
  socket, falling back to a raw socket when run as root.
- `set factory` and `reboot` run **last**. The original issued `set factory`
  before the firmware commands, which on a real AP discards them.
- The ZoneFlex and Unleashed paths are one code path parameterised by prompt
  (`dialect` in `ap/session.go`), not two copies of the same 90 lines.
- Field extraction is plain string scanning, not a regex assembled from device
  output.

## Tests

```
go test -race ./...
```

`ap/fakeap_test.go` stands up a real in-process SSH server that speaks the
Ruckus CLI, so the login fallback, both AP dialects, `%M` templating and the
command sequence are all covered without hardware. `TestConcurrencyBeatsSerial`
measures the fan-out rather than asserting it.

`ap/ping_test.go` sweeps TEST-NET addresses (RFC 5737), which are genuinely
silent, so the timeout and retry behaviour is exercised for real:
`TestSweepDeadListIsBoundedByOneTimeout` clears 300 dead addresses in ~3s
against a serial cost of 7m30s. The ICMP tests skip themselves where the
platform will not hand out an ICMP socket.

## Status

Proof of concept. Two things have not met the real world yet:

- The CLI dialogue has been exercised against the fake AP, not real hardware.
  SSH algorithm negotiation with old ZoneFlex firmware (`-legacy`) is the part
  most likely to need adjustment on first contact.
- The Windows ICMP path is compiled and vetted but has not been *run* on
  Windows; it was written against the `iphlpapi` API and tested via the
  equivalent unix path. If the sweep reports everything dead on Windows, that is
  the first thing to suspect — `-probe tcp` sidesteps it, and the tool falls
  back to TCP automatically if it cannot open ICMP at all.
