#tag Module
Protected Module CredentialsLoader

	Function LoadCredentials(filePath As String) As Dictionary
		Dim d As New Dictionary
		
		If Not FolderItem(filePath).Exists Then
			Return d
		End If
		
		Dim tis As TextInputStream = TextInputStream.Open(FolderItem(filePath))
		
		While Not tis.EOF
			Dim line As String = Trim(tis.ReadLine)
			If line = "" Then Continue
			If Left(line,1) = "#" Then Continue
			
			Dim parts() As String = line.Split("=")
			If parts.Ubound = 1 Then
				d.Value(parts(0)) = parts(1)
			End If
		Wend
		
		tis.Close
		Return d
	End Function

End Module
#tag EndModule
