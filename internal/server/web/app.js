const state = { projects: [], settings: null, filter: 'all', query: '', detailId: null };
const el = id => document.getElementById(id);
const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#039;'}[ch]));

function toast(message, kind = '') {
  const node = document.createElement('div');
  node.className = `toast ${kind}`;
  node.textContent = message;
  el('toastStack').appendChild(node);
  setTimeout(() => node.remove(), 3200);
}

async function api(path, options = {}) {
  const response = await fetch(path, options);
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || `Request failed (${response.status})`);
  return data;
}

async function mutate(path, body = {}) {
  return api(path, { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) });
}

function relativeTime(value) {
  if (!value || value.startsWith('0001-')) return 'Never';
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 10) return 'Just now';
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds/60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds/3600)}h ago`;
  return `${Math.floor(seconds/86400)}d ago`;
}

function filteredProjects() {
  const q = state.query.trim().toLowerCase();
  return state.projects.filter(p => {
    if (state.filter === 'running' && p.status !== 'running' && p.status !== 'starting') return false;
    if (state.filter === 'stopped' && p.status !== 'stopped') return false;
    if (state.filter === 'discovered' && p.managed) return false;
    if (!q) return true;
    return [p.name,p.root,p.workingDir,p.framework,p.runtime,p.url,p.port,p.startCommand].some(v => String(v ?? '').toLowerCase().includes(q));
  });
}

function renderStats() {
  const running = state.projects.filter(p => p.status === 'running' || p.status === 'starting').length;
  const managed = state.projects.filter(p => p.managed).length;
  const discovered = state.projects.filter(p => !p.managed).length;
  const covers = state.projects.filter(p => p.coverUrl).length;
  el('stats').innerHTML = [
    ['Running', running], ['Managed', managed], ['New discoveries', discovered], ['Visual covers', covers]
  ].map(([label,value]) => `<div class="stat"><strong>${value}</strong><span>${label}</span></div>`).join('');
  el('countAll').textContent = state.projects.length;
  el('countRunning').textContent = running;
  el('countStopped').textContent = state.projects.filter(p => p.status === 'stopped').length;
  el('countDiscovered').textContent = discovered;
}

function cardHTML(p) {
  const running = p.status === 'running' || p.status === 'starting';
  const cover = p.coverUrl ? `<img loading="lazy" src="${esc(p.coverUrl)}" alt="Preview of ${esc(p.name)}">` : `<div class="cover-placeholder"><div class="mini-window"></div></div>`;
  const primary = p.url ? `<a class="action-button primary" href="${esc(p.url)}" target="_blank" rel="noreferrer">Open</a>` : '';
  let serviceAction = '';
  if (running && p.managed) serviceAction = `<button class="action-button" data-action="stop" data-id="${p.id}">Stop</button>`;
  else if (p.startCommand) serviceAction = `<button class="action-button" data-action="start" data-id="${p.id}">Start</button>`;
  const manage = !p.managed ? `<button class="action-button" data-action="manage" data-id="${p.id}">Keep</button>` : '';
  return `<article class="card" data-project-id="${p.id}">
    <div class="cover" data-detail="${p.id}">
      ${cover}
      <div class="cover-top"><span class="status-badge ${esc(p.status)}">${esc(p.status)}</span><span class="confidence-badge">${p.confidence || 0}% match</span></div>
    </div>
    <div class="card-body">
      <div class="card-title-row"><div class="card-title"><h3>${esc(p.name)}</h3><p>${esc(p.workingDir || p.root || 'Location not available')}</p></div><span class="framework-pill">${esc(p.framework || p.runtime || 'service')}</span></div>
      <div class="meta-row"><span>${p.port ? `localhost:${p.port}` : 'No port'}</span><i class="meta-dot"></i><span>${esc(relativeTime(p.lastSeen))}</span>${p.pid ? `<i class="meta-dot"></i><span>PID ${p.pid}</span>` : ''}</div>
      <div class="card-actions">${primary}${serviceAction}${manage}<button class="action-button more" data-detail="${p.id}" aria-label="Details">&#8943;</button></div>
    </div>
  </article>`;
}

function render() {
  renderStats();
  const projects = filteredProjects();
  el('projectGrid').innerHTML = projects.map(cardHTML).join('');
  el('projectGrid').classList.toggle('hidden', projects.length === 0);
  el('emptyState').classList.toggle('hidden', projects.length !== 0);
  if (state.detailId) renderDetail(state.detailId);
}

async function load(silent = false) {
  try {
    const data = await api('/api/services');
    state.projects = data.projects || [];
    state.settings = data.settings;
    if (!silent) el('scanStatus').textContent = `Watching local ports - updated ${new Date().toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'})}`;
    render();
  } catch (err) {
    if (!silent) toast(err.message, 'error');
  }
}

async function scanNow() {
  const button = el('scanButton');
  button.disabled = true; el('scanStatus').textContent = 'Scanning local services...';
  try { await mutate('/api/scan'); await load(); toast('Local services scanned'); }
  catch (err) { toast(err.message, 'error'); }
  finally { button.disabled = false; }
}

