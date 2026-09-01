// The dashboard reads the same API anything else would, so there is nothing
// here that the JSON does not also say.
'use strict';

const $ = (id) => document.getElementById(id);
const state = { network: null, latest: {}, results: [] };

function text(el, s) { el.textContent = s; }

function statusOf(result) {
  let worst = 'ok';
  const rank = { ok: 1, warn: 2, fail: 3, skipped: 0 };
  for (const m of result.measurements || []) {
    if (rank[m.status] > rank[worst]) worst = m.status;
  }
  return worst;
}

function health(score) {
  if (score >= 90) return 'ok';
  if (score >= 70) return 'warn';
  return 'fail';
}

function ago(iso) {
  const seconds = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (seconds < 90) return `${Math.round(seconds)}s ago`;
  if (seconds < 5400) return `${Math.round(seconds / 60)} min ago`;
  return `${Math.round(seconds / 3600)} h ago`;
}

function fmt(value, unit) {
  if (!unit) return value ? String(Math.round(value)) : '';
  if (unit === 'ms') {
    if (value >= 1000) return `${(value / 1000).toFixed(2)} s`;
    return value < 10 ? `${value.toFixed(1)} ms` : `${Math.round(value)} ms`;
  }
  if (unit === 'MOS') return value.toFixed(2);
  if (unit === 'Mbps') return `${value.toFixed(1)} Mbps`;
  return `${Math.round(value)} ${unit}`;
}

function log(line) {
  const el = $('log');
  el.textContent = `${new Date().toLocaleTimeString()}  ${line}\n${el.textContent}`.slice(0, 20000);
}

async function api(path, options) {
  const response = await fetch(path, options);
  if (!response.ok) throw new Error(`${path}: ${response.status} ${await response.text()}`);
  return response.headers.get('content-type')?.includes('json') ? response.json() : response.text();
}

// ---- rendering ----

function drawNetworks(networks) {
  const box = $('networks');
  box.innerHTML = '';
  for (const n of networks) {
    const result = state.latest[n.name];
    const button = document.createElement('button');
    button.className = n.name === state.network ? 'on' : '';
    const colour = result ? `var(--${health(result.overall)})` : 'var(--muted)';
    button.innerHTML = `<span class="dot" style="background:${colour}"></span>${n.name}` +
      `<span class="sub">${n.ssid || n.kind}${result ? ` · ${result.overall}/100 · ${ago(result.start)}` : ' · no pass yet'}</span>`;
    button.onclick = () => { state.network = n.name; draw(); };
    box.appendChild(button);
  }
}

function drawIssues(issues) {
  const box = $('issues');
  text($('cIssues'), issues.length);
  if (!issues.length) {
    box.innerHTML = '<p class="muted">Nothing is wrong.</p>';
    return;
  }
  box.innerHTML = '';
  for (const i of issues) {
    const div = document.createElement('div');
    div.className = `issue ${i.severity}`;
    div.innerHTML = `${i.root_cause ? '<span class="root">root cause</span>' : ''}` +
      `<b>${i.network}: ${i.title}</b><span>${i.detail || ''}</span>`;
    box.appendChild(div);
  }
}

function drawCards(result) {
  const cards = $('cards');
  cards.innerHTML = '';
  if (!result) {
    cards.innerHTML = '<p class="muted">No pass has finished on this network yet.</p>';
    return;
  }
  const add = (label, value, cls, note) => {
    const div = document.createElement('div');
    div.className = `card ${cls || ''}`;
    div.innerHTML = `<i>${label}</i><b>${value}</b>${note ? `<small>${note}</small>` : ''}`;
    cards.appendChild(div);
  };
  add('health', `${result.overall}`, health(result.overall), `${ago(result.start)}, took ${(result.duration_ns / 1e9).toFixed(1)}s`);
  for (const service of ['wireless', 'dhcp', 'dns', 'internet', 'applications', 'voice']) {
    if (result.scores && result.scores[service] !== undefined) {
      add(service, result.scores[service], health(result.scores[service]));
    }
  }
  if (result.radio) {
    const r = result.radio;
    add('signal', `${r.rssi} dBm`, r.rssi > -70 ? 'ok' : (r.rssi > -80 ? 'warn' : 'fail'),
      `ch ${r.channel} ${r.band}${r.snr ? `, SNR ${r.snr} dB` : ''}`);
    if (r.neighbours) add('neighbours', r.neighbours, '', `${r.co_channel} co-channel, ${r.overlapping} overlapping`);
  }
  if (result.lease) add('address', result.lease.address, '', `via ${result.lease.server || 'DHCP'}`);
  if (result.switch) add('switch port', '', '', result.switch);
}

function drawTests(result) {
  const body = $('tests');
  body.innerHTML = '';
  if (!result) return;
  for (const m of result.measurements) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td class="muted">${m.service}</td><td>${m.test}</td>` +
      `<td><span class="pill ${m.status}">${m.status}</span></td>` +
      `<td class="num">${fmt(m.value, m.unit)}</td>` +
      `<td class="muted">${m.error || m.detail || ''}</td>`;
    body.appendChild(tr);
  }
}

function drawHistory() {
  const body = $('history');
  body.innerHTML = '';
  const rows = state.results.slice().reverse().slice(0, 200);
  for (const r of rows) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td class="muted">${new Date(r.start).toLocaleString()}</td><td>${r.network}</td>` +
      `<td><span class="pill ${health(r.overall)}">${r.overall}</span></td>` +
      `<td><span class="pill ${statusOf(r)}">${statusOf(r)}</span></td>` +
      `<td class="num">${(r.duration_ns / 1e9).toFixed(1)}s</td>`;
    body.appendChild(tr);
  }
  drawChart(state.results.map((r) => ({ at: r.start, value: r.overall })));
}

