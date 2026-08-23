# crossbreeder-engine (proof of concept)

A Go implementation of Crossbreeder's SSH core, written to answer one question:
what does it take to stop doing one AP at a time? See
[`../docs/ARCHITECTURE-REVIEW.md`](../docs/ARCHITECTURE-REVIEW.md) for the
analysis this belongs to.

It does the same job as `ChangeFW.Run` in the Xojo project — walk a CSV of
standalone Ruckus APs, collect inventory, optionally push firmware, reset,
run a command or reboot — with the APs worked in parallel.

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

- `-c` — how many APs at once. Default 25. For inventory and CLI work 50–100 is
  fine; for firmware pushes keep it to what your image server and uplink can
  carry, since every AP downloads at once.
- `-default` — also try the factory-default `super`/`sp-admin` login, as the
  GUI's "also try default" checkbox does.
- `-legacy` — on by default. Re-enables the SHA-1 KEX and CBC ciphers that
  pre-2015 ZoneFlex firmware negotiates and that modern SSH stacks refuse.
  Turn it off on a fleet that is entirely modern.
- `-timeout` — per-step timeout, default 8s. The whole-AP deadline is 12× this.

## Differences from the Xojo version

- Reachability is a TCP connect to the SSH port rather than a shell-out to
  `ping`. It costs no process, does not parse localised console text, and tests
  the thing we actually need.
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

## Status

Proof of concept. The dialogue has been exercised against the fake AP, not
against real hardware — SSH algorithm negotiation with old ZoneFlex firmware is
the part most likely to need adjustment on first contact.
