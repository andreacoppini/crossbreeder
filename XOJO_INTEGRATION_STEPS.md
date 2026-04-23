# Final Xojo integration steps

This branch adds the reusable pieces for running Crossbreeder jobs in parallel without blocking the UI:

- `APJobSettings`
- `APJobResult`
- `CredentialsLoader`
- `BatchRunner`
- credentials template

The remaining work must be applied inside the existing Xojo window and worker code because those files contain the app-specific SSH workflow and UI layout.

## 1. Add UI controls

Add the following controls to the `Crossbreeder` window:

- `PopupMenu popParallelJobs`
  - InitialValue: `1\r\n3\r\n5\r\n10`
  - Default/ListIndex: `2` for 5 workers

- `PopupMenu popRetries`
  - InitialValue: `0\r\n1\r\n2\r\n3`
  - Default/ListIndex: `1` for one retry

- `PushButton btnPauseResume`
  - Caption: `Pause`
  - Enabled: `False`

- `PushButton btnCancelBatch`
  - Caption: `Cancel`
  - Enabled: `False`

- `Timer tmrWorkers`
  - Period: `250`
  - Mode: `Off`

## 2. Add window properties

Add these properties to `Crossbreeder`:

```xojo
Private Runner As BatchRunner
Private CurrentSettings As APJobSettings
```

## 3. Replace GO button logic

The GO button should now call:

```xojo
StartBatch
```

## 4. Add `StartBatch`

```xojo
Sub StartBatch()
  Dim maxParallel As Integer = Val(popParallelJobs.Text)
  If maxParallel < 1 Then maxParallel = 5
  
  Dim maxRetries As Integer = Val(popRetries.Text)
  If maxRetries < 0 Then maxRetries = 1
  
  Runner = New BatchRunner(maxParallel, maxRetries)
  CurrentSettings = CurrentSettingsSnapshot
  
  For row As Integer = 0 To listmigrateAP.ListCount - 1
    If Trim(listmigrateAP.Cell(row, 0)) <> "" Then
      Runner.AddJob(row)
      listmigrateAP.Cell(row, 5) = "Queued"
      listmigrateAP.CellTag(row, 5) = "0"
    End If
  Next
  
  progBar.Maximum = Runner.TotalJobs
  progBar.Value = 0
  progBar.Visible = True
  
  btnMigrateGO.Enabled = False
  btnPauseResume.Enabled = True
  btnCancelBatch.Enabled = True
  btnPauseResume.Caption = "Pause"
  
  tmrWorkers.Mode = Timer.ModeMultiple
  StartNextWorkers
End Sub
```

## 5. Add `StartNextWorkers`

```xojo
Sub StartNextWorkers()
  If Runner = Nil Then Return
  
  While Runner.CanStartMore
    Dim row As Integer = Runner.NextRow
    
    Dim w As New ChangeFW
    w.HostName = listmigrateAP.Cell(row, 0)
    w.Row = row
    w.Attempt = Val(listmigrateAP.CellTag(row, 5)) + 1
    w.Settings = CurrentSettings
    
    listmigrateAP.CellTag(row, 5) = Str(w.Attempt)
    listmigrateAP.Cell(row, 5) = "Running"
    
    Runner.AddActive(w)
    w.Run
  Wend
End Sub
```

## 6. Add timer action

```xojo
Sub tmrWorkers.Action()
  If Runner = Nil Then Return
  
  For i As Integer = Runner.ActiveWorkers.Ubound DownTo 0
    Dim w As ChangeFW = Runner.ActiveWorkers(i)
    
    If w.State = Thread.NotRunning Then
      Dim r As APJobResult = w.Result
      
      If r <> Nil Then
        If r.APMAC <> "" Then listmigrateAP.Cell(r.Row, 1) = r.APMAC
        If r.APModel <> "" Then listmigrateAP.Cell(r.Row, 2) = r.APModel
        If r.APFWVersion <> "" Then listmigrateAP.Cell(r.Row, 3) = r.APFWVersion
        listmigrateAP.Cell(r.Row, 5) = r.ResultText
        
        If r.DebugText <> "" Then txtDebug.AppendText(r.DebugText)
      Else
        listmigrateAP.Cell(w.Row, 5) = "Error"
      End If
      
      Runner.RemoveActive(i)
      
      If r <> Nil And r.Success = False And r.Cancelled = False And r.Attempt <= Runner.MaxRetries Then
        listmigrateAP.Cell(r.Row, 5) = "Retrying " + Str(r.Attempt) + "/" + Str(Runner.MaxRetries)
        Runner.QueueRetry(r.Row)
      Else
        Runner.CompletedJobs = Runner.CompletedJobs + 1
        progBar.Value = Runner.CompletedJobs
      End If
    End If
  Next
  
  If Not Runner.Paused And Not Runner.Cancelled Then
    StartNextWorkers
  End If
  
  If Runner.IsFinished Then
    FinishBatch
  End If
End Sub
```

