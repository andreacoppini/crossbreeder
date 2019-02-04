#tag Window
Begin Window Crossbreeder
   BackColor       =   &cFFFFFF00
   Backdrop        =   0
   CloseButton     =   True
   Compatibility   =   ""
   Composite       =   False
   Frame           =   0
   FullScreen      =   False
   FullScreenButton=   False
   HasBackColor    =   False
   Height          =   584
   ImplicitInstance=   True
   LiveResize      =   True
   MacProcID       =   0
   MaxHeight       =   32000
   MaximizeButton  =   True
   MaxWidth        =   32000
   MenuBar         =   140601343
   MenuBarVisible  =   True
   MinHeight       =   584
   MinimizeButton  =   True
   MinWidth        =   700
   Placement       =   0
   Resizeable      =   True
   Title           =   "Crossbreeder"
   Visible         =   True
   Width           =   1028
   Begin Label lblLoadCSVPrompt
      AutoDeactivate  =   True
      Bold            =   False
      DataField       =   ""
      DataSource      =   ""
      Enabled         =   True
      Height          =   420
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   20
      LockBottom      =   True
      LockedInPosition=   False
      LockLeft        =   True
      LockRight       =   True
      LockTop         =   True
      Multiline       =   False
      Scope           =   2
      Selectable      =   False
      TabIndex        =   9
      TabPanelIndex   =   0
      TabStop         =   True
      Text            =   "Please load a CSV file\r\nor\r\nclick here for template"
      TextAlign       =   1
      TextColor       =   &c00000000
      TextFont        =   "System"
      TextSize        =   20.0
      TextUnit        =   0
      Top             =   20
      Transparent     =   True
      Underline       =   False
      Visible         =   True
      Width           =   715
   End
   Begin Listbox listmigrateAP
      AutoDeactivate  =   True
      AutoHideScrollbars=   False
      Bold            =   False
      Border          =   True
      ColumnCount     =   6
      ColumnsResizable=   True
      ColumnWidths    =   ""
      DataField       =   ""
      DataSource      =   ""
      DefaultRowHeight=   -1
      Enabled         =   True
      EnableDrag      =   False
      EnableDragReorder=   False
      GridLinesHorizontal=   2
      GridLinesVertical=   2
      HasHeading      =   True
      HeadingIndex    =   -1
      Height          =   420
      HelpTag         =   ""
      Hierarchical    =   False
      Index           =   -2147483648
      InitialParent   =   ""
      InitialValue    =   "IP Address	MAC Address	Model	Fw Version	Ping	Result\r\n10.1.1.1\r\n10.1.5.2\r\n10.5.1.1\r\n192.168.5.100"
      Italic          =   False
      Left            =   20
      LockBottom      =   True
      LockedInPosition=   False
      LockLeft        =   True
      LockRight       =   True
      LockTop         =   True
      RequiresSelection=   False
      Scope           =   0
      ScrollbarHorizontal=   True
      ScrollBarVertical=   True
      SelectionType   =   1
      ShowDropIndicator=   False
      TabIndex        =   0
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   20
      Transparent     =   False
      Underline       =   False
      UseFocusRing    =   True
      Visible         =   False
      Width           =   715
      _ScrollOffset   =   0
      _ScrollWidth    =   -1
   End
   Begin PushButton btnMigrateCSVImport
      AutoDeactivate  =   True
      Bold            =   False
      ButtonStyle     =   "0"
      Cancel          =   False
      Caption         =   "Import CSV..."
      Default         =   False
      Enabled         =   True
      Height          =   22
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   20
      LockBottom      =   True
      LockedInPosition=   False
      LockLeft        =   True
      LockRight       =   False
      LockTop         =   False
      Scope           =   2
      TabIndex        =   1
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   453
      Transparent     =   False
      Underline       =   False
      Visible         =   True
      Width           =   109
   End
   Begin Label lblorideCSVName
      AutoDeactivate  =   True
      Bold            =   False
      DataField       =   ""
      DataSource      =   ""
      Enabled         =   True
      Height          =   20
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   141
      LockBottom      =   True
      LockedInPosition=   False
      LockLeft        =   True
      LockRight       =   False
      LockTop         =   False
      Multiline       =   False
      Scope           =   0
      Selectable      =   False
      TabIndex        =   2
      TabPanelIndex   =   0
      TabStop         =   True
      Text            =   "No File Loaded"
      TextAlign       =   0
      TextColor       =   &c00000000
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   453
      Transparent     =   True
      Underline       =   False
      Visible         =   True
      Width           =   322
   End
   Begin PushButton btnMigrateGO
      AutoDeactivate  =   True
      Bold            =   True
      ButtonStyle     =   "0"
      Cancel          =   False
      Caption         =   "GO!"
      Default         =   False
      Enabled         =   False
      Height          =   22
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   878
      LockBottom      =   True
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   False
      Scope           =   2
      TabIndex        =   18
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   453
      Transparent     =   False
      Underline       =   False
      Visible         =   True
      Width           =   130
   End
   Begin TextArea txtDebug
      AcceptTabs      =   False
      Alignment       =   0
      AutoDeactivate  =   False
      AutomaticallyCheckSpelling=   False
      BackColor       =   &cFFFFFF00
      Bold            =   False
      Border          =   True
      DataField       =   ""
      DataSource      =   ""
      Enabled         =   True
      Format          =   ""
      Height          =   71
      HelpTag         =   ""
      HideSelection   =   True
      Index           =   -2147483648
      Italic          =   False
      Left            =   20
      LimitText       =   0
      LineHeight      =   0.0
      LineSpacing     =   1.0
      LockBottom      =   True
      LockedInPosition=   False
      LockLeft        =   True
      LockRight       =   True
      LockTop         =   False
      Mask            =   ""
      Multiline       =   True
      ReadOnly        =   False
      Scope           =   0
      ScrollbarHorizontal=   True
      ScrollbarVertical=   True
      Styled          =   True
      TabIndex        =   200
      TabPanelIndex   =   0
      TabStop         =   True
      Text            =   ""
      TextColor       =   &c00000000
      TextFont        =   "SmallSystem"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   493
      Transparent     =   False
      Underline       =   False
      UseFocusRing    =   True
      Visible         =   True
      Width           =   988
   End
   Begin GroupBox grpMigrateServer
      AutoDeactivate  =   True
      Bold            =   False
      Caption         =   "Firmware Server"
      Enabled         =   False
      Height          =   183
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   747
      LockBottom      =   False
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   True
      Scope           =   2
      TabIndex        =   6
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   150
      Transparent     =   False
      Underline       =   False
      Visible         =   True
      Width           =   261
      Begin TextField txtMigrateSrvIP
         AcceptTabs      =   False
         Alignment       =   0
         AutoDeactivate  =   True
         AutomaticallyCheckSpelling=   False
         BackColor       =   &cFFFFFF00
         Bold            =   False
         Border          =   True
         CueText         =   "Server Address"
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Format          =   ""
         Height          =   22
         HelpTag         =   "Server IP address"
         Index           =   -2147483648
         InitialParent   =   "grpMigrateServer"
         Italic          =   False
         Left            =   767
         LimitText       =   0
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   False
         LockRight       =   True
         LockTop         =   True
         Mask            =   ""
         Password        =   False
         ReadOnly        =   False
         Scope           =   0
         TabIndex        =   8
         TabPanelIndex   =   0
         TabStop         =   True
         Text            =   ""
         TextColor       =   &c00000000
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   210
         Transparent     =   False
         Underline       =   False
         UseFocusRing    =   True
         Visible         =   True
         Width           =   151
      End
      Begin TextField txtMigrateSrvPort
         AcceptTabs      =   False
         Alignment       =   0
         AutoDeactivate  =   True
         AutomaticallyCheckSpelling=   False
         BackColor       =   &cFFFFFF00
         Bold            =   False
         Border          =   True
         CueText         =   "Port"
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Format          =   ""
         Height          =   22
         HelpTag         =   "Server port"
         Index           =   -2147483648
         InitialParent   =   "grpMigrateServer"
         Italic          =   False
         Left            =   930
         LimitText       =   0
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   False
         LockRight       =   True
         LockTop         =   True
         Mask            =   "##999"
         Password        =   False
         ReadOnly        =   False
         Scope           =   0
         TabIndex        =   9
         TabPanelIndex   =   0
         TabStop         =   True
         Text            =   "21"
         TextColor       =   &c00000000
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   210
         Transparent     =   False
         Underline       =   False
         UseFocusRing    =   True
         Visible         =   True
         Width           =   65
      End
      Begin TextField txtMigrateSrvUser
         AcceptTabs      =   False
         Alignment       =   0
         AutoDeactivate  =   True
         AutomaticallyCheckSpelling=   False
         BackColor       =   &cFFFFFF00
         Bold            =   False
         Border          =   True
         CueText         =   "Username"
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Format          =   ""
         Height          =   22
         HelpTag         =   "Server Username"
         Index           =   -2147483648
         InitialParent   =   "grpMigrateServer"
         Italic          =   False
         Left            =   767
         LimitText       =   0
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   False
         LockRight       =   True
         LockTop         =   True
         Mask            =   ""
         Password        =   False
         ReadOnly        =   False
         Scope           =   0
         TabIndex        =   10
         TabPanelIndex   =   0
         TabStop         =   True
         Text            =   ""
         TextColor       =   &c00000000
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   244
         Transparent     =   False
         Underline       =   False
         UseFocusRing    =   True
         Visible         =   True
         Width           =   105
      End
      Begin TextField txtMigrateSrvPass
         AcceptTabs      =   False
         Alignment       =   0
         AutoDeactivate  =   True
         AutomaticallyCheckSpelling=   False
         BackColor       =   &cFFFFFF00
         Bold            =   False
         Border          =   True
         CueText         =   "Password"
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Format          =   ""
         Height          =   22
         HelpTag         =   "Server Password"
         Index           =   -2147483648
         InitialParent   =   "grpMigrateServer"
         Italic          =   False
         Left            =   884
         LimitText       =   0
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   False
         LockRight       =   True
         LockTop         =   True
         Mask            =   ""
         Password        =   False
         ReadOnly        =   False
         Scope           =   0
         TabIndex        =   11
         TabPanelIndex   =   0
         TabStop         =   True
         Text            =   ""
         TextColor       =   &c00000000
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   244
         Transparent     =   False
         Underline       =   False
         UseFocusRing    =   True
         Visible         =   True
         Width           =   111
      End
      Begin Label lblMigrateExample
         AutoDeactivate  =   True
         Bold            =   False
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Height          =   20
         HelpTag         =   ""
         Index           =   -2147483648
         InitialParent   =   "grpMigrateServer"
         Italic          =   False
         Left            =   767
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   False
         LockRight       =   True
         LockTop         =   True
         Multiline       =   False
         Scope           =   2
         Selectable      =   False
         TabIndex        =   4
         TabPanelIndex   =   0
         TabStop         =   True
         Text            =   "Example:"
         TextAlign       =   0
         TextColor       =   &cC0C0C000
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   301
         Transparent     =   False
         Underline       =   False
         Visible         =   True
         Width           =   213
      End
      Begin TextField txtMigrateFwFilename
         AcceptTabs      =   False
         Alignment       =   0
         AutoDeactivate  =   True
         AutomaticallyCheckSpelling=   False
         BackColor       =   &cFFFFFF00
         Bold            =   False
         Border          =   True
         CueText         =   "Firmware Filename Mask"
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Format          =   ""
         Height          =   22
         HelpTag         =   "Filename pattern"
         Index           =   -2147483648
         InitialParent   =   "grpMigrateServer"
         Italic          =   False
         Left            =   767
         LimitText       =   0
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   False
         LockRight       =   True
         LockTop         =   True
         Mask            =   ""
         Password        =   False
         ReadOnly        =   False
         Scope           =   0
         TabIndex        =   12
         TabPanelIndex   =   0
         TabStop         =   True
         Text            =   "%m_104.1.0.0.298.bl7"
         TextColor       =   &c00000000
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   278
         Transparent     =   False
         Underline       =   False
         UseFocusRing    =   True
         Visible         =   True
         Width           =   228
      End
      Begin PopupMenu popMigrateSrvMode
         AutoDeactivate  =   True
         Bold            =   False
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Height          =   22
         HelpTag         =   ""
         Index           =   -2147483648
         InitialParent   =   "grpMigrateServer"
         InitialValue    =   "FTP\r\nHTTP\r\nHTTPS\r\nTFTP"
         Italic          =   False
         Left            =   884
         ListIndex       =   0
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   True
         LockRight       =   False
         LockTop         =   True
         Scope           =   2
         TabIndex        =   7
         TabPanelIndex   =   0
         TabStop         =   True
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   176
         Transparent     =   False
         Underline       =   False
         Visible         =   True
         Width           =   111
      End
      Begin Label lblMigrateSrvMode
         AutoDeactivate  =   True
         Bold            =   False
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Height          =   20
         HelpTag         =   ""
         Index           =   -2147483648
         InitialParent   =   "grpMigrateServer"
         Italic          =   False
         Left            =   767
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   True
         LockRight       =   False
         LockTop         =   True
         Multiline       =   False
         Scope           =   2
         Selectable      =   False
         TabIndex        =   7
         TabPanelIndex   =   0
         TabStop         =   True
         Text            =   "Mode"
         TextAlign       =   2
         TextColor       =   &c00000000
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   177
         Transparent     =   False
         Underline       =   False
         Visible         =   True
         Width           =   105
      End
   End
   Begin GroupBox grpMigrateAPInfo
      AutoDeactivate  =   True
      Bold            =   False
      Caption         =   "AP CLI Details"
      Enabled         =   True
      Height          =   94
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   747
      LockBottom      =   False
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   True
      Scope           =   2
      TabIndex        =   3
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   20
      Transparent     =   False
      Underline       =   False
      Visible         =   True
      Width           =   261
      Begin TextField txtMigrateAPPass
         AcceptTabs      =   False
         Alignment       =   0
         AutoDeactivate  =   True
         AutomaticallyCheckSpelling=   False
         BackColor       =   &cFFFFFF00
         Bold            =   False
         Border          =   True
         CueText         =   "AP Password"
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Format          =   ""
         Height          =   22
         HelpTag         =   ""
         Index           =   -2147483648
         InitialParent   =   "grpMigrateAPInfo"
         Italic          =   False
         Left            =   884
         LimitText       =   0
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   False
         LockRight       =   True
         LockTop         =   True
         Mask            =   ""
         Password        =   False
         ReadOnly        =   False
         Scope           =   0
         TabIndex        =   4
         TabPanelIndex   =   0
         TabStop         =   True
         Text            =   ""
         TextColor       =   &c00000000
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   49
         Transparent     =   False
         Underline       =   False
         UseFocusRing    =   True
         Visible         =   True
         Width           =   111
      End
      Begin TextField txtMigrateAPUser
         AcceptTabs      =   False
         Alignment       =   0
         AutoDeactivate  =   True
         AutomaticallyCheckSpelling=   False
         BackColor       =   &cFFFFFF00
         Bold            =   False
         Border          =   True
         CueText         =   "AP Username"
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Format          =   ""
         Height          =   22
         HelpTag         =   ""
         Index           =   -2147483648
         InitialParent   =   "grpMigrateAPInfo"
         Italic          =   False
         Left            =   767
         LimitText       =   255
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   False
         LockRight       =   True
         LockTop         =   True
         Mask            =   ""
         Password        =   False
         ReadOnly        =   False
         Scope           =   0
         TabIndex        =   3
         TabPanelIndex   =   0
         TabStop         =   True
         Text            =   ""
         TextColor       =   &c00000000
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   49
         Transparent     =   False
         Underline       =   False
         UseFocusRing    =   True
         Visible         =   True
         Width           =   105
      End
      Begin CheckBox chkMigrateAlsoDefault
         AutoDeactivate  =   True
         Bold            =   False
         Caption         =   "Also try default (super/sp-admin)"
         DataField       =   ""
         DataSource      =   ""
         Enabled         =   True
         Height          =   20
         HelpTag         =   ""
         Index           =   -2147483648
         InitialParent   =   "grpMigrateAPInfo"
         Italic          =   False
         Left            =   767
         LockBottom      =   False
         LockedInPosition=   False
         LockLeft        =   False
         LockRight       =   True
         LockTop         =   True
         Scope           =   0
         State           =   1
         TabIndex        =   5
         TabPanelIndex   =   0
         TabStop         =   True
         TextFont        =   "System"
         TextSize        =   0.0
         TextUnit        =   0
         Top             =   83
         Transparent     =   False
         Underline       =   False
         Value           =   True
         Visible         =   True
         Width           =   228
      End
   End
   Begin ProgressBar progBar
      AutoDeactivate  =   True
      Enabled         =   True
      Height          =   21
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Left            =   878
      LockBottom      =   True
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   False
      Maximum         =   100
      Scope           =   0
      TabIndex        =   10
      TabPanelIndex   =   0
      Top             =   452
      Transparent     =   True
      Value           =   0
      Visible         =   False
      Width           =   130
   End
   Begin Label lblAPListCnt
      AutoDeactivate  =   True
      Bold            =   False
      DataField       =   ""
      DataSource      =   ""
      Enabled         =   True
      Height          =   20
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   798
      LockBottom      =   True
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   False
      Multiline       =   False
      Scope           =   0
      Selectable      =   False
      TabIndex        =   11
      TabPanelIndex   =   0
      TabStop         =   True
      Text            =   ""
      TextAlign       =   2
      TextColor       =   &c00000000
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   453
      Transparent     =   True
      Underline       =   False
      Visible         =   True
      Width           =   68
   End
   Begin Shell sh
      Arguments       =   ""
      Backend         =   ""
      Canonical       =   False
      ErrorCode       =   0
      Index           =   -2147483648
      IsRunning       =   False
      LockedInPosition=   False
      Mode            =   1
      PID             =   0
      Result          =   ""
      Scope           =   0
      TabPanelIndex   =   0
      TimeOut         =   1000
   End
   Begin CheckBox chkMigrateAlsoFactory
      AutoDeactivate  =   True
      Bold            =   False
      Caption         =   "Reset AP to factory defaults"
      DataField       =   ""
      DataSource      =   ""
      Enabled         =   True
      Height          =   20
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   755
      LockBottom      =   False
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   True
      Scope           =   0
      State           =   0
      TabIndex        =   13
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   340
      Transparent     =   False
      Underline       =   False
      Value           =   False
      Visible         =   True
      Width           =   228
   End
   Begin CheckBox chkMigrateAlsoReboot
      AutoDeactivate  =   True
      Bold            =   False
      Caption         =   "Reboot AP"
      DataField       =   ""
      DataSource      =   ""
      Enabled         =   True
      Height          =   20
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   755
      LockBottom      =   False
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   True
      Scope           =   0
      State           =   0
      TabIndex        =   16
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   420
      Transparent     =   False
      Underline       =   False
      Value           =   False
      Visible         =   True
      Width           =   228
   End
   Begin CheckBox chkMigrateFw
      AutoDeactivate  =   True
      Bold            =   False
      Caption         =   "Change Firmware"
      DataField       =   ""
      DataSource      =   ""
      Enabled         =   True
      Height          =   20
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   755
      LockBottom      =   False
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   True
      Scope           =   0
      State           =   0
      TabIndex        =   6
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   126
      Transparent     =   False
      Underline       =   False
      Value           =   False
      Visible         =   True
      Width           =   228
   End
   Begin CheckBox chkMigrateAlsoRun
      AutoDeactivate  =   True
      Bold            =   False
      Caption         =   "Run AP CLI Command"
      DataField       =   ""
      DataSource      =   ""
      Enabled         =   True
      Height          =   20
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   755
      LockBottom      =   False
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   True
      Scope           =   0
      State           =   0
      TabIndex        =   14
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   365
      Transparent     =   False
      Underline       =   False
      Value           =   False
      Visible         =   True
      Width           =   240
   End
   Begin TextField txtMigrateAlsoRun
      AcceptTabs      =   False
      Alignment       =   0
      AutoDeactivate  =   True
      AutomaticallyCheckSpelling=   False
      BackColor       =   &cFFFFFF00
      Bold            =   False
      Border          =   True
      CueText         =   "set scg ip..."
      DataField       =   ""
      DataSource      =   ""
      Enabled         =   False
      Format          =   ""
      Height          =   22
      HelpTag         =   "Execute CLI Command specified in the text field below.\r\nNote: Unleashed commands are not supported!"
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   767
      LimitText       =   0
      LockBottom      =   False
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   True
      Mask            =   ""
      Password        =   False
      ReadOnly        =   False
      Scope           =   0
      TabIndex        =   15
      TabPanelIndex   =   0
      TabStop         =   True
      Text            =   ""
      TextColor       =   &c00000000
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   390
      Transparent     =   False
      Underline       =   False
      UseFocusRing    =   True
      Visible         =   True
      Width           =   228
   End
   Begin PushButton btnMigrateExport
      AutoDeactivate  =   True
      Bold            =   False
      ButtonStyle     =   "0"
      Cancel          =   False
      Caption         =   "Export..."
      Default         =   False
      Enabled         =   False
      Height          =   22
      HelpTag         =   ""
      Index           =   -2147483648
      InitialParent   =   ""
      Italic          =   False
      Left            =   626
      LockBottom      =   True
      LockedInPosition=   False
      LockLeft        =   False
      LockRight       =   True
      LockTop         =   False
      Scope           =   2
      TabIndex        =   2
      TabPanelIndex   =   0
      TabStop         =   True
      TextFont        =   "System"
      TextSize        =   0.0
      TextUnit        =   0
      Top             =   453
      Transparent     =   False
      Underline       =   False
      Visible         =   True
      Width           =   109
   End
   Begin Thread thChangeFW
      Index           =   -2147483648
      LockedInPosition=   False
      Priority        =   5
      Scope           =   0
      StackSize       =   0
      TabPanelIndex   =   0
   End
