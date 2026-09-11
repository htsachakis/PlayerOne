Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####

####
## "highest", not "admin", because the installation is not always machine-wide.
##
## PlayerOne can be installed for everyone on the computer or for the person
## running the installer alone, and that is chosen on a page rather than baked
## into the manifest. "admin" would demand an administrator even for the
## for-me-only install, which needs nothing of the sort; worse, on a standard
## account it would ask for somebody else's password and then install into
## *their* profile. "highest" gives an administrator the full token they need
## for Program Files while letting a standard user carry on unelevated and
## install into their own AppData.
##
## The consequence is that the scope is only known at runtime, so the wails
## macros that decide it at compile time - wails.setShellContext, and the
## uninstaller registration - are not used. Their work is done below against
## SHELL_CONTEXT, which follows SetShellVarContext.
####
!ifndef REQUEST_EXECUTION_LEVEL
    !define REQUEST_EXECUTION_LEVEL "highest"
!endif

####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"
!include "LogicLib.nsh"
!include "nsDialogs.nsh"
!include "WinMessages.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps

# Offer to start PlayerOne when the installer finishes, ticked by default. This
# is what completes the in-app update: PlayerOne quits so its files can be
# replaced, and comes back on its own rather than leaving the viewer staring at
# a closed application.
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Start ${INFO_PRODUCTNAME}"

!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# The licence is shown before installing: the package bundles GPL binaries
# alongside MIT-licensed code, and the file explains how the two relate.
!insertmacro MUI_PAGE_LICENSE "..\..\..\LICENSE"
Page custom InstallScopePageCreate InstallScopePageLeave # Everyone, or me only.
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" # Replaced in .onInit, once the scope is known.
ShowInstDetails show # This will always show the installation details.

####
## Who the installation is for.
##
## $InstallScope is "all" or "user", and every path in this script hangs off it:
## SetShellVarContext points $SMPROGRAMS, $DESKTOP and the SHELL_CONTEXT
## registry root at the machine or at the profile, so one body of code writes
## either a machine-wide or a private installation without repeating itself.
####
Var InstallScope       ; "all" or "user"
Var ExistingScope      ; the scope of an installation already present, or ""
Var HasAdminRights     ; 1 when this process may write outside the profile
Var ScopeAllRadio
Var ScopeUserRadio

; Points the shell variables and SHELL_CONTEXT at the chosen scope, and
; proposes the folder that goes with it. An installation already present at
; that scope wins: an update should land on top of the existing copy rather
; than beside it.
Function ApplyInstallScope
    ${If} $InstallScope == "all"
        SetShellVarContext all
    ${Else}
        SetShellVarContext current
    ${EndIf}

    ReadRegStr $0 SHELL_CONTEXT "${UNINST_KEY}" "InstallLocation"
    ${If} $0 != ""
        StrCpy $INSTDIR $0
    ${ElseIf} $InstallScope == "all"
        StrCpy $INSTDIR "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
    ${Else}
        StrCpy $INSTDIR "$LOCALAPPDATA\Programs\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
    ${EndIf}
FunctionEnd

