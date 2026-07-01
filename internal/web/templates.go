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
  <script src="https://cdn.jsdelivr.net/npm/apexcharts"></script>
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
    .section-tabs-wrap { position: relative; margin-top: 22px; padding-bottom: 1px; }
    .section-tabs-wrap::after { content: ""; position: absolute; left: 0; right: 0; bottom: 0; border-bottom: 1px solid var(--line); }
    .section-tabs { position: relative; z-index: 1; display: inline-flex; gap: 8px; padding: 0 18px 0 0; background: var(--bg); }
    .section-tab { min-height: 38px; border: 1px solid transparent; border-bottom: 0; border-top-left-radius: 10px; border-top-right-radius: 10px; border-bottom-left-radius: 0; border-bottom-right-radius: 0; padding: 8px 16px 9px; background: color-mix(in srgb, var(--panel) 84%, var(--line)); color: var(--muted); }
    .section-tab[aria-selected="true"] { border-color: var(--line); background: var(--panel); color: var(--text); box-shadow: 0 1px 0 var(--panel); }
    .section-panel { margin-top: 0; padding: 18px 18px 20px; border: 1px solid var(--line); border-top: 0; border-bottom-left-radius: 14px; border-bottom-right-radius: 14px; background: var(--panel); box-shadow: 0 12px 30px rgba(15, 23, 34, .04); }
    .section-panel[hidden] { display: none; }
    .chart-card { padding: 18px; margin-top: 4px; border-radius: 12px; background: color-mix(in srgb, var(--panel) 92%, var(--bg)); }
    .chart-tabs-wrap { position: relative; margin-bottom: 16px; padding-bottom: 1px; }
    .chart-tabs-wrap::after { content: ""; position: absolute; left: 0; right: 0; bottom: 0; border-bottom: 1px solid var(--line); }
    .chart-tabs { position: relative; z-index: 1; display: inline-flex; gap: 6px; padding: 0 14px 0 0; background: color-mix(in srgb, var(--panel) 92%, var(--bg)); }
    .chart-tab { min-height: 36px; border: 1px solid transparent; border-bottom: 0; border-top-left-radius: 10px; border-top-right-radius: 10px; border-bottom-left-radius: 0; border-bottom-right-radius: 0; padding: 7px 14px 8px; background: color-mix(in srgb, var(--panel) 84%, var(--line)); color: var(--muted); }
    .chart-tab[aria-selected="true"] { border-color: var(--line); background: var(--panel); color: var(--text); box-shadow: 0 1px 0 var(--panel); }
    .chart-panel { margin-top: 0; padding: 12px 2px 0; }
    .chart-panel[hidden] { display: none; }
    .chart-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 14px; margin-bottom: 14px; flex-wrap: wrap; }
    .chart-toggle { display: inline-flex; gap: 6px; padding-left: 16px; border-left: 1px solid var(--line); }
    .chart-toggle button { min-height: 30px; border: 1px solid transparent; border-radius: 999px; padding: 4px 12px; background: transparent; color: var(--muted); }
    .chart-toggle button[aria-pressed="true"] { border-color: var(--line); background: color-mix(in srgb, var(--panel) 82%, var(--accent)); color: var(--text); }
    .chart-head { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; margin-bottom: 8px; }
    .chart-head strong { font-size: 15px; }
    .chart-shell { min-height: 320px; }
    .chart-empty { padding: 20px 0 8px; }
    .section-panel h2:first-child,
    .chart-panel .chart-head:first-child { margin-top: 0; }
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
      .section-tabs { display: flex; gap: 6px; padding-right: 0; overflow-x: auto; }
      .section-panel { padding: 16px 14px 18px; }
      .chart-tabs { display: flex; gap: 6px; padding-right: 0; overflow-x: auto; }
      .chart-card { padding: 14px; }
      .chart-toolbar { align-items: flex-start; flex-direction: column; }
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

    function ensureDashboardState() {
      if (!window.codexSpendMonitorDashboard) {
        window.codexSpendMonitorDashboard = { charts: {}, payloads: {} };
      }
      return window.codexSpendMonitorDashboard;
    }

    function destroyUsageCharts() {
      const state = ensureDashboardState();
      Object.keys(state.charts).forEach((key) => {
        if (state.charts[key]) {
          state.charts[key].destroy();
        }
      });
      state.charts = {};
      state.payloads = {};
    }

    function formatChartValue(kind, value) {
      if (kind === 'credits') {
        const fixed = value.toFixed(1).replace(/\.0$/, '');
        return fixed + ' cr';
      }
      return Math.round(value).toLocaleString();
    }

    function chartSeriesData(payload, series) {
      return series.map((item) => ({
        name: item.label,
        data: item.values.map((value, index) => ({
          x: payload.labels[index],
          y: value,
        })),
      }));
    }

    function buildChartOptions(payload, series) {
      return {
        chart: {
          type: 'area',
          height: 320,
          stacked: true,
          toolbar: { show: false },
          animations: { easing: 'easeinout', speed: 220 },
          fontFamily: 'Segoe UI, system-ui, sans-serif',
        },
        series: chartSeriesData(payload, series),
        colors: series.map((item) => item.color),
        stroke: {
          curve: 'smooth',
          width: 2,
        },
        fill: {
          type: 'solid',
          opacity: 0.58,
        },
        dataLabels: { enabled: false },
        markers: { size: 0 },
        legend: {
          position: 'top',
          horizontalAlign: 'left',
        },
        grid: {
          borderColor: getComputedStyle(document.documentElement).getPropertyValue('--line').trim() || '#d7dce2',
        },
        xaxis: {
          type: 'datetime',
          tickAmount: Math.min(6, Math.max(payload.labels.length-1, 1)),
          labels: {
            datetimeUTC: false,
            rotate: 0,
            formatter: function(value) {
              if (value == null || value === '') {
                return '';
              }
              const date = new Date(value);
              if (!Number.isNaN(date.getTime())) {
                return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
              }
              return String(value);
            },
          },
        },
        yaxis: {
          labels: {
            formatter: function(value) {
              return formatChartValue(payload.valueKind, value);
            },
          },
        },
        tooltip: {
          shared: true,
          intersect: false,
          custom: function(opts) {
            const values = opts.series.map((items) => items[opts.dataPointIndex] || 0);
            const total = values.reduce((sum, value) => sum + value, 0);
            const rows = values
              .map((value, index) => {
                if (value === 0) {
                  return '';
                }
                const color = opts.w.globals.colors[index];
                const name = opts.w.globals.seriesNames[index];
                return '<div style="display:flex;align-items:center;justify-content:space-between;gap:12px;margin-top:4px;font-size:13px;line-height:1.35"><span><span style="display:inline-block;width:10px;height:10px;border-radius:999px;background:' + color + ';margin-right:6px"></span>' + name + '</span><span style="font-weight:500">' + formatChartValue(payload.valueKind, value) + '</span></div>';
              })
              .filter(Boolean)
              .join('');
            return '<div style="padding:10px 12px;min-width:220px;font-size:13px;line-height:1.35"><div style="font-weight:600;margin-bottom:2px">' + new Date(payload.labels[opts.dataPointIndex]).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' }) + '</div>' + rows + '<div style="display:flex;align-items:center;justify-content:space-between;gap:12px;margin-top:8px;padding-top:8px;border-top:1px solid rgba(120,120,120,.25)"><span>Total</span><span style="font-weight:600">' + formatChartValue(payload.valueKind, total) + '</span></div></div>';
          },
        },
      };
    }

    function parseChartPayload(id) {
      const state = ensureDashboardState();
      if (state.payloads[id]) {
        return state.payloads[id];
      }
      const node = document.getElementById(id);
      if (!node) {
        return null;
      }
      state.payloads[id] = JSON.parse(node.textContent);
      return state.payloads[id];
    }

    function renderUsageChart(chartKey, elementID, payload, series) {
      const state = ensureDashboardState();
      const mount = document.getElementById(elementID);
      if (!mount || !payload || !series || series.length === 0) {
        return;
      }
      if (state.charts[chartKey]) {
        state.charts[chartKey].destroy();
      }
      const chart = new ApexCharts(mount, buildChartOptions(payload, series));
      chart.render();
      state.charts[chartKey] = chart;
    }

    function setTokenChartMode(mode) {
      const payload = parseChartPayload('token-history-chart-data');
      if (!payload) {
        return;
      }
      const series = mode === 'split' ? payload.alternateSeries : payload.series;
      renderUsageChart('token', 'token-history-chart', payload, series);
      document.querySelectorAll('[data-token-mode]').forEach((button) => {
        const active = button.dataset.tokenMode === mode;
        button.setAttribute('aria-pressed', String(active));
      });
    }

    function activateGraphTab(tabID) {
      document.querySelectorAll('[data-graph-tab]').forEach((button) => {
        const active = button.dataset.graphTab === tabID;
        button.setAttribute('aria-selected', String(active));
        button.tabIndex = active ? 0 : -1;
      });
      document.querySelectorAll('[data-graph-panel]').forEach((panel) => {
        panel.hidden = panel.dataset.graphPanel !== tabID;
      });
      if (tabID === 'tokens') {
        setTokenChartMode('total');
      } else if (tabID === 'credits') {
        const payload = parseChartPayload('credit-history-chart-data');
        if (payload) {
          renderUsageChart('credit', 'credit-history-chart', payload, payload.series);
        }
      }
    }

    function activateSectionTab(tabID) {
      document.querySelectorAll('[data-section-tab]').forEach((button) => {
        const active = button.dataset.sectionTab === tabID;
        button.setAttribute('aria-selected', String(active));
        button.tabIndex = active ? 0 : -1;
      });
      document.querySelectorAll('[data-section-panel]').forEach((panel) => {
        panel.hidden = panel.dataset.sectionPanel !== tabID;
      });
      if (tabID === 'usage') {
        activateGraphTab('tokens');
      }
    }

    function handleGraphTabKeydown(event) {
      const tabs = Array.from(document.querySelectorAll('[data-graph-tab]'));
      const index = tabs.indexOf(event.currentTarget);
      if (index === -1) {
        return;
      }
      let next = index;
      switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        next = (index + 1) % tabs.length;
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        next = (index - 1 + tabs.length) % tabs.length;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = tabs.length - 1;
        break;
      case 'Enter':
      case ' ':
        activateGraphTab(event.currentTarget.dataset.graphTab);
        event.preventDefault();
        return;
      default:
        return;
      }
      tabs[next].focus();
      event.preventDefault();
    }

    function handleSectionTabKeydown(event) {
      const tabs = Array.from(document.querySelectorAll('[data-section-tab]'));
      const index = tabs.indexOf(event.currentTarget);
      if (index === -1) {
        return;
      }
      let next = index;
      switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        next = (index + 1) % tabs.length;
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        next = (index - 1 + tabs.length) % tabs.length;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = tabs.length - 1;
        break;
      case 'Enter':
      case ' ':
        activateSectionTab(event.currentTarget.dataset.sectionTab);
        event.preventDefault();
        return;
      default:
        return;
      }
      tabs[next].focus();
      event.preventDefault();
    }

    function initializeUsageCharts() {
      destroyUsageCharts();
      document.querySelectorAll('[data-section-tab]').forEach((button) => {
        button.addEventListener('click', function() {
          activateSectionTab(button.dataset.sectionTab);
        });
        button.addEventListener('keydown', handleSectionTabKeydown);
      });
      document.querySelectorAll('[data-graph-tab]').forEach((button) => {
        button.addEventListener('click', function() {
          activateGraphTab(button.dataset.graphTab);
        });
        button.addEventListener('keydown', handleGraphTabKeydown);
      });
      document.querySelectorAll('[data-token-mode]').forEach((button) => {
        button.addEventListener('click', function() {
          setTokenChartMode(button.dataset.tokenMode);
        });
      });
      activateSectionTab('projects');
    }

    if (!window.codexSpendMonitorDashboardEventsBound) {
      document.addEventListener('DOMContentLoaded', initializeUsageCharts);
      document.addEventListener('htmx:afterSwap', initializeUsageCharts);
      window.codexSpendMonitorDashboardEventsBound = true;
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

    <div class="section-tabs-wrap">
      <div class="section-tabs" role="tablist" aria-label="Dashboard sections">
        <button class="section-tab" type="button" role="tab" id="section-tab-projects" aria-selected="true" aria-controls="section-panel-projects" tabindex="0" data-section-tab="projects">Projects</button>
        <button class="section-tab" type="button" role="tab" id="section-tab-usage" aria-selected="false" aria-controls="section-panel-usage" tabindex="-1" data-section-tab="usage">Usage Over Time</button>
      </div>
    </div>

    <section class="section-panel" id="section-panel-projects" role="tabpanel" aria-labelledby="section-tab-projects" data-section-panel="projects">
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
    </section>

    <section class="section-panel" id="section-panel-usage" role="tabpanel" aria-labelledby="section-tab-usage" data-section-panel="usage" hidden>
      <h2>Usage Over Time</h2>
      <section class="tile chart-card">
        <div class="chart-tabs-wrap">
          <div class="chart-tabs" role="tablist" aria-label="Usage graphs">
            <button class="chart-tab" type="button" role="tab" id="graph-tab-tokens" aria-selected="true" aria-controls="graph-panel-tokens" tabindex="0" data-graph-tab="tokens">Tokens</button>
            <button class="chart-tab" type="button" role="tab" id="graph-tab-credits" aria-selected="false" aria-controls="graph-panel-credits" tabindex="-1" data-graph-tab="credits">Credits</button>
          </div>
        </div>

        <section class="chart-panel" id="graph-panel-tokens" role="tabpanel" aria-labelledby="graph-tab-tokens" data-graph-panel="tokens">
          <div class="chart-toolbar">
            <div class="chart-head">
              <strong>{{.TokenChart.Title}}</strong>
              <span class="meta">{{.TokenChart.Subtitle}}</span>
            </div>
            {{if not .TokenChart.Empty}}
            <div class="chart-toggle" aria-label="Token chart mode">
              <button type="button" aria-pressed="true" data-token-mode="total" id="token-chart-mode-total">Model total</button>
              <button type="button" aria-pressed="false" data-token-mode="split" id="token-chart-mode-split">Type split</button>
            </div>
            {{end}}
          </div>
          {{if .TokenChart.Empty}}
          <div class="meta chart-empty">No token activity in the last 30 days.</div>
          {{else}}
          <div id="{{.TokenChart.ID}}" class="chart-shell"></div>
          <script type="application/json" id="{{.TokenChart.DataID}}">{{.TokenChart.DataJSON}}</script>
          {{end}}
        </section>

        <section class="chart-panel" id="graph-panel-credits" role="tabpanel" aria-labelledby="graph-tab-credits" data-graph-panel="credits" hidden>
          <div class="chart-head">
            <strong>{{.CreditChart.Title}}</strong>
            <span class="meta">{{.CreditChart.Subtitle}}</span>
          </div>
          {{if .CreditChart.Empty}}
          <div class="meta chart-empty">No priced model usage in the last 30 days.</div>
          {{else}}
          <div id="{{.CreditChart.ID}}" class="chart-shell"></div>
          <script type="application/json" id="{{.CreditChart.DataID}}">{{.CreditChart.DataJSON}}</script>
          {{end}}
        </section>
      </section>
    </section>
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
