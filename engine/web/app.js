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
  'sshPort', 'timeoutS', 'legacy', 'fwProto', 'fwPort', 'servePort', 'serveWaitS', 'serveDir',
  'watchIntervalS'];

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
  'srvMode', 'serveIp', 'fwFile', 'fwHost', 'fwUser', 'hosts', 'watch'];

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
  try {
    sessionStorage.setItem('cb-pass', $('pass').value);
    sessionStorage.setItem('cb-newpass', $('newPass').value);
  } catch (e) { /* ignore */ }
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
    const np = sessionStorage.getItem('cb-newpass');
    if (np) $('newPass').value = np;
  } catch (e) { /* ignore */ }
  hostsChanged();
  actionsChanged();
  credsChanged();
  restoreMode();
  pollServer();
}

/* ---------- targets ---------- */

function hostList() {
  return $('hosts').value.split('\n')
    .map((l) => l.split(',')[0].trim().replace(/^"|"$/g, '').replace(/^﻿/, ''))
    .filter((s) => /^\d{1,3}(\.\d{1,3}){3}$/.test(s));
}

let ipTimer = null;
function hostsChanged() {
  clearTimeout(ipTimer);
  ipTimer = setTimeout(() => { if (internalMode()) refreshIPs(); }, 400);
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
  $('watchOpts').hidden = !$('watch').checked;
  const wh = $('watchHint');
  wh.className = 'hint';
  wh.textContent = $('watch').checked
    ? 'The first pass does the above; after that it keeps pinging and re-reading the version until you press Stop.'
    : 'The run finishes after one pass.';
  $('fwSection').classList.toggle('collapsed', !$('firmware').checked);
  if ($('firmware').checked && internalMode()) { refreshFirmware(); refreshIPs(); }
  persist();
}

for (const id of ['firmware', 'factory', 'reboot', 'command', 'watch']) {
  $(id).addEventListener('change', actionsChanged);
  $(id).addEventListener('input', actionsChanged);
}
for (const k of SAVE) $(k)?.addEventListener('change', persist);
$('pass').addEventListener('input', persist);
$('newPass').addEventListener('input', persist);
$('newPass').addEventListener('input', credsChanged);
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
      ($('alsoDefault').checked ? ', then super / sp-admin.' : '.') +
      ($('newPass').value ? ' An AP demanding a password change will be set to the new password.' : '');
  }
}

// Collapsible config sections.
document.querySelectorAll('section > h2').forEach((h) => {
  h.onclick = () => h.parentElement.classList.toggle('collapsed');
});

/* ---------- firmware server ---------- */

const internalMode = () => $('modeInternal').checked;

function srvModeChanged() {
  const internal = internalMode();
  $('internalOpts').hidden = !internal;
  $('externalOpts').hidden = internal;
  persistMode();
  if (internal) { refreshFirmware(); refreshIPs(); }
}
$('modeInternal').onchange = srvModeChanged;
$('modeExternal').onchange = srvModeChanged;

function persistMode() {
  try { localStorage.setItem('cb-mode', internalMode() ? 'internal' : 'external'); } catch (e) { /* ignore */ }
}

function restoreMode() {
  let m = 'internal';
  try { m = localStorage.getItem('cb-mode') || 'internal'; } catch (e) { /* ignore */ }
  $(m === 'external' ? 'modeExternal' : 'modeInternal').checked = true;
  srvModeChanged();
}

