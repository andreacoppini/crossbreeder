# Crossbreeder multithreaded runner design

This branch starts the refactor needed to make Crossbreeder run AP jobs in parallel without freezing the UI.

## Current problem

`ChangeFW` inherits `Thread`, but the useful work is currently implemented as a regular method:

```xojo
Function Run(HostName as string, Row as Integer, optional TimeOut as Integer = 5000) As String
```

That means code can call `Run(...)` synchronously from the button handler, which blocks the main UI thread. The method also reads and writes UI controls directly, for example:

- `Crossbreeder.txtDebug.AppendText(...)`
- `Crossbreeder.listmigrateAP.Cell(Row, ...) = ...`
- `Crossbreeder.txtMigrateAPUser.Text`
- `Crossbreeder.chkMigrateFw.State`

Direct UI access from worker threads is not safe in Xojo. So the correct solution is not just to start several `ChangeFW` objects in parallel. The worker needs to receive a snapshot of settings, do SSH work off the UI thread, and hand results back to the window for UI updates.

## Proposed approach

1. Convert `ChangeFW` into a real worker thread:
   - Add properties such as `HostName`, `Row`, `TimeOutMs`, and a `Settings` snapshot.
   - Move the SSH logic into the thread event `Run`/`Run()` with no parameters.
   - Start workers with `.Run`, not by calling a parameterized function directly.

2. Add a thread-safe result handoff:
   - Worker writes status into its own properties, for example `ResultText`, `DebugText`, `APModel`, `APFWVersion`, `APMAC`.
   - Worker raises an event/callback or is polled by a UI `Timer`.
   - Only the main window updates `txtDebug`, `listmigrateAP`, `progBar`, and button state.

3. Add bounded parallelism:
   - Maintain two arrays on the window: pending row indexes and active workers.
   - Start up to `MaxParallelJobs`, for example 5.
   - When a worker completes, update that row, remove it from active workers, then start the next pending job.

4. Add a UI control for concurrency:
   - Either a `PopupMenu` or `TextField` labelled `Parallel Jobs`.
   - Default to 5 so SSH/TFTP/FTP servers and APs are not overwhelmed.

## Skeleton logic

```xojo
Private PendingRows() As Integer
Private ActiveWorkers() As ChangeFW
Private CompletedJobs As Integer
Private TotalJobs As Integer
Private MaxParallelJobs As Integer = 5

Sub StartBatch()
  PendingRows.RemoveAll
  ActiveWorkers.RemoveAll
  CompletedJobs = 0
  TotalJobs = listmigrateAP.ListCount

  For row As Integer = 0 To listmigrateAP.ListCount - 1
    If Trim(listmigrateAP.Cell(row, 0)) <> "" Then PendingRows.Append(row)
  Next

  btnMigrateGO.Enabled = False
  progBar.Maximum = PendingRows.Ubound + 1
  progBar.Value = 0
  progBar.Visible = True

  StartNextWorkers
End Sub

Sub StartNextWorkers()
  While ActiveWorkers.Ubound + 1 < MaxParallelJobs And PendingRows.Ubound >= 0
    Dim row As Integer = PendingRows(0)
    PendingRows.Remove(0)

    Dim w As New ChangeFW
    w.HostName = listmigrateAP.Cell(row, 0)
    w.Row = row
    w.Settings = CurrentSettingsSnapshot
    AddHandler w.UserInterfaceUpdate, AddressOf WorkerUIUpdate
    ActiveWorkers.Append(w)
    w.Run
  Wend
End Sub

Sub WorkerUIUpdate(worker As ChangeFW)
  // Main/UI thread only: update listbox, debug text, progress bar.
  listmigrateAP.Cell(worker.Row, 1) = worker.APMAC
  listmigrateAP.Cell(worker.Row, 2) = worker.APModel
  listmigrateAP.Cell(worker.Row, 3) = worker.APFWVersion
  listmigrateAP.Cell(worker.Row, 5) = worker.ResultText
  txtDebug.AppendText(worker.DebugText)

  CompletedJobs = CompletedJobs + 1
  progBar.Value = CompletedJobs

  RemoveWorker(worker)
  StartNextWorkers

  If CompletedJobs = TotalJobs Then
    btnMigrateGO.Enabled = True
    progBar.Visible = False
  End If
End Sub
```

## Important notes

- The existing `Run(HostName, Row, TimeOut)` should be renamed to something like `ExecuteAPJob` so it is not confused with the thread entrypoint.
- Any read from the window should be replaced by values captured before the worker starts.
- Any write to the window should be replaced by worker result properties and applied by the main window.
- Parallel firmware changes should be capped. Unlimited parallel SSH sessions can overload APs, the firmware server, or the LAN.
