# The original Crossbreeder (archived)

The Xojo application that Crossbreeder Plus replaces. It is kept because it is
the reference for what the tool is meant to do — several behaviours in the Go
version were recovered from here rather than reinvented — but it is **no longer
developed**. Fixes and releases go to [`../engine`](../engine).

The last builds of it are also at **https://dogtag.tacoppini.com**.

| | |
|---|---|
| `Crossbreeder.xojo_binary_project` | the current source, and the one to read |
| `*.xojo_code`, `.xojo_window`, `.xojo_project`, `.xojo_menu` | an older text export, kept for diffing |
| `Crossbreeder-Windows.zip`, `Crossbreeder-MacOS.zip` | the last builds |
| `multithreading-attempt/` | see below |

Read the **binary project**, not the text export. The export predates the
password-change handling, the Unleashed setup-wizard bypass and the `-legacy`
algorithms, so anyone treating it as the reference gets an incomplete answer —
which happened once already while fixing
[#5](https://github.com/andreacoppini/crossbreeder/issues/5).

## `multithreading-attempt/`

An abandoned attempt to make this version work several APs at once, from the
`multithreaded-parallel-runner` branch ([#2](https://github.com/andreacoppini/crossbreeder/pull/2)):
a batch runner, a concurrency helper, a credentials loader and design notes.

It was never finished, and the reason is in
[`../docs/ARCHITECTURE-REVIEW.md`](../docs/ARCHITECTURE-REVIEW.md): Xojo's
cooperative threads share one OS thread, so a blocking SSH call stalls every
other job rather than overlapping with it. Getting real concurrency meant
leaving the runtime, which is what Crossbreeder Plus did.

Kept because the notes record what was tried and why it did not work.
