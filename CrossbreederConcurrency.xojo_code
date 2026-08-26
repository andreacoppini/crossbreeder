#tag Class
Protected Class APJobSettings
	#tag Property, Flags = &h0
		APUser As String
	#tag EndProperty
	#tag Property, Flags = &h0
		APPass As String
	#tag EndProperty
	#tag Property, Flags = &h0
		AlsoTryDefault As Boolean
	#tag EndProperty
	#tag Property, Flags = &h0
		ChangeFirmware As Boolean
	#tag EndProperty
	#tag Property, Flags = &h0
		FactoryReset As Boolean
	#tag EndProperty
	#tag Property, Flags = &h0
		RebootAP As Boolean
	#tag EndProperty
	#tag Property, Flags = &h0
		RunExtraCommand As Boolean
	#tag EndProperty
	#tag Property, Flags = &h0
		ExtraCommand As String
	#tag EndProperty
	#tag Property, Flags = &h0
		SrvProto As String
	#tag EndProperty
	#tag Property, Flags = &h0
		SrvIP As String
	#tag EndProperty
	#tag Property, Flags = &h0
		SrvPort As String
	#tag EndProperty
	#tag Property, Flags = &h0
		SrvUser As String
	#tag EndProperty
	#tag Property, Flags = &h0
		SrvPass As String
	#tag EndProperty
	#tag Property, Flags = &h0
		FwFilenameMask As String
	#tag EndProperty
	#tag Property, Flags = &h0
		TimeOutMs As Integer = 5000
	#tag EndProperty
End Class
#tag EndClass

#tag Class
Protected Class APJobResult
	#tag Property, Flags = &h0
		Row As Integer
	#tag EndProperty
	#tag Property, Flags = &h0
		HostName As String
	#tag EndProperty
	#tag Property, Flags = &h0
		Attempt As Integer
	#tag EndProperty
	#tag Property, Flags = &h0
		ResultText As String
	#tag EndProperty
	#tag Property, Flags = &h0
		DebugText As String
	#tag EndProperty
	#tag Property, Flags = &h0
		APModel As String
	#tag EndProperty
	#tag Property, Flags = &h0
		APFWVersion As String
	#tag EndProperty
	#tag Property, Flags = &h0
		APMAC As String
	#tag EndProperty
	#tag Property, Flags = &h0
		Success As Boolean
	#tag EndProperty
	#tag Property, Flags = &h0
		Cancelled As Boolean
	#tag EndProperty
End Class
#tag EndClass