Function .onInit
    !insertmacro wails.checkArchitecture

    ; The 64-bit view for everything that follows. A 32-bit installer writing
    ; Software\Classes lands in Wow6432Node otherwise, where the shell does
    ; not look for "Open with" entries.
    SetRegView 64

    UserInfo::GetAccountType
    Pop $0
    ${If} $0 == "Admin"
        StrCpy $HasAdminRights 1
    ${Else}
        StrCpy $HasAdminRights 0
    ${EndIf}

    ; An installation already on the machine decides the default, so an update
    ; repeats the choice made the first time instead of quietly moving house.
    StrCpy $ExistingScope ""
    ReadRegStr $0 HKLM "${UNINST_KEY}" "UninstallString"
    ${If} $0 != ""
        StrCpy $ExistingScope "all"
    ${Else}
        ReadRegStr $0 HKCU "${UNINST_KEY}" "UninstallString"
        ${If} $0 != ""
            StrCpy $ExistingScope "user"
        ${EndIf}
    ${EndIf}

    ${If} $ExistingScope != ""
        StrCpy $InstallScope $ExistingScope
    ${ElseIf} $HasAdminRights == 1
        StrCpy $InstallScope "all"
    ${Else}
        StrCpy $InstallScope "user"
    ${EndIf}

    ; /ALLUSERS and /CURRENTUSER settle the scope without the page, which is
    ; the only way to say it for a silent (/S) install.
    ${GetParameters} $R0
    ClearErrors
    ${GetOptions} $R0 "/ALLUSERS" $R1
    ${IfNot} ${Errors}
        StrCpy $InstallScope "all"
    ${EndIf}
    ClearErrors
    ${GetOptions} $R0 "/CURRENTUSER" $R1
    ${IfNot} ${Errors}
        StrCpy $InstallScope "user"
    ${EndIf}
    ClearErrors

    ; A machine-wide installation from an unelevated process would fail
    ; halfway through and leave half an application behind.
    ${If} $InstallScope == "all"
    ${AndIf} $HasAdminRights == 0
        ${If} ${Silent}
            SetErrorLevel 66
            Abort
        ${EndIf}
        StrCpy $InstallScope "user"
    ${EndIf}

    Call ApplyInstallScope
FunctionEnd

Function InstallScopePageCreate
    !insertmacro MUI_HEADER_TEXT "Choose who to install for" "${INFO_PRODUCTNAME} can be installed for everyone who uses this computer, or for you alone."

    nsDialogs::Create 1018
    Pop $R0
    ${If} $R0 == error
        Abort
    ${EndIf}

    UserInfo::GetName
    Pop $R1

    ${NSD_CreateRadioButton} 0u 0u 100% 12u "Install for &anyone who uses this computer"
    Pop $ScopeAllRadio
    ${NSD_CreateLabel} 12u 14u 92% 26u "Installs into Program Files. The shortcuts and the Open with entry appear for every account. Needs administrator rights."
    Pop $R2

    ${NSD_CreateRadioButton} 0u 46u 100% 12u "Install for &me only ($R1)"
    Pop $ScopeUserRadio
    ${NSD_CreateLabel} 12u 60u 92% 26u "Installs into your own AppData folder. Nothing outside your account is changed, and no administrator rights are needed."
    Pop $R2

    ; A button that cannot work would only produce a failed installation.
    ${If} $HasAdminRights == 0
        EnableWindow $ScopeAllRadio 0
        ${NSD_CreateLabel} 0u 94u 100% 26u "Installing for everyone is unavailable because this installer is not running as an administrator. Start it from an administrator account to enable it."
        Pop $R2
    ${ElseIf} $ExistingScope != ""
        ${NSD_CreateLabel} 0u 94u 100% 26u "${INFO_PRODUCTNAME} is already installed on this computer. Leaving the choice as it is will update that copy in place."
        Pop $R2
    ${EndIf}

    ${If} $InstallScope == "all"
        ${NSD_Check} $ScopeAllRadio
    ${Else}
        ${NSD_Check} $ScopeUserRadio
    ${EndIf}

    nsDialogs::Show
FunctionEnd

Function InstallScopePageLeave
    ${NSD_GetState} $ScopeAllRadio $R0
    ${If} $R0 == ${BST_CHECKED}
        StrCpy $InstallScope "all"
    ${Else}
        StrCpy $InstallScope "user"
    ${EndIf}

    ; Switching scope does not remove the copy that is already there, so say
    ; so plainly rather than leaving two PlayerOnes and no explanation.
    ${If} $ExistingScope != ""
    ${AndIf} $ExistingScope != $InstallScope
        ${If} $ExistingScope == "all"
            StrCpy $R1 "${INFO_PRODUCTNAME} is already installed for all users. Installing it for you alone adds a second copy; the existing one stays until it is uninstalled."
        ${Else}
            StrCpy $R1 "${INFO_PRODUCTNAME} is already installed for your account only. Installing it for all users adds a second copy; the existing one stays until it is uninstalled."
        ${EndIf}
        MessageBox MB_YESNO|MB_ICONEXCLAMATION "$R1$\n$\nContinue?" IDYES scopeAccepted
        Abort ; Back to the page, with the choice still there to change.
        scopeAccepted:
    ${EndIf}

    Call ApplyInstallScope
