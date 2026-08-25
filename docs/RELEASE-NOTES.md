**Crossbreeder Plus** is a rebuild of Crossbreeder's engine that works access
points **in parallel** instead of one at a time, with a browser console and a
scriptable command line over the same engine.

Measured against a live 558-AP estate: a full inventory sweep finishes in
**6.5 seconds**. The original tool, working one AP at a time, takes over an hour
on a list that size.

## Getting started

Download the file for your platform, unzip it, and run it. There is no
installer and nothing to configure.

- **Windows** — `crossbreeder-plus-windows-amd64.exe` (use `-arm64` for Surface
  and Snapdragon machines). Double-click it, or run it from a terminal.
- **macOS** — `crossbreeder-plus-macos.zip`. One universal binary for Intel and
  Apple Silicon. It is unsigned, so the first launch needs
  `xattr -d com.apple.quarantine crossbreeder-plus-macos-universal`, or
  right-click → Open.
- **Linux** — `crossbreeder-plus-linux-amd64.tar.gz` (or `-arm64`).

Started with no arguments it opens a console in your browser, bound to
localhost. Everything it can do is also available as flags: run it with `-h`.

`SHA256SUMS.txt` covers every file in this release.

## What it does

- **Ping sweep first.** Most addresses on a site list are dead; each one costs a
  single unanswered packet rather than an SSH handshake against a timeout.
  Addresses that never answer are listed, folded into ranges, and can be written
  out to re-run later.
- **Inventory, firmware, factory reset, reboot, arbitrary CLI commands**, across
  ZoneFlex and Unleashed APs, at whatever concurrency the site can take.
- **Built-in image server.** A firmware push needs nothing installed beyond this
  binary: it serves the images itself, works out which of the machine's
  addresses the APs can actually reach, and shows what each AP is downloading.
- **Re-scans until you stop it.** The first pass does what you asked; every pass
  after that pings and re-reads the version, so an AP that drops off reads as
  rebooting and one that comes back on a new version is reported as upgraded.
- **CSV and JSON output**, with a row for every address including the silent
  ones.

## Known limits

- The binaries are unsigned on both Windows and macOS, so both will warn on
  first run.
- The Unleashed dialect and pre-2015 ZoneFlex firmware (`-legacy`) have been
  exercised against a simulated AP, not real hardware.
- A firmware push is only ever *started* by the tool; the AP downloads and
  reboots on its own schedule, which is what the re-scan is there to follow.

Full detail, including why this was rebuilt rather than optimised in place, is
in [`docs/ARCHITECTURE-REVIEW.md`](https://github.com/andreacoppini/crossbreeder/blob/master/docs/ARCHITECTURE-REVIEW.md).
