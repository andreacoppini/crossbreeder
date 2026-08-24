'use strict';

const $ = (id) => document.getElementById(id);
const rowsEl = $('rows');

// One row per address, keyed by IP, so live updates land in place.
const rows = new Map();
let order = [];
let sortKey = null, sortDir = 1;
let filterText = '', statusFilter = null;
let running = false;

/* ---------- settings ---------- */

// The form is seeded from the flags the process was started with, so the
// console and the command line agree on what "default" means.
const FIELDS = ['user', 'concurrency', 'probe', 'pingTimeoutMs', 'pingRetries', 'pingConcurrency',
  'sshPort', 'timeoutS', 'legacy', 'fwProto', 'fwPort', 'servePort', 'serveWaitS', 'serveDir'];

async function loadDefaults() {
  const d = await (await fetch('/api/defaults')).json();
  $('ver').textContent = d.version || '';
  for (const k of FIELDS) {
    const el = $(k);
    if (!el || d[k] === undefined || d[k] === null || d[k] === '') continue;
    if (el.type === 'checkbox') el.checked = !!d[k]; else el.value = d[k];
  }
  restore();
}

// Everything except the passwords is remembered between sessions.
const SAVE = [...FIELDS, 'alsoDefault', 'firmware', 'factory', 'reboot', 'command',
  'serve', 'serveIp', 'fwFile', 'fwHost', 'fwUser', 'hosts'];

function persist() {
  const s = {};
  for (const k of SAVE) {
    const el = $(k);
    if (el) s[k] = el.type === 'checkbox' ? el.checked : el.value;
  }
  try { localStorage.setItem('crossbreeder', JSON.stringify(s)); } catch (e) { /* private mode */ }
  // The password is deliberately not in SAVE: it must not survive on disk. It
  // is held for this browser session only, so a reload mid-job does not empty
  // the field without the operator noticing.
  try { sessionStorage.setItem('cb-pass', $('pass').value); } catch (e) { /* ignore */ }
}

function restore() {
  let s;
  try { s = JSON.parse(localStorage.getItem('crossbreeder') || '{}'); } catch (e) { return; }
  for (const [k, v] of Object.entries(s)) {
    const el = $(k);
    if (!el) continue;
    if (el.type === 'checkbox') el.checked = v; else if (v !== '') el.value = v;
  }
  try {
    const p = sessionStorage.getItem('cb-pass');
    if (p) $('pass').value = p;
  } catch (e) { /* ignore */ }
  hostsChanged();
  actionsChanged();
  credsChanged();
}

/* ---------- targets ---------- */

function hostList() {
  return $('hosts').value.split('\n')
    .map((l) => l.split(',')[0].trim().replace(/^"|"$/g, '').replace(/^﻿/, ''))
    .filter((s) => /^\d{1,3}(\.\d{1,3}){3}$/.test(s));
}

function hostsChanged() {
  const n = hostList().length;
  const lines = $('hosts').value.split('\n').filter((l) => l.trim()).length;
  $('nHosts').textContent = n ? `(${n})` : '';
  $('hostHint').textContent = !lines ? 'No addresses yet.'
    : n === lines ? `${n} address${n === 1 ? '' : 'es'}.`
    : `${n} of ${lines} lines are addresses; the rest are ignored.`;
  persist();
}

$('hosts').addEventListener('input', hostsChanged);
$('pick').onclick = () => $('file').click();
$('clear').onclick = () => { $('hosts').value = ''; hostsChanged(); };
$('file').onchange = (e) => {
  const f = e.target.files[0];
  if (!f) return;
  const r = new FileReader();
  r.onload = () => { $('hosts').value = r.result; hostsChanged(); };
  r.readAsText(f);
};

/* ---------- actions ---------- */

function destructiveList() {
  const out = [];
  if ($('firmware').checked) out.push('push new firmware');
  if ($('factory').checked) out.push('factory reset (which forces a reboot)');
  if ($('reboot').checked && !$('factory').checked) out.push('reboot');
  if ($('command').value.trim()) out.push(`run: ${$('command').value.trim()}`);
  return out;
}

function actionsChanged() {
  const d = destructiveList();
  const el = $('destructive');
  el.hidden = d.length === 0;
  el.textContent = d.length ? 'This run changes the APs. You will be asked to confirm.' : '';
  $('fwSection').classList.toggle('collapsed', !$('firmware').checked);
  persist();
}