FunctionEnd

####
## Registering PlayerOne as a player Windows knows about.
##
## The goal is for PlayerOne to appear under "Open with" for media files, and in
## Settings > Default apps so it can be chosen deliberately. It must NOT take
## over any extension by itself: whatever already opens .mkv was the user's
## choice, and an installer that silently overrules it is a bad neighbour. That
## is why Wails' own wails.associateFiles is not used here - it writes the
## default handler for each extension.
##
## Two registrations do the polite version:
##   * Applications\PlayerOne.exe with SupportedTypes  -> "Choose another app"
##   * .ext\OpenWithProgIds\<ProgID>                   -> the "Open with" list
## Neither touches the default.
##
## Every key goes under SHELL_CONTEXT, which is HKLM for a machine-wide
## installation and HKCU for a private one. The shell merges the two, so a
## for-me-only copy gets the same entries within that one account.
####

!define PLAYERONE_PROGID "PlayerOne.Media"

; Adds one extension to the Open-with list, leaving its default alone.
!macro PLAYERONE_REGISTER_EXT EXT
  WriteRegStr SHELL_CONTEXT "Software\Classes\${EXT}\OpenWithProgIds" "${PLAYERONE_PROGID}" ""
  WriteRegStr SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}\SupportedTypes" "${EXT}" ""
  WriteRegStr SHELL_CONTEXT "Software\${INFO_PRODUCTNAME}\Capabilities\FileAssociations" "${EXT}" "${PLAYERONE_PROGID}"
!macroend

!macro PLAYERONE_UNREGISTER_EXT EXT
  DeleteRegValue SHELL_CONTEXT "Software\Classes\${EXT}\OpenWithProgIds" "${PLAYERONE_PROGID}"
!macroend

; Mirrors mediaExtensions in app.go. Add to one and add to the other, or the
; installer will offer to open something the application then refuses.
!macro PLAYERONE_EACH_EXT MACRO
  ; Video
  !insertmacro ${MACRO} ".mkv"
  !insertmacro ${MACRO} ".mp4"
  !insertmacro ${MACRO} ".webm"
  !insertmacro ${MACRO} ".mov"
  !insertmacro ${MACRO} ".avi"
  !insertmacro ${MACRO} ".m4v"
  !insertmacro ${MACRO} ".mpg"
  !insertmacro ${MACRO} ".mpeg"
  !insertmacro ${MACRO} ".m2v"
  !insertmacro ${MACRO} ".ts"
  !insertmacro ${MACRO} ".m2ts"
  !insertmacro ${MACRO} ".mts"
  !insertmacro ${MACRO} ".wmv"
  !insertmacro ${MACRO} ".asf"
  !insertmacro ${MACRO} ".flv"
  !insertmacro ${MACRO} ".f4v"
  !insertmacro ${MACRO} ".ogv"
  !insertmacro ${MACRO} ".3gp"
  !insertmacro ${MACRO} ".3g2"
  !insertmacro ${MACRO} ".vob"
  !insertmacro ${MACRO} ".divx"
  !insertmacro ${MACRO} ".rmvb"
  !insertmacro ${MACRO} ".mxf"

  ; Audio
  !insertmacro ${MACRO} ".mp3"
  !insertmacro ${MACRO} ".m4a"
  !insertmacro ${MACRO} ".m4b"
  !insertmacro ${MACRO} ".flac"
  !insertmacro ${MACRO} ".opus"
  !insertmacro ${MACRO} ".wav"
  !insertmacro ${MACRO} ".aac"
  !insertmacro ${MACRO} ".ogg"
  !insertmacro ${MACRO} ".oga"
  !insertmacro ${MACRO} ".wma"
  !insertmacro ${MACRO} ".mka"
  !insertmacro ${MACRO} ".ape"
  !insertmacro ${MACRO} ".aiff"
!macroend

