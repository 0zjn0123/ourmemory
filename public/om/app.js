/* ourmemory frontend — vanilla, no build step. */
const app = document.getElementById('app');
const toastEl = document.getElementById('toast');
const modal = document.getElementById('modal');
const modalCard = document.getElementById('modal-card');
const state = { me: null, profile: null, titles: [], route: '' };

const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const attr = (s) => esc(s).replace(/`/g, '&#96;');
const fhref = (f) => f && f.id ? `/f/${encodeURIComponent(f.id)}/${encodeURIComponent(f.name || 'file')}` : '#';
const fmtBytes = (n) => {
  n = Number(n || 0);
  if (n < 1024) return n + ' B';
  const u = ['KB', 'MB', 'GB', 'TB'];
  let i = -1;
  do { n /= 1024; i++; } while (n >= 1024 && i < u.length - 1);
  return n.toFixed(n >= 100 ? 0 : 1) + ' ' + u[i];
};
const fmtNum = (n) => Number(n || 0).toLocaleString('zh-CN');
const ago = (t) => {
  if (!t) return '';
  const d = (Date.now() - new Date(t).getTime()) / 1000;
  if (d < 60) return '刚刚';
  if (d < 3600) return Math.floor(d / 60) + ' 分钟前';
  if (d < 86400) return Math.floor(d / 3600) + ' 小时前';
  if (d < 86400 * 30) return Math.floor(d / 86400) + ' 天前';
  return new Date(t).toLocaleDateString('zh-CN');
};
const dt = (t) => t ? new Date(t).toLocaleString('zh-CN') : '';
const icon = (f) => f.is_dir ? '📁' : f.kind === 'video' ? '🎬' : f.kind === 'audio' ? '🎵' : f.kind === 'image' ? '🖼️' : f.kind === 'html' ? '🌐' : f.kind === 'pdf' ? '📕' : f.kind === 'text' ? '📝' : '📄';
const initials = (s) => (s || '?').slice(0, 1).toUpperCase();
const textExt = new Set(['txt','md','markdown','json','csv','tsv','log','xml','yml','yaml','ini','conf','toml','css','js','ts','java','sql','sh','bat','ps1','env']);
function previewKind(f) {
  if (!f || f.is_dir) return '';
  const ext = String(f.name || '').split('.').pop().toLowerCase();
  if (['image', 'video', 'audio', 'html', 'pdf'].includes(f.kind)) return f.kind;
  if (f.kind === 'text' || textExt.has(ext)) return 'text';
  return '';
}

function toast(msg) {
  toastEl.textContent = msg;
  toastEl.classList.add('show');
  clearTimeout(toast._t);
  toast._t = setTimeout(() => toastEl.classList.remove('show'), 2600);
}
function openModal(html, cls = '') { modalCard.className = 'modal-card' + (cls ? ' ' + cls : ''); modalCard.innerHTML = html; modal.classList.remove('hidden'); }
function closeModal() { modal.classList.add('hidden'); modalCard.innerHTML = ''; modalCard.className = 'modal-card'; }
modal.addEventListener('click', (e) => { if (e.target === modal) closeModal(); });

async function api(path, opts = {}) {
  const o = { credentials: 'same-origin', ...opts };
  if (o.body && !(o.body instanceof FormData) && typeof o.body !== 'string') {
    o.headers = { 'Content-Type': 'application/json', ...(o.headers || {}) };
    o.body = JSON.stringify(o.body);
  }
  const res = await fetch('/api/om' + path, o);
  let j = null;
  try { j = await res.json(); } catch (_) {}
  if (!res.ok || !j || j.code !== 200) throw new Error((j && j.message) || ('请求失败 (' + res.status + ')'));
  return j.data;
}
async function uploadFile(file, scope, parent) {
  const fd = new FormData();
  fd.append('scope', scope);
  fd.append('parent', parent || '');
  fd.append('file', file);
  return api('/files/upload', { method: 'POST', body: fd });
}

async function previewFile(f) {
  const kind = previewKind(f);
  if (!kind) { toast('这个类型暂不支持在线预览，请下载查看'); return; }
  const src = fhref(f);
  let body = '';
  try {
    if (kind === 'text') {
      const res = await fetch(src, { credentials: 'same-origin' });
      if (!res.ok) throw new Error('读取失败 (' + res.status + ')');
      let text = await res.text();
      if (text.length > 220000) text = text.slice(0, 220000) + '\n\n…（内容过长，已截断）';
      body = `<pre>${esc(text)}</pre>`;
    } else if (kind === 'image') {
      body = `<img src="${attr(src)}" alt="${attr(f.name)}">`;
    } else if (kind === 'video') {
      body = `<video src="${attr(src)}" controls preload="metadata"></video>`;
    } else if (kind === 'audio') {
      body = `<audio src="${attr(src)}" controls preload="metadata"></audio>`;
    } else if (kind === 'pdf') {
      body = `<iframe src="${attr(src)}" title="${attr(f.name)}"></iframe>`;
    } else if (kind === 'html') {
      body = `<iframe src="${attr(src)}" sandbox="allow-scripts allow-forms" title="${attr(f.name)}"></iframe>`;
    }
  } catch (e) { body = `<div class="empty">${esc(e.message)}</div>`; }
  openModal(`<div style="display:flex;justify-content:space-between;align-items:flex-start;gap:12px"><div><h2 style="font-size:18px;word-break:break-all">${esc(f.name)}</h2><div class="small faint">${fmtBytes(f.size)} · ${esc(f.kind)}</div></div><button class="btn small ghost" id="preview-close">关闭</button></div>
    <div class="preview-body" style="margin-top:14px">${body}</div>
    <div class="actions"><a class="btn" href="/dl/${attr(f.id)}/${encodeURIComponent(f.name || 'file')}">下载原文件</a></div>`, 'preview');
  document.getElementById('preview-close').onclick = closeModal;
}

function renderTop() {
  const topright = document.getElementById('topright');
  if (state.me) {
    topright.innerHTML = `
      <a class="btn small ghost" href="/settings" data-link>设置</a>
      <a href="/u/${encodeURIComponent(state.me.username)}" data-link style="display:flex;align-items:center;gap:8px">
        <span class="avatar">${state.profile?.avatar_file_id ? `<img src="/f/${attr(state.profile.avatar_file_id)}" alt="">` : esc(initials(state.profile?.display_name || state.me.username))}</span>
        <span class="small">${esc(state.profile?.display_name || state.me.username)}</span>
      </a>
      <button class="btn small ghost" id="logout">退出</button>`;
    document.getElementById('logout').onclick = async () => {
      try { await api('/auth/logout', { method: 'POST' }); } catch (_) {}
      state.me = null; state.profile = null; state.titles = []; renderTop(); goto('/'); toast('已退出');
    };
  } else {
    topright.innerHTML = `<a class="btn small ghost" href="/login" data-link>登录</a><a class="btn small primary" href="/register" data-link>注册</a>`;
  }
  const adm = document.getElementById('nav-admin');
  if (adm) adm.style.display = state.me && state.me.role === 2 ? '' : 'none';
  const activePath = effectivePath();
  document.querySelectorAll('#mainnav a').forEach(a => {
    const href = a.getAttribute('href');
    a.classList.toggle('active', activePath === href);
  });
}

function goto(path, replace = false) {
  if (replace) history.replaceState({}, '', path); else history.pushState({}, '', path);
  route();
}
window.addEventListener('popstate', route);
document.addEventListener('click', (e) => {
  const ext = e.target.closest('a[href^="/files"]');
  if (ext) { e.preventDefault(); location.href = ext.getAttribute('href'); return; }
  const a = e.target.closest('a[data-link]');
  if (a) { e.preventDefault(); goto(a.getAttribute('href')); }
});
function nav(path, label, active = false) {
  return `<a class="btn small ${active ? 'primary' : 'ghost'}" href="${path}" data-link>${esc(label)}</a>`;
}
function needLogin() {
  if (state.me) return false;
  app.innerHTML = `<div class="auth-wrap"><div class="card auth-card"><h1>需要登录</h1><p class="sub">这个页面需要 ourmemory 账号。</p><div class="actions-row"><a class="btn primary" href="/login" data-link>去登录</a><a class="btn" href="/register" data-link>注册</a></div></div></div>`;
  return true;
}

/* ---------------- Auth ---------------- */
function renderLogin() {
  app.innerHTML = `<div class="auth-wrap"><div class="card auth-card">
    <h1>欢迎回来</h1><p class="sub">登录后上传、创作、聊天，并管理你的私人加密空间。</p>
    <form id="login-form">
      <label class="field"><span>用户名</span><input class="input" name="username" autocomplete="username" required></label>
      <label class="field"><span>密码</span><input class="input" name="password" type="password" autocomplete="current-password" required></label>
      <button class="btn primary" style="width:100%" type="submit">登录</button>
    </form>
    <div class="form-note">还没有账号？<a href="/register" data-link style="color:var(--brand)">邮箱注册</a></div>
  </div></div>`;
  document.getElementById('login-form').onsubmit = async (e) => {
    e.preventDefault();
    const f = new FormData(e.target);
    try {
      await api('/auth/login', { method: 'POST', body: { username: f.get('username'), password: f.get('password') } });
      await loadMe(); renderTop(); goto('/'); toast('登录成功');
    } catch (err) { toast(err.message); }
  };
}
function renderRegister() {
  app.innerHTML = `<div class="auth-wrap"><div class="card auth-card">
    <h1>创建账号</h1><p class="sub">邮箱验证后即可获得 1GB 私人空间（需手动开通）。</p>
    <form id="reg-form">
      <label class="field"><span>用户名</span><input class="input" name="username" pattern="[A-Za-z0-9_-]{3,32}" required placeholder="3-32 位字母数字下划线"></label>
      <label class="field"><span>邮箱</span><input class="input" name="email" type="email" required></label>
      <label class="field"><span>密码</span><input class="input" name="password" type="password" minlength="8" required></label>
      <button class="btn primary" style="width:100%" type="submit">发送验证码</button>
    </form>
    <div class="form-note">已有账号？<a href="/login" data-link style="color:var(--brand)">去登录</a></div>
  </div></div>`;
  document.getElementById('reg-form').onsubmit = async (e) => {
    e.preventDefault();
    const f = new FormData(e.target);
    try {
      const d = await api('/auth/register/request', { method: 'POST', body: { username: f.get('username'), email: f.get('email'), password: f.get('password') } });
      openModal(`<h2>输入邮箱验证码</h2><p class="small muted">验证码已发送到 ${esc(f.get('email'))}，10 分钟内有效。</p>
        <form id="verify-form"><label class="field"><span>6 位验证码</span><input class="input" name="code" inputmode="numeric" maxlength="6" required></label>
        <div class="actions"><button type="button" class="btn ghost" id="cancel">取消</button><button class="btn primary" type="submit">完成注册</button></div></form>`);
      document.getElementById('cancel').onclick = closeModal;
      document.getElementById('verify-form').onsubmit = async (ev) => {
        ev.preventDefault();
        try {
          await api('/auth/register/verify', { method: 'POST', body: { register_id: d.register_id, code: new FormData(ev.target).get('code') } });
          closeModal(); await loadMe(); renderTop(); goto('/'); toast('注册成功');
        } catch (err) { toast(err.message); }
      };
    } catch (err) { toast(err.message); }
  };
}

/* ---------------- Home ---------------- */
async function renderHome() {
  let counts = { videos: 0, images: 0, posts: 0 };
  try {
    const [v, i, p] = await Promise.all([api('/videos?page=1'), api('/images?page=1'), api('/posts?page=1')]);
    counts = { videos: v.total || 0, images: i.total || 0, posts: p.total || 0 };
  } catch (_) {}
  app.innerHTML = `
    <section class="hero">
      <div class="eyebrow">community-owned cloud</div>
      <h1>把每个人的一小块硬盘，<br>拼成所有人的记忆。</h1>
      <p>ourmemory 是一个共享云盘：所有资源放在一个按需解压的归档里，由贡献者共同承载。你也可以开通一个加密的私人压缩包。视频、图片、文字和聊天都用同一套存储。</p>
      <div class="actions">
        <a class="btn primary" href="/files">进入文件</a>
        <a class="btn" href="/video" data-link>看视频</a>
        ${state.me ? '' : '<a class="btn ghost" href="/register" data-link>用邮箱注册</a>'}
      </div>
      <div class="statrow">
        <div class="stat"><b>${fmtNum(counts.videos)}</b><span>视频 / 音频</span></div>
        <div class="stat"><b>${fmtNum(counts.images)}</b><span>图片作品</span></div>
        <div class="stat"><b>${fmtNum(counts.posts)}</b><span>社区动态</span></div>
        <div class="stat"><b>1+</b><span>共享归档包</span></div>
      </div>
    </section>
    <div class="section-head"><h2>从这里开始</h2><span class="faint small">所有子站共享同一份存储</span></div>
    <div class="grid">
      <a class="card pad" href="/files"><h3>📦 文件浏览器</h3><p class="small">OpenList 原生文件管理器，直接读写共享盘与私人盘。图片、视频、音频、静态 HTML 可在线打开。修改和删除仅限自己上传的内容。</p></a>
      <a class="card pad" href="/video" data-link><h3>🎬 视频站</h3><p class="small">视频与音频汇总。播放、弹幕、点赞、投币、收藏、评论、转发。</p></a>
      <a class="card pad" href="/image" data-link><h3>🖼️ 图片站</h3><p class="small">标签、收藏、评论与排行，不设弹幕。</p></a>
      <a class="card pad" href="/weibo" data-link><h3>💬 动态</h3><p class="small">纯文字正文存在主服务器，媒体文件存在共享盘。</p></a>
      <a class="card pad" href="/im" data-link><h3>🗣️ 聊天</h3><p class="small">公共大厅的聊天记录是共享盘里的一个文件；私人群存在创建者的私人加密盘。</p></a>
      <a class="card pad" href="/files"><h3>🔐 私人空间</h3><p class="small">开通你的加密压缩包，在文件浏览器的 /private 里使用。普通 1GB，捐赠档 11/61/121GB。</p></a>
    </div>
    <div class="section-head"><h2>关于这个盘</h2></div>
    <div class="card pad">
      <div class="tier">
        <div class="t"><b>普通用户</b><p>1GB 私人 · 2 站同存 · CDN · 压缩 7</p></div>
        <div class="t"><b>Ⅰ级捐赠</b><p>11GB · 3 站 · CDN · 压缩 5</p></div>
        <div class="t"><b>Ⅱ级捐赠</b><p>61GB · 5 站 · 主站下载 · 压缩 5</p></div>
        <div class="t"><b>Ⅲ级捐赠</b><p>121GB · 8 站 · 主站下载 · 压缩 3</p></div>
      </div>
      <p class="small" style="margin:14px 0 0">贡献硬盘的人会获得“存储贡献者”头衔，并按提供容量的 50%×90% 折算私人空间；离线 30 天回收。预留包可保证空间不被共享挤占。</p>
    </div>`;
}


/* ---------------- Video ---------------- */
async function renderVideoList() {
  const q = new URLSearchParams(location.search);
  let d;
  try { d = await api('/videos?' + q.toString()); } catch (e) { app.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  const items = d.items || [];
  app.innerHTML = `
    <div class="section-head"><div><h1>视频站</h1><p class="small">所有视频与音频都在这里。资源来自共享盘。</p></div>
      ${state.me ? `<button class="btn primary" id="publish-video">发布视频 / 音频</button>` : `<a class="btn primary" href="/login" data-link>登录后发布</a>`}</div>
    <div class="toolbar">
      <form id="search" class="toolbar grow" style="margin:0"><input class="input" name="q" placeholder="搜索标题、简介、标签" value="${attr(q.get('q') || '')}" style="max-width:360px"><button class="btn">搜索</button></form>
      <div class="seg"><button class="${!q.get('sort') ? 'active' : ''}" data-sort="">最新</button><button class="${q.get('sort') === 'hot' ? 'active' : ''}" data-sort="hot">最热</button><button class="${q.get('sort') === 'like' ? 'active' : ''}" data-sort="like">最多赞</button></div>
    </div>
    ${items.length ? `<div class="grid wide">${items.map(v => `
      <a class="video-card" href="/video/${v.video.id}" data-link>
        <div class="thumb">${v.cover ? `<img src="${fhref(v.cover)}" alt="">` : `<div style="position:absolute;inset:0;display:grid;place-items:center;font-size:44px">${v.file?.kind === 'audio' ? '🎵' : '🎬'}</div>`}<div class="play">▶</div></div>
        <div class="body"><h3>${esc(v.video.title)}</h3><div class="by"><span>${esc(v.uploader.username || 'unknown')}</span><span>${fmtNum(v.video.views)} 播放 · ${fmtNum(v.video.like_count)} 赞</span></div></div>
      </a>`).join('')}</div>` : `<div class="empty">还没有视频。上传一个文件，然后发布它。</div>`}`;
  document.getElementById('search').onsubmit = (e) => { e.preventDefault(); const f = new FormData(e.target); const n = new URLSearchParams(); if (f.get('q')) n.set('q', f.get('q')); if (q.get('sort')) n.set('sort', q.get('sort')); goto('/video?' + n.toString()); };
  document.querySelectorAll('[data-sort]').forEach(b => b.onclick = () => { const n = new URLSearchParams(q); if (b.dataset.sort) n.set('sort', b.dataset.sort); else n.delete('sort'); goto('/video?' + n.toString()); });
  const pub = document.getElementById('publish-video');
  if (pub) pub.onclick = () => publishVideoModal();
}

function publishVideoModal() {
  openModal(`<h2>发布视频 / 音频</h2><p class="small muted">文件会先上传到共享盘，再创建播放条目。</p>
    <form id="pv-form">
      <label class="field"><span>文件</span><input class="input" type="file" name="file" accept="video/*,audio/*" required></label>
      <label class="field"><span>标题</span><input class="input" name="title" required></label>
      <label class="field"><span>简介</span><textarea class="input" name="description"></textarea></label>
      <div class="two-col"><label class="field"><span>分类</span><input class="input" name="category" placeholder="动画 / 音乐 / 纪录片"></label><label class="field"><span>标签（逗号分隔）</span><input class="input" name="tags" placeholder="原创, 旅行"></label></div>
      <div class="actions"><button type="button" class="btn ghost" id="cancel">取消</button><button class="btn primary" type="submit">上传并发布</button></div>
    </form>`);
  document.getElementById('cancel').onclick = closeModal;
  document.getElementById('pv-form').onsubmit = async (e) => {
    e.preventDefault();
    const f = new FormData(e.target);
    const file = f.get('file');
    if (!file || !file.size) return toast('请选择文件');
    try {
      toast('上传中…');
      const up = await uploadFile(file, 'shared', '');
      const d = await api('/videos', { method: 'POST', body: { file_id: up.file.id, title: f.get('title'), description: f.get('description'), category: f.get('category'), tags: f.get('tags') } });
      closeModal(); toast('已发布'); goto('/video/' + d.video.id);
    } catch (err) { toast(err.message); }
  };
}

async function renderVideoDetail(id) {
  let d;
  try { d = await api('/videos/' + id); } catch (e) { app.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  const v = d.video, file = d.file;
  if (!file) { app.innerHTML = `<div class="empty">视频源文件不存在。</div>`; return; }
  const [comments, danmaku] = await Promise.all([
    api(`/videos/${id}/comments`).catch(() => ({ items: [] })),
    api(`/videos/${id}/danmaku`).catch(() => ({ items: [] }))
  ]);
  const mine = new Set(d.my_reactions || []);
  app.innerHTML = `
    <div class="detail-grid">
      <div>
        <div class="player-wrap" id="player-wrap">
          ${file.kind === 'audio' ? `<audio id="player" controls src="${fhref(file)}" style="width:100%"></audio>` : `<video id="player" controls playsinline src="${fhref(file)}" ${d.cover ? `poster="${fhref(d.cover)}"` : ''}></video>`}
          <div class="danmaku" id="danmaku"></div>
        </div>
        <h1 style="margin-top:16px">${esc(v.title)}</h1>
        <div class="meta-line"><span>${fmtNum(v.views)} 播放</span><span>${fmtNum(v.danmaku_count || danmaku.items.length)} 弹幕</span><span>${ago(v.created_at)}</span><span>${esc(v.category || '未分类')}</span></div>
        <div class="actions-row">
          <button class="btn ${mine.has('like') ? 'primary' : ''}" data-react="like">👍 ${fmtNum(v.like_count)}</button>
          <button class="btn ${mine.has('coin') ? 'primary' : ''}" data-react="coin">🪙 ${fmtNum(v.coin_count)}</button>
          <button class="btn ${mine.has('favorite') ? 'primary' : ''}" data-react="favorite">⭐ ${fmtNum(v.fav_count)}</button>
          <button class="btn ghost" id="repost-video">🔁 转发</button>
          <a class="btn ghost" href="/u/${encodeURIComponent(d.uploader.username || '')}" data-link>UP：${esc(d.uploader.username || 'unknown')}</a>
        </div>
        <div class="side-card" style="margin-top:12px"><p style="white-space:pre-wrap;margin:0">${esc(v.description || '暂无简介')}</p><div class="chips" style="margin-top:10px">${(v.tags || '').split(',').filter(Boolean).map(t => `<span class="chip">${esc(t.trim())}</span>`).join('')}</div></div>
        <div class="section-head"><h2>评论 ${fmtNum(comments.items.length)}</h2></div>
        ${state.me ? `<form id="comment-form" style="display:flex;gap:8px;margin-bottom:12px"><input class="input" name="content" placeholder="发一条友善的评论" required><button class="btn primary">发送</button></form>` : `<p class="small"><a href="/login" data-link style="color:var(--brand)">登录</a>后评论。</p>`}
        <div id="comments">${comments.items.length ? comments.items.map(c => `<div class="comment"><div class="avatar">${esc(initials(c.username))}</div><div class="body"><div class="who">${esc(c.username || '用户')} · ${ago(c.comment.created_at)}</div><div class="txt">${esc(c.comment.content)}</div></div></div>`).join('') : '<div class="empty">还没有评论。</div>'}</div>
      </div>
      <aside>
        <div class="side-card"><h3 style="font-size:15px;margin-bottom:10px">弹幕</h3><p class="small">发送一条弹幕，它会出现在播放器的当前时间。</p>
          ${state.me ? `<form id="dm-form" style="display:flex;gap:6px;margin-top:10px"><input class="input" name="content" maxlength="80" placeholder="说点什么…" required><button class="btn small primary">发送</button></form>` : `<p class="small"><a href="/login" data-link style="color:var(--brand)">登录</a>后发送弹幕。</p>`}
        </div>
        <div class="side-card" style="margin-top:12px"><h3 style="font-size:15px;margin-bottom:10px">标签</h3><div class="chips">${(v.tags || '').split(',').filter(Boolean).map(t => `<span class="chip green">${esc(t.trim())}</span>`).join('') || '<span class="faint small">暂无</span>'}</div></div>
      </aside>
    </div>`;
  document.querySelectorAll('[data-react]').forEach(b => b.onclick = async () => {
    if (!state.me) return goto('/login');
    try { await api(`/videos/${id}/react`, { method: 'POST', body: { kind: b.dataset.react } }); toast('已记录'); route(); } catch (e) { toast(e.message); }
  });
  const cf = document.getElementById('comment-form');
  if (cf) cf.onsubmit = async (e) => {
    e.preventDefault();
    try { await api(`/videos/${id}/comments`, { method: 'POST', body: { content: new FormData(e.target).get('content') } }); route(); } catch (err) { toast(err.message); }
  };
  const df = document.getElementById('dm-form');
  if (df) df.onsubmit = async (e) => {
    e.preventDefault();
    const p = document.getElementById('player');
    const ms = Math.round((p?.currentTime || 0) * 1000);
    try { await api(`/videos/${id}/danmaku`, { method: 'POST', body: { content: new FormData(e.target).get('content'), time_ms: ms, color: '#ffffff', mode: 0 } }); toast('弹幕已发送'); route(); } catch (err) { toast(err.message); }
  };
  const rv = document.getElementById('repost-video');
  if (rv) rv.onclick = () => {
    openModal(`<h2>转发到动态</h2><form id="repost-form"><label class="field"><span>说点什么（可选）</span><textarea class="input" name="content"></textarea></label><div class="actions"><button type="button" class="btn ghost" id="cancel">取消</button><button class="btn primary" type="submit">转发</button></div></form>`);
    document.getElementById('cancel').onclick = closeModal;
    document.getElementById('repost-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        const note = new FormData(e.target).get('content');
        await api('/posts', { method: 'POST', body: { content: (note ? note + '\n' : '') + '转发视频：' + v.title + '\n' + location.origin + '/video/' + id, media_file_ids: [file.id] } });
        closeModal(); toast('已转发到动态');
      } catch (err) { toast(err.message); }
    };
  };
  setupPlayer(id, danmaku.items);
}

function setupPlayer(videoID, items) {
  const player = document.getElementById('player');
  const layer = document.getElementById('danmaku');
  if (!player || !layer) return;
  const shown = new Set();
  const colors = ['#fff', '#63e6be', '#7aa7ff', '#ffd479', '#ff7ab6'];
  function tick() {
    const ms = player.currentTime * 1000;
    items.forEach((x, i) => {
      if (x.danmaku.time_ms <= ms && x.danmaku.time_ms > ms - 1200 && !shown.has(x.danmaku.id)) {
        shown.add(x.danmaku.id);
        const el = document.createElement('div');
        el.className = 'dm';
        el.textContent = x.danmaku.content;
        el.style.top = (8 + (i * 13) % 70) + '%';
        el.style.color = x.danmaku.color || colors[i % colors.length];
        el.style.animationDuration = (7 + (i % 5)) + 's';
        layer.appendChild(el);
        setTimeout(() => el.remove(), 12000);
      }
    });
  }
  player.addEventListener('timeupdate', tick);
  let last = 0;
  player.addEventListener('timeupdate', () => {
    if (!state.me) return;
    const now = Date.now();
    if (now - last < 10000) return;
    last = now;
    api(`/videos/${videoID}/progress`, { method: 'POST', body: { position: Math.round(player.currentTime), duration: Math.round(player.duration || 0) } }).catch(() => {});
  });
  api(`/videos/${videoID}/progress`).then(d => { if (d && d.position > 0 && d.position < (player.duration || 1e9) - 10) player.currentTime = d.position; }).catch(() => {});
}

/* ---------------- Image ---------------- */
async function renderImageList() {
  const q = new URLSearchParams(location.search);
  let d;
  try { d = await api('/images?' + q.toString()); } catch (e) { app.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  const items = d.items || [];
  app.innerHTML = `
    <div class="section-head"><div><h1>图片站</h1><p class="small">作品、标签与收藏，不设弹幕。</p></div>
      ${state.me ? `<button class="btn primary" id="publish-image">发布图片</button>` : `<a class="btn primary" href="/login" data-link>登录后发布</a>`}</div>
    <form id="search" class="toolbar"><input class="input" name="q" placeholder="搜索标题或标签" value="${attr(q.get('q') || '')}" style="max-width:360px"><button class="btn">搜索</button></form>
    ${items.length ? `<div class="grid masonry">${items.map(x => `
      <a class="image-card" href="/image/${x.image.id}" data-link>
        <img src="${fhref(x.file)}" alt="${attr(x.image.title)}" loading="lazy">
        <div class="cap">${esc(x.image.title || '无题')} · ${esc(x.uploader.username || '')}</div>
      </a>`).join('')}</div>` : `<div class="empty">还没有图片。</div>`}`;
  document.getElementById('search').onsubmit = (e) => { e.preventDefault(); const f = new FormData(e.target); goto('/image?' + (f.get('q') ? 'q=' + encodeURIComponent(f.get('q')) : '')); };
  const b = document.getElementById('publish-image');
  if (b) b.onclick = () => publishImageModal();
}
function publishImageModal() {
  openModal(`<h2>发布图片</h2><form id="pi-form">
    <label class="field"><span>图片文件</span><input class="input" type="file" name="file" accept="image/*" required></label>
    <label class="field"><span>标题</span><input class="input" name="title"></label>
    <label class="field"><span>描述</span><textarea class="input" name="description"></textarea></label>
    <label class="field"><span>标签（逗号分隔）</span><input class="input" name="tags" placeholder="插画, 风景"></label>
    <div class="actions"><button type="button" class="btn ghost" id="cancel">取消</button><button class="btn primary" type="submit">上传并发布</button></div></form>`);
  document.getElementById('cancel').onclick = closeModal;
  document.getElementById('pi-form').onsubmit = async (e) => {
    e.preventDefault();
    const f = new FormData(e.target);
    try {
      const up = await uploadFile(f.get('file'), 'shared', '');
      const d = await api('/images', { method: 'POST', body: { file_id: up.file.id, title: f.get('title'), description: f.get('description'), tags: f.get('tags') } });
      closeModal(); toast('已发布'); goto('/image/' + d.image.id);
    } catch (err) { toast(err.message); }
  };
}
async function renderImageDetail(id) {
  let d;
  try { d = await api('/images/' + id); } catch (e) { app.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  const img = d.image, file = d.file;
  const mine = new Set(d.my_reactions || []);
  app.innerHTML = `<div class="detail-grid"><div>
    <div class="card" style="overflow:hidden;background:#05080f"><img src="${fhref(file)}" alt="${attr(img.title)}" style="width:100%;max-height:78vh;object-fit:contain;display:block"></div>
    <h1 style="margin-top:16px">${esc(img.title || '无题')}</h1>
    <div class="meta-line"><span>${fmtNum(img.views)} 浏览</span><span>${ago(img.created_at)}</span><a href="/u/${encodeURIComponent(d.uploader.username || '')}" data-link>作者：${esc(d.uploader.username || '')}</a></div>
    <div class="actions-row"><button class="btn ${mine.has('like') ? 'primary' : ''}" data-react="like">👍 ${fmtNum(img.like_count)}</button><button class="btn ${mine.has('favorite') ? 'primary' : ''}" data-react="favorite">⭐ ${fmtNum(img.fav_count)}</button></div>
    <div class="side-card"><p style="white-space:pre-wrap;margin:0">${esc(img.description || '暂无描述')}</p><div class="chips" style="margin-top:10px">${(img.tags || '').split(',').filter(Boolean).map(t => `<span class="chip pink">${esc(t.trim())}</span>`).join('')}</div></div>
  </div><aside class="side-card"><h3 style="font-size:15px">作品信息</h3><dl class="kv" style="margin-top:12px"><dt>作者</dt><dd>${esc(d.uploader.username || '')}</dd><dt>尺寸</dt><dd>${img.width || '?'} × ${img.height || '?'}</dd><dt>文件</dt><dd>${fmtBytes(file?.size)}</dd><dt>发布</dt><dd>${dt(img.created_at)}</dd></dl></aside></div>`;
  document.querySelectorAll('[data-react]').forEach(b => b.onclick = async () => {
    if (!state.me) return goto('/login');
    try { await api(`/images/${id}/react`, { method: 'POST', body: { kind: b.dataset.react } }); toast('已记录'); route(); } catch (e) { toast(e.message); }
  });
}

/* ---------------- Weibo ---------------- */
async function renderWeibo() {
  const q = new URLSearchParams(location.search);
  let d;
  try { d = await api('/posts?' + q.toString()); } catch (e) { app.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  const items = d.items || [];
  app.innerHTML = `
    <div class="section-head"><div><h1>动态</h1><p class="small">正文只存主服务器；图片和视频存在共享盘，所有站点可见。</p></div></div>
    ${state.me ? `<form id="post-form" class="card pad" style="margin-bottom:16px">
      <textarea class="input" name="content" maxlength="2000" placeholder="此刻的想法…"></textarea>
      <div class="toolbar" style="margin:10px 0 0"><input type="file" name="media" id="post-media" accept="image/*,video/*,audio/*" multiple class="input" style="max-width:320px"><div class="grow"></div><button class="btn primary">发布</button></div>
    </form>` : `<div class="notice">登录后可以发布动态、点赞、评论和转发。</div>`}
    <div id="feed">${items.length ? items.map(p => postHTML(p)).join('') : '<div class="empty">还没有动态。</div>'}</div>`;
  const pf = document.getElementById('post-form');
  if (pf) pf.onsubmit = async (e) => {
    e.preventDefault();
    const f = new FormData(e.target);
    try {
      const ids = [];
      for (const file of document.getElementById('post-media').files) {
        toast('上传媒体…'); const up = await uploadFile(file, 'shared', ''); ids.push(up.file.id);
      }
      await api('/posts', { method: 'POST', body: { content: f.get('content'), media_file_ids: ids } });
      toast('已发布'); route();
    } catch (err) { toast(err.message); }
  };
  bindPostActions();
}
function postHTML(x) {
  const p = x.post;
  return `<div class="post">
    <div class="head"><span class="avatar">${x.author.avatar ? `<img src="/f/${attr(x.author.avatar)}" alt="">` : esc(initials(x.author.username))}</span>
      <div><a href="/u/${encodeURIComponent(x.author.username || '')}" data-link style="font-weight:600">${esc(x.author.username || '用户')}</a><div class="small faint">${ago(p.created_at)}</div></div></div>
    ${p.repost_of ? `<div class="small faint" style="margin:8px 0 0">转发了一条动态</div>` : ''}
    ${p.content ? `<div class="content">${esc(p.content)}</div>` : ''}
    ${x.original ? `<div class="side-card" style="margin-top:8px"><div class="small faint">@${esc(x.original.user_id)}</div><div class="small" style="white-space:pre-wrap">${esc(x.original.content)}</div></div>` : ''}
    ${x.media?.length ? `<div class="media">${x.media.map(m => m.kind === 'video' ? `<video src="${attr(m.url)}" controls></video>` : `<a href="${attr(m.url)}" target="_blank"><img src="${attr(m.url)}" alt="${attr(m.name)}" loading="lazy"></a>`).join('')}</div>` : ''}
    <div class="foot"><button data-preact="${p.id}" data-kind="like">👍 ${fmtNum(p.like_count)}</button><button data-pcomment="${p.id}">💬 ${fmtNum(p.comment_count)}</button><button data-prepost="${p.id}">🔁 ${fmtNum(p.repost_count)}</button></div>
  </div>`;
}
function bindPostActions() {
  document.querySelectorAll('[data-preact]').forEach(b => b.onclick = async () => {
    if (!state.me) return goto('/login');
    try { await api(`/posts/${b.dataset.preact}/react`, { method: 'POST', body: { kind: 'like' } }); route(); } catch (e) { toast(e.message); }
  });
  document.querySelectorAll('[data-prepost]').forEach(b => b.onclick = () => {
    if (!state.me) return goto('/login');
    openModal(`<h2>转发动态</h2><form id="rp-form"><label class="field"><span>附言（可选）</span><textarea class="input" name="content"></textarea></label><div class="actions"><button type="button" class="btn ghost" id="cancel">取消</button><button class="btn primary" type="submit">转发</button></div></form>`);
    document.getElementById('cancel').onclick = closeModal;
    document.getElementById('rp-form').onsubmit = async (e) => {
      e.preventDefault();
      try { await api(`/posts/${b.dataset.prepost}/repost`, { method: 'POST', body: { content: new FormData(e.target).get('content') } }); closeModal(); toast('已转发'); route(); } catch (err) { toast(err.message); }
    };
  });
  document.querySelectorAll('[data-pcomment]').forEach(b => b.onclick = async () => {
    const id = b.dataset.pcomment;
    let items = [];
    try { items = (await api(`/posts/${id}/comments`)).items || []; } catch (_) {}
    openModal(`<h2>评论</h2><div style="max-height:320px;overflow:auto">${items.length ? items.map(c => `<div class="comment"><div class="avatar">${esc(initials(c.username))}</div><div class="body"><div class="who">${esc(c.username)} · ${ago(c.comment.created_at)}</div><div class="txt">${esc(c.comment.content)}</div></div></div>`).join('') : '<div class="empty">还没有评论。</div>'}</div>
      ${state.me ? `<form id="pc-form" style="display:flex;gap:8px;margin-top:12px"><input class="input" name="content" placeholder="写评论…" required><button class="btn primary small">发送</button></form>` : ''}`);
    const f = document.getElementById('pc-form');
    if (f) f.onsubmit = async (e) => {
      e.preventDefault();
      try { await api(`/posts/${id}/comments`, { method: 'POST', body: { content: new FormData(e.target).get('content') } }); closeModal(); toast('已评论'); route(); } catch (err) { toast(err.message); }
    };
  });
}

/* ---------------- IM ---------------- */
async function renderIM() {
  if (needLogin()) return;
  let halls = [], groups = [];
  try { [halls, groups] = await Promise.all([api('/halls'), api('/groups')]); } catch (e) { app.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  const q = new URLSearchParams(location.search);
  const groupList = (groups.items || []).map(x => x.group || x);
  const sel = q.get('room') || (halls.items[0] ? 'hall-' + halls.items[0].hall.id : groupList[0] ? 'group-' + groupList[0].id : '');
  app.innerHTML = `
    <div class="section-head"><div><h1>聊天</h1><p class="small">公共大厅的聊天记录是共享盘里的一个 JSONL 文件；私人群存在创建者的私人加密盘。</p></div>
      <div class="toolbar" style="margin:0"><button class="btn" id="new-hall">创建大厅</button><button class="btn" id="new-group">创建私人群</button></div></div>
    <div class="im-layout">
      <div class="card pad" style="padding:12px"><div class="small faint" style="margin:4px 6px 8px">大厅（共享）</div>
        <div class="room-list">${halls.items.map(x => `<div class="room ${sel === 'hall-' + x.hall.id ? 'active' : ''}" data-room="hall-${x.hall.id}"><div class="n">${esc(x.hall.name)}</div><div class="d">${x.members} 人 · ${x.hall.join_policy} · ${esc(x.hall.description || '')}</div></div>`).join('') || '<div class="empty small">还没有大厅</div>'}</div>
        <div class="small faint" style="margin:12px 6px 8px">私人群（加密）</div>
        <div class="room-list">${groupList.map(g => `<div class="room ${sel === 'group-' + g.id ? 'active' : ''}" data-room="group-${g.id}"><div class="n">🔒 ${esc(g.name)}</div><div class="d">创建者 #${g.owner_id}</div></div>`).join('') || '<div class="empty small">还没有群</div>'}</div>
      </div>
      <div class="card chat" id="chat-pane"><div class="empty" style="margin:auto">选择左边的一个房间</div></div>
    </div>`;
  document.querySelectorAll('[data-room]').forEach(el => el.onclick = () => goto('/im?room=' + el.dataset.room));
  document.getElementById('new-hall').onclick = () => {
    openModal(`<h2>创建公共大厅</h2><form id="nh-form"><label class="field"><span>名称</span><input class="input" name="name" required></label><label class="field"><span>简介</span><textarea class="input" name="description"></textarea></label><div class="two-col"><label class="field"><span>加入策略</span><select class="input" name="join_policy"><option value="open">任何人可加入</option><option value="approval">需申请</option><option value="closed">关闭</option></select></label><label class="field"><span>读取策略</span><select class="input" name="read_policy"><option value="public">所有人可读</option><option value="members">仅成员可读</option></select></label></div><div class="actions"><button type="button" class="btn ghost" id="cancel">取消</button><button class="btn primary" type="submit">创建</button></div></form>`);
    document.getElementById('cancel').onclick = closeModal;
    document.getElementById('nh-form').onsubmit = async (e) => {
      e.preventDefault(); const f = new FormData(e.target);
      try { const h = await api('/halls', { method: 'POST', body: { name: f.get('name'), description: f.get('description'), join_policy: f.get('join_policy'), read_policy: f.get('read_policy') } }); closeModal(); toast('已创建'); goto('/im?room=hall-' + h.id); } catch (err) { toast(err.message); }
    };
  };
  document.getElementById('new-group').onclick = () => {
    openModal(`<h2>创建私人群</h2><p class="small muted">群聊记录会写入你的私人加密存储，其他成员通过本平台读取。</p><form id="ng-form"><label class="field"><span>名称</span><input class="input" name="name" required></label><label class="field"><span>简介</span><textarea class="input" name="description"></textarea></label><div class="actions"><button type="button" class="btn ghost" id="cancel">取消</button><button class="btn primary" type="submit">创建</button></div></form>`);
    document.getElementById('cancel').onclick = closeModal;
    document.getElementById('ng-form').onsubmit = async (e) => {
      e.preventDefault(); const f = new FormData(e.target);
      try { const g = await api('/groups', { method: 'POST', body: { name: f.get('name'), description: f.get('description') } }); closeModal(); toast('已创建'); goto('/im?room=group-' + g.id); } catch (err) { toast(err.message); }
    };
  };
  if (sel) openRoom(sel);
}
async function openRoom(sel) {
  const [type, id] = sel.split('-');
  const pane = document.getElementById('chat-pane');
  if (!pane) return;
  let info = null;
  let d = { items: [] };
  try {
    if (type === 'hall') {
      const halls = await api('/halls');
      const entry = halls.items.find(x => String(x.hall.id) === id);
      if (!entry) throw new Error('大厅不存在');
      info = { name: entry.hall.name, desc: entry.hall.description, policy: entry.hall.join_policy, read: entry.hall.read_policy, joined: entry.joined, owner: entry.hall.owner_id };
      if (entry.joined || entry.hall.read_policy === 'public') {
        d = await api('/halls/' + id + '/messages');
      }
    } else {
      const gs = (await api('/groups')).items.map(x => x.group || x);
      const g = gs.find(x => String(x.id) === id);
      if (!g) throw new Error('群组不存在');
      info = { name: g.name, desc: '', owner: g.owner_id };
      d = await api('/groups/' + id + '/messages');
    }
  } catch (e) {
    pane.innerHTML = `<div class="empty" style="margin:auto">${esc(e.message)}</div>`; return;
  }
  const canSend = type === 'group' || info.joined || info.policy === 'open';
  const canManage = state.me && (state.me.role === 2 || (info.owner && Number(info.owner) === Number(state.me.id)));
  pane.innerHTML = `<div class="chat-head"><div><div style="font-weight:700">${type === 'group' ? '🔒 ' : ''}${esc(info.name)}</div><div class="small faint">${esc(info.desc || '')}${info.policy ? ' · ' + esc(info.policy) : ''}${info.read ? ' · 读取 ' + esc(info.read) : ''}</div></div><div>${type === 'hall' ? `${info.joined ? '' : (info.policy === 'closed' ? '' : `<button class="btn small ghost" id="join-room">${info.policy === 'approval' ? '申请加入' : '加入'}</button>`)}${canManage ? `<button class="btn small ghost" id="hall-settings">设置</button>` : ''}` : `${canManage ? `<button class="btn small ghost" id="add-member">邀请</button>` : ''}`}</div></div>
    <div class="chat-log" id="chat-log">${(d.items || []).map(m => msgHTML(m, type)).join('') || `<div class="empty">${type === 'hall' && !info.joined && info.read !== 'public' ? '该大厅仅成员可读，加入后即可查看聊天记录。' : '还没有消息，打个招呼吧。'}</div>`}</div>
    ${canSend ? `<form class="chat-input" id="send-form"><input class="input" name="content" placeholder="输入消息…" autocomplete="off" required><button class="btn primary">发送</button></form>` : `<div class="chat-input"><span class="small muted">${info.policy === 'approval' ? '申请通过后即可发言。' : '加入大厅后即可发言。'}</span></div>`}`;
  const log = document.getElementById('chat-log');
  log.scrollTop = log.scrollHeight;
  const sendForm = document.getElementById('send-form');
  if (sendForm) sendForm.onsubmit = async (e) => {
    e.preventDefault();
    const input = e.target.querySelector('input');
    try {
      await api(type === 'hall' ? `/halls/${id}/messages` : `/groups/${id}/messages`, { method: 'POST', body: { content: input.value } });
      openRoom(sel);
    } catch (err) { toast(err.message); }
  };
  const join = document.getElementById('join-room');
  if (join) join.onclick = async () => {
    try {
      const r = await api(`/halls/${id}/join`, { method: 'POST' });
      toast(r.status === 'pending' ? '已提交申请，等待管理员通过' : '已加入');
      openRoom(sel);
    } catch (e) { toast(e.message); }
  };
  const hs = document.getElementById('hall-settings');
  if (hs) hs.onclick = () => {
    openModal(`<h2>大厅设置</h2><form id="hs-form"><div class="two-col"><label class="field"><span>加入策略</span><select class="input" name="join_policy"><option value="open">开放</option><option value="approval">需申请</option><option value="closed">关闭</option></select></label><label class="field"><span>读取策略</span><select class="input" name="read_policy"><option value="public">所有人</option><option value="members">仅成员</option></select></label></div><div class="actions"><button type="button" class="btn ghost" id="cancel">取消</button><button class="btn primary" type="submit">保存</button></div></form>`);
    document.getElementById('cancel').onclick = closeModal;
    document.getElementById('hs-form').onsubmit = async (e) => {
      e.preventDefault(); const f = new FormData(e.target);
      try { await api(`/halls/${id}/settings`, { method: 'POST', body: { join_policy: f.get('join_policy'), read_policy: f.get('read_policy') } }); closeModal(); toast('已保存'); openRoom(sel); } catch (err) { toast(err.message); }
    };
  };
  const am = document.getElementById('add-member');
  if (am) am.onclick = () => {
    openModal(`<h2>邀请成员</h2><form id="am-form"><label class="field"><span>用户名</span><input class="input" name="username" required></label><div class="actions"><button type="button" class="btn ghost" id="cancel">取消</button><button class="btn primary" type="submit">加入群组</button></div></form>`);
    document.getElementById('cancel').onclick = closeModal;
    document.getElementById('am-form').onsubmit = async (e) => {
      e.preventDefault();
      try { await api(`/groups/${id}/members`, { method: 'POST', body: { username: new FormData(e.target).get('username') } }); closeModal(); toast('已加入'); } catch (err) { toast(err.message); }
    };
  };
}
function msgHTML(m, type) {
  const mine = state.me && m.message.user_id === state.me.id;
  return `<div class="msg ${mine ? 'mine' : ''}"><div class="avatar">${esc(initials(m.username))}</div><div><div class="who">${esc(m.username || '用户')} · ${ago(m.message.created_at)}</div><div class="bubble">${esc(m.message.content)}</div></div></div>`;
}

/* ---------------- Settings / Profile / Admin ---------------- */
async function renderSettings() {
  if (needLogin()) return;
  let priv;
  try { priv = await api('/private'); } catch (e) { app.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  let keys = { items: [] };
  try { keys = await api('/keys'); } catch (_) {}
  const p = state.profile || {};
  const usedPct = Math.min(100, priv.quota_bytes ? (priv.used_bytes / priv.quota_bytes) * 100 : 0);
  app.innerHTML = `<div class="section-head"><div><h1>设置</h1><p class="small">账户、私人存储与开发者 API。</p></div></div>
    <div class="grid wide">
      <div class="card pad"><h3>私人存储</h3>
        <p class="small">${priv.enabled ? '已开通。私人文件使用 zstd + AES-256-GCM 写入你的归档包。' : '尚未开通。开通后才会创建你的加密压缩包。'}</p>
        <dl class="kv"><dt>额度</dt><dd>${fmtBytes(priv.quota_bytes)}</dd><dt>已用</dt><dd>${fmtBytes(priv.used_bytes)}</dd><dt>预留</dt><dd>${fmtBytes(priv.reserved_bytes)}</dd><dt>压缩等级</dt><dd>${priv.compression}</dd><dt>同存份数</dt><dd>${priv.copies} 站</dd><dt>共享盘已用</dt><dd>${fmtBytes(priv.shared_used)} / ${fmtBytes(priv.shared_max)}</dd></dl>
        <div class="bar"><i style="width:${usedPct}%"></i></div>
        ${priv.enabled ? '' : `<button class="btn primary" id="enable-private" style="margin-top:14px">手动开通 1GB 私人空间</button>`}
        <div class="tier"><div class="t t"><b>Ⅰ级</b><p>15 元 · 11GB</p></div><div class="t"><b>Ⅱ级</b><p>50 元 · 61GB</p></div><div class="t"><b>Ⅲ级</b><p>100 元 · 121GB</p></div></div>
        <form id="redeem-form" style="display:flex;gap:8px;margin-top:12px"><input class="input" name="code" placeholder="兑换码"><button class="btn">兑换</button></form>
      </div>
      <div class="card pad"><h3>个人资料</h3>
        <form id="profile-form"><label class="field"><span>显示名称</span><input class="input" name="display_name" value="${attr(p.display_name || state.me.username)}"></label><label class="field"><span>简介</span><textarea class="input" name="bio">${esc(p.bio || '')}</textarea></label><label class="field"><span>头像文件 ID（可选）</span><input class="input" name="avatar_file_id" value="${attr(p.avatar_file_id || '')}"></label><button class="btn primary">保存资料</button></form>
        <div class="chips" style="margin-top:12px">${state.titles.map(t => `<span class="chip green">${esc(t.name)}</span>`).join('') || '<span class="faint small">暂无头衔</span>'}</div>
      </div>
      <div class="card pad"><h3>开发者 API</h3><p class="small">API key 可代表你访问存储 API。密钥只显示一次。</p>
        <button class="btn" id="new-key">创建 API key</button>
        <div id="keys" style="margin-top:12px">${(keys.items || []).map(k => `<div class="file-row"><div class="icon">🔑</div><div><div class="name">${esc(k.name)}</div><div class="meta"><code>${esc(k.prefix)}…</code> · ${esc(k.scopes)} · 最后使用 ${k.last_used_at ? ago(k.last_used_at) : '从未'}</div></div><button class="btn small danger" data-delkey="${k.id}">删除</button></div>`).join('') || '<div class="empty small">还没有 API key。</div>'}</div>
      </div>
    </div>`;
  const en = document.getElementById('enable-private');
  if (en) en.onclick = async () => { try { await api('/private/enable', { method: 'POST' }); toast('已开通'); route(); } catch (e) { toast(e.message); } };
  document.getElementById('redeem-form').onsubmit = async (e) => {
    e.preventDefault();
    try { const d = await api('/redeem', { method: 'POST', body: { code: new FormData(e.target).get('code') } }); toast('兑换成功：' + (d.message || '')); await loadMe(); route(); } catch (err) { toast(err.message); }
  };
  document.getElementById('profile-form').onsubmit = async (e) => {
    e.preventDefault(); const f = new FormData(e.target);
    try { state.profile = await api('/profile', { method: 'POST', body: { display_name: f.get('display_name'), bio: f.get('bio'), avatar_file_id: f.get('avatar_file_id') } }); renderTop(); toast('已保存'); } catch (err) { toast(err.message); }
  };
  document.getElementById('new-key').onclick = async () => {
    try { const k = await api('/keys', { method: 'POST', body: { name: 'web', scopes: 'files:read,files:write' } });
      openModal(`<h2>API key 已创建</h2><p class="small">只显示这一次，请立即复制。</p><div class="code" style="padding:12px;word-break:break-all">${esc(k.secret)}</div><div class="actions"><button class="btn primary" id="copy-key">复制</button><button class="btn ghost" id="close-key">关闭</button></div>`);
      document.getElementById('close-key').onclick = () => { closeModal(); route(); };
      document.getElementById('copy-key').onclick = () => { navigator.clipboard?.writeText(k.secret); toast('已复制'); };
    } catch (e) { toast(e.message); }
  };
  document.querySelectorAll('[data-delkey]').forEach(b => b.onclick = async () => { if (!confirm('删除这个 API key？')) return; try { await api(`/keys/${b.dataset.delkey}/delete`, { method: 'POST' }); toast('已删除'); route(); } catch (e) { toast(e.message); } });
}

async function renderUser(name) {
  let d;
  try { d = await api('/u/' + encodeURIComponent(name)); } catch (e) { app.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  const u = d.user || {}, p = d.profile || {};
  app.innerHTML = `<div class="hero-mini" style="display:flex;gap:18px;align-items:center;flex-wrap:wrap">
      <span class="avatar" style="width:76px;height:76px;font-size:26px">${p.avatar_file_id ? `<img src="/f/${attr(p.avatar_file_id)}" alt="">` : esc(initials(p.display_name || u.username))}</span>
      <div><h1 style="font-size:30px">${esc(p.display_name || u.username || name)}</h1><div class="small muted">@${esc(u.username || name)} · 加入于 ${dt(d.created_at)}</div><div class="chips" style="margin-top:8px">${(d.titles || []).map(t => `<span class="chip gold">${esc(t.name)}</span>`).join('')}</div></div>
      ${state.me && state.me.id !== u.id ? `<button class="btn" id="follow-btn">关注</button>` : ''}
    </div>
    <div class="grid wide"><div class="card pad"><h3>简介</h3><p style="white-space:pre-wrap">${esc(p.bio || '这个人很神秘。')}</p></div>
    <div class="card pad"><h3>统计</h3><dl class="kv"><dt>共享文件</dt><dd>${fmtNum((d.stats||{}).files || 0)}</dd><dt>视频</dt><dd>${fmtNum((d.stats||{}).videos || 0)}</dd><dt>图片</dt><dd>${fmtNum((d.stats||{}).images || 0)}</dd><dt>动态</dt><dd>${fmtNum((d.stats||{}).posts || 0)}</dd><dt>关注</dt><dd>${fmtNum((d.stats||{}).following || 0)} / ${fmtNum((d.stats||{}).followers || 0)}</dd></dl></div></div>
    <div class="section-head"><h2>TA 的视频</h2></div><div id="u-videos" class="grid wide"></div>
    <div class="section-head"><h2>TA 的图片</h2></div><div id="u-images" class="grid masonry"></div>
    <div class="section-head"><h2>TA 的动态</h2></div><div id="u-posts"></div>`;
  const fb = document.getElementById('follow-btn');
  if (fb) fb.onclick = async () => { try { await api('/follow/' + u.id, { method: 'POST' }); toast('已关注'); } catch (e) { toast(e.message); } };
  try {
    const [v, i, p] = await Promise.all([api('/videos?uploader=' + u.id), api('/images?uploader=' + u.id), api('/posts?user=' + u.id)]);
    document.getElementById('u-videos').innerHTML = (v.items || []).map(x => `<a class="video-card" href="/video/${x.video.id}" data-link><div class="thumb">${x.cover ? `<img src="${fhref(x.cover)}">` : '<div style="position:absolute;inset:0;display:grid;place-items:center;font-size:40px">🎬</div>'}</div><div class="body"><h3>${esc(x.video.title)}</h3></div></a>`).join('') || '<div class="empty">还没有视频</div>';
    document.getElementById('u-images').innerHTML = (i.items || []).map(x => `<a class="image-card" href="/image/${x.image.id}" data-link><img src="${fhref(x.file)}" loading="lazy"><div class="cap">${esc(x.image.title)}</div></a>`).join('') || '<div class="empty">还没有图片</div>';
    document.getElementById('u-posts').innerHTML = (p.items || []).map(postHTML).join('') || '<div class="empty">还没有动态</div>';
    bindPostActions();
  } catch (_) {}
}

async function renderDocs() {
  app.innerHTML = `<div class="section-head"><div><h1>文档</h1><p class="small">存储 API、开放接口与部署说明。</p></div></div>
    <div class="card pad" id="docs-body"><div class="empty small">加载中…</div></div>`;
  let md = '';
  try {
    const r = await fetch('/api/om/docs');
    if (r.ok) md = await r.text();
  } catch (_) {}
  if (!md) md = '# ourmemory API\n\n文档暂未就绪，请稍后再试。';
  const el = document.getElementById('docs-body');
  el.innerHTML = mdToHtml(md);
}

// Minimal markdown to HTML for the docs page. Supports headings, code fences,
// inline code, lists, tables (pipe rows), links and bold. Not a full parser —
// docs/API.md is the source of truth and stays plain markdown.
function mdToHtml(md) {
  const escAll = (s) => esc(s);
  const inline = (s) => esc(s)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  const lines = md.split(/\r?\n/);
  let out = '', inCode = false, codeBuf = [], listBuf = [], tableBuf = [];
  const flushList = () => { if (listBuf.length) { out += '<ul>' + listBuf.map((l) => `<li>${inline(l)}</li>`).join('') + '</ul>'; listBuf = []; } };
  const flushTable = () => {
    if (tableBuf.length) {
      const rows = tableBuf.filter((r) => !/^\s*\|[\s:|-]+\|\s*$/.test(r));
      if (rows.length) {
        out += '<table class="tbl"><thead><tr>' + rows[0].split('|').slice(1, -1).map((c) => `<th>${inline(c.trim())}</th>`).join('') + '</tr></thead><tbody>';
        for (const r of rows.slice(1)) out += '<tr>' + r.split('|').slice(1, -1).map((c) => `<td>${inline(c.trim())}</td>`).join('') + '</tr>';
        out += '</tbody></table>';
      }
      tableBuf = [];
    }
  };
  for (const line of lines) {
    if (line.trim().startsWith('```')) {
      if (inCode) { out += `<pre class="code">${escAll(codeBuf.join('\n'))}</pre>`; codeBuf = []; }
      inCode = !inCode;
      continue;
    }
    if (inCode) { codeBuf.push(line); continue; }
    if (/^\s*\|.*\|\s*$/.test(line)) { flushList(); tableBuf.push(line); continue; }
    flushTable();
    const hm = line.match(/^(#{1,4})\s+(.*)$/);
    if (hm) { flushList(); out += `<h${hm[1].length}>${inline(hm[2])}</h${hm[1].length}>`; continue; }
    const lm = line.match(/^\s*[-*]\s+(.*)$/);
    if (lm) { listBuf.push(lm[1]); continue; }
    flushList();
    if (line.trim() === '') continue;
    if (/^\s*\d+\.\s/.test(line)) { out += `<p class="small">${inline(line.trim())}</p>`; continue; }
    out += `<p>${inline(line)}</p>`;
  }
  if (inCode) out += `<pre class="code">${escAll(codeBuf.join('\n'))}</pre>`;
  flushList(); flushTable();
  return out;
}

