#tag Class
Protected Class BatchRunner

	#tag Method, Flags = &h0
		Sub Constructor(maxParallelJobs As Integer, maxRetries As Integer)
			If maxParallelJobs < 1 Then maxParallelJobs = 1
			If maxRetries < 0 Then maxRetries = 0
			Self.MaxParallelJobs = maxParallelJobs
			Self.MaxRetries = maxRetries
		End Sub
	#tag EndMethod

	#tag Method, Flags = &h0
		Sub AddJob(row As Integer)
			PendingRows.Append(row)
			TotalJobs = TotalJobs + 1
		End Sub
	#tag EndMethod

	#tag Method, Flags = &h0
		Function CanStartMore() As Boolean
			Return Not Paused And Not Cancelled And ActiveWorkers.Ubound + 1 < MaxParallelJobs And PendingRows.Ubound >= 0
		End Function
	#tag EndMethod

	#tag Method, Flags = &h0
		Function NextRow() As Integer
			Dim row As Integer = PendingRows(0)
			PendingRows.Remove(0)
			Return row
		End Function
	#tag EndMethod

	#tag Method, Flags = &h0
		Sub AddActive(worker As ChangeFW)
			ActiveWorkers.Append(worker)
		End Sub
	#tag EndMethod

	#tag Method, Flags = &h0
		Sub RemoveActive(index As Integer)
			ActiveWorkers.Remove(index)
		End Sub
	#tag EndMethod

	#tag Method, Flags = &h0
		Sub QueueRetry(row As Integer)
			PendingRows.Append(row)
		End Sub
	#tag EndMethod

	#tag Method, Flags = &h0
		Function IsFinished() As Boolean
			Return CompletedJobs >= TotalJobs And ActiveWorkers.Ubound < 0 And PendingRows.Ubound < 0
		End Function
	#tag EndMethod

	#tag Method, Flags = &h0
		Sub RequestCancel()
			Cancelled = True
			PendingRows.RemoveAll
			For Each worker As ChangeFW In ActiveWorkers
				worker.CancelRequested = True
			Next
		End Sub
	#tag EndMethod

	#tag Property, Flags = &h0
		PendingRows() As Integer
	#tag EndProperty
	#tag Property, Flags = &h0
		ActiveWorkers() As ChangeFW
	#tag EndProperty
	#tag Property, Flags = &h0
		CompletedJobs As Integer
	#tag EndProperty
	#tag Property, Flags = &h0
		TotalJobs As Integer
	#tag EndProperty
	#tag Property, Flags = &h0
		MaxParallelJobs As Integer = 5
	#tag EndProperty
	#tag Property, Flags = &h0
		MaxRetries As Integer = 1
	#tag EndProperty
	#tag Property, Flags = &h0
		Paused As Boolean
	#tag EndProperty
	#tag Property, Flags = &h0
		Cancelled As Boolean
	#tag EndProperty

End Class
#tag EndClass