####
## PlayerOne has to be closed before its files can be touched.
##
## Windows refuses to overwrite or delete the executable of a running process,
## so installing over an open PlayerOne used to fail partway through with a
## file-in-use error and leave a half-written folder behind. The viewer is
## asked to close it instead, and offered the door: close it yourself and
## retry, have the installer close it, or stop and change nothing.
##
## What is tested is the file, not the process list. "Can these files be
## replaced?" is the question that actually matters, and opening the
## executable for writing answers it exactly - including for a copy running
## under another account, which no scan of this session's processes would see.
####

Var AppLocked  ; 1 when the installed files cannot be written to
Var LockTarget ; the file TestOneFileLock looks at
Var WaitTicks  ; half-second ticks WaitForAppLock is allowed to spend

!macro PLAYERONE_CLOSE_FUNCTIONS UN VERB

Function ${UN}TestOneFileLock
    IfFileExists "$LockTarget" 0 clear
    ClearErrors
    FileOpen $R8 "$LockTarget" a
    IfErrors locked
    FileClose $R8
    Goto clear
  locked:
    StrCpy $AppLocked 1
  clear:
FunctionEnd

Function ${UN}TestAppLock
    StrCpy $AppLocked 0

    StrCpy $LockTarget "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Call ${UN}TestOneFileLock
    StrCmp $AppLocked 1 done

    ; mpv is PlayerOne's child and goes when it goes, though not always in the
    ; same instant. Without this the copy can still trip over an engine that
    ; has not quite finished exiting.
    StrCpy $LockTarget "$INSTDIR\bin\mpv.exe"
    Call ${UN}TestOneFileLock

  done:
FunctionEnd

; Polls until the files are free or $WaitTicks half-seconds have gone by,
; whichever comes first. $AppLocked is left holding the answer.
Function ${UN}WaitForAppLock
  poll:
    Call ${UN}TestAppLock
    StrCmp $AppLocked 0 done
    IntCmp $WaitTicks 0 done done
    Sleep 500
    IntOp $WaitTicks $WaitTicks - 1
    Goto poll
  done:
FunctionEnd

; Asks PlayerOne to close the way the window's X does, so that settings and
; the resume position are written before it goes. taskkill without /F posts
; WM_CLOSE and waits for the application to agree.
;
; By image name, because the lock says a PlayerOne is holding the files but
; not which one. A portable copy open at the same time is closed too - the
; smaller harm, and only ever after the viewer has asked for this.
Function ${UN}RequestAppClose
    DetailPrint "Closing ${INFO_PRODUCTNAME}"
    nsExec::Exec 'taskkill /IM "${PRODUCT_EXECUTABLE}"'
    Pop $R7
FunctionEnd

Function ${UN}CloseRunningApp
    ; Five seconds of patience before saying anything. The in-app update quits
    ; PlayerOne the moment it hands over to the installer, so the lock is
    ; usually a second from clearing on its own, and a dialog about a problem
    ; that is already solving itself is just noise.
    StrCpy $WaitTicks 10
    Call ${UN}WaitForAppLock
    StrCmp $AppLocked 0 done

    ; Nobody to ask: request a clean shutdown, and stop rather than force
    ; anything with no one watching.
    ${If} ${Silent}
        Call ${UN}RequestAppClose
        StrCpy $WaitTicks 20
        Call ${UN}WaitForAppLock
        StrCmp $AppLocked 0 done
        SetErrorLevel 67
        Abort
    ${EndIf}

  ask:
    MessageBox MB_ABORTRETRYIGNORE|MB_ICONEXCLAMATION \
        "${INFO_PRODUCTNAME} is running, and Windows will not let a running application's files be ${VERB}.$\n$\n\
Retry - close ${INFO_PRODUCTNAME} yourself, then click this.$\n\
Ignore - close it for me now. Playback stops; your place in it is saved first.$\n\
Abort - stop here and change nothing." \
        IDRETRY retry IDIGNORE force
    Abort "${INFO_PRODUCTNAME} is still running." ; IDABORT falls through to here

  retry:
    StrCpy $WaitTicks 4 ; a moment for the window to finish going away
    Call ${UN}WaitForAppLock
    StrCmp $AppLocked 0 done
    Goto ask

  force:
    Call ${UN}RequestAppClose
    StrCpy $WaitTicks 20
    Call ${UN}WaitForAppLock
    StrCmp $AppLocked 0 done

    ; It would not go quietly - a modal dialog of its own, or a copy in
    ; another session that never receives the message. Pull it down. The
    ; resume position is written every ten seconds anyway, so what is lost is
    ; at most a few seconds of progress.
    DetailPrint "Forcing ${INFO_PRODUCTNAME} to close"
    nsExec::Exec 'taskkill /F /IM "${PRODUCT_EXECUTABLE}"'
    Pop $R7
    StrCpy $WaitTicks 10
    Call ${UN}WaitForAppLock
    StrCmp $AppLocked 0 done
    Goto ask ; still held by something - back to the question

  done:
