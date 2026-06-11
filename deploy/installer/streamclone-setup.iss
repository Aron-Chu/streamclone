; Streamclone Windows installer (Inno Setup 6).
; Build: scripts/build-windows-installer.ps1 -Version v0.1.4

#ifndef Version
  #define Version "dev"
#endif
#ifndef SourceDir
  #define SourceDir "..\..\dist\streamclone-dev"
#endif

#define MyAppName "Streamclone"
#define MyAppPublisher "Streamclone contributors"
#define MyAppURL "https://github.com/Aron-Chu/streamclone"

[Setup]
AppId={{A7C4E2F1-8B3D-4F6A-9E1C-2D5B8A0F3C7E}
AppName={#MyAppName}
AppVersion={#Version}
AppVerName={#MyAppName} {#Version}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}/issues
AppUpdatesURL={#MyAppURL}/releases/latest
DefaultDirName={%USERPROFILE}\streamclone
UsePreviousAppDir=no
DisableDirPage=yes
DisableProgramGroupPage=yes
LicenseFile=..\..\LICENSE
OutputDir=..\..\dist
OutputBaseFilename=Streamclone-Setup-{#Version}
SetupIconFile=icon.ico
UninstallDisplayIcon={app}\deploy\installer\icon.ico
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=lowest
ArchitecturesInstallIn64BitMode=x64compatible
CloseApplications=no
MinVersion=10.0

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[CustomMessages]
english.DockerHint=Docker Desktop must be installed and running before setup continues.

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "icon.ico"; DestDir: "{app}\deploy\installer"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#MyAppName}\Start Streamclone"; Filename: "{app}\Start Streamclone.cmd"; WorkingDir: "{app}"
Name: "{autoprograms}\{#MyAppName}\Stop Streamclone"; Filename: "{app}\Stop Streamclone.cmd"; WorkingDir: "{app}"
Name: "{autoprograms}\{#MyAppName}\Manage Streamclone"; Filename: "{app}\Manage Streamclone.cmd"; WorkingDir: "{app}"
Name: "{autoprograms}\{#MyAppName}\Uninstall Streamclone"; Filename: "{uninstallexe}"

[Code]
var
  ProgressFile: string;

function DockerLooksReady: Boolean;
var
  ResultCode: Integer;
  DockerExe: string;
begin
  Result := Exec(ExpandConstant('{cmd}'), '/c docker info >nul 2>&1', '', SW_HIDE, ewWaitUntilTerminated, ResultCode)
    and (ResultCode = 0);
  if Result then
    Exit;

  DockerExe := ExpandConstant('{commonpf}\Docker\Docker\resources\bin\docker.exe');
  if FileExists(DockerExe) then
    Result := Exec(DockerExe, 'info', '', SW_HIDE, ewWaitUntilTerminated, ResultCode)
      and (ResultCode = 0);
end;

function PowerShellExe: string;
var
  NativePowerShell: string;
begin
  NativePowerShell := ExpandConstant('{sysnative}\WindowsPowerShell\v1.0\powershell.exe');
  if IsWin64 and FileExists(NativePowerShell) then
    Result := NativePowerShell
  else
    Result := ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe');
end;

function RemoveIncompleteInstallDir: Boolean;
var
  InstallDir: string;
begin
  Result := True;
  InstallDir := ExpandConstant('{%USERPROFILE}\streamclone');
  if DirExists(InstallDir)
    and not FileExists(InstallDir + '\scripts\start-streamclone.ps1')
    and not FileExists(InstallDir + '\.env')
    and not FileExists(InstallDir + '\unins000.exe') then
  begin
    Result := DelTree(InstallDir, True, True, True);
  end;
end;

function ReadSetupProgress(var Title, Detail, StatusLine: string): Boolean;
var
  Lines: TArrayOfString;
  I: Integer;
begin
  Result := False;
  Title := '';
  Detail := '';
  StatusLine := '';
  if not FileExists(ProgressFile) then
    Exit;
  if LoadStringsFromFile(ProgressFile, Lines) then
  begin
    for I := 0 to GetArrayLength(Lines) - 1 do
    begin
      if Pos('TITLE=', Lines[I]) = 1 then
        Title := Copy(Lines[I], 7, MaxInt);
      if Pos('DETAIL=', Lines[I]) = 1 then
        Detail := Copy(Lines[I], 8, MaxInt);
      if Pos('STATUS=', Lines[I]) = 1 then
        StatusLine := Copy(Lines[I], 8, MaxInt);
    end;
    Result := True;
  end;
end;

function RunHiddenUninstall: Boolean;
var
  ResultCode: Integer;
  Title, Detail, StatusLine: string;
  PollCount: Integer;
  Params: string;
begin
  Result := False;
  ProgressFile := ExpandConstant('{%TEMP}\streamclone-uninstall-progress.txt');
  DeleteFile(ProgressFile);

  Params := '-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "' +
    ExpandConstant('{app}\scripts\uninstall-streamclone.ps1') +
    '" -InstallDir "' + ExpandConstant('{app}') +
    '" -NonInteractive -KeepInstallDir -ProgressFile "' + ProgressFile + '"';

  WizardForm.ProgressGauge.Style := npbstMarquee;
  WizardForm.StatusLabel.Caption := 'Removing Streamclone...';
  WizardForm.PageDescriptionLabel.Caption := 'Stopping Docker and deleting local data.';
  try
    if not Exec(PowerShellExe,
      Params, '', SW_HIDE, ewNoWait, ResultCode) then
      Exit;

    PollCount := 0;
    while PollCount < 750 do
    begin
      if ReadSetupProgress(Title, Detail, StatusLine) then
      begin
        if Title <> '' then
        begin
          WizardForm.StatusLabel.Caption := Title;
          WizardForm.PageDescriptionLabel.Caption := Detail;
        end;
        if Pos('done|', StatusLine) = 1 then
        begin
          Result := (Copy(StatusLine, 6, 1) = '0');
          Break;
        end;
      end;
      PollCount := PollCount + 1;
      Sleep(400);
    end;
    if (PollCount >= 750) and not Result then
      MsgBox('Uninstall timed out. You may need to run Stop Streamclone and retry from Settings → Apps.',
        mbError, MB_OK);
  finally
    WizardForm.ProgressGauge.Style := npbstNormal;
  end;
end;

function RunHiddenSetup: Boolean;
var
  ResultCode: Integer;
  Title, Detail, StatusLine: string;
  PollCount: Integer;
  Params: string;
begin
  Result := False;
  ProgressFile := ExpandConstant('{%TEMP}\streamclone-setup-progress.txt');
  DeleteFile(ProgressFile);

  Params := '-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "' +
    ExpandConstant('{app}\scripts\install-setup-progress.ps1') +
    '" -InstallDir "' + ExpandConstant('{app}') +
    '" -ProgressFile "' + ProgressFile + '"';

  WizardForm.ProgressGauge.Style := npbstMarquee;
  WizardForm.StatusLabel.Caption := 'Preparing setup...';
  WizardForm.PageDescriptionLabel.Caption := 'This may take 3-8 minutes on first install.';
  try
    if not Exec(PowerShellExe,
      Params, '', SW_HIDE, ewNoWait, ResultCode) then
      Exit;

    PollCount := 0;
    while PollCount < 2250 do
    begin
      if ReadSetupProgress(Title, Detail, StatusLine) then
      begin
        if Title <> '' then
        begin
          WizardForm.StatusLabel.Caption := Title;
          WizardForm.PageDescriptionLabel.Caption := Detail;
        end;
        if Pos('blocked|', StatusLine) = 1 then
        begin
          WizardForm.StatusLabel.Caption := 'Setup blocked';
          WizardForm.PageDescriptionLabel.Caption := Copy(StatusLine, 9, MaxInt);
          if not WizardSilent then
            MsgBox('Streamclone setup is blocked:' + #13#10 + #13#10 +
              Copy(StatusLine, 9, MaxInt) + #13#10#13#10 +
              'Start Docker Desktop, wait until it is running, then run setup again.',
              mbError, MB_OK);
          Result := False;
          Break;
        end;
        if Pos('done|', StatusLine) = 1 then
        begin
          Result := (Copy(StatusLine, 6, 1) = '0');
          Break;
        end;
      end;
      PollCount := PollCount + 1;
      Sleep(400);
    end;
    if (PollCount >= 2250) and not Result then
      MsgBox('Setup timed out after 15 minutes. Check Docker Desktop and try again.',
        mbError, MB_OK);
  finally
    WizardForm.ProgressGauge.Style := npbstNormal;
  end;
end;

function InitializeSetup: Boolean;
begin
  Result := True;
  if not RemoveIncompleteInstallDir then
  begin
    if not WizardSilent then
      MsgBox('Setup could not remove an incomplete previous install. Delete %USERPROFILE%\streamclone and retry.',
        mbError, MB_OK);
    Result := False;
    Exit;
  end;

  if not DockerLooksReady then
  begin
    if WizardSilent then
    begin
      Result := False;
      Exit;
    end;
    if MsgBox(ExpandConstant('{cm:DockerHint}') + #13#10 + #13#10 +
      'Continue anyway?', mbConfirmation, MB_YESNO) = IDNO then
      Result := False;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    if not RunHiddenSetup then
    begin
      if WizardSilent then
        Abort
      else
        MsgBox('Streamclone setup failed. Ensure Docker Desktop is running, then run Install again or use Start Streamclone from the install folder.',
          mbError, MB_OK);
    end;
  end;
end;

function InitializeUninstall: Boolean;
begin
  Result := MsgBox(
    'Uninstall Streamclone?' + #13#10#13#10 +
    'This will:' + #13#10 +
    '  - Stop all Docker containers' + #13#10 +
    '  - Delete database and MinIO data (volumes)' + #13#10 +
    '  - Remove secrets and Desktop shortcuts' + #13#10 +
    '  - Remove the install folder' + #13#10#13#10 +
    'Docker images stay cached unless you run uninstall with -PruneImages manually.',
    mbConfirmation, MB_YESNO) = IDYES;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
  begin
    if not RunHiddenUninstall then
      MsgBox('Streamclone Docker teardown failed. Stop containers manually, then retry uninstall from Settings → Apps.',
        mbError, MB_OK);
  end;
end;