for (const id of ['firmware', 'factory', 'reboot', 'command']) {
  $(id).addEventListener('change', actionsChanged);
  $(id).addEventListener('input', actionsChanged);
}
for (const k of SAVE) $(k)?.addEventListener('change', persist);
$('pass').addEventListener('input', persist);
$('user').addEventListener('input', credsChanged);
$('pass').addEventListener('input', credsChanged);
$('alsoDefault').addEventListener('change', credsChanged);

// Show what will actually be tried, so a blank field cannot turn into a
// puzzling "Login Failed" from the AP.
function credsChanged() {
  const u = $('user').value.trim(), p = $('pass').value;
  const el = $('credHint');
  if (!u && !p) {
    el.className = 'hint';
    el.textContent = 'No username: the factory default super / sp-admin will be tried.';
  } else if (u && !p) {
    el.className = 'hint warn';
    el.textContent = `No password for "${u}". Passwords are not kept after the browser closes — re-enter it.`;
  } else if (!u && p) {
    el.className = 'hint warn';
    el.textContent = 'A password with no username cannot be used. Enter the username.';
  } else {
    el.className = 'hint';
    el.textContent = `Will try "${u}" (${p.length}-character password)` +
      ($('alsoDefault').checked ? ', then super / sp-admin.' : '.');
  }
}

// Collapsible config sections.
document.querySelectorAll('section > h2').forEach((h) => {
  h.onclick = () => h.parentElement.classList.toggle('collapsed');
});

/* ---------- the table ---------- */

function statusClass(s) {
  if (s === 'Done') return 'st-done';
  if (s === 'Running') return 'st-run';
  if (/Fail|Error/i.test(s)) return 'st-fail';
  return 'st-skip';
}

function upsert(r) {
  rows.set(r.ip, r);
  if (!order.includes(r.ip)) order.push(r.ip);
}

function visible() {
  let list = order.map((ip) => rows.get(ip));
  if (statusFilter) list = list.filter((r) => (statusFilter === 'failed'
    ? /Fail|Error/i.test(r.status) : r.status === statusFilter));
  if (filterText) {
    const q = filterText.toLowerCase();
    list = list.filter((r) => [r.ip, r.mac, r.model, r.firmware, r.status, r.error, r.fw]
      .some((v) => (v || '').toLowerCase().includes(q)));
  }
  if (sortKey) {
    list = list.slice().sort((a, b) => {
      const x = a[sortKey] ?? '', y = b[sortKey] ?? '';
      if (sortKey === 'ping') return (parseFloat(x) || 1e9) < (parseFloat(y) || 1e9) ? -sortDir : sortDir;
      if (sortKey === 'ip') return cmpIP(a.ip, b.ip) * sortDir;
      return String(x).localeCompare(String(y)) * sortDir;
    });
  }
  return list;
}

const cmpIP = (a, b) => {
  const p = (s) => s.split('.').reduce((n, o) => n * 256 + (+o), 0);
  return p(a) - p(b);
};

let selected = null;

function render() {
  const list = visible();
  $('tableEmpty').hidden = list.length > 0;
  $('shown').textContent = list.length === order.length
    ? `${order.length} rows` : `${list.length} of ${order.length} rows`;

  const frag = document.createDocumentFragment();
  for (const r of list) {
    const tr = document.createElement('tr');
    if (r.ip === selected) tr.className = 'sel';
    tr.onclick = () => showTranscript(r.ip);
    tr.innerHTML =
      `<td>${esc(r.ip)}</td><td>${esc(r.mac)}</td><td>${esc(r.model)}</td><td>${esc(r.firmware)}</td>` +
      `<td class="num">${r.reachable ? esc(r.ping) : '—'}</td>` +
      `<td><span class="st ${statusClass(r.status)}">${esc(r.status)}</span></td>` +
      `<td>${esc(r.fw)}</td><td class="err">${esc(r.error)}</td>`;
    frag.appendChild(tr);
  }
  rowsEl.replaceChildren(frag);
  updateChips();
}