FunctionEnd

!macroend

!insertmacro PLAYERONE_CLOSE_FUNCTIONS "" "replaced"
!insertmacro PLAYERONE_CLOSE_FUNCTIONS "un." "removed"

Section
    ; Before anything is written, and before the WebView2 bootstrapper has a
    ; chance to run: nothing should happen at all if the answer is Abort.
    Call CloseRunningApp

    SetRegView 64

    ; The WebView2 runtime installs per machine or per user, and a for-me-only
    ; installation on an account that already has its own copy must not drag
    ; the bootstrapper in again. wails.webview2runtime only knows about the
    ; machine-wide key, so the per-user one is checked here first.
    ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${If} $0 == ""
        !insertmacro wails.webview2runtime
    ${EndIf}
    SetRegView 64

    SetOutPath $INSTDIR

    !insertmacro wails.files

    ; The media engine ships beside the executable. PlayerOne looks in this
    ; folder before falling back to PATH, so an installed copy works without
    ; the user installing mpv separately.
    ; /nonfatal keeps the installer buildable on a checkout where bin/ has not
    ; been populated yet; the application then falls back to PATH and explains
    ; itself if nothing is found.
    SetOutPath "$INSTDIR\bin"
    File /nonfatal "..\..\..\bin\mpv.exe"
    File /nonfatal "..\..\..\bin\mpv.com"
    File /nonfatal "..\..\..\bin\ffmpeg.exe"
    File /nonfatal "..\..\..\bin\ffprobe.exe"
    File /nonfatal "..\..\..\bin\d3dcompiler_43.dll"
    SetOutPath "$INSTDIR"

    ; $SMPROGRAMS and $DESKTOP follow the scope chosen on the page: every
    ; account's Start menu and desktop, or only this one's.
    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    ; --- Make Windows aware that PlayerOne can open media files ---

    ; The ProgID: how PlayerOne opens a file, and what the entry is called.
    WriteRegStr SHELL_CONTEXT "Software\Classes\${PLAYERONE_PROGID}" "" "Video file"
    WriteRegStr SHELL_CONTEXT "Software\Classes\${PLAYERONE_PROGID}" "FriendlyTypeName" "Video file"
    WriteRegStr SHELL_CONTEXT "Software\Classes\${PLAYERONE_PROGID}\DefaultIcon" "" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr SHELL_CONTEXT "Software\Classes\${PLAYERONE_PROGID}\shell\open" "FriendlyAppName" "${INFO_PRODUCTNAME}"
    WriteRegStr SHELL_CONTEXT "Software\Classes\${PLAYERONE_PROGID}\shell\open\command" "" '"$INSTDIR\${PRODUCT_EXECUTABLE}" "%1"'

    ; The application entry, which is what "Choose another app" reads.
    WriteRegStr SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}" "FriendlyAppName" "${INFO_PRODUCTNAME}"
    WriteRegStr SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}\DefaultIcon" "" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}\shell\open\command" "" '"$INSTDIR\${PRODUCT_EXECUTABLE}" "%1"'

    ; Capabilities, so PlayerOne can be picked in Settings > Default apps.
    ; Listing an association here offers it; it does not take it.
    WriteRegStr SHELL_CONTEXT "Software\${INFO_PRODUCTNAME}\Capabilities" "ApplicationName" "${INFO_PRODUCTNAME}"
    WriteRegStr SHELL_CONTEXT "Software\${INFO_PRODUCTNAME}\Capabilities" "ApplicationDescription" "Plays local videos with chapters and a searchable transcript."
    WriteRegStr SHELL_CONTEXT "Software\RegisteredApplications" "${INFO_PRODUCTNAME}" "Software\${INFO_PRODUCTNAME}\Capabilities"

    !insertmacro PLAYERONE_EACH_EXT PLAYERONE_REGISTER_EXT

    ; Explorer caches associations; without this the new entry only appears
    ; after a sign-out.
    System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'

    !insertmacro wails.associateCustomProtocols

    ; --- Add or remove programs ---
    ;
    ; Written here rather than through wails.writeUninstaller, which chooses
    ; HKLM or HKCU when the installer is compiled. The scope is only known
    ; now, so the entry goes to SHELL_CONTEXT: a machine-wide installation
    ; appears for everyone, a private one only in this account's list.
    ; InstallLocation is what tells the uninstaller, and the next installer,
    ; which of the two this copy is.
    WriteUninstaller "$INSTDIR\uninstall.exe"

    WriteRegStr SHELL_CONTEXT "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr SHELL_CONTEXT "${UNINST_KEY}" "DisplayName" "${INFO_PRODUCTNAME}"
    WriteRegStr SHELL_CONTEXT "${UNINST_KEY}" "DisplayVersion" "${INFO_PRODUCTVERSION}"
    WriteRegStr SHELL_CONTEXT "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr SHELL_CONTEXT "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr SHELL_CONTEXT "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr SHELL_CONTEXT "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"

    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD SHELL_CONTEXT "${UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