async function renderAdmin() {
  if (needLogin()) return;
  if (state.me.role !== 2) { app.innerHTML = `<div class="empty">需要管理员权限。</div>`; return; }
  let stats = {}, codes = {}, users = {}, nodes = {}, packs = {};
  try { [stats, codes, users, nodes, packs] = await Promise.all([api('/admin/stats'), api('/admin/codes'), api('/admin/users'), api('/admin/nodes'), api('/admin/packs')]); } catch (e) { app.innerHTML = `<div class="empty">${esc(e.message)}</div>`; return; }
  app.innerHTML = `<div class="section-head"><div><h1>管理后台</h1><p class="small">容量、用户、兑换码与贡献节点。</p></div><a class="btn" href="/files/@manage" target="_blank" rel="noreferrer">OpenList 完整管理台</a></div>
    <div class="admin-banner"><div><h3>OpenList 原生管理功能</h3><div class="small muted">用户管理、存储驱动、站点设置、任务与索引，都在独立的 OpenList 管理台里。首次进入需要再登录一次 OpenList 管理员账号。</div><div class="chips" style="margin-top:8px"><a class="chip" href="/files/@manage/users" target="_blank" rel="noreferrer">用户</a><a class="chip" href="/files/@manage/storages" target="_blank" rel="noreferrer">存储</a><a class="chip" href="/files/@manage/settings" target="_blank" rel="noreferrer">设置</a><a class="chip" href="/files/@manage/tasks" target="_blank" rel="noreferrer">任务</a></div></div><a class="btn primary" href="/files/@manage" target="_blank" rel="noreferrer">打开管理台</a></div>
    <div class="stat-cards">${Object.entries(stats).map(([k, v]) => `<div class="stat-card"><b>${typeof v === 'number' ? fmtNum(v) : esc(v)}</b><span>${esc(k)}</span></div>`).join('')}</div>
    <div class="section-head"><h2>兑换码</h2></div>
    <div class="card pad"><form id="code-form" class="toolbar"><select class="input" name="kind" style="max-width:150px"><option value="tier">等级</option><option value="reservation">预留包</option><option value="quota">额度</option></select><input class="input" name="tier" type="number" min="0" max="3" value="1" style="max-width:100px" placeholder="tier"><input class="input" name="bytes" type="number" placeholder="bytes" style="max-width:180px"><input class="input" name="days" type="number" placeholder="天数" style="max-width:120px"><input class="input" name="count" type="number" min="1" max="50" value="1" style="max-width:100px"><button class="btn primary">生成</button></form>
      <div style="margin-top:10px">${(codes.items || []).map(c => `<div class="file-row"><div class="icon">🎟️</div><div><div class="name"><code>${esc(c.code)}</code></div><div class="meta">${esc(c.kind)} · tier ${c.tier} · ${fmtBytes(c.bytes)} · ${c.days} 天 · ${c.used_by ? '已用' : '未用'}</div></div></div>`).join('') || '<div class="empty small">还没有兑换码</div>'}</div></div>
    <div class="section-head"><h2>用户</h2></div>
    <div class="card pad" style="overflow:auto"><table class="tbl"><thead><tr><th>ID</th><th>用户名</th><th>角色</th><th>等级</th><th>邮箱</th><th>私人额度</th><th>已用</th><th>贡献</th></tr></thead><tbody>${(users.items || []).map(u => `<tr><td>${u.user_id}</td><td>${esc(u.username)}</td><td>${esc(u.role)}</td><td>${u.tier}</td><td>${esc(u.email || '')}</td><td>${fmtBytes(u.private_quota_bytes)}</td><td>${fmtBytes(u.private_used_bytes)}</td><td>${fmtBytes(u.contribution_quota_bytes)}</td></tr>`).join('')}</tbody></table></div>
    <div class="section-head"><h2>贡献节点</h2></div>
    <div class="card pad"><table class="tbl"><thead><tr><th>ID</th><th>名称</th><th>状态</th><th>容量</th><th>已用</th><th>私人</th><th>最后心跳</th></tr></thead><tbody>${(nodes.items || []).map(n => `<tr><td>${n.id}</td><td>${esc(n.name)}</td><td>${esc(n.status)}</td><td>${fmtBytes(n.capacity_bytes)}</td><td>${fmtBytes(n.used_bytes)}</td><td>${n.private_capable ? '是' : '否'}</td><td>${ago(n.last_seen_at)}</td></tr>`).join('')}</tbody></table></div>
    <div class="section-head"><h2>存储包</h2></div>
    <div class="card pad"><table class="tbl"><thead><tr><th>ID</th><th>范围</th><th>大小</th><th>可回收</th><th></th></tr></thead><tbody>${(packs.items || []).map(p => `<tr><td><code>${esc(p.id)}</code></td><td>${esc(p.scope)}</td><td>${fmtBytes(p.size)}</td><td>${fmtBytes(p.waste)}</td><td><button class="btn" data-compact="${esc(p.id)}">整理</button></td></tr>`).join('') || '<tr><td colspan="5" class="small">还没有存储包</td></tr>'}</tbody></table></div>`;
  document.getElementById('code-form').onsubmit = async (e) => {
    e.preventDefault(); const f = new FormData(e.target);
    try {
      const d = await api('/admin/codes', { method: 'POST', body: { kind: f.get('kind'), tier: Number(f.get('tier') || 0), bytes: Number(f.get('bytes') || 0), days: Number(f.get('days') || 0), count: Number(f.get('count') || 1) } });
      openModal(`<h2>兑换码</h2><div class="code" style="padding:12px;white-space:pre-wrap">${d.codes.map(esc).join('\n')}</div><div class="actions"><button class="btn primary" id="close-codes">关闭</button></div>`);
      document.getElementById('close-codes').onclick = () => { closeModal(); route(); };
    } catch (err) { toast(err.message); }
  };
  document.querySelectorAll('[data-compact]').forEach(b => {
    b.onclick = async () => {
      if (!confirm('整理会重写该存储包，期间暂停写入，继续？')) return;
      b.disabled = true;
      try { const d = await api('/admin/compact', { method: 'POST', body: { pack: b.dataset.compact } }); toast(`已整理，回收 ${fmtBytes(d.saved)}`); route(); }
      catch (err) { toast(err.message); b.disabled = false; }
    };
  });
}

