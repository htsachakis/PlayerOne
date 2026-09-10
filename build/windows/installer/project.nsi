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
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
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
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
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

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

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

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

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

    !insertmacro wails.deleteUninstaller
SectionEnd