const esc = (s) => (s == null ? '' : String(s)
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'));

document.querySelectorAll('thead th').forEach((th) => {
  th.onclick = () => {
    const k = th.dataset.k;
    sortDir = sortKey === k ? -sortDir : 1;
    sortKey = k;
    render();
  };
});

$('filter').addEventListener('input', (e) => { filterText = e.target.value; render(); });

function updateChips() {
  const counts = new Map();
  for (const r of rows.values()) counts.set(r.status, (counts.get(r.status) || 0) + 1);
  const el = $('chips');
  el.replaceChildren();
  for (const [status, n] of [...counts].sort((a, b) => b[1] - a[1])) {
    const c = document.createElement('div');
    c.className = 'chip' + (statusFilter === status ? ' on' : '');
    c.textContent = `${status} ${n}`;
    c.onclick = () => { statusFilter = statusFilter === status ? null : status; render(); };
    el.appendChild(c);
  }
}

/* ---------- drawer ---------- */

document.querySelectorAll('.tab').forEach((t) => {
  t.onclick = () => {
    document.querySelectorAll('.tab').forEach((x) => x.classList.toggle('on', x === t));
    for (const [p, id] of [['log', 'pLog'], ['sweep', 'pSweep'], ['xfer', 'pXfer'], ['tx', 'pTx']]) {
      $(id).hidden = p !== t.dataset.p;
    }
  };
});
$('grow').onclick = () => {
  const d = $('drawer');
  d.classList.toggle('big');
  $('grow').textContent = d.classList.contains('big') ? 'Shrink' : 'Expand';
};

function logLine(pane, text) {
  const el = $(pane);
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
  const t = new Date().toLocaleTimeString();
  el.insertAdjacentHTML('beforeend', `<span class="t">${t}</span>  ${esc(text)}\n`);
  if (atBottom) el.scrollTop = el.scrollHeight;
}

function showTranscript(ip) {
  selected = ip;
  const r = rows.get(ip);
  $('txWho').textContent = `(${ip})`;
  $('pTx').textContent = r && r.transcript ? r.transcript
    : 'No session recorded for this address.';
  document.querySelector('.tab[data-p=tx]').click();
  render();
}

/* ---------- run ---------- */

function request() {
  const num = (id) => parseInt($(id).value, 10) || 0;
  return {
    hosts: hostList(),
    user: $('user').value, pass: $('pass').value, alsoDefault: $('alsoDefault').checked,
    concurrency: num('concurrency'),
    probe: $('probe').value,
    pingTimeoutMs: num('pingTimeoutMs'), pingRetries: num('pingRetries'),
    pingConcurrency: num('pingConcurrency'),
    firmware: $('firmware').checked, factory: $('factory').checked,
    reboot: $('reboot').checked, command: $('command').value.trim(),
    serve: $('firmware').checked && $('serve').checked,
    serveDir: $('serveDir').value, serveIp: $('serveIp').value, servePort: num('servePort'),
    fwProto: $('fwProto').value, fwHost: $('fwHost').value, fwPort: $('fwPort').value,
    fwUser: $('fwUser').value, fwPass: $('fwPass').value, fwFile: $('fwFile').value,
    sshPort: $('sshPort').value, timeoutS: num('timeoutS'), legacy: $('legacy').checked,
    serveWaitS: num('serveWaitS'),
  };
}

$('run').onclick = () => {
  const hosts = hostList();
  if (!hosts.length) return toast('No addresses to work on.');
  const u = $('user').value.trim(), pw = $('pass').value;
  if (u && !pw) {
    $('pass').focus();
    return toast(`No password for "${u}" — it is not kept after the browser closes.`);
  }
  if (!u && pw) {
    $('user').focus();
    return toast('A password with no username cannot be used.');
  }
  const d = destructiveList();
  if (!d.length) return start();
  $('cN').textContent = hosts.length;
  $('cList').replaceChildren(...d.map((x) => { const li = document.createElement('li'); li.textContent = x; return li; }));
  $('confirm').hidden = false;
};
$('cCancel').onclick = () => { $('confirm').hidden = true; };
$('cGo').onclick = () => { $('confirm').hidden = true; start(); };

async function start() {
  rows.clear(); order = []; selected = null;
  // Show every address up front, so the grid is the full worklist from the
  // first second rather than filling in as results trickle back.
  for (const ip of hostList()) {
    upsert({ ip, mac: '', model: '', firmware: '', ping: '', reachable: false,
      status: 'Queued', fw: '', error: '', transcript: '' });
  }
  for (const p of ['pLog', 'pSweep', 'pXfer']) $(p).replaceChildren();
  $('pTx').textContent = 'Select an AP to see its session.';
  $('nDead').textContent = ''; $('nXfer').textContent = '';
  nDone = 0; nFail = 0; nXfer = 0;
  setCount('cTotal', hostList().length); setCount('cAlive', 0);
  setCount('cDone', 0); setCount('cFail', 0);
  render();

  const res = await fetch('/api/run', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request()),
  });
  if (!res.ok) {
    const e = await res.json().catch(() => ({ error: 'could not start' }));
    return toast(e.error);
  }
  setRunning(true);
}