End
#tag EndWindow

#tag WindowCode
	#tag Event
		Sub Open()
		  Dim ssh As New Chilkat.Ssh
		  
		  Me.Title = "Crossbreeder " + Str(App.MajorVersion) + "." + Str(App.MinorVersion) + "." + Str(App.BugVersion) + "." + Str(App.NonReleaseVersion) + " ("+ Str(App.BuildDate.AbbreviatedDate) + ")"
		  
		  //  If a license is purchased, replace "Anything for 30-day trial" with the purchased unlock code.
		  Dim success As Boolean
		  success = ssh.UnlockComponent("RUCKUS.CB1122019_WCdYB2dzkXmo ")
		  If (success <> True) Then
		    System.DebugLog(ssh.LastErrorText)
		    System.DebugLog("unlock failed.")
		    Return
		  End If
		  
		  //  If debugging, you can examine the LastErrorText even when a method is successful.
		  //  This allows one to see what transpired within the method call, especially
		  //  if the VerboseLogging property is turned on.
		  //  In this case, a programmer can examine the LastErrorText to see if success
		  //  was with a purchased (and recognized) unlock code, or if it was successful in trial mode.
		  
		  System.DebugLog(ssh.LastErrorText)
		  System.DebugLog("unlock successful.")
		  
		  dim CmdAll, Args(), ArgPair(1) as string
		  Args = split(system.commandLine, " ")
		  
		  For Each Arg As String In Args() 
		    If Arg.InStr("=") >0  Then
		      ArgPair = Split(Arg, "=")
		      
		      Select Case ArgPair(0)
		      Case "-ip","-ipaddr","-ipaddress"
		        txtDebug.AppendText("Found IP " + ArgPair(1) + " on command line... setting IP" + EndOfLine)
		        txtMigrateSrvIP.Text = ArgPair(1)
		      Case "-usr","-user"
		        txtDebug.AppendText("Found User " + ArgPair(1)+ " on command line... setting User" + EndOfLine)
		        txtMigrateSrvUser.Text = ArgPair(1)
		      Case "-pwd","-password"
		        txtDebug.AppendText("Found Password [hidden] on command line... setting Password" + EndOfLine)
		        txtMigrateSrvPass.Text = ArgPair(1)
		      Case "-prt","-port"
		        txtDebug.AppendText("Found Port " + ArgPair(1) + " on command line... setting Port" + EndOfLine)
		        txtMigrateSrvPort.Text = ArgPair(1)
		      Case "-debug"
		        if (ArgPair(1) = "yes" or ArgPair(1) = "y" or ArgPair(1) = "true") Then
		          Me.Height = Me.Height + 175
		          txtDebug.Visible = True
		        End if
		      End Select
		    End If
		    
		    
		  Next
		  
		End Sub
	#tag EndEvent


	#tag Method, Flags = &h0
		Function CSVExport(pList As ListBox, pTitle As String) As Boolean
		  'Generic Listbox Export Routine for Xojo
		  'Accepts pList as a ListBox Object and pType as a String
		  'Returns a Boolean Result
		  'Example Method Call:  dim booResult as Boolean = GenericExport(",",ListBox1)
		  'Developer: SJC
		  'Edited: March 15th 2016 @ 2300
		  'Note: mXBB_Globals.pEncoding is a Global Variable of Type TextEncoding with the Value UTF8
		  
		  
		  if pList.ListCount = 0 then Return False
		  Dim pType As String
		  dim aHeadings(-1) as String = pList.Heading(-1).Split(chr(9))
		  dim booResult as Boolean = False
		  dim intRow, intColumn as Integer = 0
		  dim strKey, strValue as String = ""
		  dim f as FolderItem
		  dim t as TextOutputStream
		  
		  Dim csvType As New FileType
		  csvType.Name = "CSV File (*.csv)"
		  csvType.MacType = "TEXT"
		  csvType.Extensions = "csv"
		  
		  Dim txtType As New FileType
		  txtType.Name = "Text File (*.txt)"
		  txtType.MacType = "TEXT"
		  txtType.Extensions = "txt"
		  
		  Dim jsonType As New FileType
		  jsonType.Name = "JSON File (*.json)"
		  jsonType.MacType = "TEXT"
		  jsonType.Extensions = "json"
		  
		  Dim dlg As New SaveAsDialog
		  dlg.SuggestedFileName = "export"
		  dlg.Title = pTitle
		  dlg.Filter = csvType + txtType + jsonType
		  f = dlg.ShowModal
		  If f <> Nil Then
		    MsgBox(f.Type)
		    If Right(f.Type, 4) = "csv)" Then
		      pType = ","
		    ElseIf Right(f.Type, 4) = "txt)" Then
		      pType = chr(9)
		    ElseIf Right(f.Type, 5) = "json)" Then
		      pType = "JSON"
		    Else
		      MsgBox("Wrong file extension entered.  Please select a file extension to determine file format.")
		      Return False
		    End If
		  Else
		    Return False
		  End If
		  
		  
		  dim strDelim as String = pType
		  t = f.CreateTextFile
		  'All Export Types Except JSON Need Column Headings First
		  if pType <> "JSON" then
		    for intColumn = 0 to aHeadings.Ubound
		      t.Write """" + aHeadings(intColumn) + """"
		      if intColumn < pList.ColumnCount then t.Write strDelim
		    next
		  end if
		  'Start Writing the Export File
		  if pType <> "JSON" then t.Write Chr(13)
		  if pType = "JSON" then t.Write "["
		  'Loop the Rows
		  for intRow = 0 to pList.ListCount-1
		    if pType = "JSON" and intRow <> 0 then t.Write ","
		    if pType = "JSON" then t.Write Chr(13) + "{"
		    'Loop the Columns
		    for intColumn = 0 to pList.ColumnCount-1
		      if pType = "JSON" then strKey = ""
		      if pType = "JSON" and aHeadings.Ubound >= intColumn then strKey = aHeadings(intColumn)
		      if pType = "JSON" and strKey = "" then strKey = "Key"
		      if pType = "JSON" then t.Write """" + strKey + """"
		      if pType = "JSON" then t.Write ":"
		      if pType = "JSON" then strValue = pList.Cell(intRow,intColumn)
		      if pType = "JSON" and len(strValue) = 1 and (asc(strValue) < 32) then strValue = ""
		      if pType = "JSON" then t.Write """" + strValue + """"
		      if pType = "JSON" and intColumn <> pList.ColumnCount - 1 then t.Write ","
		      if pType <> "JSON" then t.Write """"
		      if pType <> "JSON" then t.Write pList.Cell(intRow,intColumn)
		      if pType <> "JSON" then t.Write """"
		      if pType <> "JSON" and intColumn < pList.ColumnCount - 1 then t.Write strDelim
		    next
		    if pType = "JSON" then t.Write "}"
		    if pType <> "JSON" then t.Write Chr(13)
		  next
		  if pType = "JSON" then t.Write Chr(13) + "]"
		  
		  t.Close
		  booResult = True
		  Return booResult
		  
		  
		  Exception
		    MsgBox ("Couldn't export file")
		    Return False
		End Function
	#tag EndMethod

	#tag Method, Flags = &h0
		Function Ping(HostName as string, optional TimeOut as Integer = 3600) As Double
		  'PING by Christian Wheel
		  '
		  ' Inputs: Hostname, TimeOut (optional, defaults to 3600 ms)
		  '
		  '
		  ' Returns:
		  '
		  ' -1: Could not resolve hostname
		  '  -2: Ping timed out
		  ' >=0: Ping time in ms
		  '
		  ' Note that OS X may return a decimal, while Windows will always return a whole integer.
		  ' 
		  ' Max timeout on OS X is 3600 ms. 
		  '
		  
		  dim s as new shell, result as string
		  s.Mode=1 //ASynchronous mode
		  #If TargetWin32 then
		    s.execute "ping -n 1 -w "+TimeOut.ToText+" " + trim(Hostname)
		    Do 
		      App.DoEvents
		      s.Poll
		    Loop Until s.IsRunning = False
		    Result=lowercase(s.readall)
		    txtDebug.AppendText (Result + EndOfLine)
		    if instr(Result, "could not find host") > 0 then return -1
		    if instr(Result, "100% loss") > 0 then return -2
		    Result=mid(Result, instr(Result, "time=")+5)
		    Return Val(Result)
		  #Elseif TargetMacOS then
		    s.execute "ping -c 1 -t " + TimeOut.ToText+ " "+ Hostname
		    Do 
		      s.Poll
		      App.DoEvents
		    Loop Until s.IsRunning = False
		    Result=lowercase(s.readall)
		    txtDebug.AppendText (Result + EndOfLine)
		    if instr(Result, "cannot resolve") > 0 then return -1
		    if instr(Result, "100.0% packet loss") > 0 then return -2
		    Result=mid(Result, instr(Result, "time=")+5)
		    Return Val(Result)
		  #Endif
		End Function
	#tag EndMethod

	#tag Method, Flags = &h0
		Shared Function StrBetween(FullString as String, Prefix as String, Suffix as String) As String
		  Dim rg As New RegEx
		  Dim myMatch As RegExMatch
		  rg.SearchPattern = "(?<="+Prefix+")(.*)(?="+Suffix+")"
		  
		  myMatch = rg.Search(FullString)
		  If myMatch <> Nil Then
		    Return myMatch.SubExpressionString(0)
		  Else
		    Return ""
		  End If
		  Exception err As RegExException
		    MsgBox(err.Message)
		End Function
	#tag EndMethod

	#tag Method, Flags = &h0
		Function subChangeFW(HostName as string, Row as Integer, optional TimeOut as Integer = 5000) As String
		  
		  Dim ssh As New Chilkat.Ssh
		  Dim port As Int32
		  Dim success As Boolean
		  Dim strOutput,sshRx As String
		  Dim saPrompts As New Chilkat.StringArray
		  Dim APType,APModel,APFWVersion,APMAC As String
		  Dim APFWFilename,APFWULString As String
		  
		  APFWFilename = Replace(txtMigrateFwFilename.Text,"%M","R720")
		  
		  port = 22
		  ssh.ConnectTimeoutMs = TimeOut
		  ssh.IdleTimeoutMs = TimeOut
		  ssh.ReadTimeoutMs = TimeOut
		  
		  //  Open SSH connection 
		  success = ssh.Connect(HostName,port)
		  txtDebug.AppendText("Connected: " + HostName + "("+ Str(success) + ")" + EndOfLine)
		  
		  //  Authenticate using login/password:
		  success = ssh.AuthenticatePw(txtMigrateAPUser.Text,txtMigrateAPPass.Text)
		  
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
		    txtDebug.AppendText("SSH Failed to connect to "+ HostName + "." + EndOfLine)
		    Return "SSH Failed"
		  End If
		  
		  txtDebug.AppendText(ssh.GetReceivedText(channelNum,"utf-8") + EndOfLine)
		  
		  success = ssh.ChannelSendString(channelNum,txtMigrateAPUser.Text + EndOfLine.Unix,"utf-8")
		  success = ssh.ChannelReceiveUntilMatch(channelNum,"assword :","utf-8",FALSE)
		  txtDebug.AppendText(ssh.GetReceivedText(channelNum,"utf-8") + EndOfLine)
		  
		  success = ssh.ChannelSendString(channelNum,txtMigrateAPPass.Text + EndOfLine.Unix,"utf-8")
		  
		  success = saPrompts.Append("rkscli: ")   ' logged in to AP CLI
		  success = saPrompts.Append("Login incorrect")    ' wrong credentials
		  success = saPrompts.Append("> ")           ' logged in to Unleashed Configured AP
		  
		  success = ssh.ChannelReceiveUntilMatchN(channelNum,saPrompts,"utf-8",FALSE)
		  sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		  txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		  
		  If InStr(sshRx,"Login incorrect")>0 Then
		    success = False
		    If chkMigrateAlsoDefault.State = Checkbox.CheckedStates.Checked Then
		      txtDebug.AppendText("-Trying defaults.." + EndOfLine)
		      txtDebug.AppendText(ssh.GetReceivedText(channelNum,"utf-8") + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum, "super" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"assword :","utf-8",FALSE)
		      txtDebug.AppendText(ssh.GetReceivedText(channelNum,"utf-8") + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"sp-admin" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatchN(channelNum,saPrompts,"utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("-RX|" + sshRx + "||RXEnd-" + EndOfLine)
		      if (InStr(sshRx,"rkscli: ")>0 or InStr(sshRx,"> ")>0) Then 
		        success=True
		      Else
		        success=False
		      End If
		    End If
		  End if
		  If (success <> True) Then
		    txtDebug.AppendText("Login Failed!" + EndOfLine)
		    Return "Login Failed"
		  End If
		  
		  APType = ""
		  If inStr(sshRx,"rkscli: ")>0 Then APType = "zf"
		  If inStr(sshRx,"> ")>0 Then APType= "ul"
		  
		  Select Case APType
		  Case "zf"
		    txtDebug.AppendText("Starting ZoneFlex firmware process" + EndOfLine)
		    success = ssh.ChannelSendString(channelNum,"get version" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    
		    APModel = StrBetween(sshRx,"Ruckus "," Multimedia Hotzone Wireless AP")
		    APFWVersion = StrBetween(sshRx,"Version: ","")
		    
		    listmigrateAP.cell(Row,2) = APModel
		    listmigrateAP.cell(Row,3) = APFWVersion
		    APFWFilename = Replace(txtMigrateFwFilename.Text,"%M",APModel)
		    
		    success = ssh.ChannelSendString(channelNum,"get boarddata" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    
		    APMAC = StrBetween(sshRx, ", base ","")
		    listmigrateAP.cell(Row,1) = APMAC
		    
		    If chkMigrateAlsoFactory.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"set factory" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End if
		    
		    if chkMigrateFw.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"fw auto disable" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set proto " + srvProto + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set port " + txtMigrateSrvPort.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set control " + APFWFilename + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set host " + txtMigrateSrvIP.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set user " + txtMigrateSrvUser.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set password " + txtMigrateSrvPass.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw auto enable" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw update" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End If
		    
		    if chkMigrateAlsoRun.State = CheckBox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum, txtMigrateAlsoRun.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End If
		    
		    If chkMigrateAlsoReboot.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"reboot" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"rkscli: ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End if
		    
		  Case "ul"
		    txtDebug.AppendText("Starting Unleashed firmware process" + EndOfLine)
		    listmigrateAP.cell(Row,2) = "Unleashed"
		    
		    success = ssh.ChannelSendString(channelNum,"enable force" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"# ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    
		    success = ssh.ChannelSendString(channelNum,"show sysinfo" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"# ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    
		    txtDebug.AppendText(sshRx + EndOfLine)
		    
		    APModel = StrBetween (sshRx, "Model= ","")
		    APFWVersion = Replace(StrBetween(sshRx, "Version= ","")," Build ",".")
		    APMAC = Uppercase(StrBetween (sshRx, "MAC Address= ",""))
		    
		    listmigrateAP.cell(Row,1) = APMAC
		    listmigrateAP.cell(Row,2) = APModel
		    listmigrateAP.cell(Row,3) = APFWVersion
		    APFWFilename = Replace(txtMigrateFwFilename.Text,"%M",APModel)
		    
		    success = ssh.ChannelSendString(channelNum,"ap-mode" + EndOfLine.Unix,"utf-8")
		    success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		    sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		    txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    
		    If chkMigrateAlsoFactory.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"set factory" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End if
		    
		    if chkMigrateFw.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"fw auto disable" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set proto " + srvProto + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set port " + txtMigrateSrvPort.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set control " + APFWFilename + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set host " + txtMigrateSrvIP.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set user " + txtMigrateSrvUser.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw set password " + txtMigrateSrvPass.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw auto enable" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		      success = ssh.ChannelSendString(channelNum,"fw update" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		      
		    End If
		    
		    if chkMigrateAlsoRun.State = CheckBox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum, txtMigrateAlsoRun.Text + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End If
		    
		    If chkMigrateAlsoReboot.State = Checkbox.CheckedStates.Checked Then
		      success = ssh.ChannelSendString(channelNum,"reboot" + EndOfLine.Unix,"utf-8")
		      success = ssh.ChannelReceiveUntilMatch(channelNum,"(ap-mode)# ","utf-8",FALSE)
		      sshRx = ssh.GetReceivedText(channelNum,"utf-8")
		      txtDebug.AppendText("Sending Commands..." + EndOfLine + sshRx + EndOfLine)
		    End if
		    
		    
		  Else
		    txtDebug.AppendText("Unknown AP type! Skipping..." + EndOfLine)
		  End Select
		  
		  
		  If (ssh.LastMethodSuccess <> True) Then
		    txtDebug.AppendText(ssh.LastErrorText + EndOfLine)
		    Return "Error"
		  End If
		  
		  ssh.Disconnect
		  txtDebug.AppendText("disconnect" + EndOfLine)
		  
		  Return "Done"
		  
		End Function
	#tag EndMethod


	#tag Property, Flags = &h0
		srvProto As String = "ftp"
	#tag EndProperty


#tag EndWindowCode

#tag Events lblLoadCSVPrompt
	#tag Event
		Function MouseDown(X As Integer, Y As Integer) As Boolean
		  Return True
		End Function
	#tag EndEvent
	#tag Event
		Sub MouseUp(X As Integer, Y As Integer)
		  // get a reference to the folderitem (which may or may not exist)
		  dim f as folderitem 
		  
		  Dim csvType As New FileType
		  csvType.Name = "CSV File (*.csv)"
		  csvType.MacType = "TEXT"
		  csvType.Extensions = "csv"
		  
		  f = GetSaveFolderItem(csvType, "Crossbreeder-template" + ".csv")
		  
		  // If user hits cancel
		  if f <> Nil then
		    // try and create the file at the location the folderitem refers to
		    dim tos as TextOutputStream = TextOutputStream.Create(f) 
		    
		    // write my text into the new text output stream
		    tos.write "IP"+EndOfLine+"10.1.1.1"+EndOfLine+"10.1.1.2"+EndOfLine+"10.5.5.5"+EndOfLine+"192.168.5.10"+EndOfLine
		    tos = nil
		  end if
		  
		  // and nil things so they get flushed etc
		  f = nil
		End Sub
	#tag EndEvent
#tag EndEvents
#tag Events listmigrateAP
	#tag Event
		Function KeyDown(Key As String) As Boolean
		  dim a As integer 
		  a=asc(key) 
		  
		  if a=8 or a=127 then  
		    for i as integer=listmigrateAP.listcount-1 downto 0
		      if listmigrateAP.selected(i) then
		        listmigrateAP.removeRow(i)
		      end if
		    next
		    lblAPListCnt.Text = Str(listmigrateAP.ListCount)+" APs"
		  end if 
		  
		  
		  
		End Function
	#tag EndEvent
	#tag Event
		Function CellBackgroundPaint(g As Graphics, row As Integer, column As Integer) As Boolean
		  If row Mod 2 = 0 Then
		    g.ForeColor = RGB(244,244,244)
		  Else
		    g.ForeColor = RGB(252,252,252)
		  End If
		  g.FillRect 0,0,g.Width,g.Height
		  
		End Function
	#tag EndEvent
#tag EndEvents
#tag Events btnMigrateCSVImport
	#tag Event
		Sub Action()
		  Dim csvType As New FileType
		  csvType.Name = "CSV File (*.csv)"
		  csvType.MacType = "TEXT"
		  csvType.Extensions = "csv"
		  
		  Dim dlg As OpenDialog
		  Dim f As FolderItem
		  dlg = New OpenDialog
		  
		  dlg.Title = "Select a CSV file"
		  dlg.Filter = csvType
		  f = dlg.ShowModal
		  
		  Dim tFile, arrFile(), arrLine() As String
		  Dim cntEntry As Integer = 0
		  
		  If f <> Nil Then
		    If f.Exists Then
		      // Be aware that TextInputStream.Open coud raise an exception
		      Dim t As TextInputStream
		      Try
		        t = TextInputStream.Open(f)
		        t.Encoding =  Encodings.UTF8
		        tFile = t.ReadAll
		        tFile = ReplaceLineEndings(tFile, EndOfLine) ' Handle different types of EOL encodings across platforms
		        arrFile = tFile.Split(EndOfLine)
		        listmigrateAP.DeleteAllRows
		        Dim rg As New RegEx
		        Dim myMatch As RegExMatch
		        rg.SearchPattern = "\b(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b""?$" 'RegEx for IP Address
		        
		        For i As Integer = 0 To arrFile.UBound
		          ' Add to a table or list
		          If Trim(arrFile(i)) <> "" Then
		            arrLine = arrFile(i).Split(",")
		            myMatch = rg.Search(arrLine(0)) 'check line for IP Address, otherwise skip
		            If myMatch <> Nil Then
		              listmigrateAP.AddRow(arrLine(0))
		              listmigrateAP.Cell(listmigrateAP.LastIndex,1) = "" ' Remove any imported text from the Result column
		              cntEntry = cntEntry + 1
		              
		              For j As Integer = 0 to listmigrateAP.ColumnCount    ' Remove leading and trailing quotes
		                If Left(listmigrateAP.Cell(listmigrateAP.LastIndex, j),1) = """" Then listmigrateAP.Cell(listmigrateAP.LastIndex,j) = Right(listmigrateAP.Cell(listmigrateAP.LastIndex,j),Len(listmigrateAP.Cell(listmigrateAP.LastIndex,j))-1)
		                If Right(listmigrateAP.Cell(listmigrateAP.LastIndex, j),1) = """" Then listmigrateAP.Cell(listmigrateAP.LastIndex,j) = Left(listmigrateAP.Cell(listmigrateAP.LastIndex,j),Len(listmigrateAP.Cell(listmigrateAP.LastIndex,j))-1)
		              Next
		              
		            End If
		          End If
		        Next
		        lblorideCSVName.Text = f.Name
		        lblLoadCSVPrompt.Enabled = False
		        lblAPListCnt.Text = Str(cntEntry)+" APs"
		        listmigrateAP.Visible=True
		        btnMigrateGo.Enabled=True
		        
		        btnMigrateExport.Enabled=True
		        btnMigrateGo.SetFocus
		      Catch e As IOException
		        t.Close
		        MsgBox("Error accessing file.")
		      End Try
		    End If
		  End If
		  
		  
		End Sub
	#tag EndEvent
#tag EndEvents
#tag Events btnMigrateGO
	#tag Event
		Sub Action()
		  Dim apIP As String
		  Me.Enabled = False
		  
		  If chkMigrateFw.State = Checkbox.CheckedStates.UnChecked or (len(txtMigrateSrvIP.Text) > 0 and len(txtMigrateSrvPort.Text) > 0 and len(txtMigrateFwFilename.Text) > 0) Then  
		    If listMigrateAP.ListCount > 0 Then
		      progBar.Value = 0
		      progBar.Maximum = listmigrateAP.listcount
		      progBar.Visible = True
		      
		      for i as integer=0 to listmigrateAP.ListCount-1
		        listmigrateAP.cell(i,5) = ""
		      Next
		      
		      if len(txtMigrateAPUser.Text) = 0 and len(txtMigrateAPPass.Text) = 0 Then
		        If chkMigrateAlsoDefault.State = Checkbox.CheckedStates.Checked Then
		          txtMigrateAPUser.Text = "super"
		          txtMigrateAPPass.Text = "sp-admin"
		        Else
		          MsgBox ("No AP credentials defined.  Please set AP credentials or enable the ""Also try default"" option.")
		        End If
		      End If
		      
		      if len(txtMigrateAPUser.Text) > 0 Then
		        for i as integer=0 to listmigrateAP.listcount-1 //this loops through all of the rows
		          listmigrateAP.cell(i,4) = "Pinging..."
		          apIP = listmigrateAP.cell(i,0)
		          listmigrateAP.cell(i,4) = str(ping(apIP,1000))
		          
		          Select Case val(listmigrateAP.cell(i,4))
		          Case Is >=0  
		            txtDebug.AppendText("IP is responding.  Trying SSH..." + EndOfLine)
		            listmigrateAP.cell(i,5) = "Running..."
		            App.DoEvents
		            listmigrateAP.Refresh
		            listmigrateAP.cell(i,5) = str(ChangeFW.Run(apIP,i))
		          Case -2
		            listmigrateAP.cell(i,4) = "Timeout"
		            listmigrateAP.cell(i,5) = "Skipped"
		          Case -1
		            listmigrateAP.cell(i,4) = "Invalid Host"
		            listmigrateAP.cell(i,5) = "Skipped"
		          End Select
		          ProgBar.Value = i
		        next
		      End If
		      progBar.Visible = False
		      txtdebug.AppendText("All done!" + EndOfLine + "--" + EndOfLine)
		    Else
		      MsgBox("No APs to configure  Please select a CSV file.")
		      txtdebug.AppendText("No APs to configure  Please select a CSV file." + EndOfLine + "--" + EndOfLine)
		    End If
		  else
		    MsgBox("Server details not set. Please set firmware server details before continuing.")
		  End If
		  
		  Me.Enabled = True
		End Sub
	#tag EndEvent
#tag EndEvents
#tag Events txtDebug
	#tag Event
		Sub TextChange()
		  txtDebug.ScrollPosition = 9999
		End Sub
	#tag EndEvent
#tag EndEvents
#tag Events txtMigrateFwFilename
	#tag Event
		Sub TextChange()
		  lblMigrateExample.Text = Replace(txtMigrateFwFilename.Text,"%M","R720")
		End Sub
	#tag EndEvent
#tag EndEvents
#tag Events popMigrateSrvMode
	#tag Event
		Sub Change()
		  Select Case me.getString
		  Case "FTP"
		    txtMigrateSrvPass.Enabled = True
		    txtMigrateSrvUser.Enabled = True
		    txtMigrateSrvPort.Enabled = True
		    txtMigrateSrvPort.Text = "21"
		    srvProto = "ftp"
		  Case "TFTP"
		    txtMigrateSrvPass.Enabled = False
		    txtMigrateSrvUser.Enabled = False
		    txtMigrateSrvPort.Enabled = False
		    txtMigrateSrvPort.Text = "69"
		    srvProto = "tftp"
		  Case "HTTP"
		    txtMigrateSrvPass.Enabled = False
		    txtMigrateSrvUser.Enabled = False
		    txtMigrateSrvPort.Enabled = True
		    txtMigrateSrvPort.Text = "80"
		    srvProto = "http"
		  Case "HTTPS"
		    txtMigrateSrvPass.Enabled = False
		    txtMigrateSrvUser.Enabled = False
		    txtMigrateSrvPort.Enabled = True
		    txtMigrateSrvPort.Text = "443"
		    srvProto = "https"
		  End Select
		  
		End Sub
	#tag EndEvent
#tag EndEvents
#tag Events chkMigrateFw
	#tag Event
		Sub Action()
		  If chkMigrateFw.State = Checkbox.CheckedStates.Checked Then
		    grpMigrateServer.Enabled = True
		  ElseIf chkMigrateFw.State = Checkbox.CheckedStates.Unchecked Then
		    grpMigrateServer.Enabled = False
		  End If
		End Sub
	#tag EndEvent
#tag EndEvents
#tag Events chkMigrateAlsoRun
	#tag Event
		Sub Action()
		  If chkMigrateAlsoRun.State = Checkbox.CheckedStates.Checked Then
		    txtMigrateAlsoRun.Enabled = True
		  ElseIf chkMigrateAlsoRun.State = Checkbox.CheckedStates.Unchecked Then
		    txtMigrateAlsoRun.Enabled = False
		  End If
		End Sub
	#tag EndEvent
#tag EndEvents
#tag Events btnMigrateExport
	#tag Event
		Sub Action()
		  Dim Result as Boolean
		  Result = CSVExport(listMigrateAP,"Export results...")
		  
		  
		End Sub
	#tag EndEvent
#tag EndEvents
#tag Events thChangeFW
	#tag Event
		Sub Run()
		  
		End Sub
	#tag EndEvent
#tag EndEvents
#tag ViewBehavior
	#tag ViewProperty
		Name="Name"
		Visible=true
		Group="ID"
		Type="String"
		EditorType="String"
	#tag EndViewProperty
	#tag ViewProperty
		Name="Interfaces"
		Visible=true
		Group="ID"
		Type="String"
		EditorType="String"
	#tag EndViewProperty
	#tag ViewProperty
		Name="Super"
		Visible=true
		Group="ID"
		Type="String"
		EditorType="String"
	#tag EndViewProperty
	#tag ViewProperty
		Name="Width"
		Visible=true
		Group="Size"
		InitialValue="600"
		Type="Integer"
	#tag EndViewProperty
	#tag ViewProperty
		Name="Height"
		Visible=true
		Group="Size"
		InitialValue="400"
		Type="Integer"
	#tag EndViewProperty
	#tag ViewProperty
		Name="MinWidth"
		Visible=true
		Group="Size"
		InitialValue="64"
		Type="Integer"
	#tag EndViewProperty
	#tag ViewProperty
		Name="MinHeight"
		Visible=true
		Group="Size"
		InitialValue="64"
		Type="Integer"
	#tag EndViewProperty
	#tag ViewProperty
		Name="MaxWidth"
		Visible=true
		Group="Size"
		InitialValue="32000"
		Type="Integer"
	#tag EndViewProperty
	#tag ViewProperty
		Name="MaxHeight"
		Visible=true
		Group="Size"
		InitialValue="32000"
		Type="Integer"
	#tag EndViewProperty
	#tag ViewProperty
		Name="Frame"
		Visible=true
		Group="Frame"
		InitialValue="0"
		Type="Integer"
		EditorType="Enum"
		#tag EnumValues
			"0 - Document"
			"1 - Movable Modal"
			"2 - Modal Dialog"
			"3 - Floating Window"
			"4 - Plain Box"
			"5 - Shadowed Box"
			"6 - Rounded Window"
			"7 - Global Floating Window"
			"8 - Sheet Window"
			"9 - Metal Window"
			"11 - Modeless Dialog"
		#tag EndEnumValues
	#tag EndViewProperty
	#tag ViewProperty
		Name="Title"
		Visible=true
		Group="Frame"
		InitialValue="Untitled"
		Type="String"
	#tag EndViewProperty
	#tag ViewProperty
		Name="CloseButton"
		Visible=true
		Group="Frame"
		InitialValue="True"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="Resizeable"
		Visible=true
		Group="Frame"
		InitialValue="True"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="MaximizeButton"
		Visible=true
		Group="Frame"
		InitialValue="True"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="MinimizeButton"
		Visible=true
		Group="Frame"
		InitialValue="True"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="FullScreenButton"
		Visible=true
		Group="Frame"
		InitialValue="False"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="Composite"
		Group="OS X (Carbon)"
		InitialValue="False"
		Type="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="MacProcID"
		Group="OS X (Carbon)"
		InitialValue="0"
		Type="Integer"
	#tag EndViewProperty
	#tag ViewProperty
		Name="ImplicitInstance"
		Visible=true
		Group="Behavior"
		InitialValue="True"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="Placement"
		Visible=true
		Group="Behavior"
		InitialValue="0"
		Type="Integer"
		EditorType="Enum"
		#tag EnumValues
			"0 - Default"
			"1 - Parent Window"
			"2 - Main Screen"
			"3 - Parent Window Screen"
			"4 - Stagger"
		#tag EndEnumValues
	#tag EndViewProperty
	#tag ViewProperty
		Name="Visible"
		Visible=true
		Group="Behavior"
		InitialValue="True"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="LiveResize"
		Group="Behavior"
		InitialValue="True"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="FullScreen"
		Group="Behavior"
		InitialValue="False"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="HasBackColor"
		Visible=true
		Group="Background"
		InitialValue="False"
		Type="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="BackColor"
		Visible=true
		Group="Background"
		InitialValue="&hFFFFFF"
		Type="Color"
	#tag EndViewProperty
	#tag ViewProperty
		Name="Backdrop"
		Visible=true
		Group="Background"
		Type="Picture"
		EditorType="Picture"
	#tag EndViewProperty
	#tag ViewProperty
		Name="MenuBar"
		Visible=true
		Group="Menus"
		Type="MenuBar"
		EditorType="MenuBar"
	#tag EndViewProperty
	#tag ViewProperty
		Name="MenuBarVisible"
		Visible=true
		Group="Deprecated"
		InitialValue="True"
		Type="Boolean"
		EditorType="Boolean"
	#tag EndViewProperty
	#tag ViewProperty
		Name="srvProto"
		Group="Behavior"
		InitialValue="ftp"
		Type="String"
		EditorType="MultiLineEditor"
	#tag EndViewProperty
#tag EndViewBehavior