// Show the file that would actually be sent, rather than promising to pick one.
async function refreshFirmware() {
  const dir = $('serveDir').value;
  const sel = $('fwFileSel');
  const hint = $('fwFileHint');
  try {
    const r = await (await fetch('/api/firmware?dir=' + encodeURIComponent(dir))).json();
    const chosen = sel.value;
    sel.replaceChildren();
    for (const c of r.candidates || []) {
      const o = document.createElement('option');
      o.value = c; o.textContent = c;
      sel.appendChild(o);
    }
    if (r.error) {
      hint.className = 'hint warn';
      hint.textContent = r.error;
      if (!r.candidates?.length) sel.replaceChildren(new Option('(nothing to push)', ''));
      return;
    }
    sel.value = (r.candidates || []).includes(chosen) ? chosen : r.picked;
    const auto = sel.value === r.picked;
    const warn = /\.rcks$/i.test(sel.value) ? '' : (r.warn || '');
    hint.className = warn ? 'hint warn' : 'hint';
    hint.textContent = (auto ? `Picked automatically: ${r.reason}. ` : 'Chosen manually. ') + warn;
  } catch (e) {
    hint.className = 'hint warn';
    hint.textContent = 'Could not read that folder.';
  }
}

async function refreshIPs() {
  const sel = $('serveIp');
  const keep = sel.value;
  try {
    const r = await (await fetch('/api/ips?hosts=' + encodeURIComponent($('hosts').value))).json();
    sel.replaceChildren(new Option('Automatic', ''));
    for (const ip of r.ips || []) sel.appendChild(new Option(ip.label, ip.ip));
    sel.value = keep;
  } catch (e) { /* leave the automatic option alone */ }
}

$('serveDir').addEventListener('change', () => { refreshFirmware(); persist(); });

/* ---- folder picker ---- */

let pkCurrent = '';

async function openPicker(path) {
  const r = await (await fetch('/api/browse?path=' + encodeURIComponent(path || $('serveDir').value))).json();
  if (r.error) return toast(r.error);
  pkCurrent = r.path;
  $('pkPath').value = r.path;
  $('pkUp').disabled = !r.parent;
  $('pkUp').dataset.path = r.parent || '';

  $('pkRoots').replaceChildren(...(r.roots || []).map((d) => {
    const c = document.createElement('div');
    c.className = 'chip';
    c.textContent = d.name;
    c.onclick = () => openPicker(d.path);
    return c;
  }));

  $('pkList').replaceChildren(...(r.dirs || []).map((d) => {
    const el = document.createElement('div');
    el.textContent = '\u{1F4C1}  ' + d.name;
    el.onclick = () => openPicker(d.path);
    return el;
  }));
  if (!r.dirs?.length) {
    const el = document.createElement('div');
    el.className = 'empty';
    el.textContent = 'No sub-folders here.';
    $('pkList').replaceChildren(el);
  }

  const n = (r.firmware || []).length;
  $('pkFound').className = n ? 'hint' : 'hint warn';
  $('pkFound').textContent = n
    ? `${n} firmware file${n === 1 ? '' : 's'} here: ${r.firmware.slice(0, 3).join(', ')}${n > 3 ? '…' : ''}`
    : 'No .rcks or .bl7 files in this folder.';
  $('picker').hidden = false;
}

$('browse').onclick = (e) => { e.preventDefault(); openPicker(); };
$('pkUp').onclick = (e) => { e.preventDefault(); openPicker(e.target.dataset.path); };
$('pkCancel').onclick = () => { $('picker').hidden = true; };
$('pkUse').onclick = () => {
  $('serveDir').value = pkCurrent;
  $('picker').hidden = true;
  refreshFirmware();
  persist();
};
$('pkPath').addEventListener('keydown', (e) => { if (e.key === 'Enter') openPicker($('pkPath').value); });

/* ---- live server status ---- */