; Which installation this uninstaller belongs to. It sits inside $INSTDIR, so
; the entry whose InstallLocation is this folder is the one that put it there.
; Getting this wrong would delete another scope's registry keys and leave this
; scope's behind, so the fallback stays on the cautious side and assumes the
; private installation unless Program Files says otherwise.
Function un.onInit
    SetRegView 64

    ReadRegStr $0 HKLM "${UNINST_KEY}" "InstallLocation"
    ${If} $0 == $INSTDIR
        SetShellVarContext all
        Return
    ${EndIf}

    ReadRegStr $0 HKCU "${UNINST_KEY}" "InstallLocation"
    ${If} $0 == $INSTDIR
        SetShellVarContext current
        Return
    ${EndIf}

    ; No usable record - go by where the files actually are.
    StrLen $1 "$PROGRAMFILES64"
    StrCpy $2 "$INSTDIR" $1
    ${If} $2 == "$PROGRAMFILES64"
        SetShellVarContext all
    ${Else}
        SetShellVarContext current
    ${EndIf}
FunctionEnd

Section "uninstall"
    ; Same reasoning as the install: a running PlayerOne cannot be deleted, and
    ; RMDir would otherwise half-empty the folder and report success.
    Call un.CloseRunningApp

    SetRegView 64

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    ; Settings, history and logs in %APPDATA%\PlayerOne are deliberately left
    ; behind: reinstalling should not lose the user's resume positions.

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    ; --- Undo the media registration ---
    ;
    ; Every key written during install is removed. Leaving an OpenWithProgIds
    ; entry behind would keep offering an application that is no longer on the
    ; machine, and a stale ProgID would leave files showing PlayerOne's icon
    ; with nothing to open them.
    !insertmacro PLAYERONE_EACH_EXT PLAYERONE_UNREGISTER_EXT

    DeleteRegKey SHELL_CONTEXT "Software\Classes\${PLAYERONE_PROGID}"
    DeleteRegKey SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}"
    DeleteRegValue SHELL_CONTEXT "Software\RegisteredApplications" "${INFO_PRODUCTNAME}"
    DeleteRegKey SHELL_CONTEXT "Software\${INFO_PRODUCTNAME}"

    ; Tell Explorer to forget its cached associations, so the entry disappears
    ; immediately rather than at the next sign-out.
    System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    ; The counterpart of the registration above: same key, same context.
    Delete "$INSTDIR\uninstall.exe"
    DeleteRegKey SHELL_CONTEXT "${UNINST_KEY}"
SectionEnd
