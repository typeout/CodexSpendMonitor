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
    form.toolbar { display: grid; grid-template-columns: minmax(220px, 1fr) auto auto auto auto; gap: 8px; align-items: center; }
    input { width: 100%; min-height: 36px; border: 1px solid var(--line); border-radius: 6px; padding: 7px 9px; background: var(--panel); color: var(--text); }
    button, a.button { min-height: 36px; border: 1px solid var(--line); border-radius: 6px; padding: 7px 11px; background: var(--panel); color: var(--text); text-decoration: none; cursor: pointer; white-space: nowrap; }
    button.primary { border-color: var(--accent); background: var(--accent); color: #fff; }
    .meta { color: var(--muted); font-size: 13px; }
    .notice, .error { margin: 14px 0; padding: 10px 12px; border-radius: 6px; }
    .notice { border: 1px solid #96d2b8; background: rgba(31, 122, 92, .12); }
    .error { border: 1px solid #e09a9a; background: rgba(186, 47, 47, .12); }
    .daily { display: grid; grid-template-columns: repeat(4, minmax(150px, 1fr)); gap: 10px; }
    .tile { border: 1px solid var(--line); border-radius: 8px; background: var(--panel); padding: 12px; }
    .tile strong { display: block; font-size: 22px; margin-top: 5px; }
    table { width: 100%; border-collapse: collapse; background: var(--panel); border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
    th, td { padding: 10px 11px; border-bottom: 1px solid var(--line); text-align: left; vertical-align: top; }
    th { font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; }
    td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
    tr:last-child td { border-bottom: 0; }
    .name { font-weight: 650; }
    .path { color: var(--muted); font-size: 12px; max-width: 360px; overflow-wrap: anywhere; }
    .project-cell { padding: 0; }
    tr.project-row { cursor: pointer; }
    tr.project-row:hover td { background: rgba(31, 122, 92, .08); }
    tr.project-row:focus { outline: 2px solid var(--accent); outline-offset: -2px; }
    .session-details { border-bottom: 1px solid var(--line); padding: 10px 12px 12px; background: color-mix(in srgb, var(--panel) 92%, var(--line)); }
    .session-details table { border-radius: 6px; }
    .session-details td, .session-details th { font-size: 13px; }
    .project-detail[hidden] { display: none; }
    @media (max-width: 760px) {
      header { align-items: flex-start; flex-direction: column; }
      form.toolbar { grid-template-columns: 1fr; width: 100%; }
      .daily { grid-template-columns: 1fr 1fr; }
      table { font-size: 13px; }
      .hide-mobile, th.hide-mobile, td.hide-mobile, .session-details th:nth-child(3), .session-details td:nth-child(3) { display: none; }
    }
  </style>
  <script>
    function toggleProject(row) {
      const detail = document.getElementById(row.getAttribute('aria-controls'));
      const expanded = row.getAttribute('aria-expanded') === 'true';
      row.setAttribute('aria-expanded', String(!expanded));
      detail.hidden = expanded;
    }
  </script>
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
      <a class="button" href="/pricing">Pricing</a>
    </form>
    <form id="scan-form" method="post" action="/scan" hx-post="/scan" hx-target="body"></form>
    <form id="pricing-form" method="post" action="/pricing/refresh" hx-post="/pricing/refresh" hx-target="body"></form>

    {{if .Message}}<div class="notice">{{.Message}}</div>{{end}}
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}

    <h2>Spend</h2>
    <div class="daily">
      <div class="tile">
        <span class="meta">Today</span>
        <strong>{{money .Totals.Today}}</strong>
      </div>
      <div class="tile">
        <span class="meta">Yesterday</span>
        <strong>{{money .Totals.Yesterday}}</strong>
      </div>
      <div class="tile">
        <span class="meta">Week to date</span>
        <strong>{{money .Totals.WeekToDate}}</strong>
      </div>
      <div class="tile">
        <span class="meta">Month to date</span>
        <strong>{{money .Totals.MonthToDate}}</strong>
      </div>
    </div>

    <h2>Projects</h2>
    <table>
      <colgroup>
        <col>
        <col style="width: 120px">
        <col style="width: 180px">
        <col style="width: 180px">
      </colgroup>
      <thead>
        <tr>
          <th>Project</th>
          <th class="num">Sessions</th>
          <th class="num hide-mobile">Tokens</th>
          <th class="num">Running cost</th>
        </tr>
      </thead>
      <tbody>
        {{range $index, $project := .Projects}}
        <tr class="project-row" tabindex="0" role="button" aria-expanded="false" aria-controls="project-{{$index}}" onclick="toggleProject(this)" onkeydown="if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); toggleProject(this); }">
          <td>
            <div class="name">{{$project.Name}}</div>
            <div class="path">{{$project.Path}}</div>
            {{if $project.UnpricedEvents}}<div class="meta">{{$project.UnpricedEvents}} unpriced events</div>{{end}}
          </td>
          <td class="num">{{$project.SessionCount}}</td>
          <td class="num hide-mobile">{{$project.TotalTokens}}</td>
          <td class="num">{{money $project.TotalCost}}</td>
        </tr>
        <tr id="project-{{$index}}" class="project-detail" hidden>
          <td colspan="4" class="project-cell">
            <div class="session-details">
              <table>
                <thead>
                  <tr>
                    <th>Session</th>
                    <th>Models</th>
                    <th class="num">Events</th>
                    <th class="num">Tokens</th>
                    <th class="num">Cost</th>
                  </tr>
                </thead>
                <tbody>
                  {{range $project.Sessions}}
                  <tr>
                    <td><a href="/sessions/{{.ID}}">{{shortID .ID}}</a><div class="name">{{.Title}}</div><div class="meta">{{dateTime .StartedAt}}</div></td>
                    <td>{{.Models}}{{if .UnpricedEvents}}<div class="meta">{{.UnpricedEvents}} unpriced</div>{{end}}</td>
                    <td class="num">{{.EventCount}}</td>
                    <td class="num">{{.TotalTokens}}</td>
                    <td class="num">{{money .TotalCost}}</td>
                  </tr>
                  {{end}}
                </tbody>
              </table>
            </div>
          </td>
        </tr>
        {{else}}
        <tr><td colspan="4">No sessions imported yet.</td></tr>
        {{end}}
      </tbody>
    </table>
  </main>
</body>
</html>
{{end}}

{{define "pricing"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Pricing - Codex Spend Monitor</title>
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  <style>
    :root { color-scheme: light dark; --line: #d7dce2; --muted: #637083; --accent: #1f7a5c; --bg: #f7f8fa; --panel: #ffffff; --text: #16202a; }
    @media (prefers-color-scheme: dark) { :root { --line: #303842; --muted: #a2adba; --accent: #58c49d; --bg: #11161c; --panel: #171e26; --text: #eef2f5; } }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: ui-sans-serif, system-ui, Segoe UI, Arial, sans-serif; background: var(--bg); color: var(--text); }
    main { max-width: 1180px; margin: 0 auto; padding: 20px; }
    a { color: inherit; }
    h1 { font-size: 22px; margin: 8px 0 4px; }
    h2 { font-size: 15px; margin: 28px 0 10px; }
    .meta { color: var(--muted); font-size: 13px; }
    .notice, .error { margin: 14px 0; padding: 10px 12px; border-radius: 6px; }
    .notice { border: 1px solid #96d2b8; background: rgba(31, 122, 92, .12); }
    .error { border: 1px solid #e09a9a; background: rgba(186, 47, 47, .12); }
    input, select { min-height: 36px; border: 1px solid var(--line); border-radius: 6px; padding: 7px 9px; background: var(--panel); color: var(--text); }
    button, a.button { min-height: 36px; border: 1px solid var(--line); border-radius: 6px; padding: 7px 11px; background: var(--panel); color: var(--text); text-decoration: none; cursor: pointer; white-space: nowrap; }
    button.primary { border-color: var(--accent); background: var(--accent); color: #fff; }
    form.add { display: grid; grid-template-columns: 1.4fr .8fr .8fr .8fr .8fr .8fr auto; gap: 8px; align-items: end; }
    form.tool-add { grid-template-columns: 1fr 1.4fr 1fr .8fr .8fr auto; }
    table { width: 100%; border-collapse: collapse; background: var(--panel); border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
    th, td { padding: 8px; border-bottom: 1px solid var(--line); text-align: left; vertical-align: middle; }
    th { font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; }
    td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
    td form { display: inline; }
    .actions { display: flex; align-items: center; gap: 6px; justify-content: flex-start; }
    .actions form { display: inline-flex; }
    button.save { border-color: #9bd5a8; background: #e9f8ed; color: #185b29; }
    button.delete { border-color: #e7a0a0; background: #fdecec; color: #8a1f1f; }
    input[form^="pricing-update-"], select[form^="pricing-update-"], input[form^="tool-pricing-update-"] { width: 100%; }
    @media (max-width: 900px) {
      form.add { grid-template-columns: 1fr 1fr; }
      table { font-size: 13px; }
    }
  </style>
</head>
<body>
  <main id="app">
    <a href="/">Back to dashboard</a>
    <h1>Pricing</h1>
    <div class="meta">Model prices are per 1M tokens. Tool prices use their saved unit size. Blank cached input on save uses the normal input price.</div>

    {{if .Message}}<div class="notice">{{.Message}}</div>{{end}}
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}

    <h2>Add Model Pricing</h2>
    <form class="add" method="post" action="/pricing" hx-post="/pricing" hx-target="body">
      <input name="model" placeholder="model" aria-label="Model">
      <select name="billing_tier" aria-label="Billing tier">
        <option value="standard">Standard</option>
        <option value="batch">Batch</option>
        <option value="flex">Flex</option>
        <option value="priority">Priority</option>
      </select>
      <select name="context_kind" aria-label="Context">
        <option value="short">Short</option>
        <option value="long">Long</option>
      </select>
      <input name="input_per_million" placeholder="input" aria-label="Input price">
      <input name="cached_input_per_million" placeholder="cached" aria-label="Cached input price">
      <input name="output_per_million" placeholder="output" aria-label="Output price">
      <button class="primary" type="submit">Add</button>
    </form>

    <h2>Model Pricing</h2>
    <table>
      <thead>
        <tr>
          <th>Model</th>
          <th>Tier</th>
          <th>Context</th>
          <th class="num">Input</th>
          <th class="num">Cached</th>
          <th class="num">Output</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {{range .Prices}}
        <tr>
            <td><input form="pricing-update-{{.ID}}" name="model" value="{{.Model}}" aria-label="Model"></td>
            <td>
              <select form="pricing-update-{{.ID}}" name="billing_tier" aria-label="Billing tier">
                <option value="standard" {{if eq .BillingTier "standard"}}selected{{end}}>Standard</option>
                <option value="batch" {{if eq .BillingTier "batch"}}selected{{end}}>Batch</option>
                <option value="flex" {{if eq .BillingTier "flex"}}selected{{end}}>Flex</option>
                <option value="priority" {{if eq .BillingTier "priority"}}selected{{end}}>Priority</option>
              </select>
            </td>
            <td>
              <select form="pricing-update-{{.ID}}" name="context_kind" aria-label="Context">
                <option value="short" {{if eq .ContextKind "short"}}selected{{end}}>Short</option>
                <option value="long" {{if eq .ContextKind "long"}}selected{{end}}>Long</option>
              </select>
            </td>
            <td class="num"><input form="pricing-update-{{.ID}}" name="input_per_million" value="{{printf "%.4g" .InputPerMillion}}" aria-label="Input price"></td>
            <td class="num"><input form="pricing-update-{{.ID}}" name="cached_input_per_million" value="{{printf "%.4g" .CachedInputPerMillion}}" aria-label="Cached input price"></td>
            <td class="num"><input form="pricing-update-{{.ID}}" name="output_per_million" value="{{printf "%.4g" .OutputPerMillion}}" aria-label="Output price"></td>
            <td class="actions">
              <form id="pricing-update-{{.ID}}" method="post" action="/pricing/{{.ID}}/update" hx-post="/pricing/{{.ID}}/update" hx-target="body"></form>
              <input form="pricing-update-{{.ID}}" type="hidden" name="source_url" value="{{.SourceURL}}">
              <button class="save" type="submit" form="pricing-update-{{.ID}}">Save</button>
              <form method="post" action="/pricing/{{.ID}}/delete" hx-post="/pricing/{{.ID}}/delete" hx-target="body">
                <button class="delete" type="submit">Delete</button>
              </form>
            </td>
        </tr>
        {{else}}
        <tr><td colspan="7">No pricing options saved.</td></tr>
        {{end}}
      </tbody>
    </table>

    <h2>Add Tool Pricing</h2>
    <form class="add tool-add" method="post" action="/pricing/tools" hx-post="/pricing/tools" hx-target="body">
      <input name="tool_key" placeholder="tool key" aria-label="Tool key">
      <input name="display_name" placeholder="display name" aria-label="Display name">
      <input name="unit_label" placeholder="unit label" aria-label="Unit label">
      <input name="unit_size" placeholder="unit size" aria-label="Unit size">
      <input name="price_per_unit" placeholder="price" aria-label="Price per unit">
      <button class="primary" type="submit">Add</button>
    </form>

    <h2>Tool Pricing</h2>
    <table>
      <thead>
        <tr>
          <th>Tool key</th>
          <th>Name</th>
          <th>Unit</th>
          <th class="num">Unit size</th>
          <th class="num">Price</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {{range .ToolPrices}}
        <tr>
          <td><input form="tool-pricing-update-{{.ID}}" name="tool_key" value="{{.ToolKey}}" aria-label="Tool key"></td>
          <td><input form="tool-pricing-update-{{.ID}}" name="display_name" value="{{.DisplayName}}" aria-label="Display name"></td>
          <td><input form="tool-pricing-update-{{.ID}}" name="unit_label" value="{{.UnitLabel}}" aria-label="Unit label"></td>
          <td class="num"><input form="tool-pricing-update-{{.ID}}" name="unit_size" value="{{printf "%.4g" .UnitSize}}" aria-label="Unit size"></td>
          <td class="num"><input form="tool-pricing-update-{{.ID}}" name="price_per_unit" value="{{printf "%.4g" .PricePerUnit}}" aria-label="Price per unit"></td>
          <td class="actions">
            <form id="tool-pricing-update-{{.ID}}" method="post" action="/pricing/tools/{{.ID}}/update" hx-post="/pricing/tools/{{.ID}}/update" hx-target="body"></form>
            <input form="tool-pricing-update-{{.ID}}" type="hidden" name="source_url" value="{{.SourceURL}}">
            <button class="save" type="submit" form="tool-pricing-update-{{.ID}}">Save</button>
            <form method="post" action="/pricing/tools/{{.ID}}/delete" hx-post="/pricing/tools/{{.ID}}/delete" hx-target="body">
              <button class="delete" type="submit">Delete</button>
            </form>
          </td>
        </tr>
        {{else}}
        <tr><td colspan="6">No tool pricing options saved.</td></tr>
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
    h2 { font-size: 15px; margin: 26px 0 8px; }
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
          <th>Pricing</th>
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
          <td>{{.BillingTier}} / {{.ContextKind}}</td>
          <td class="num">{{.InputTokens}}</td>
          <td class="num">{{.CachedInputTokens}}</td>
          <td class="num">{{.OutputTokens}}</td>
          <td class="num">{{.ReasoningOutputTokens}}</td>
          <td class="num">{{.TotalTokens}}</td>
          <td class="num">{{money .Cost}}</td>
        </tr>
        {{else}}
        <tr><td colspan="9">No token-count events imported for this session.</td></tr>
        {{end}}
      </tbody>
    </table>
    <h2>Tool usage</h2>
    <table>
      <thead>
        <tr>
          <th>Time</th>
          <th>Tool</th>
          <th>Key</th>
          <th class="num">Quantity</th>
          <th>Unit</th>
          <th class="num">Cost</th>
        </tr>
      </thead>
      <tbody>
        {{range .ToolEvents}}
        <tr>
          <td>{{dateTime .Timestamp}}</td>
          <td>{{.ToolName}}{{if not .Priced}}<div class="meta">unpriced</div>{{end}}</td>
          <td>{{.ToolKey}}</td>
          <td class="num">{{quantity .Quantity}}</td>
          <td>{{if .UnitLabel.Valid}}{{.UnitLabel.String}}{{end}}</td>
          <td class="num">{{money .Cost}}</td>
        </tr>
        {{else}}
        <tr><td colspan="6">No billable hosted tool calls detected for this session.</td></tr>
        {{end}}
      </tbody>
    </table>
  </main>
</body>
</html>
{{end}}
`