async function pollServer() {
  try {
    const s = await (await fetch('/api/server')).json();
    const up = !!s.addr && s.running;
    $('srvDot').className = 'dot' + (up ? ' on' : '');
    $('srvState').textContent = !s.addr ? 'Stopped' : (s.running ? 'Started' : 'Stopped');
    $('srvAddr').textContent = s.addr ? `http://${s.addr}${s.file ? '  ·  ' + s.file : ''}` : 'not started yet';

    $('srvActive').replaceChildren(...(s.active || []).map((a) => {
      const d = document.createElement('div');
      d.className = 'conn';
      d.innerHTML = `${esc(a.client)} &nbsp; ${esc(a.path)} &nbsp; ${esc(a.human)}` +
        (a.total ? ` / ${esc(humanBytes(a.total))} (${a.percent.toFixed(0)}%)` : '') +
        ` &nbsp; ${a.seconds}s<div class="meter"><i style="width:${Math.min(100, a.percent || 0)}%"></i></div>`;
      return d;
    }));

    const r = s.recent || [];
    $('srvRecent').textContent = r.length
      ? `${r.length} completed — last: ${r[0].client} ${r[0].path} ${r[0].human}`
      : (s.addr ? 'No downloads yet.' : '');
  } catch (e) { /* the console outlives individual runs */ }
}
setInterval(pollServer, 1000);

const humanBytes = (n) => n >= 1048576 ? (n / 1048576).toFixed(1) + ' MiB'
  : n >= 1024 ? (n / 1024).toFixed(1) + ' KiB' : n + ' B';

/* ---------- the table ---------- */

function statusClass(s) {
  if (s === 'Done') return 'st-done';
  if (s === 'Running') return 'st-run';
  if (/Fail|Error/i.test(s)) return 'st-fail';
  return 'st-skip';
}

const removed = new Set();
// Addresses currently being re-read. The row dims until its new value lands, so
// a long pass over hundreds of APs looks like work rather than a frozen table.
const scanning = new Set();
// Transcripts accumulate for the whole session: every connection to an AP is
// appended, so a re-scan adds to the record instead of replacing it. Cleared
// only when the next run starts.
const transcripts = new Map();

const clockOf = (iso) => {
  if (!iso) return '?';
  const d = new Date(iso);
  return isNaN(d) ? '?' : d.toLocaleTimeString();
};

function appendTranscript(ip, text, started, ended) {
  if (!text) return;
  const prev = transcripts.get(ip) || '';
  const head = `${'='.repeat(20)} ${clockOf(started)} \u2192 ${clockOf(ended)} ${'='.repeat(20)}\n`;
  transcripts.set(ip, prev + (prev ? '\n\n' : '') + head + text.replace(/\s+$/, ''));
  if (ip === selected) paintTranscript(ip);
}

function paintTranscript(ip) {
  const t = transcripts.get(ip);
  const pane = $('pTx');
  const atBottom = pane.scrollHeight - pane.scrollTop - pane.clientHeight < 60;
  pane.textContent = t || 'No session recorded for this address yet.';
  $('txWho').textContent = `(${ip})`;
  if (atBottom) pane.scrollTop = pane.scrollHeight;
}

function upsert(r) {
  if (removed.has(r.ip)) return;
  // Watch updates carry only what changed, so merge rather than replace.
  rows.set(r.ip, { ...(rows.get(r.ip) || {}), ...r });
  if (!order.includes(r.ip)) order.push(r.ip);
}

