# crossbreeder-engine (proof of concept)

A Go implementation of Crossbreeder's SSH core, written to answer one question:
what does it take to stop doing one AP at a time? See
[`../docs/ARCHITECTURE-REVIEW.md`](../docs/ARCHITECTURE-REVIEW.md) for the
analysis this belongs to.

It does the same job as `ChangeFW.Run` in the Xojo project — walk a CSV of
standalone Ruckus APs, collect inventory, optionally push firmware, reset,
run a command or reboot — with the APs worked in parallel, and with the
firmware server built in so a push needs no other software.

It runs in two phases:

1. **Ping sweep.** Every address gets an ICMP echo request, hundreds at a time,
   with a 1.5s timeout and one retry. On a site list where most addresses are
   dead — the normal case — this is where the run is won: a dead address costs
   one unanswered packet, not an SSH handshake against a timeout.
2. **SSH**, over the addresses that answered, `-c` at a time.

Whatever stays silent is reported, not just counted. Consecutive addresses fold
into ranges, so a dead block reads as one line instead of sixty:

```
499 of 558 answered in 3.054s; 59 skipped

59 did not answer ping:
  172.20.43.87
  172.20.44.151
  172.20.45.50    - 172.20.45.57    (8)
  172.20.45.130   - 172.20.45.171   (42)
  172.20.46.55
```

`-dead <file>` writes those addresses one per line; the file is accepted as
`-csv`, so once the power or cabling is sorted you can re-run just them. They
also appear in the results file with `Result = No ping reply`.

Measured on a 500-address list holding 40 live APs:

```
ping sweep + SSH   6.6s      (460 skipped in the sweep, 40 contacted)
no gate (-probe none)  1m36s  (SSH attempted against all 500)
```

## The console

Run the binary with no arguments — or double-click it — and it opens a browser
console instead of printing a usage error at a window that vanishes:

```
crossbreeder-engine            # or crossbreeder-engine -ui
Crossbreeder console: http://127.0.0.1:52413
```

It is the same engine: the console and the command line run one job through one
event stream, so they cannot drift apart. The page is bound to **localhost
only** — the firmware server has to be reachable by the APs, but nothing should
be able to drive a fleet of access points from off the machine.

The layout is an operator console: targets, credentials, actions and tuning down
the left; a live results grid that shows every address from the first second,
including the ones that never answer; and a drawer with the log, the addresses
that did not answer, the firmware server's transfer log, and the full SSH
transcript of whichever AP you click.

Anything that changes an AP — firmware, factory reset, reboot, a CLI command —
is listed back to you for confirmation before the run starts. Everything except
the passwords is remembered between sessions.

Rows select the way a file manager's do: click, shift-click for a range,
ctrl-click (cmd on a Mac) to add or drop one, ctrl-A for everything the current
filter shows, Escape to clear. **Remove** or the Delete key drops the selected
rows, and takes those addresses out of the target list too, so a re-run does not
bring them back.

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
IP address, so the CSVs the GUI already accepts work unchanged — including ones
with stray quotes mid-field, which strict CSV parsing rejects outright. `%M` in
`-fw-file` is replaced with the model detected on each AP. `-out` writes CSV or
JSON depending on the extension. `-v` dumps the full session transcript.

Run `crossbreeder-engine -h` for the rest.

### Passwords

Do not pass a password as a command-line argument if you can avoid it. `cmd.exe`
treats `^` as an escape character and eats it, PowerShell and POSIX shells each
claim a different set, and the argument is visible in the process list either
way. A password that works when typed into an SSH client and fails here is
almost always the shell, not the AP.

```
crossbreeder-engine -csv aps.csv -user admin -ask-pass          # prompt, no echo
set CBPASS=...                                                   # cmd
crossbreeder-engine -csv aps.csv -user admin -pass-env CBPASS
```

`-ask-pass` also reads a piped line, so it stays scriptable. Giving `-user`
without `-pass` on a terminal prompts rather than trying an empty password.

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
- `-dead <file>` — the addresses that stayed silent, one per line, re-feedable
  as `-csv`.
- `-default` — also try the factory-default `super`/`sp-admin` login, as the
  GUI's "also try default" checkbox does.
- `-ask-pass`, `-pass-env` — see **Passwords** above.
- `-legacy` — on by default. Re-enables the SHA-1 KEX and CBC ciphers that
  pre-2015 ZoneFlex firmware negotiates and that modern SSH stacks refuse.
  Turn it off on a fleet that is entirely modern.
- `-timeout` — per-step timeout, default 8s. The whole-AP deadline is 12× this.

### Hosting the images

In the console the same thing is a choice between **Internal** and **External**
at the top of the firmware section: internal shares a folder from this machine
and shows what the server is doing — started or stopped, the address it is
listening on, what each AP is downloading right now with a progress bar, and
what has already completed. External is the classic arrangement where the APs
fetch from a server you already run.

`-serve` turns the tool into the firmware server as well as the client, so a
push needs nothing installed but this binary. With the images sitting next to
the exe, that is the whole command:

```
crossbreeder-engine -csv aps.csv -user admin -ask-pass -fw -serve
```

`-serve` on its own shares the directory the tool was started in. To share a
different one, name it — either form works:

```
crossbreeder-engine ... -fw -serve C:\firmware
crossbreeder-engine ... -fw -serve=C:\firmware
```

It binds an HTTP server on that directory, works out which of this machine's
addresses the APs should fetch from, and fills in `-fw-proto http`, `-fw-host`
and `-fw-port` itself.

