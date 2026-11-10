Unicode true

!include "MUI2.nsh"
!include "LogicLib.nsh"

!ifndef APP_VERSION
  !define APP_VERSION "0.2.1"
!endif

!define APP_NAME "TailSQL"
!define APP_PUBLISHER "TailSQL contributors"
!define APP_EXE "TailSQL.exe"
!define ROOT "${__FILEDIR__}\.."
!define SOURCE_EXE "${ROOT}\build\bin\TailSQL-${APP_VERSION}.exe"
!define SOURCE_ASSETS "${ROOT}\build\bin\assets"
!define OUT_FILE "${ROOT}\dist\TailSQL-Setup-${APP_VERSION}.exe"
!define ICON "${ROOT}\build\windows\icon.ico"

Name "${APP_NAME} ${APP_VERSION}"
OutFile "${OUT_FILE}"
InstallDir "$PROGRAMFILES64\TailSQL"
InstallDirRegKey HKLM "Software\TailSQL" "InstallDir"
RequestExecutionLevel admin
SetCompressor /SOLID lzma
SetCompressorDictSize 64
BrandingText "TailSQL ${APP_VERSION}"
ShowInstDetails show
ShowUninstDetails show

VIProductVersion "${APP_VERSION}.0"
VIAddVersionKey "ProductName" "${APP_NAME}"
VIAddVersionKey "ProductVersion" "${APP_VERSION}"
VIAddVersionKey "FileVersion" "${APP_VERSION}.0"
VIAddVersionKey "FileDescription" "TailSQL installer"
VIAddVersionKey "CompanyName" "${APP_PUBLISHER}"
VIAddVersionKey "LegalCopyright" "TailSQL contributors"

!define MUI_ABORTWARNING
!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_WELCOMEPAGE_TITLE "Install ${APP_NAME} ${APP_VERSION}"
!define MUI_WELCOMEPAGE_TEXT "TailSQL is a local-first H2 database manager with a table editor, SQL workbench, Tailscale hosting, and public Funnel support.$\r$\n$\r$\nJava 11 or newer is required. Tailscale is optional."
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Launch TailSQL"
!define MUI_FINISHPAGE_LINK "Read the documentation"
!define MUI_FINISHPAGE_LINK_LOCATION "https://godfatherthr.github.io/tailsql/docs.html"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Function .onInit
  SetShellVarContext all
  SetRegView 64
  ReadRegStr $0 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL" "InstallLocation"
  ${If} $0 != ""
    StrCpy $INSTDIR $0
  ${EndIf}
FunctionEnd

Section "TailSQL" SecMain
  SectionIn RO

  SetOutPath "$INSTDIR"
  File "/oname=${APP_EXE}" "${SOURCE_EXE}"

  SetOutPath "$INSTDIR\assets"
  File "${SOURCE_ASSETS}\h2.jar"

  SetOutPath "$INSTDIR"
  WriteUninstaller "$INSTDIR\Uninstall.exe"

  CreateDirectory "$SMPROGRAMS\TailSQL"
  CreateShortcut "$SMPROGRAMS\TailSQL\TailSQL.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
  WriteINIStr "$SMPROGRAMS\TailSQL\Documentation.url" "InternetShortcut" "URL" "https://godfatherthr.github.io/tailsql/docs.html"
  CreateShortcut "$SMPROGRAMS\TailSQL\Uninstall TailSQL.lnk" "$INSTDIR\Uninstall.exe"
  CreateShortcut "$DESKTOP\TailSQL.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0

  WriteRegStr HKLM "Software\TailSQL" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\TailSQL" "Version" "${APP_VERSION}"

  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL" "DisplayName" "${APP_NAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL" "Publisher" "${APP_PUBLISHER}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL" "DisplayIcon" "$INSTDIR\${APP_EXE}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL" "NoRepair" 1
SectionEnd

Section "Uninstall"
  SetShellVarContext all
  SetRegView 64

  Delete "$DESKTOP\TailSQL.lnk"
  Delete "$SMPROGRAMS\TailSQL\TailSQL.lnk"
  Delete "$SMPROGRAMS\TailSQL\Documentation.url"
  Delete "$SMPROGRAMS\TailSQL\Uninstall TailSQL.lnk"
  RMDir "$SMPROGRAMS\TailSQL"

  Delete "$INSTDIR\${APP_EXE}"
  Delete "$INSTDIR\assets\h2.jar"
  RMDir "$INSTDIR\assets"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TailSQL"
  DeleteRegKey HKLM "Software\TailSQL"
SectionEnd