## 7. Add pause/resume button action

```xojo
Sub btnPauseResume.Action()
  If Runner = Nil Then Return
  
  Runner.Paused = Not Runner.Paused
  
  If Runner.Paused Then
    btnPauseResume.Caption = "Resume"
    txtDebug.AppendText("Batch paused. Active workers will finish, but no new APs will start." + EndOfLine)
  Else
    btnPauseResume.Caption = "Pause"
    txtDebug.AppendText("Batch resumed." + EndOfLine)
    StartNextWorkers
  End If
End Sub
```

## 8. Add cancel button action

```xojo
Sub btnCancelBatch.Action()
  If Runner = Nil Then Return
  
  Runner.RequestCancel
  txtDebug.AppendText("Cancellation requested. Active workers will stop at the next safe checkpoint." + EndOfLine)
End Sub
```

## 9. Add `FinishBatch`

```xojo
Sub FinishBatch()
  tmrWorkers.Mode = Timer.ModeOff
  
  progBar.Visible = False
  btnMigrateGO.Enabled = True
  btnPauseResume.Enabled = False
  btnCancelBatch.Enabled = False
  btnPauseResume.Caption = "Pause"
  
  If Runner <> Nil And Runner.Cancelled Then
    txtDebug.AppendText("Batch cancelled." + EndOfLine)
  Else
    txtDebug.AppendText("Batch completed." + EndOfLine)
  End If
End Sub
```

## 10. Add `CurrentSettingsSnapshot`

```xojo
Function CurrentSettingsSnapshot() As APJobSettings
  Dim s As New APJobSettings
  
  Dim creds As Dictionary = CredentialsLoader.LoadCredentials("crossbreeder.credentials.txt")
  
  If creds.HasKey("AP_USER") Then s.APUser = creds.Value("AP_USER")
  If creds.HasKey("AP_PASS") Then s.APPass = creds.Value("AP_PASS")
  If creds.HasKey("AP_DEFAULT_USER") Then s.DefaultUser = creds.Value("AP_DEFAULT_USER")
  If creds.HasKey("AP_DEFAULT_PASS") Then s.DefaultPass = creds.Value("AP_DEFAULT_PASS")
  
  If txtMigrateAPUser.Text <> "" Then s.APUser = txtMigrateAPUser.Text
  If txtMigrateAPPass.Text <> "" Then s.APPass = txtMigrateAPPass.Text
  
  s.AlsoTryDefault = chkMigrateAlsoDefault.State = CheckBox.CheckedStates.Checked
  s.ChangeFirmware = chkMigrateFw.State = CheckBox.CheckedStates.Checked
  s.FactoryReset = chkMigrateAlsoFactory.State = CheckBox.CheckedStates.Checked
  s.RebootAP = chkMigrateAlsoReboot.State = CheckBox.CheckedStates.Checked
  
  s.RunExtraCommand = chkMigrateAlsoRun.State = CheckBox.CheckedStates.Checked
  s.ExtraCommand = txtMigrateAlsoRun.Text
  
  s.SrvProto = srvProto
  s.SrvIP = txtMigrateSrvIP.Text
  s.SrvPort = txtMigrateSrvPort.Text
  s.SrvUser = txtMigrateSrvUser.Text
  s.SrvPass = txtMigrateSrvPass.Text
  s.FwFilenameMask = txtMigrateFwFilename.Text
  s.TimeOutMs = 5000
  
  Return s
End Function
```

## 11. Update `ChangeFW`

`ChangeFW` must become a real thread worker:

- Add properties:
  - `HostName As String`
  - `Row As Integer`
  - `Attempt As Integer`
  - `Settings As APJobSettings`
  - `Result As APJobResult`
  - `CancelRequested As Boolean`

- Replace the parameterized function:

```xojo
Function Run(HostName as string, Row as Integer, optional TimeOut as Integer = 5000) As String
```

with:

```xojo
Sub Run()
  Result = ExecuteAPJob
End Sub
```

- Rename the old body to:

```xojo
Function ExecuteAPJob() As APJobResult
```

- Replace UI reads with `Settings.*`.
- Replace UI writes with `Result.*`.
- Replace debug appends with `Result.DebugText = Result.DebugText + ...`.
- Return `Result` instead of a string.

## Notes

The worker must not read or write `Crossbreeder` UI controls directly. Only the timer on the window should update controls.