async function projectAction(id, action) {
  try {
    await mutate(`/api/projects/${id}/${action}`);
    if (action === 'capture') toast('Cover capture queued');
    else toast(`${action[0].toUpperCase()+action.slice(1)} complete`);
    await load(true);
  } catch (err) { toast(err.message, 'error'); }
}

async function renderDetail(id) {
  const p = state.projects.find(x => x.id === id);
  if (!p) return;
  let logs = '';
  try { logs = (await api(`/api/projects/${id}/logs`)).logs || ''; } catch (_) {}
  const running = p.status === 'running' || p.status === 'starting';
  el('detailContent').innerHTML = `<div class="detail-shell">
    <div class="detail-hero">${p.coverUrl ? `<img src="${esc(p.coverUrl)}" alt="Preview">` : `<div class="cover-placeholder"><div class="mini-window"></div></div>`}<button class="close-button detail-close" data-close="detailDialog">&times;</button></div>
    <div class="detail-title"><div><h2>${esc(p.name)}</h2><p>${esc(p.framework || p.runtime || 'Local service')} ${p.port ? `on port ${p.port}` : ''}</p></div><span class="status-badge ${esc(p.status)}">${esc(p.status)}</span></div>
    <div class="detail-grid">
      <div class="detail-item"><small>Working directory</small><code>${esc(p.workingDir || p.root || 'Unknown')}</code></div>
      <div class="detail-item"><small>Start command</small><code>${esc(p.startCommand || 'Not inferred')}</code></div>
      <div class="detail-item"><small>URL</small><code>${esc(p.url || 'Not detected')}</code></div>
      <div class="detail-item"><small>Identity</small><span>${esc(p.id)} - confidence ${p.confidence || 0}%</span></div>
    </div>
    <div class="log-box"><div class="log-head"><span>SESSION LOGS</span><span>Only commands started by DevHub</span></div><pre>${esc(logs || 'No DevHub-managed output yet.')}</pre></div>
    <div class="detail-actions">
      ${p.url ? `<a class="action-button primary" href="${esc(p.url)}" target="_blank" rel="noreferrer">Open app</a><button class="action-button" data-action="capture" data-id="${p.id}">Refresh cover</button>` : ''}
      ${running && p.managed ? `<button class="action-button" data-action="stop" data-id="${p.id}">Stop</button>` : (!running && p.managed && p.startCommand ? `<button class="action-button" data-action="start" data-id="${p.id}">Start</button>` : '')}
      ${!p.managed ? `<button class="action-button" data-action="manage" data-id="${p.id}">Keep project</button>` : ''}
      <button class="action-button danger" data-action="forget" data-id="${p.id}">Forget</button>
    </div>
  </div>`;
}

function openDetails(id) {
  state.detailId = id;
  renderDetail(id);
  el('detailDialog').showModal();
}

el('tabs').addEventListener('click', e => {
  const button = e.target.closest('[data-filter]'); if (!button) return;
  state.filter = button.dataset.filter;
  document.querySelectorAll('.tab').forEach(x => x.classList.toggle('active', x === button));
  render();
});
el('search').addEventListener('input', e => { state.query = e.target.value; render(); });
el('scanButton').addEventListener('click', scanNow);
el('addButton').addEventListener('click', () => el('addDialog').showModal());
el('emptyAdd').addEventListener('click', () => el('addDialog').showModal());
el('settingsButton').addEventListener('click', () => {
  const form = el('settingsForm');
  form.autoRemember.checked = !!state.settings?.autoRemember;
  form.autoCapture.checked = !!state.settings?.autoCapture;
  form.scanIntervalSeconds.value = state.settings?.scanIntervalSeconds || 5;
  el('settingsDialog').showModal();
});
document.addEventListener('click', e => {
  const close = e.target.closest('[data-close]'); if (close) { el(close.dataset.close).close(); if (close.dataset.close === 'detailDialog') state.detailId = null; return; }
  const detail = e.target.closest('[data-detail]'); if (detail) { openDetails(detail.dataset.detail); return; }
  const action = e.target.closest('[data-action]');
  if (action) {
    const kind = action.dataset.action;
    if (kind === 'forget' && !confirm('Forget this project from DevHub? Files on disk will not be deleted.')) return;
    projectAction(action.dataset.id, kind);
  }
});
el('detailDialog').addEventListener('close', () => { state.detailId = null; });

el('addForm').addEventListener('submit', async e => {
  e.preventDefault();
  const data = Object.fromEntries(new FormData(e.currentTarget).entries());
  try { await mutate('/api/projects', data); e.currentTarget.reset(); el('addDialog').close(); await load(); toast('Project added'); }
  catch (err) { toast(err.message, 'error'); }
});
el('settingsForm').addEventListener('submit', async e => {
  e.preventDefault(); const f = e.currentTarget;
  try {
    await mutate('/api/settings', {autoRemember:f.autoRemember.checked,autoCapture:f.autoCapture.checked,scanIntervalSeconds:Number(f.scanIntervalSeconds.value)});
    el('settingsDialog').close(); await load(true); toast('Settings saved');
  } catch (err) { toast(err.message, 'error'); }
});

load();
setInterval(() => load(true), 4000);
