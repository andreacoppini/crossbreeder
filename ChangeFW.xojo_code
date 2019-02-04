#tag Class
Protected Class ChangeFW
Inherits Thread
	#tag Method, Flags = &h0
		Function Run(HostName as string, Row as Integer, optional TimeOut as Integer = 5000) As String
		  
		  Dim ssh As New Chilkat.Ssh
		  Dim port As Int32
		  Dim success As Boolean
		  Dim strOutput,sshRx As String
		  Dim saPrompts As New Chilkat.StringArray
		  Dim APType,APModel,APFWVersion,APMAC As String
		  Dim APFWFilename,APFWULString As String
		  
		  APFWFilename = Replace(Crossbreeder.txtMigrateFwFilename.Text,"%M","R720")
		  
		  port = 22
		  ssh.ConnectTimeoutMs = TimeOut
		  ssh.IdleTimeoutMs = TimeOut
		  ssh.ReadTimeoutMs = TimeOut
		  
		  //  Open SSH connection 
		  success = ssh.Connect(HostName,port)
		  Crossbreeder.txtDebug.AppendText("Connected: " + HostName + "("+ Str(success) + ")" + EndOfLine)
		  
		  //  Authenticate using login/password:
		  success = ssh.AuthenticatePw(Crossbreeder.txtMigrateAPUser.Text,Crossbreeder.txtMigrateAPPass.Text)
		  
		  // Determine assigned SSH channel number
		  Dim channelNum As Int32
		  channelNum = ssh.OpenSessionChannel()
		  
		  // Set TTY Mode
		  Dim termType As String
		  termType = "dumb"
		  Dim widthInChars As Int32
		  widthInChars = 120
		  Dim heightInChars As Int32
		  heightInChars = 40
		  //  Use 0 for pixWidth and pixHeight when the dimensions
		  //  are set in number-of-chars.
		  Dim pixWidth As Int32
		  pixWidth = 0
		  Dim pixHeight As Int32
		  pixHeight = 0
		  success = ssh.SendReqPty(channelNum,termType,widthInChars,heightInChars,pixWidth,pixHeight)
		  
		  //  Start a shell on the channel:
		  success = ssh.SendReqShell(channelNum)
		  If (success <> True) Then
		    Crossbreeder.txtDebug.AppendText("SSH Failed to connect to "+ HostName + "." + EndOfLine)
		    Return "SSH Failed"
		  End If
		  
		  Crossbreeder.txtDebug.AppendText(ssh.GetReceivedText(channelNum,"utf-8") + EndOfLine)
		  
		  success = ssh.ChannelSendString(channelNum,Crossbreeder.txtMigrateAPUser.Text + EndOfLine.Unix,"utf-8")
		  success = ssh.ChannelReceiveUntilMatch(channelNum,"assword :","utf-8",FALSE)
		  Crossbreeder.txtDebug.AppendText(ssh.GetReceivedText(channelNum,"utf-8") + EndOfLine)
		  
		  success = ssh.ChannelSendString(channelNum,Crossbreeder.txtMigrateAPPass.Text + EndOfLine.Unix,"utf-8")
		  
		  success = saPrompts.Append("rkscli: ")   ' logged in to AP CLI
		  success = saPrompts.Append("Login incorrect")    ' wrong credentials
		  success = saPrompts.Append("> ")           ' logged in to Unleashed Configured AP
		  
		  success = ssh.ChannelReceiveUntilMatchN(channelNum,saPrompts,"utf-8",FALSE)
		  sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		  Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		  
		  If InStr(sshRx,"Login incorrect")>0 Then
		    success = False
		    If Crossbreeder.chkMigrateAlsoDefault.State = Checkbox.CheckedStates.Checked Then
		      Crossbreeder.txtDebug.AppendText("-Trying defaults.." + EndOfLine)
		      Crossbreeder.txtDebug.AppendText(ssh.GetReceivedText(channelNum,"utf-8") + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum, "super" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"assword :","utf-8",FALSE)
		      Crossbreeder.txtDebug.AppendText(ssh.GetReceivedText(channelNum,"utf-8") + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"sp-admin" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatchN(channelNum,saPrompts,"utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("-RX|" + sshRx + "||RXEnd-" + EndOfLine)
		      if (InStr(sshRx,"rkscli: ")>0 or InStr(sshRx,"> ")>0) Then 
		        success=True
		      Else
		        success=False
		      End If
		    End If
		  End if
		  If (success <> True) Then
		    Crossbreeder.txtDebug.AppendText("Login Failed!" + EndOfLine)
		    Return "Login Failed"
		  End If
		  
		  APType = ""
		  If inStr(sshRx,"rkscli: ")>0 Then APType = "zf"
		  If inStr(sshRx,"> ")>0 Then APType= "ul"
		  
		  Select Case APType
		  Case "zf"
		    Crossbreeder.txtDebug.AppendText("Starting ZoneFlex firmware process" + EndOfLine)
		    success = ssh.ChannelSendString(channelNum,"get version" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    
		    APModel = Crossbreeder.StrBetween(sshRx,"Ruckus "," Multimedia Hotzone Wireless AP")
		    APFWVersion = Crossbreeder.StrBetween(sshRx,"Version: ","")
		    
		    Crossbreeder.listmigrateAP.cell(Row,2) = APModel
		    Crossbreeder.listmigrateAP.cell(Row,3) = APFWVersion
		    APFWFilename = Replace(Crossbreeder.txtMigrateFwFilename.Text,"%M",APModel)
		    
		    success = ssh.ChannelSendString(channelNum,"get boarddata" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    
		    APMAC = Crossbreeder.StrBetween(sshRx, ", base ","")
		    Crossbreeder.listmigrateAP.cell(Row,1) = APMAC
		    
		    If Crossbreeder.chkMigrateAlsoFactory.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"set factory" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End if
		    
		    if Crossbreeder.chkMigrateFw.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"fw auto disable" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set proto " + Crossbreeder.srvProto + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set port " + Crossbreeder.txtMigrateSrvPort.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set control " + APFWFilename + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set host " + Crossbreeder.txtMigrateSrvIP.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set user " + Crossbreeder.txtMigrateSrvUser.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set password " + Crossbreeder.txtMigrateSrvPass.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw auto enable" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw update" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End If
		    
		    if Crossbreeder.chkMigrateAlsoRun.State = CheckBox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum, Crossbreeder.txtMigrateAlsoRun.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End If
		    
		    If Crossbreeder.chkMigrateAlsoReboot.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"reboot" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End if
		    
		  Case "ul"
		    Crossbreeder.txtDebug.AppendText("Starting Unleashed firmware process" + EndOfLine)
		    Crossbreeder.listmigrateAP.cell(Row,2) = "Unleashed"
		    
		    success = ssh.ChannelSendString(channelNum,"enable force" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"# ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    
		    success = ssh.ChannelSendString(channelNum,"show sysinfo" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"# ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    
		    Crossbreeder.txtDebug.AppendText(sshRx + EndOfLine)
		    
		    APModel = Crossbreeder.StrBetween (sshRx, "Model= ","")
		    APFWVersion = Replace(Crossbreeder.StrBetween(sshRx, "Version= ","")," Build ",".")
		    APMAC = Uppercase(Crossbreeder.StrBetween (sshRx, "MAC Address= ",""))
		    
		    Crossbreeder.listmigrateAP.cell(Row,1) = APMAC
		    Crossbreeder.listmigrateAP.cell(Row,2) = APModel
		    Crossbreeder.listmigrateAP.cell(Row,3) = APFWVersion
		    APFWFilename = Replace(Crossbreeder.txtMigrateFwFilename.Text,"%M",APModel)
		    
		    success = ssh.ChannelSendString(channelNum,"ap-mode" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    
		    If Crossbreeder.chkMigrateAlsoFactory.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"set factory" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End if
		    
		    if Crossbreeder.chkMigrateFw.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"fw auto disable" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set proto " + Crossbreeder.srvProto + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set port " + Crossbreeder.txtMigrateSrvPort.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set control " + APFWFilename + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set host " + Crossbreeder.txtMigrateSrvIP.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set user " + Crossbreeder.txtMigrateSrvUser.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set password " + Crossbreeder.txtMigrateSrvPass.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw auto enable" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw update" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		    End If
		    
		    if Crossbreeder.chkMigrateAlsoRun.State = CheckBox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum, Crossbreeder.txtMigrateAlsoRun.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End If
		    
		    If Crossbreeder.chkMigrateAlsoReboot.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"reboot" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      Crossbreeder.txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End if
		    
		    
		  Else
		    Crossbreeder.txtDebug.AppendText("Unknown AP type! Skipping..." + EndOfLine)
		  End Select
		  
		  
		  If (ssh.LastMethodSuccess <> True) Then
		    Crossbreeder.txtDebug.AppendText(ssh.LastErrorText + EndOfLine)
		    Return "Error"
		  End If
		  
		  ssh.Disconnect
		  Crossbreeder.txtDebug.AppendText("disconnect" + EndOfLine)
		  
		  Return "Done"
		  
		End Function
	#tag EndMethod


	#tag ViewBehavior
		#tag ViewProperty
			Name="Name"
			Visible=true
			Group="ID"
			Type="String"
			EditorType="String"
		#tag EndViewProperty
		#tag ViewProperty
			Name="Index"
			Visible=true
			Group="ID"
			Type="Integer"
			EditorType="Integer"
		#tag EndViewProperty
		#tag ViewProperty
			Name="Super"
			Visible=true
			Group="ID"
			Type="String"
			EditorType="String"
		#tag EndViewProperty
		#tag ViewProperty
			Name="Priority"
			Visible=true
			Group="Behavior"
			InitialValue="5"
			Type="Integer"
		#tag EndViewProperty
		#tag ViewProperty
			Name="StackSize"
			Visible=true
			Group="Behavior"
			InitialValue="0"
			Type="Integer"
		#tag EndViewProperty
	#tag EndViewBehavior
End Class
#tag EndClass
