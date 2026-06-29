# Codex Spend Monitor

Go + HTMX app for monitoring a Codex `.codex` directory and calculating API-equivalent spend from session token-count events.

## Current Behavior

- Scans `sessions/**/*.jsonl` and `archived_sessions/**/*.jsonl`.
- Stores imported sessions, usage events, settings, and pricing snapshots in SQLite.
- Polls every 10 seconds after startup and skips unchanged files by size and mtime.
- Shows a dashboard with daily spend and session totals.
- Shows a session drilldown with per-event token usage and calculated cost.
- Fetches OpenAI pricing from `https://openai.com/api/pricing/` when requested, with seeded fallback rates for GPT-5.5, GPT-5.4, and GPT-5.4 mini.

## Run

```powershell
go run ./cmd/codex-spend-monitor
```

The app starts the local web server and places an icon in the Windows notification area. Left-click the tray icon to open `http://127.0.0.1:5077`, or right-click it to open the tray menu and quit.

Use `-addr` to change the bind address and `-db` to change the SQLite file path.

For a background-style Windows executable without a console window:

```powershell
.\scripts\build-gui.ps1
```

The GUI build always writes `dist\CodexSependMonitor.exe`.

## Windows Startup

After building the GUI executable, run this PowerShell command to start it when you sign in:

```powershell
$startup = [Environment]::GetFolderPath("Startup"); $target = (Resolve-Path ".\dist\CodexSependMonitor.exe").Path; $shortcut = Join-Path $startup "CodexSependMonitor.lnk"; $shell = New-Object -ComObject WScript.Shell; $link = $shell.CreateShortcut($shortcut); $link.TargetPath = $target; $link.WorkingDirectory = Split-Path $target; $link.Save()
```

To remove it from startup:

```powershell
Remove-Item -LiteralPath (Join-Path ([Environment]::GetFolderPath("Startup")) "CodexSependMonitor.lnk")
```

## Notes

The displayed spend is API-equivalent: it is calculated from locally observed token counts and OpenAI API-style prices. It is not an invoice reconciliation tool.

The dashboard usage charts load ApexCharts from a CDN. ApexCharts is subject to its own license and terms:

- https://apexcharts.com/pricing/
- https://apexcharts.com/legal/terms/