$('stop').onclick = () => fetch('/api/stop', { method: 'POST' });
$('expCsv').onclick = () => { location.href = '/api/export?kind=csv'; };
$('expJson').onclick = () => { location.href = '/api/export?kind=json'; };

function setRunning(v) {
  running = v;
  $('run').disabled = v;
  $('stop').disabled = !v;
  if (!v) { $('bar').style.width = '0%'; }
}

const setCount = (id, n) => { $(id).textContent = n; };

function toast(msg) {
  const t = $('toast');
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => { t.hidden = true; }, 4000);
}

/* ---------- event stream ---------- */

let nFail = 0, nDone = 0, nXfer = 0;

const src = new EventSource('/api/events');
src.onmessage = (m) => {
  const e = JSON.parse(m.data);
  switch (e.kind) {
    case 'log':
      logLine('pLog', e.message);
      break;

    case 'phase':
      $('phase').textContent =
        e.phase === 'sweep' ? `probing ${e.total}` :
        e.phase === 'ssh' ? `connecting to ${e.total}` :
        e.phase === 'download' ? `${e.total} downloading` : e.phase;
      setRunning(true);
      break;

    case 'progress':
      if (e.total) $('bar').style.width = `${(e.done / e.total) * 100}%`;
      if (e.phase === 'download') $('phase').textContent = `downloaded ${e.done}/${e.total}`;
      break;

    case 'sweep':
      setCount('cAlive', e.done);
      for (const ip of e.dead || []) {
        const r = rows.get(ip);
        if (r) { r.status = e.message || 'No reply'; }
      }
      render();
      $('nDead').textContent = e.dead && e.dead.length ? `(${e.dead.length})` : '';
      if (e.dead && e.dead.length) $('pSweep').textContent = e.dead.join('\n');
      else $('pSweep').innerHTML = '<span class="empty">Everything answered.</span>';
      break;

    case 'result': {
      const r = e.result;
      upsert({
        ip: r.ip, mac: r.mac || '', model: r.model || '', firmware: r.firmware || '',
        ping: r.reachable ? (r.ping_ms || 0).toFixed(1) : '', reachable: r.reachable,
        status: r.status, fw: r.fw_status || '', error: r.error || '',
        transcript: e.transcript || '',
      });
      if (r.status === 'Done') nDone++; else if (/Fail|Error/i.test(r.status)) nFail++;
      setCount('cDone', nDone); setCount('cFail', nFail);
      if (e.total) $('bar').style.width = `${(e.done / e.total) * 100}%`;
      render();
      break;
    }

    case 'server':
      logLine('pLog', `Serving ${e.server.dir} on http://${e.server.addr}`);
      logLine('pLog', `  pushing ${e.server.file}`);
      if (e.server.reason) logLine('pLog', `  address chosen: ${e.server.reason}`);
      break;

    case 'transfer':
      nXfer++;
      $('nXfer').textContent = `(${nXfer})`;
      logLine('pXfer', e.message.trim());
      break;

    case 'done':
      $('phase').textContent = `finished in ${e.elapsed}`;
      $('bar').style.width = '100%';
      setRunning(false);
      break;

    case 'error':
      toast(e.message);
      logLine('pLog', `ERROR: ${e.message}`);
      $('phase').textContent = 'failed';
      setRunning(false);
      break;
  }
};

// A run started before this tab opened is still streaming; reflect that.
fetch('/api/state').then((r) => r.json()).then((s) => setRunning(s.running)).catch(() => {});

loadDefaults();