The address is chosen by looking at the AP list, not at the routing table —
asking the OS how it would reach one AP gets you the VPN's address whenever a
VPN is up, and the APs cannot reach that. In order of preference:

1. the address whose own subnet holds the **most** APs in the CSV,
2. any address whose subnet holds **an** AP,
3. an RFC 1918 address (`10/8`, `172.16/12`, `192.168/16` — note this excludes
   Tailscale's `100.64/10`, which is RFC 6598 shared space),
4. anything else that is up.

Ties within a tier fall back to what the routing table would have picked, then
to a stable order, so repeated runs choose the same address. The choice is
printed with its reason, and `-v` lists every address that was considered:

```
Serving C:\firmware on http://192.168.77.105:8080
  address chosen: 192.168.77.105/24 on Ethernet covers 558 of 558 AP(s)
```

`-serve-ip` still overrides it outright. If `-fw-file` is not given and the directory holds exactly one `.rcks`
control file (or, failing that, one `.bl7`), that is what gets pushed.

Because the APs download in the background, well after their SSH sessions have
closed, the tool keeps serving and waits for them:

```
Serving C:\firmware on http://192.168.77.105:8080 — waiting for 3 AP(s) to download (Ctrl-C to stop)
  192.168.77.115  200 118.2.0.0.875.rcks       61 B in 2ms
  192.168.77.115  200 118.2.0.0.875.bl7     46.0 MiB in 41.2s
All 3 AP(s) took the image.
```

It stops as soon as every AP has taken a full copy, at `-serve-wait` (default
30m), or on Ctrl-C — whichever comes first, and it says which APs were still
outstanding. An AP that reaches the server over a different interface arrives
with a source address that is not the one it was driven on; those downloads are
counted too and called out rather than being reported as missing.

Other flags: `-serve-port` (default 8080, `0` picks a free one) and `-serve-ip`
to override the advertised address. The server is GET/HEAD only and confined to
the directory it is given, but it is still a listening port serving files to the
network. Since the default is the directory you started in, run it from one
holding firmware rather than from a general-purpose folder — and name a
directory explicitly if you are unsure what is in the current one. Windows will
prompt for a firewall exception the first time.

### Firmware pushes

`fw update` only *starts* the job. The AP answers "In progress" and then fetches
the image in the background, long after this tool has disconnected, so a "Done"
row means the AP accepted the instruction — not that the image landed. The AP's
own answer is kept in the `Firmware Push` column; `-fw-wait 30s` holds the
session open afterwards and captures whatever progress it prints.

Watch the image size against the protocol. Classic TFTP (RFC 1350) numbers its
512-byte blocks in 16 bits, so it tops out at 65,535 x 512 = **32 MiB**. Current
Ruckus images are larger than that — a 46 MiB `.bl7` needs 94,295 blocks — and
TFTP's lockstep ack-per-block makes it slow and fragile besides. Use
`-fw-proto http` for anything over 32 MiB.

`fw set` values that are empty are skipped: the AP rejects the whole command and
prints its usage page rather than accepting a blank setting, so `-fw-user` and
`-fw-pass` are only sent when you supply them.

### Re-scanning until you stop it

A firmware push, a factory reset and a reboot all end with the AP going away and
coming back some minutes later, so the run finishing tells you nothing about
whether it worked.

In the console, **Keep re-scanning until I press Stop** is on by default. The
first pass does whatever was ticked; every pass after that only looks:

- it pings every address on the list — one that was up and stops answering reads
  as **Rebooting** rather than failed, and one that was down and starts
  answering joins the table;
- it re-reads the version on whatever answers, so the firmware column tracks
  what is actually running;
- a version different from the one it started with is reported as
  **Upgraded from &lt;old&gt;**.

The interval is the rest *between* passes, counted from the end of one to the
start of the next, so a pass over a large estate can never be overtaken by the
next one however short the interval is set. If a pass runs longer than the
interval, the log says so.

It never stops on its own; Stop ends it. When a firmware push is in flight the
image server stays up alongside it and downloads are reported in the Transfers
pane and the server panel, so an AP that never finishes a download cannot hold
the run up. Actions are never re-issued, which
matters on an AP halfway through a reboot.

On the command line the same thing is `-watch`, which runs until interrupted;
`-watch-for 20m` caps it so a scripted run still terminates. It is off by
default there, because a command that never returns is no use in a script.

### Factory resets

`set factory` stages the reset; the AP does not act on it until it reboots. So
`-factory` implies `-reboot` — a reset on its own would leave the AP exactly as
it was.

## Differences from the Xojo version

- The ping sweep is in-process ICMP, not a shell-out to `ping.exe`. No process
  per address, no parsing of localised console text, and hundreds in flight
  instead of one at a time. On Windows it goes through `iphlpapi`'s
  `IcmpSendEcho` — the same unprivileged path `ping.exe` uses, so no
  administrator rights are needed; on macOS and Linux it uses an ICMP datagram
  socket, falling back to a raw socket when run as root.
- `set factory` and `reboot` run **last**. The original issued `set factory`
  before the firmware commands, which on a real AP discards them. `-factory`
  also implies a reboot, without which the reset never happens.
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
- The Windows build has been run against a live 558-AP estate: the sweep cleared
  59 silent addresses in 3.0s and the whole run finished in 6.5s at `-c 100`.
  `-reboot`, `-factory` and the `fw` sequence have been exercised on a real
  R550 (7.2.0). What has *not* been confirmed end to end is an image actually
  landing and booting - see the TFTP size note above.
- The Unleashed dialect and pre-2015 ZoneFlex firmware (`-legacy`) have still
  only met the fake AP.