/* ---------------- Router ---------------- */
async function loadMe() {
  const d = await api('/me').catch(() => null);
  state.me = d ? d.user : null; state.profile = d ? d.profile : null; state.titles = d ? d.titles || [] : [];
}
// A subdomain such as video.om.example.com is rewritten to /video on the
// server, but the browser still sees "/". Remember the entry subdomain so the
// first render matches what the visitor typed.
const SUBDOMAIN_PAGES = ['video', 'image', 'weibo', 'im'];
function effectivePath() {
  const path = location.pathname;
  if (path !== '/' && path !== '') return path;
  const host = location.hostname.toLowerCase();
  const sub = SUBDOMAIN_PAGES.find((s) => host.startsWith(s + '.'));
  return sub ? '/' + sub : path || '/';
}
async function route() {
  const path = effectivePath();
  state.route = path;
  document.body.classList.toggle('home', path === '/');
  renderTop();
  if (path === '/login') return renderLogin();
  if (path === '/register') return renderRegister();
  if (path === '/video') return renderVideoList();
  const vm = path.match(/^\/video\/(\d+)$/); if (vm) return renderVideoDetail(vm[1]);
  if (path === '/image') return renderImageList();
  const im = path.match(/^\/image\/(\d+)$/); if (im) return renderImageDetail(im[1]);
  if (path === '/weibo') return renderWeibo();
  if (path === '/im') return renderIM();
  if (path === '/settings') return renderSettings();
  if (path === '/admin') return renderAdmin();
  if (path === '/docs') return renderDocs();
  const um = path.match(/^\/u\/([^/]+)$/); if (um) return renderUser(decodeURIComponent(um[1]));
  return renderHome();
}
(async function boot() {
  await loadMe();
  renderTop();
  route();
})();
