# Crossbreeder — architecture review and migration options

**Question asked:** would rebuilding this in C (or similar native code) make it
faster and easier to maintain? And can we stop doing one AP at a time?

**Short answer:** the one-AP-at-a-time problem is real and is worth roughly a
20–30× speed-up, but native code has almost nothing to do with it. The runtime
is ~99.9% network waiting, so a C rewrite would optimise the 0.1% that is CPU.
The concurrency win is available in any language. What Xojo actually costs us is
*maintainability* and the *inability to fan out*, and the recommendation below
is Go rather than C for exactly those reasons.

---

## 1. What the tool does today

`Crossbreeder.xojo_window` renders a form; the operator loads a CSV of AP
addresses and presses **Go**. `btnMigrateGO.Action`
(`Crossbreeder.xojo_window:1759`) is the entire execution engine:

```
for i = 0 to listmigrateAP.listcount-1        ' :1759
    listmigrateAP.cell(i,4) = str(ping(apIP,1000))   ' :1762  shells out to /bin/ping
    ...
    listmigrateAP.cell(i,5) = str(ChangeFW.Run(apIP,i))  ' :1770  full SSH session
next
```

`ChangeFW.Run` opens an SSH connection, logs in twice (once to the SSH
transport, once to the AP's own CLI), detects whether it is talking to a
ZoneFlex (`rkscli: `) or an Unleashed AP (`> `), reads inventory, and then
optionally issues `set factory`, the nine-command `fw …` sequence, a custom
command and `reboot`.

Every one of those steps is a blocking round trip. Nothing overlaps.

## 2. Where the time actually goes

A single AP costs, in round numbers:

| Step | Wall time | CPU |
|---|---|---|
| `ping` shell-out (process spawn + 1 ICMP + parse) | 20–1000 ms | ~1 ms |
| TCP + SSH handshake (KEX, host key, auth) | 200–2000 ms | ~5 ms |
| AP CLI login (two prompts) | 200–1000 ms | ~0 |
| Inventory (2 commands) | 200–1000 ms | ~0 |
| Firmware sequence (9 commands) | 1–10 s | ~0 |
| `fw update` / `reboot` acknowledgement | 1–5 s | ~0 |

Per AP that is on the order of **5–20 seconds of wall clock and 5–10
milliseconds of CPU**. The process is asleep on a socket for well over 99% of
its life.

That is the whole argument about C. Rewriting in C might take those ~8 ms of
CPU down to ~3 ms. It cannot touch the 8000 ms the AP spends thinking. On a
500-AP site the current tool takes **over an hour**; a perfectly optimised
single-threaded C version would still take over an hour.

The lever is concurrency, and concurrency is free here precisely *because* the
work is I/O: 50 SSH sessions in flight cost 50 sockets and a few hundred KB of
memory, not 50 cores.

## 3. Why it can't just be threaded where it stands

Four things in the current design block a fan-out, and they are worth naming
because they are the real reason a rewrite is on the table:

1. **The worker isn't actually a thread.** `ChangeFW` declares
   `Inherits Thread`, and there is a `thChangeFW` Thread instance on the window
   (`:1045`) whose `Run` event is empty. But the code that does the work is a
   *method* called synchronously — `str(ChangeFW.Run(apIP,i))` assigns its
   return value, so the caller blocks until it finishes. The threading is
   decorative.

2. **The worker writes straight to the UI.** `ChangeFW.Run` calls
   `Crossbreeder.txtDebug.AppendText(...)` 43 times and assigns
   `Crossbreeder.listmigrateAP.cell(Row, n)` directly. Touching controls from a
   thread is not allowed, so the class cannot be made a real thread without
   first being rewritten to hand results back through a queue.

3. **Xojo threads (2018-era, which is what `RBProjectVersion=2018.04` pins us
   to) are cooperative, not pre-emptive** — they are green threads multiplexed
   onto one OS thread, and they only yield at points the runtime chooses. A
   blocking Chilkat socket call inside one does not yield; it stalls the whole
   application. So even a correctly-written Xojo thread would not have given us
   parallel SSH here.

4. **`Ping` blocks the event loop by design.** It spawns `/bin/ping` or
   `ping.exe` and busy-polls with `App.DoEvents` (`:1222`). That is a process
   spawn per AP, it parses *localised* console text (`"could not find host"`,
   `"100% loss"`), and — because it is inside the same serial loop — it is
   itself a large part of the cost. On a site list where most addresses are
   dead, which is the normal case, the tool spends most of its time waiting out
   ping timeouts one address at a time. ICMP is the right check; doing it
   serially, through a subprocess, is not.

## 4. Other things worth fixing while we're in here

These are maintainability costs, and they are the stronger half of the case for
a rewrite:

- **The core logic exists four times.** `ChangeFW.Run` (347 lines) and
  `Crossbreeder.subChangeFW` (`:1287`, 302 lines) are near-identical copies of
  each other; and *inside* each, the `Case "zf"` and `Case "ul"` branches are
  near-identical copies that differ only in which prompt string they wait for
  (`"rkscli: "` vs `"(ap-mode)# "`). Adding one command to the firmware
  sequence today means editing four places correctly.
- **Return values are ignored.** `success = ssh.ChannelSendString(...)` is
  assigned about sixty times and checked twice. A command that silently failed
  mid-sequence still reports `"Done"`.
- **Ordering bug:** `set factory` is issued *before* the `fw …` commands. On a
  real AP a factory reset wipes config and reboots, so every firmware setting
  sent afterwards is lost. Selecting both options together does not do what the
  UI implies.
- **`StrBetween` builds a regex out of device output** (`:1270`):
  `"(?<=" + Prefix + ")(.*)(?=" + Suffix + ")"`. Any regex metacharacter in the
  marker — or a variable-length prefix — breaks the match or throws.
- **A commercial licence key is committed in the source** (`:1065`,
  `ssh.UnlockComponent("RUCKUS.CB1122019_…")`). Chilkat is also a paid,
  closed-source, per-platform binary dependency: it is the single biggest reason
  the project is hard to build, hard to CI, and hard to hand to someone else.
- **32 MB of build artefacts are committed** (`Crossbreeder-MacOS.zip`,
  `Crossbreeder-Windows.zip`), which is most of the repository.
- **No tests, and no way to write one.** Nothing can be exercised without a real
  AP on the other end, because the logic and the UI are the same object.

## 5. Language choice

| | C | Rust | **Go** | Python | Stay on Xojo |
|---|---|---|---|---|---|
| Fixes the real bottleneck | via pthreads/epoll, by hand | yes (tokio) | **yes (goroutines)** | yes (asyncio) | no (see §3) |
| SSH library | libssh2, manual | russh / thrussh | **`golang.org/x/crypto/ssh`** | asyncssh | Chilkat (paid) |
| 500 concurrent sessions | 500 threads @ 8 MB stack, or hand-rolled state machine | async, needs care | **500 goroutines @ ~8 KB, no ceremony** | fine | not possible |
| Ships as one file, no runtime | yes | yes | **yes** | no (needs interpreter/bundler) | yes |
| Cross-compile Win+macOS from one box | painful | doable | **`GOOS=windows go build`** | n/a | needs a licence per platform |
| Memory safety while parsing device output | manual | yes | **yes** | yes | yes |
| GUI | separate per platform | separate per platform | Wails/Fyne, or keep the Xojo UI | Qt/Tk | native |

C is the weakest option on this list. It would take the most work, add a
memory-safety burden precisely where we parse untrusted device output, need a
hand-written concurrency layer, and buy back milliseconds that do not matter.

**Recommendation: Go.** It matches the current distribution model exactly (unzip
and run a single self-contained binary, as the README already promises), it
cross-compiles to Windows and macOS from one machine, its SSH library is
maintained and free — which retires the Chilkat licence — and concurrency is
the thing it is actually good at.

## 6. Proof of concept

`engine/` in this repository is a working implementation of the SSH core,
provided so the numbers above can be checked rather than believed. It keeps the
behaviour that matters — ZoneFlex/Unleashed detection, the `super`/`sp-admin`
fallback, `%M` model templating in the firmware filename, CSV in, CSV/JSON out —
and folds the four copies of the command sequence into one `dialect` table
(`engine/ap/session.go`), where the two AP families differ by a prompt string
and a preamble.

It ships with an in-process fake Ruckus AP (`engine/ap/fakeap_test.go`), which
is what makes the logic testable without a lab.

It runs in two phases, because on a real site list the two costs are different
problems. An **ICMP sweep** clears the dead addresses hundreds at a time with a
1.5s timeout, then the **SSH phase** works the survivors at `-c` at a time.

Measured on 60 simulated APs, each with a 500 ms stall, running the real binary
end to end — this is the SSH phase alone, every address live:

```
60 APs in 30.319s  (1 worker  — what the tool does today)
60 APs in  1.056s  (50 workers)
```

**28.7×**, and the fake APs are far faster to answer than real ones, so the
field gap would be wider. The unit test measures the same effect at 18.9× under
the race detector.

Measured on a more realistic list — 500 addresses holding 40 live APs:

```
ping sweep + SSH        6.6s     (460 skipped by the sweep, 40 contacted)
no gate (-probe none)  1m36s     (SSH attempted against all 500)
```

Both of those numbers already have the SSH phase running 40-wide. The serial
tool on the same list would spend roughly 460 × 1s working through the dead
addresses before it reached the live ones.

And on real hardware — a live 558-AP estate, Windows client:

```
Probing 558 addresses (icmp, 1.5s timeout, 1 retries, 256 at a time)...
499 of 558 answered in 3.054s; 59 skipped
558 addresses, 499 alive, 499 contacted in 6.548s
```

The sweep's own bound is measured too: 300 genuinely silent addresses (TEST-NET,
RFC 5737) clear in ~3s against a serial cost of 7m30s. Full test run:

