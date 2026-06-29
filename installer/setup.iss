; ─────────────────────────────────────────────────────────────────────────────
; PrinklyPrint — Inno Setup script.
;
; Genera dist/PrinklyPrint-Setup-{version}.exe a partir de dist/prinklyprint.exe.
;
; Diseñado para upgrades limpios:
;   - AppId fijo (GUID estable) → Inno detecta la versión anterior y la
;     pisa en el mismo InstallDir sin pedirle al usuario qué hacer.
;   - AppMutex = el mismo nombre que usa internal/app/singleton_windows.go.
;     Si la app está corriendo, Inno la cierra elegantemente antes de pisar
;     el .exe. Sin esto, "archivo en uso" rompía la actualización.
;   - CloseApplications=force → si la app no respeta el cierre, Inno la mata.
;
; Variables inyectadas por línea de comandos desde el job build-installer de
; .github/workflows/release.yml (runner Windows nativo):
;   ISCC.exe /DAppVersion=1.0.0 setup.iss
; ─────────────────────────────────────────────────────────────────────────────

#ifndef AppVersion
  #define AppVersion "1.0.0"
#endif

#define MyAppName        "PrinklyPrint"
#define MyAppPublisher   "LautaroTiamat"
#define MyAppURL         "https://github.com/LautaroTiamat/PrinklyPrint"
#define MyAppExeName     "prinklyprint.exe"
#define MyAppMutex       "Global\PrinklyPrintSingletonMutex_v1"

[Setup]
; AppId determina la identidad de la app entre versiones — NO CAMBIAR.
AppId={{C7E2F491-7B2D-4F8E-9C3B-2D8A11B5F034}
AppName={#MyAppName}
AppVersion={#AppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}/issues
AppUpdatesURL={#MyAppURL}/releases
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
UninstallDisplayIcon={app}\{#MyAppExeName}
UninstallDisplayName={#MyAppName}
VersionInfoVersion={#AppVersion}
VersionInfoProductName={#MyAppName}
VersionInfoCompany={#MyAppPublisher}
Compression=lzma2/ultra64
SolidCompression=yes
OutputDir=..\dist
; Nombre del .exe sin versión para que el link
; /releases/latest/download/PrinklyPrint-Setup.exe siempre funcione.
; La versión queda embebida en VersionInfoVersion (visible en Propiedades del .exe).
OutputBaseFilename=PrinklyPrint-Setup
SetupIconFile=..\icons\PrinklyPrint.ico
WizardStyle=modern
WizardSizePercent=110
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesInstallIn64BitMode=x64compatible
ArchitecturesAllowed=x64compatible
; Cierra la instancia previa de PrinklyPrint antes de pisar archivos.
CloseApplications=force
RestartApplications=no
AppMutex={#MyAppMutex}
DisableProgramGroupPage=yes
DisableReadyPage=no
DisableDirPage=auto
ShowLanguageDialog=auto

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "portuguese"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "autostart"; Description: "Iniciar {#MyAppName} cuando inicie Windows"; GroupDescription: "Inicio del sistema:"

[Files]
Source: "..\dist\prinklyprint.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Registry]
; "Iniciar con Windows" — entrada estándar HKCU\...\Run.
; Si el usuario tildó la task autostart, la creamos al instalar y la borramos
; al desinstalar. La app también puede toggle-arla en runtime desde la pestaña
; General (escribe el mismo valor con el mismo nombre).
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; \
  ValueType: string; ValueName: "PrinklyPrint"; ValueData: """{app}\{#MyAppExeName}"""; \
  Flags: uninsdeletevalue; Tasks: autostart

[Run]
; Registra el source del Windows Event Log (requiere admin; el instalador ya
; corre elevado). Habilita los eventos de seguridad que recolecta el SIEM.
Filename: "{app}\{#MyAppExeName}"; Parameters: "--register-eventlog"; \
  Flags: runhidden; StatusMsg: "Registrando el Event Log de seguridad..."
Filename: "{app}\{#MyAppExeName}"; Description: "Iniciar {#MyAppName} ahora"; \
  Flags: nowait postinstall skipifsilent

[UninstallRun]
; Quita el source del Event Log antes de borrar el .exe.
Filename: "{app}\{#MyAppExeName}"; Parameters: "--unregister-eventlog"; \
  Flags: runhidden; RunOnceId: "UnregEventLog"
; Siempre intentamos borrar la entrada del Run, aunque el usuario haya
; toggle-ado autostart desde la app después de instalar.
Filename: "reg.exe"; \
  Parameters: "delete ""HKCU\Software\Microsoft\Windows\CurrentVersion\Run"" /v PrinklyPrint /f"; \
  Flags: runhidden; RunOnceId: "DelAutostartReg"