// A small line chart, drawn by hand: one canvas beats carrying a charting
// library onto a sensor that has to work on a closed network.
function drawChart(points) {
  const canvas = $('chart');
  const width = canvas.clientWidth || 600;
  const height = canvas.height;
  canvas.width = width;
  const ctx = canvas.getContext('2d');
  ctx.clearRect(0, 0, width, height);
  if (points.length < 2) return;

  const pad = 24;
  const x = (i) => pad + (i * (width - pad * 2)) / (points.length - 1);
  const y = (v) => height - pad - (Math.max(0, Math.min(100, v)) / 100) * (height - pad * 2);

  ctx.strokeStyle = '#262e39';
  ctx.lineWidth = 1;
  for (const level of [0, 50, 100]) {
    ctx.beginPath();
    ctx.moveTo(pad, y(level));
    ctx.lineTo(width - pad, y(level));
    ctx.stroke();
    ctx.fillStyle = '#8794a5';
    ctx.font = '10px system-ui';
    ctx.fillText(String(level), 4, y(level) + 3);
  }
  ctx.strokeStyle = '#4da3ff';
  ctx.lineWidth = 2;
  ctx.beginPath();
  points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.value)) : ctx.moveTo(x(i), y(p.value))));
  ctx.stroke();
}

function draw() {
  const networks = state.networks || [];
  if (!state.network && networks.length) state.network = networks[0].name;
  drawNetworks(networks);
  const result = state.latest[state.network];
  drawCards(result);
  drawTests(result);
  drawHistory();
  const scores = Object.values(state.latest).map((r) => r.overall);
  text($('cScore'), scores.length ? Math.min(...scores) : '–');
}

// ---- loading ----

async function refresh() {
  const [status, latest, issues] = await Promise.all([
    api('/api/state'), api('/api/latest'), api('/api/issues'),
  ]);
  text($('ver'), status.version || '');
  text($('sensorName'), status.sensor);
  text($('sensorSite'), [status.site, status.group].filter(Boolean).join(' · '));
  text($('phase'), status.state.running
    ? `testing ${status.state.current}`
    : status.state.next ? `next pass ${new Date(status.state.next).toLocaleTimeString()}` : 'idle');
  state.networks = status.networks || [];
  state.latest = {};
  for (const r of latest) state.latest[r.network] = r;
  drawIssues(issues);
  state.results = await api(`/api/results?from=-24h&network=${encodeURIComponent(state.network || '')}`);
  draw();
}

function listen() {
  const events = new EventSource('/api/events');
  events.addEventListener('pass', (e) => {
    const result = JSON.parse(e.data);
    state.latest[result.network] = result;
    if (result.network === state.network) state.results.push(result);
    log(`${result.network}: ${statusOf(result)}, health ${result.overall}`);
    draw();
    refresh().catch(() => {});
  });
  events.onerror = () => text($('phase'), 'reconnecting…');
}

// ---- controls ----

$('run').onclick = async () => {
  await api('/api/run', { method: 'POST' });
  log('a pass has been asked for');
};
$('exportCsv').onclick = () => {
  window.location = `/api/export?from=-7d&network=${encodeURIComponent(state.network || '')}`;
};
$('scan').onclick = async () => {
  showTab('tools');
  text($('toolout'), 'scanning…');
  try {
    const out = await api('/api/scan');
    const lines = out.radios.map((b) =>
      `${b.Signal.toString().padStart(4)} dBm  ch ${String(b.Channel).padStart(3)} ${b.Band.padEnd(8)} ${b.Security.padEnd(16)} ${b.BSSID}  ${b.SSID || '(hidden)'}`);
    const survey = (out.survey || []).filter((s) => s.InUse)
      .map((s) => `channel ${s.Channel}: ${Math.round(100 * s.BusyMs / Math.max(1, s.ActiveMs))}% of the air time in use, noise ${s.Noise} dBm`);
    text($('toolout'), [...survey, '', ...lines].join('\n'));
  } catch (err) { text($('toolout'), String(err)); }
};
$('trace').onclick = async () => {
  showTab('tools');
  const target = $('traceTarget').value.trim() || '1.1.1.1';
  text($('toolout'), `tracing ${target}…`);
  try {
    const out = await api(`/api/traceroute?target=${encodeURIComponent(target)}`);
    if (out.Err) { text($('toolout'), out.Err); return; }
    text($('toolout'), (out.Hops || []).map((h) =>
      `${String(h.TTL).padStart(2)}  ${h.Timeout ? '*' : `${h.Addr}${h.Name ? ` (${h.Name})` : ''}  ${(h.RTT / 1e6).toFixed(1)} ms`}`).join('\n'));
  } catch (err) { text($('toolout'), String(err)); }
};
$('capture').onclick = () => {
  const iface = encodeURIComponent($('capIface').value.trim());
  const seconds = encodeURIComponent($('capSeconds').value || 30);
  log(`capturing for ${seconds}s — the download starts as soon as the first packet arrives`);
  window.location = `/api/capture?interface=${iface}&seconds=${seconds}`;
};

function showTab(name) {
  for (const tab of document.querySelectorAll('.tab')) {
    tab.classList.toggle('on', tab.dataset.tab === name);
    $(`tab-${tab.dataset.tab}`).hidden = tab.dataset.tab !== name;
  }
}
for (const tab of document.querySelectorAll('.tab')) {
  tab.onclick = () => showTab(tab.dataset.tab);
}

refresh().then(listen).catch((err) => log(String(err)));
setInterval(() => refresh().catch(() => {}), 30000);