```
$ cd engine && go test -race ./...
ok  github.com/andreacoppini/crossbreeder/engine/ap  12.5s
```

One caveat that shaped the design: an AP can be up with ICMP blocked by an ACL,
in which case a pure ping gate would skip it. `-probe both` (alive if ping *or*
the SSH port answers) exists for that, and `-probe tcp` for sites that filter
ICMP wholesale.

## 7. Suggested path

The lowest-risk order, each step shippable on its own:

1. **Take the engine as a CLI.** It already replaces the batch use case, and it
   is scriptable and CI-able in a way the GUI never was. Nothing has to change
   in the Xojo app for this to be useful.
2. **Validate against real hardware.** Done for the 7.x estate — 558 APs
   (R350, R670, H550, T350C/SE) inventoried from Windows in 6.5s, including the
   `iphlpapi` ICMP path. Still untested: pre-2015 ZoneFlex firmware, which needs
   the SHA-1 KEX and CBC ciphers modern SSH stacks disable by default
   (`-legacy`, on by default, re-enables them), and the Unleashed dialect.
3. **Point the existing UI at the engine** if you want to keep the Xojo front
   end: shell out to the binary and read its JSON. That gets the concurrency
   into the GUI immediately, with no Xojo threading involved.
4. **Replace the front end when convenient** — Wails (HTML UI) or Fyne, both
   still a single binary. Not urgent; the engine is where the value is.
5. **Housekeeping:** drop the committed zips from the repo and publish them as
   release assets, and rotate the Chilkat key that is in git history.

## 8. One thing it removes

The Xojo tool's README says "You need to supply your own HTTP, FTP or TFTP
server". The engine no longer does: `-serve <dir>` hosts the images from the
same binary, works out which local address routes to the APs, fills in the
firmware host and port from what it actually bound, and keeps serving until the
APs have taken the image. That is a second piece of software off the field
engineer's laptop, and one fewer thing to get wrong in a hurry.

## 9. What this does not change

Concurrency has a ceiling that is not in our code. A firmware push has every AP
pulling an image from one HTTP/FTP/TFTP server at once — 200 APs × ~30 MB is
6 GB through one server and one uplink. The engine's `-c` flag exists for that
reason: for inventory and CLI work, run it wide (50–100); for firmware pushes,
size it to what the firmware server and the WAN link can carry.