function visible() {
  let list = order.map((ip) => rows.get(ip));
  if (statusFilter) list = list.filter((r) => (statusFilter === 'failed'
    ? /Fail|Error/i.test(r.status) : r.status === statusFilter));
  if (filterText) {
    const q = filterText.toLowerCase();
    list = list.filter((r) => [r.ip, r.mac, r.model, r.firmware, r.status, r.error, r.fw, r.note]
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

// Selection is a set of IPs plus an anchor, so shift-click can extend a range
// the way every table the operator already uses does.
const picked = new Set();
let anchor = null;
let selected = null;

function rowClick(ev, ip, listNow) {
  const idx = listNow.indexOf(ip);
  if (ev.shiftKey && anchor !== null) {
    const a = listNow.indexOf(anchor);
    if (a >= 0) {
      picked.clear();
      const [lo, hi] = a < idx ? [a, idx] : [idx, a];
      for (let i = lo; i <= hi; i++) picked.add(listNow[i]);
    }
  } else if (ev.ctrlKey || ev.metaKey) {
    picked.has(ip) ? picked.delete(ip) : picked.add(ip);
    anchor = ip;
  } else {
    picked.clear();
    picked.add(ip);
    anchor = ip;
    showTranscript(ip);
    return;
  }
  render();
}

function removeSelected() {
  if (running) return toast('Stop the run before changing the list.');
  if (!picked.size) return;
  for (const ip of picked) {
    removed.add(ip);
    rows.delete(ip);
    transcripts.delete(ip);
  }
  order = order.filter((ip) => !picked.has(ip));
  // Take them out of the target list too, so a re-run does not bring them back.
  const gone = new Set(picked);
  $('hosts').value = $('hosts').value.split('\n')
    .filter((l) => !gone.has(l.split(',')[0].trim().replace(/^"|"$/g, ''))).join('\n');
  hostsChanged();
  picked.clear();
  anchor = null;
  recount();
  render();
}

$('removeSel').onclick = removeSelected;

document.addEventListener('keydown', (e) => {
  const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName || '');
  if (typing) return;
  if ((e.key === 'Delete' || e.key === 'Backspace') && picked.size) {
    e.preventDefault();
    removeSelected();
  }
  if ((e.ctrlKey || e.metaKey) && e.key === 'a' && order.length) {
    e.preventDefault();
    visible().forEach((r) => picked.add(r.ip));
    render();
  }
  if (e.key === 'Escape' && picked.size) { picked.clear(); render(); }
});

// Counters are counted, not accumulated: a re-scan emits a row per AP per pass,
// so incrementing per event made "done" climb with every scan.
function recount() {
  let done = 0, fail = 0;
  for (const r of rows.values()) {
    if (r.status === 'Done') done++;
    else if (/Fail|Error/i.test(r.status)) fail++;
  }
  setCount('cDone', done);
  setCount('cFail', fail);
}

function render() {
  const list = visible();
  $('tableEmpty').hidden = list.length > 0;
  $('shown').textContent = list.length === order.length
    ? `${order.length} rows` : `${list.length} of ${order.length} rows`;

  const ips = list.map((r) => r.ip);
  const frag = document.createDocumentFragment();
  for (const r of list) {
    const tr = document.createElement('tr');
    tr.className = (picked.has(r.ip) ? 'pick ' : '') + (scanning.has(r.ip) ? 'scanning ' : '') +
      (r.ip === selected ? 'sel' : '');
    tr.onclick = (ev) => rowClick(ev, r.ip, ips);
    tr.innerHTML =
      `<td>${esc(r.ip)}</td><td>${esc(r.mac)}</td><td>${esc(r.model)}</td><td>${esc(r.firmware)}</td>` +
      `<td class="num">${r.reachable ? esc(r.ping) : '—'}</td>` +
      `<td><span class="st ${statusClass(r.status)}">${esc(r.status)}</span></td>` +
      `<td>${esc(r.fw)}</td><td class="${r.error ? 'err' : 'note'}">${esc(r.error || r.note)}</td>`;
    frag.appendChild(tr);
  }
  rowsEl.replaceChildren(frag);
  $('selInfo').textContent = picked.size ? `${picked.size} selected` : '';
  $('removeSel').disabled = running || picked.size === 0;
  $('removeSel').textContent = picked.size ? `Remove ${picked.size}` : 'Remove';
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
  paintTranscript(ip);
  document.querySelector('.tab[data-p=tx]').click();
  render();
}

/* ---------- run ---------- */

function request() {
  const num = (id) => parseInt($(id).value, 10) || 0;
  return {
    hosts: hostList(),
    user: $('user').value, pass: $('pass').value, newPass: $('newPass').value,
    alsoDefault: $('alsoDefault').checked,
    concurrency: num('concurrency'),
    probe: $('probe').value,
    pingTimeoutMs: num('pingTimeoutMs'), pingRetries: num('pingRetries'),
    pingConcurrency: num('pingConcurrency'),
    firmware: $('firmware').checked, factory: $('factory').checked,
    reboot: $('reboot').checked, command: $('command').value.trim(),
    serve: $('firmware').checked && internalMode(),
    serveDir: $('serveDir').value, serveIp: $('serveIp').value, servePort: num('servePort'),
    fwProto: $('fwProto').value, fwHost: $('fwHost').value, fwPort: $('fwPort').value,
    fwUser: $('fwUser').value, fwPass: $('fwPass').value,
    fwFile: internalMode() ? $('fwFileSel').value : $('fwFile').value,
    watch: $('watch').checked, watchIntervalS: num('watchIntervalS'),
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
  picked.clear(); removed.clear(); anchor = null;
  // The transcript log spans a whole run and survives Stop; a new run resets it.
  transcripts.clear();
  // Show every address up front, so the grid is the full worklist from the
  // first second rather than filling in as results trickle back.
  for (const ip of hostList()) {
    upsert({ ip, mac: '', model: '', firmware: '', ping: '', reachable: false,
      status: 'Queued', fw: '', error: '', transcript: '' });
  }
  for (const p of ['pLog', 'pSweep', 'pXfer']) $(p).replaceChildren();
  $('pTx').textContent = 'Select an AP to see its sessions.';
  $('nDead').textContent = ''; $('nXfer').textContent = '';
  nXfer = 0;
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
  // A run only ends when it is stopped, so the button has to say so.
  $('stop').classList.toggle('armed', v);
  lockPanel(v);
  $('removeSel').disabled = v || picked.size === 0;
  if (!v) { $('bar').style.width = '0%'; }
}

// The left column is the run's input. Letting it be edited mid-run would leave
// the form describing a run other than the one in flight — and removing rows
// from a list the engine is still working through is worse than useless.
function lockPanel(locked) {
  const aside = document.querySelector('aside');
  aside.classList.toggle('locked', locked);
  $('lockNote').hidden = !locked;
  for (const el of aside.querySelectorAll('input, select, textarea, button')) {
    el.disabled = locked;
  }
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

let nXfer = 0;

const src = new EventSource('/api/events');
src.onmessage = (m) => {
  const e = JSON.parse(m.data);
  switch (e.kind) {
    case 'log':
      logLine('pLog', e.message);
      break;

    case 'phase':
      if (e.phase === 'rescan') {
        scanning.clear();
        for (const ip of order) scanning.add(ip);
        $('phase').textContent = `re-scan ${e.done} · pinging ${e.total}`;
        render();
        break;
      }
      $('phase').textContent =
        e.phase === 'sweep' ? `probing ${e.total}` :
        e.phase === 'ssh' ? `connecting to ${e.total}` :
        e.phase === 'download' ? `${e.total} downloading` :
        e.phase === 'watch' ? `watching ${e.total}` : e.phase;
      setRunning(true);
      break;

    case 'progress':
      const done = e.done ?? 0, total = e.total ?? 0;
      if (total) $('bar').style.width = `${(done / total) * 100}%`;
      if (e.phase === 'download') $('phase').textContent = `downloaded ${done}/${total}`;
      if (e.phase === 'rescan-ping') $('phase').textContent = `pinging ${done}/${total}`;
      if (e.phase === 'rescan-read') $('phase').textContent = `re-reading ${done}/${total}`;
      if (e.phase === 'watch') {
        // The pass is over; anything still marked never reported back.
        scanning.clear();
        $('phase').textContent = `waiting — ${done} of ${total} on new firmware`;
        render();
      }
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
        status: r.status, fw: r.fw_status || '', error: r.error || '', note: r.note || '',
      });
      appendTranscript(r.ip, e.transcript, r.started, r.ended);
      scanning.delete(r.ip);
      recount();
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
