package web

const pagesHTML = `
{{define "dashboard"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Codex Spend Monitor</title>
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  <style>
    :root { color-scheme: light dark; --line: #d7dce2; --muted: #637083; --accent: #1f7a5c; --bg: #f7f8fa; --panel: #ffffff; --text: #16202a; }
    @media (prefers-color-scheme: dark) { :root { --line: #303842; --muted: #a2adba; --accent: #58c49d; --bg: #11161c; --panel: #171e26; --text: #eef2f5; } }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: ui-sans-serif, system-ui, Segoe UI, Arial, sans-serif; background: var(--bg); color: var(--text); }
    header, main { max-width: 1180px; margin: 0 auto; padding: 20px; }
    header { display: flex; align-items: center; justify-content: space-between; gap: 16px; border-bottom: 1px solid var(--line); }
    h1 { font-size: 22px; margin: 0; }
    h2 { font-size: 15px; margin: 28px 0 10px; }
    form.toolbar { display: grid; grid-template-columns: minmax(220px, 1fr) auto auto auto; gap: 8px; align-items: center; }
    input { width: 100%; min-height: 36px; border: 1px solid var(--line); border-radius: 6px; padding: 7px 9px; background: var(--panel); color: var(--text); }
    button, a.button { min-height: 36px; border: 1px solid var(--line); border-radius: 6px; padding: 7px 11px; background: var(--panel); color: var(--text); text-decoration: none; cursor: pointer; white-space: nowrap; }
    button.primary { border-color: var(--accent); background: var(--accent); color: #fff; }
    .meta { color: var(--muted); font-size: 13px; }
    .notice, .error { margin: 14px 0; padding: 10px 12px; border-radius: 6px; }
    .notice { border: 1px solid #96d2b8; background: rgba(31, 122, 92, .12); }
    .error { border: 1px solid #e09a9a; background: rgba(186, 47, 47, .12); }
    .daily { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px; }
    .tile { border: 1px solid var(--line); border-radius: 8px; background: var(--panel); padding: 12px; }
    .tile strong { display: block; font-size: 22px; margin-top: 5px; }
    table { width: 100%; border-collapse: collapse; background: var(--panel); border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
    th, td { padding: 10px 11px; border-bottom: 1px solid var(--line); text-align: left; vertical-align: top; }
    th { font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; }
    td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
    tr:last-child td { border-bottom: 0; }
    .name { font-weight: 650; }
    .path { color: var(--muted); font-size: 12px; max-width: 360px; overflow-wrap: anywhere; }
    @media (max-width: 760px) { header { align-items: flex-start; flex-direction: column; } form.toolbar { grid-template-columns: 1fr; width: 100%; } table { font-size: 13px; } th:nth-child(4), td:nth-child(4) { display: none; } }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>Codex Spend Monitor</h1>
      <div class="meta">API-equivalent totals from Codex token-count events. Pricing refreshed: {{.PricingRefresh}}</div>
    </div>
  </header>
  <main id="app">
    <form class="toolbar" method="post" action="/settings/codex-path" hx-post="/settings/codex-path" hx-target="body">
      <input name="codex_path" value="{{.CodexPath}}" aria-label="Codex folder path">
      <button class="primary" type="submit">Save Path</button>
      <button type="submit" form="scan-form">Rescan</button>
      <button type="submit" form="pricing-form">Refresh Pricing</button>
    </form>
    <form id="scan-form" method="post" action="/scan" hx-post="/scan" hx-target="body"></form>
    <form id="pricing-form" method="post" action="/pricing/refresh" hx-post="/pricing/refresh" hx-target="body"></form>

    {{if .Message}}<div class="notice">{{.Message}}</div>{{end}}
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}

    <h2>Daily Spend</h2>
    <div class="daily">
      {{range .Daily}}
      <div class="tile">
        <span class="meta">{{.Day}}</span>
        <strong>{{money .TotalCost}}</strong>
        {{if .UnpricedEvents}}<span class="meta">{{.UnpricedEvents}} unpriced events</span>{{end}}
      </div>
      {{else}}
      <div class="tile"><span class="meta">No usage imported yet.</span></div>
      {{end}}
    </div>

    <h2>Sessions</h2>
    <table>
      <thead>
        <tr>
          <th>Session</th>
          <th>Name</th>
          <th>Models</th>
          <th class="num">Events</th>
          <th class="num">Tokens</th>
          <th class="num">Total running cost</th>
        </tr>
      </thead>
      <tbody>
        {{range .Sessions}}
        <tr>
          <td><a href="/sessions/{{.ID}}">{{shortID .ID}}</a><div class="meta">{{dateTime .StartedAt}}</div></td>
          <td><div class="name">{{.Title}}</div><div class="path">{{.CWD}}</div></td>
          <td>{{.Models}}{{if .UnpricedEvents}}<div class="meta">{{.UnpricedEvents}} unpriced</div>{{end}}</td>
          <td class="num">{{.EventCount}}</td>
          <td class="num">{{.TotalTokens}}</td>
          <td class="num">{{money .TotalCost}}</td>
        </tr>
        {{else}}
        <tr><td colspan="6">No sessions imported yet.</td></tr>
        {{end}}
      </tbody>
    </table>
  </main>
</body>
</html>
{{end}}

{{define "session"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Session.Title}} - Codex Spend Monitor</title>
  <style>
    :root { color-scheme: light dark; --line: #d7dce2; --muted: #637083; --bg: #f7f8fa; --panel: #ffffff; --text: #16202a; }
    @media (prefers-color-scheme: dark) { :root { --line: #303842; --muted: #a2adba; --bg: #11161c; --panel: #171e26; --text: #eef2f5; } }
    body { margin: 0; font-family: ui-sans-serif, system-ui, Segoe UI, Arial, sans-serif; background: var(--bg); color: var(--text); }
    main { max-width: 1180px; margin: 0 auto; padding: 20px; }
    a { color: inherit; }
    h1 { font-size: 22px; margin: 8px 0 4px; }
    .meta { color: var(--muted); font-size: 13px; overflow-wrap: anywhere; }
    table { margin-top: 18px; width: 100%; border-collapse: collapse; background: var(--panel); border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
    th, td { padding: 10px 11px; border-bottom: 1px solid var(--line); text-align: left; vertical-align: top; }
    th { font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; }
    td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
  </style>
</head>
<body>
  <main>
    <a href="/">Back to dashboard</a>
    <h1>{{.Session.Title}}</h1>
    <div class="meta">{{.Session.ID}} · {{.Session.CWD}} · {{dateTime .Session.StartedAt}}</div>
    <table>
      <thead>
        <tr>
          <th>Time</th>
          <th>Model</th>
          <th class="num">Input</th>
          <th class="num">Cached</th>
          <th class="num">Output</th>
          <th class="num">Reasoning</th>
          <th class="num">Total</th>
          <th class="num">Cost</th>
        </tr>
      </thead>
      <tbody>
        {{range .Events}}
        <tr>
          <td>{{dateTime .Timestamp}}</td>
          <td>{{.Model}}{{if not .Priced}}<div class="meta">unpriced</div>{{end}}</td>
          <td class="num">{{.InputTokens}}</td>
          <td class="num">{{.CachedInputTokens}}</td>
          <td class="num">{{.OutputTokens}}</td>
          <td class="num">{{.ReasoningOutputTokens}}</td>
          <td class="num">{{.TotalTokens}}</td>
          <td class="num">{{money .Cost}}</td>
        </tr>
        {{else}}
        <tr><td colspan="8">No token-count events imported for this session.</td></tr>
        {{end}}
      </tbody>
    </table>
  </main>
</body>
</html>
{{end}}
`
