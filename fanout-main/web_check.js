
const $ = s => document.querySelector(s);
const $$ = s => Array.from(document.querySelectorAll(s));
const ICON = {
  copy:'<svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>',
  stop:'<svg viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>',
  redo:'<svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/></svg>',
  ok:'<svg viewBox="0 0 24 24"><path d="M20 6 9 17l-5-5"/></svg>',
  bad:'<svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>',
  run:'<svg viewBox="0 0 24 24" class="spin"><path d="M21 12a9 9 0 1 1-6.2-8.5"/></svg>',
  wait:'<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/></svg>',
  plus:'<svg viewBox="0 0 24 24"><path d="M12 5v14"/><path d="M5 12h14"/></svg>',
  trash:'<svg viewBox="0 0 24 24"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
  x:'<svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>',
  lock:'<svg viewBox="0 0 24 24" class="lock"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>'
};

// 界面挂在随机前缀下，动态解析当前相对基础路径并加超时保护
function apiPath(p) {
  const clean = p.replace(/^\//, '');
  let path = window.location.pathname || '/';
  if (/\.[a-zA-Z0-9]+$/.test(path)) {
    path = path.substring(0, path.lastIndexOf('/') + 1);
  } else if (!path.endsWith('/')) {
    path = path + '/';
  }
  return path + clean;
}

async function api(path, opts = {}){
  const timeoutMs = opts.timeout || (
    path.includes('/refresh') || path.includes('/provision') || path.includes('/update/apply') || path.includes('/import') || path.includes('/inbound') ? 60000 : 25000
  );
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs);
  opts.signal = controller.signal;
  try {
    const r = await fetch(apiPath(path), opts);
    clearTimeout(timeoutId);
    const d = await r.json().catch(()=>({}));
    if(!r.ok) throw new Error(d.error || ('HTTP ' + r.status));
    return d;
  } catch(err) {
    clearTimeout(timeoutId);
    if (err.name === 'AbortError') {
      throw new Error('请求超时 (' + Math.round(timeoutMs/1000) + '秒)，请检查后端运行状态');
    }
    throw err;
  }
}
function esc(s){ return String(s == null ? '' : s).replace(/[&<>"']/g, c =>
  ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }

let toastTimer;
function toast(msg, bad){
  const el = $('#toast');
  el.textContent = msg;
  el.className = 'toast show' + (bad ? ' bad' : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.className = 'toast'; }, 2400);
}
async function copy(text){
  // navigator.clipboard 只在 HTTPS/localhost 下存在，而面板通常是 http://IP 访问，
  // 所以必须留一条 execCommand 兜底路径，否则复制在正常使用场景里必然失败。
  if(navigator.clipboard && window.isSecureContext){
    try{ await navigator.clipboard.writeText(text); toast('已复制'); return; }
    catch(e){}
  }
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly', '');
  // 放在视口内但不可见：置于视口外会让 iOS 在聚焦时滚动页面
  ta.style.cssText = 'position:fixed;top:0;left:0;width:1px;height:1px;opacity:0;padding:0;border:0';
  document.body.appendChild(ta);
  const prev = document.activeElement;
  ta.focus();
  ta.setSelectionRange(0, ta.value.length);
  let ok = false;
  try{ ok = document.execCommand('copy'); }catch(e){}
  ta.remove();
  if(prev && prev.focus) prev.focus();
  toast(ok ? '已复制' : '复制失败，请手动选中', !ok);
}

let view = {exits:[], direct:[], panel:'', backend:'', public_ip:''};
let inbounds = [];

// 自建模式下入站由 fanout 自己管，界面要提供新建入口；
// 接管 3x-ui 时入站归面板管，这里只读不写。
function isNative(){ return view.backend === 'native'; }
// xray-cf-lite 模式下节点归它管，fanout 只改路由，界面不给新建入口
function isXCL(){ return view.backend === 'xray-cf-lite'; }
const BACKEND_NAME = {'native':'自建 Xray', '3x-ui':'3x-ui', 'xray-cf-lite':'xray-cf-lite'};
function backendName(){ return BACKEND_NAME[view.backend] || '3x-ui'; }

const STATUS = {up:'已连通', starting:'连接中', failed:'失败', stopped:'已停止'};

function getFlagEmoji(countryCode) {
  if (!countryCode) return '🌐';
  const code = countryCode.toUpperCase();
  if (code === 'GOV') return '🏛️';
  if (code === 'EDU') return '🎓';
  if (code === 'CUSTOM' || code === 'GLOBAL') return '🌐';
  if (code.length !== 2) return '🌐';
  const c1 = code.charCodeAt(0) - 65 + 0x1F1E6;
  const c2 = code.charCodeAt(1) - 65 + 0x1F1E6;
  if (c1 >= 0x1F1E6 && c1 <= 0x1F1FF && c2 >= 0x1F1E6 && c2 <= 0x1F1FF) {
    return String.fromCodePoint(c1, c2);
  }
  return '🌐';
}

const COUNTRY_ZH = {
  GOV: '政府公共网络', EDU: '海外高校学术网络', GLOBAL: '全球公网', CUSTOM: '自定义',
  // 亚太地区
  JP: '日本', KR: '韩国', HK: '中国香港', TW: '中国台湾', SG: '新加坡',
  MY: '马来西亚', TH: '泰国', VN: '越南', PH: '菲律宾', ID: '印尼',
  IN: '印度', AU: '澳大利亚', NZ: '新西兰', MO: '中国澳门', KH: '柬埔寨',
  LA: '老挝', MM: '缅甸', BD: '孟加拉', PK: '巴基斯坦', LK: '斯里兰卡',
  NP: '尼泊尔', MV: '马尔代夫', MN: '蒙古',
  // 美洲地区
  US: '美国', CA: '加拿大', BR: '巴西', MX: '墨西哥', AR: '阿根廷',
  CL: '智利', CO: '哥伦比亚', PE: '秘鲁', CR: '哥斯达黎加', PA: '巴拿马',
  UY: '乌拉圭', EC: '厄瓜多尔', VE: '委内瑞拉', BO: '玻利维亚', PY: '巴拉圭',
  DO: '多米尼加', JM: '牙买加', TT: '特立尼达和多巴哥', BS: '巴哈马',
  // 欧洲地区
  GB: '英国', DE: '德国', FR: '法国', NL: '荷兰', IT: '意大利',
  ES: '西班牙', CH: '瑞士', SE: '瑞典', NO: '挪威', FI: '芬兰',
  DK: '丹麦', IE: '爱尔兰', BE: '比利时', AT: '奥地利', PL: '波兰',
  CZ: '捷克', HU: '匈牙利', PT: '葡萄牙', GR: '希腊', RO: '罗马尼亚',
  BG: '保加利亚', UA: '乌克兰', RU: '俄罗斯', TR: '土耳其', IS: '冰岛',
  LU: '卢森堡', EE: '爱沙尼亚', LV: '拉脱维亚', LT: '立陶宛', HR: '克罗地亚',
  RS: '塞尔维亚', SI: '斯洛文尼亚', SK: '斯洛伐克', CY: '塞浦路斯', MT: '马耳他',
  MD: '摩尔多瓦', BY: '白俄罗斯', GE: '格鲁吉亚', AM: '亚美尼亚', AZ: '阿塞拜疆',
  // 中东与中亚
  AE: '阿联酋', SA: '沙特阿拉伯', IL: '以色列', KZ: '哈萨克斯坦', UZ: '乌兹别克斯坦',
  // 非洲地区
  ZA: '南非', EG: '埃及', MA: '摩洛哥', DZ: '阿尔及利亚', TN: '突尼斯',
  NG: '尼日利亚', KE: '肯尼亚', GH: '加纳'
};

function formatCountry(code, name) {
  if (!code || code === 'CUSTOM') return '🌐 ' + (name || '自定义');
  if (code === 'GOV') return '🏛️ 全球政府公共机构专网' + (name && name !== 'GOV' && name !== '政府公共网络' ? ' · ' + name : '');
  if (code === 'EDU') return '🎓 海外高校学术科研网' + (name && name !== 'EDU' && name !== '教育网高校' && name !== '海外高校学术网络' ? ' · ' + name : '');
  const flag = getFlagEmoji(code);
  const zh = COUNTRY_ZH[code.toUpperCase()] || '';
  if (zh) {
    return flag + ' ' + code + ' ' + zh + (name && name !== code && name !== zh ? ' · ' + name : '');
  }
  return flag + ' ' + code + (name ? ' · ' + name : '');
}

function renderExits(){
  const list = $('#list');
  const n = view.exits.filter(e => e.status === 'up').length;
  const total = view.exits.length;
  $('#ecount').textContent = n ? n + ' 个' + (total !== n ? '（总计 ' + total + '）' : '') : (total ? '0 个可用' : '');
  const hasInboundsOrExits = view.exits.some(e => (e.inbounds && e.inbounds.length) || e.status === 'up');
  $('#exportAll').disabled = !hasInboundsOrExits;
  $('#stopall').disabled = !n;

  const govCount = view.exits.filter(e => e.ip_type === 'gov').length;
  const eduCount = view.exits.filter(e => e.ip_type === 'edu').length;
  const resCount = view.exits.filter(e => e.ip_type === 'residential').length;
  const purities = view.exits.map(e => e.purity_score || 55);
  const avgPurity = purities.length ? Math.round(purities.reduce((a, b) => a + b, 0) / purities.length) : 0;

  const summaryBar = '<div class="stats-summary">'
    + '<span class="stat-pill">健康出口<b>' + n + '</b></span>'
    + (govCount ? '<span class="stat-pill">🏛️ 政府专网<b style="color:#ebb237">' + govCount + '</b></span>' : '')
    + (eduCount ? '<span class="stat-pill">🎓 学术科研<b style="color:#a855f7">' + eduCount + '</b></span>' : '')
    + '<span class="stat-pill">🏡 住宅家宽<b style="color:#3fa66b">' + resCount + '</b></span>'
    + '<span class="stat-pill">平均纯净度<b style="color:' + (avgPurity>=80?'#3fa66b':'#c9903a') + '">' + (n ? avgPurity + '%' : '—') + '</b></span>'
    + '<span class="stat-pill">联动后端<b>' + esc(backendName()) + '</b></span>'
    + '</div>';

  if(!n){
    list.innerHTML = summaryBar + '<div class="empty">还没有出口'
      + '<div><button class="primary" id="newexit2">'
      + '<svg viewBox="0 0 24 24"><path d="M12 5v14"/><path d="M5 12h14"/></svg>'
      + '新建出口</button></div></div>';
    return;
  }

  list.innerHTML = summaryBar + view.exits.map(e => {
    const label = e.exit_ip || (e.status === 'starting' ? '连接中…' : '—');
    const isGov = e.ip_type === 'gov';
    const isEdu = e.ip_type === 'edu';
    const isRes = e.ip_type === 'residential';
    const typeTag = isGov ? '<span class="tag-gov" title="政府/公共机构专网">🏛️ 政府</span>'
      : (isEdu ? '<span class="tag-edu" title="海外高校学术科研网络">🎓 学术</span>'
      : (isRes ? '<span class="tag-res" title="家庭宽带住宅 IP">🏡 住宅</span>'
      : (e.ip_type === 'mobile' ? '<span class="tag-mob" title="移动蜂窝网络 IP">📱 移动</span>'
      : '<span class="tag-host" title="数据中心机房 IP">🏢 机房</span>')));

    const purity = e.purity_score || 55;
    let pColor = '#c25450';
    if(purity >= 85) pColor = '#3fa66b';
    else if(purity >= 70) pColor = '#4a9eda';
    else if(purity >= 50) pColor = '#c9903a';
    const purityTag = '<span class="tag-purity" style="color:' + pColor + '" title="IP 纯净度评分">' + purity + '%</span>';

    const activeInbounds = (e.inbounds || []).slice(0, 1);
    const chips = activeInbounds.length
      ? activeInbounds.map(i =>
          '<span class="chip-item">'
          + '<button class="chip" data-detail="' + i.id + '" title="'
          +   esc((i.remark || i.protocol) + ' · ' + i.protocol + ' :' + i.port + ' (点击配置客户端/查看详情)') + '">'
          +   esc(i.remark || i.protocol) + ' :' + i.port + '</button>'
          + '<select class="obind chip-select" data-tag="' + esc(i.tag) + '" title="自由切换此节点出口或恢复直连">'
          +   exitOptions(e.host)
          + '</select>'
          + '</span>').join('')
      : '<span class="chip none">无节点</span>';
    const err = e.status === 'failed' && e.err
      ? '<div class="errline" title="' + esc(e.err) + '">' + esc(e.err) + '</div>' : '';
    const place = formatCountry(e.region || e.country_code, e.country);
    const ispText = e.isp ? ' · ' + esc(e.isp) : (e.host ? ' · ' + esc(e.host) : '');

    return '<div class="exit">'
      + '<div class="row">'
      +   '<span class="dot ' + e.status + '" title="' + (STATUS[e.status] || e.status) + '"></span>'
      +   '<span class="ip">' + esc(label) + typeTag + purityTag + '</span>'
      +   '<span class="meta" title="' + esc(place + ispText) + '">' + esc(place + ispText) + '</span>'
      +   '<span class="chips">' + chips + '</span>'
      +   '<span class="socks"><button data-cred="' + e.slot + '" title="SOCKS5 访问凭据">'
      +     ICON.lock + ':' + e.port + '</button></span>'
      +   '<span class="acts">'
      +     '<button class="icon" data-swap="' + e.slot + '" title="换一个节点">' + ICON.redo + '</button>'
      +     '<button class="icon" data-stop="' + e.slot + '" title="停止这个出口">' + ICON.stop + '</button>'
      +   '</span>'
      + '</div>' + err + '</div>';
  }).join('');
}

// 停掉出口后它的入站会留在面板里。这些入站现在走直连，
// 用户既看不出它们和 fanout 的关系，也没有清理入口，所以单独列出来。
function renderOrphans(){
  const box = $('#orphans');
  const list = view.direct || [];
  if(!list.length){ box.innerHTML = ''; return; }
  const hasUp = view.exits.some(e => e.status === 'up');
  box.innerHTML = '<div class="orphan"><div class="top">'
    + '<h3>未绑定出口的入站</h3><span class="count">' + list.length + ' 个，走直连</span>'
    + '<span class="spacer"></span>'
    + (isXCL() ? ''
        : '<button data-delorphans="1" title="删除这些入站">' + ICON.trash + '清理</button>')
    + '</div>'
    + list.map(i =>
        '<div class="orow">'
        + '<button class="chip" data-detail="' + i.id + '" title="'
        +   esc((i.remark || i.protocol) + ' · ' + i.protocol + ' :' + i.port) + '">'
        +   esc(i.remark || i.protocol) + ' :' + i.port + '</button>'
        + '<span class="spacer"></span>'
        + (hasUp
            ? '<select class="obind" data-tag="' + esc(i.tag) + '">' + exitOptions('') + '</select>'
            : '<span class="dim">先开一个出口</span>')
        + (isXCL() ? ''
            : '<button class="icon danger" data-delone="' + i.id + '" data-name="'
              + esc((i.remark || i.protocol) + ' :' + i.port) + '" title="删除这个入站">'
              + ICON.trash + '</button>')
        + '</div>').join('')
    + '</div>';
}

let showAllFailed = false;

function renderJobs(jobs){
  const box = $('#jobs');
  if(!jobs || !jobs.length){
    box.innerHTML = '';
    return;
  }

  const header = '<div class="jobs-bar">'
    + '<span>任务进度 (' + jobs.length + ')</span>'
    + '<span class="spacer"></span>'
    + '<button class="btn-xs" id="clearAllDoneJobs" title="清空所有已完成/失败的任务卡片">🧹 清空任务卡片</button>'
    + '<button class="btn-xs" id="toggleAllFailedBtn">' + (showAllFailed ? '🙈 隐藏全部爆红' : '👁️ 显示全部爆红') + '</button>'
    + '</div>';

  const items = jobs.map(j => {
    const okSteps = j.steps.filter(s => s.status === 'ok');
    const runningSteps = j.steps.filter(s => s.status === 'running' || s.status === 'pending');
    const failedSteps = j.steps.filter(s => s.status === 'failed');

    const renderStep = s => {
      const ic = {ok:ICON.ok, failed:ICON.bad, running:ICON.run}[s.status] || ICON.wait;
      const t = s.detail ? s.label + ' — ' + s.detail : s.label;
      return '<span class="step ' + s.status + '" title="' + esc(t) + '">' + ic
        + esc(s.status === 'ok' && s.detail ? s.detail : s.label) + '</span>';
    };

    let stepsHtml = '';
    stepsHtml += okSteps.map(renderStep).join('');
    stepsHtml += runningSteps.map(renderStep).join('');

    if(failedSteps.length > 0){
      const isRunning = j.status === 'running';
      // 跑完后默认优雅自动折叠隐藏爆红失败项！
      const isFailedOpen = showAllFailed || (isRunning && failedSteps.length < 4);
      stepsHtml += '<button class="btn-xs step failed-summary" data-togglejobfailed="' + esc(j.id) + '" title="点击展开/折叠未连通候选">'
        + (isFailedOpen ? '▲ 收起 ' : '▼ 查看 ') + failedSteps.length + ' 个未连通候选</button>';
      stepsHtml += '<button class="btn-xs danger" data-cleanjobfailed="' + esc(j.id) + '" title="彻底清除此任务里的爆红记录">🧹 清理爆红</button>';
      stepsHtml += '<div class="failed-steps-wrap' + (isFailedOpen ? ' open' : '') + '" id="failed_wrap_' + esc(j.id) + '">'
        + failedSteps.map(renderStep).join('')
        + '</div>';
    }

    const close = j.status === 'running' ? ''
      : '<button class="icon" data-job="' + esc(j.id) + '" title="关闭">' + ICON.x + '</button>';

    return '<div class="job"><div class="top"><strong>' + esc(j.summary) + '</strong>'
      + '<span class="count">' + j.done + '/' + j.total
      + (okSteps.length ? ' · <span style="color:var(--ok)">已成功 ' + okSteps.length + '</span>' : '')
      + (failedSteps.length ? ' · <span style="color:var(--bad)">失败 ' + failedSteps.length + '</span>' : '')
      + '</span>'
      + '<span class="spacer"></span>' + close + '</div>'
      + '<div class="steps">' + stepsHtml + '</div></div>';
  }).join('');

  box.innerHTML = header + items;
}

async function loadGlobalExitLimit(){
  try{
    const s = await api('/api/settings');
    $('#globalExitLimitEnabled').checked = s.exit_limit_enabled !== false;
    $('#globalExitLimit').value = Math.max(1, Math.min(1000, Number(s.exit_limit || 5)));
    $('#globalExitLimitMode').value = (s.exit_limit_mode === 'random' ? 'random' : 'isp');
    $('#globalExitLimitStatus').textContent = (s.exit_limit_enabled !== false ? '已启用：默认上限 ' + (s.exit_limit || 5) + ' / 国家' : '已关闭');
  }catch(e){
    $('#globalExitLimitStatus').textContent = '读取失败';
  }
}

$('#saveGlobalExitLimit').onclick = async () => {
  const btn = $('#saveGlobalExitLimit');
  btn.disabled = true;
  try{
    const limit = Math.max(1, Math.min(1000, parseInt($('#globalExitLimit').value || '5', 10)));
    const enabled = $('#globalExitLimitEnabled').checked;
    const mode = $('#globalExitLimitMode').value === 'random' ? 'random' : 'isp';
    const s = await api('/api/settings', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body:JSON.stringify({exit_limit_enabled: enabled, exit_limit: limit, exit_limit_mode: mode})
    });
    $('#globalExitLimit').value = s.exit_limit || limit;
    $('#globalExitLimitStatus').textContent = enabled ? '已保存：每个国家最多 ' + (s.exit_limit || limit) + ' 个' : '已保存：限额已关闭';
    toast(enabled ? ('全局国家限额已保存 · ' + (mode === 'isp' ? '运营商去重' : '允许运营商重复')) : '全局国家限额已关闭');
    poll();
  }catch(e){
    toast('保存全局限额失败: ' + e.message, true);
  }finally{ btn.disabled = false; }
};

async function poll(){
  if(document.hidden) return;
  try{
    view = await api('/api/exits');
    $('#panel').textContent = view.panel
      ? (backendName() + ': ' + view.panel)
      : (view.panel_info || '');
    // xray-cf-lite 的节点由它自己生成，fanout 这边只管把它们导到哪条出口
    $('#newnode').hidden = isXCL();
    // 链接由 xray-cf-lite 的订阅体系发，fanout 这边导不出来
    $('#exportAll').hidden = isXCL();
    renderExits();
    renderOrphans();
  }catch(e){}
  try{ renderJobs(await api('/api/jobs') || []); }catch(e){}
}

// ---- 新建向导 ----
let regions = [], region = '', regionsLoaded = false;

function openModal(id){ $('#' + id).classList.add('open'); }
function closeModal(id){ $('#' + id).classList.remove('open'); }

document.addEventListener('click', e => {
  const c = e.target.closest('[data-close]');
  if(c) closeModal(c.dataset.close);
});
document.addEventListener('keydown', e => {
  if(e.key === 'Escape') document.querySelectorAll('.modal.open')
    .forEach(m => m.classList.remove('open'));
});
document.querySelectorAll('.modal').forEach(m => {
  m.onclick = e => { if(e.target === m) m.classList.remove('open'); };
});


function clampCount(val) {
  let n = parseInt(val, 10);
  if (isNaN(n) || n < 1) n = 3;
  if (n > 100) n = 100;
  return n;
}

function renderRegions(){
  const kw = ($('#rgfilter').value || '').trim().toLowerCase();
  const sourceList = regions;
  const list = sourceList.filter(r => {
    if(!kw) return true;
    const zh = (COUNTRY_ZH[r.code.toUpperCase()] || '').toLowerCase();
    return r.code.toLowerCase().includes(kw)
      || r.name.toLowerCase().includes(kw)
      || zh.includes(kw);
  });

  const availTotal = sourceList.reduce((a, r) => a + r.available, 0);

  const btns = ['<button class="rg' + (region === '' ? ' sel' : '')
      + '" data-rg=""><b>🌐 不限地区 (自动推荐)</b><em>共 ' + availTotal + ' 个可用节点 · 测速最优</em></button>']
    .concat(list.map(r => {
      const resHint = r.residential ? ' · 🏡 ' + r.residential + ' 住宅' : '';
      const purityHint = r.avg_purity ? ' · ' + r.avg_purity + '% 纯净' : '';
      const title = formatCountry(r.code, r.name);
      return '<button class="rg' + (region === r.code ? ' sel' : '')
        + '" data-rg="' + esc(r.code) + '"><b>' + esc(title) + '</b>'
        + '<em>' + r.available + ' 个空闲' + resHint + purityHint + '</em></button>';
    }));

  if(!regions.length){
    btns.push('<div style="grid-column:1/-1;padding:8px 12px;background:#141820;border-radius:6px;border:1px dashed var(--line);margin-top:6px;display:flex;justify-content:space-between;align-items:center">'
      + '<span style="color:var(--dim);font-size:12px">⚡ 正在载入更多节点或离线状态，可直接使用上述预设地区</span>'
      + '<button class="primary" id="wzRefreshBtn" type="button" style="font-size:11px;padding:4px 8px">🔄 刷新节点源</button>'
      + '</div>');
  }

  $('#regions').innerHTML = btns.join('');
  let dl = $('#countryCodeList');
  if(!dl){ dl = document.createElement('datalist'); dl.id = 'countryCodeList'; document.body.appendChild(dl); }
  dl.innerHTML = sourceList.map(r => '<option value="' + esc(r.code) + '">' + esc(r.name || '') + '</option>').join('');
  updateAvail();
  updateJapanPolicyUI();
}

function availOf(code){
  const sourceList = regions;
  if(code === '') return sourceList.reduce((a, r) => a + r.available, 0);
  const r = sourceList.find(x => x.code === code);
  return r ? r.available : 0;
}

let countryPolicies = [];

function policyRow(p = {}){
  const id = 'cp_' + Math.random().toString(36).slice(2,9);
  const code = esc(p.region || region || '');
  const count = Number(p.count || $('#count')?.value || 3);
  const mode = p.mode === 'isp' ? 'isp' : 'random';
  return '<div class="country-policy-row" data-cpid="' + id + '" style="display:grid;grid-template-columns:minmax(120px,1.5fr) 80px minmax(145px,1fr) 32px;gap:7px;align-items:center;margin-bottom:7px">'
    + '<input class="cp-region" list="countryCodeList" value="' + code + '" placeholder="国家，如 JP / 日本" style="min-width:0">'
    + '<input class="cp-count" type="number" min="1" max="100" value="' + count + '" title="出口数量">'
    + '<select class="cp-mode"><option value="random" ' + (mode==='random'?'selected':'') + '>随机运营商</option><option value="isp" ' + (mode==='isp'?'selected':'') + '>运营商必须不同</option></select>'
    + '<button type="button" class="cp-remove" title="删除国家">×</button>'
    + '</div>';
}

function renderCountryPolicies(){
  const box = $('#countryPolicies'); if(!box) return;
  if(!countryPolicies.length){ countryPolicies = [{region: region || 'JP', count: Number($('#count')?.value || 3), mode:'random'}]; }
  box.innerHTML = countryPolicies.map(policyRow).join('');
}

function readCountryPolicies(){
  return Array.from(document.querySelectorAll('.country-policy-row')).map(row => ({
    region: row.querySelector('.cp-region')?.value.trim() || '',
    count: Math.max(1, Math.min(100, Number(row.querySelector('.cp-count')?.value || 1))),
    mode: row.querySelector('.cp-mode')?.value || 'random'
  })).filter(p => p.region);
}

function updateJapanPolicyUI(){ renderCountryPolicies(); }

if($('#addCountryPolicy')) $('#addCountryPolicy').onclick = () => {
  countryPolicies = readCountryPolicies();
  countryPolicies.push({region:'', count:Number($('#count')?.value || 3), mode:'random'});
  renderCountryPolicies();
  const rows = document.querySelectorAll('.country-policy-row');
  rows[rows.length-1]?.querySelector('.cp-region')?.focus();
};

document.addEventListener('click', e => {
  const rm = e.target.closest('.cp-remove');
  if(!rm) return;
  countryPolicies = readCountryPolicies();
  const row = rm.closest('.country-policy-row');
  const idx = Array.from(document.querySelectorAll('.country-policy-row')).indexOf(row);
  if(idx >= 0) countryPolicies.splice(idx,1);
  renderCountryPolicies();
});

function updateAvail(){
  const countEl = $('#count');
  countEl.value = clampCount(countEl.value);
  const want = Number(countEl.value) || 3;
  const avail = availOf(region);
  const hint = $('#availhint');
  const goBtn = $('#go');

  hint.textContent = avail ? '当前来源已实测通过 ' + avail + ' 个节点' : '当前来源暂无实测通过节点（死节点不显示）';
  hint.className = 'hint' + (want > avail && avail ? ' bad' : '');
  if(want > avail && avail) hint.textContent = '只剩 ' + avail + ' 个，将全部使用';
  goBtn.disabled = selectedWzHosts.size === 0 && avail <= 0;
}

let wizardRegionRetryTimer = null;
let wizardRegionRetry = 0;
async function loadWizard(selectedSrc){
  const src = selectedSrc !== undefined ? selectedSrc : ($('#wzSource') ? $('#wzSource').value : 'all');
  wizardRegionRetry = 0;
  if(wizardRegionRetryTimer){ clearTimeout(wizardRegionRetryTimer); wizardRegionRetryTimer = null; }
  const regionsBox = $('#regions');
  if (!regions.length) {
    regionsBox.innerHTML = '<div style="padding:16px;text-align:center;color:var(--dim);font-size:12px">'
      + '<div class="spin" style="display:inline-block;margin-bottom:8px">' + ICON.run + '</div>'
      + '<div>正在载入该来源的实时国家列表（死节点不显示）…</div></div>';
  }

  // 1. 独立异步读取选定节点源的地区列表
  api('/api/regions?live=1&source=' + encodeURIComponent(src || 'all')).then(res => {
    regions = res || [];
    regionsLoaded = true;
    renderRegions();
    // 单一来源第一次切换时后端可能正在补拉源；自动重试，不需要用户反复点。
    if(!regions.length && src !== 'all' && wizardRegionRetry < 10){
      wizardRegionRetry++;
      wizardRegionRetryTimer = setTimeout(() => loadWizard(src), 1800);
    }
  }).catch(e => {
    toast('读取地区提示: ' + e.message, true);
    renderRegions();
  });

  const sel = $('#tpl');
  // xray-cf-lite 模式不能复制节点，向导退化成"只开出口"，之后在节点详情里挑出口
  if(isXCL()){
    $('#tplwrap').hidden = true;
    sel.innerHTML = '<option value="0">只开出口，不建节点</option>';
    return;
  }
  $('#tplwrap').hidden = false;

  // 2. 独立异步读取入站模板
  api('/api/exits').then(v => {
    const free = v.direct || [];
    const bound = (v.exits || []).flatMap(e => e.inbounds || []);
    inbounds = free.concat(bound);
    if(!inbounds.length){
      sel.innerHTML = '<option value="0">只开出口，不建节点（稍后在详情里绑定）</option>';
      $('#tplhint').textContent = '您也可先在主界面点击「新建节点」创建一个，再批量挂到出口上';
      return;
    }
    const opt = i => '<option value="' + i.id + '">'
      + esc(i.remark || ('端口 ' + i.port)) + ' · ' + esc(i.protocol)
      + ' :' + i.port + '</option>';
    sel.innerHTML =
      (free.length ? '<optgroup label="未绑定出口">' + free.map(opt).join('') + '</optgroup>' : '')
      + (bound.length ? '<optgroup label="已挂在出口上">' + bound.map(opt).join('') + '</optgroup>' : '')
      + '<option value="0">只开出口，不建节点</option>';
    $('#tplhint').textContent = '每个出口复制一份，客户端 UUID 保持一致，只有端口不同';
  }).catch(e => {
    sel.innerHTML = '<option value="0">只开出口，不建节点</option>';
    $('#tplhint').textContent = '当前模式: 联动 3x-ui';
  });
}

document.addEventListener('click', e => {
  if(e.target.closest('#newexit') || e.target.closest('#newexit2')){
    openModal('wizard');
    if(!regionsLoaded) loadWizard(); else { renderRegions(); loadWizard(); }
  }
  const rg = e.target.closest('[data-rg]');
  if(rg){
    region = rg.dataset.rg;
    if(countryPolicies.length === 1 && !countryPolicies[0].region) countryPolicies[0].region = region;
    if(countryPolicies.length === 1 && countryPolicies[0].region && countryPolicies[0].region !== region) countryPolicies[0].region = region;
    renderRegions();
    updateJapanPolicyUI();
    // 点国家后直接展开并加载该国家的实时节点，避免“点了没反应”。
    if(region){
      wzCandidatesExpanded = true;
      const wrap = $('#wzCandidatesWrap'); if(wrap) wrap.style.display = 'block';
      const arrow = $('#wzCandArrow'); if(arrow) arrow.textContent = '▼';
      const status = $('#wzCandStatus'); if(status) status.textContent = '正在实测该国家节点…';
      loadWizardCandidates();
    }
  }
});

// ---- 新建节点 ----
document.addEventListener('click', e => {
  if(e.target.closest('#newnode') || e.target.closest('#newnode2')){
    $('#nnhint').textContent = '';
    syncNodeForm();
    openModal('newnodebox');
  }
});

// 表单随协议/传输/安全层联动：只露出当前组合真正用得到的字段
function syncNodeForm(){
  const proto = $('#nproto').value;
  const net   = $('#nnet').value;
  const sec   = $('#nsec').value;

  // REALITY 靠模仿 TLS 握手工作，套在自带头部的传输上没有意义
  const realityOK = net === 'tcp' || net === 'xhttp' || net === 'grpc';
  const secSel = $('#nsec');
  for(const o of secSel.options){
    if(o.value === 'reality') o.disabled = !realityOK;
  }
  if(secSel.value === 'reality' && !realityOK) secSel.value = 'none';

  const cur = secSel.value;
  $('#nsniwrap').hidden  = cur !== 'tls';
  $('#ncertwrap').hidden = cur !== 'tls';
  $('#nkeywrap').hidden  = cur !== 'tls';
  $('#ndestwrap').hidden = cur !== 'reality';

  // Vision 只在 VLESS + 裸 TCP + TLS/REALITY 下有效
  const visionOK = proto === 'vless' && net === 'tcp' && cur !== 'none';
  $('#nvisionwrap').hidden = !visionOK;
  if(!visionOK) $('#nvision').checked = false;

  const needPath = net === 'ws' || net === 'httpupgrade' || net === 'xhttp' || net === 'grpc';
  $('#npathwrap').hidden = !needPath;
  $('#npathlabel').textContent = net === 'grpc' ? '服务名' : '路径';

  $('#nsechint').textContent =
    cur === 'reality' ? '密钥与 shortId 自动生成' :
    cur === 'tls'     ? '不填证书就用自签，链接会带证书指纹' : '';
}
$('#nproto').onchange = syncNodeForm;
$('#nnet').onchange = syncNodeForm;
$('#nsec').onchange = syncNodeForm;

$('#ncreate').onclick = async e => {
  const q = new URLSearchParams({
    protocol: $('#nproto').value,
    network:  $('#nnet').value,
    security: $('#nsec').value,
    port:     ($('#nport').value || '').trim(),
    remark:   ($('#nremark').value || '').trim(),
    path:     ($('#npath').value || '').trim(),
    sni:      ($('#nsni').value || '').trim(),
    cert:     ($('#ncert').value || '').trim(),
    key:      ($('#nkey').value || '').trim(),
    dest:     ($('#ndest').value || '').trim(),
  });
  if($('#nvision').checked) q.set('vision', '1');
  const btn = $('#ncreate');
  const oldText = btn.textContent;
  btn.disabled = true;
  btn.textContent = '创建中...';
  $('#nnhint').textContent = '正在配置入站规则...';
  try{
    const r = await api('/api/panel/inbound/new?' + q.toString(), {method:'POST', timeout: 60000});
    toast('已创建 ' + r.protocol + ' 节点，端口 ' + r.port);
    closeModal('newnodebox');
    $('#nport').value = '';
    $('#nremark').value = '';
    $('#nnhint').textContent = '';
    poll();
  }catch(err){
    toast(err.message, true);
    $('#nnhint').textContent = err.message;
  }finally{
    btn.disabled = false;
    btn.textContent = oldText;
  }
};

$('#rgfilter').oninput = renderRegions;
$('#minus').onclick = () => { step(-1); };
$('#plus').onclick = () => { step(1); };
function step(d){
  const el = $('#count');
  el.value = clampCount((parseInt(el.value, 10) || 3) + d);
  updateAvail();
}
$('#count').oninput = updateAvail;
$('#count').onchange = updateAvail;
$('#count').onblur = updateAvail;

if($('#wzSource')){
  $('#wzSource').onchange = () => {
    region = ''; regions = []; regionsLoaded = false; selectedWzHosts.clear();
    wzCandidatesList = [];
    if($('#wzCandidatesList')) $('#wzCandidatesList').innerHTML = '<div style="padding:12px;text-align:center;color:var(--dim)">正在切换来源并只筛选该来源的实时节点…</div>';
    loadWizard($('#wzSource').value);
  };
}

let wzCandidatesExpanded = false;
let wzCandidatesList = [];
let selectedWzHosts = new Set();

async function loadWizardCandidates(){
  const wrap = $('#wzCandidatesWrap');
  if(!wrap || !wzCandidatesExpanded) return;
  const listEl = $('#wzCandidatesList');
  const countEl = $('#wzCandCount');
  const src = $('#wzSource') ? $('#wzSource').value : 'all';
  listEl.innerHTML = '<div style="padding:12px;text-align:center;color:var(--dim);font-size:12px">正在载入候选节点并根据网络质量测速优选推荐…</div>';

  try{
    const res = await api('/api/nodes?live=1&region=' + encodeURIComponent(region || '') + '&source=' + encodeURIComponent(src || 'all') + '&limit=120');
    wzCandidatesList = (res && res.nodes) ? res.nodes : [];
    if(countEl) countEl.textContent = wzCandidatesList.length;
    renderWizardCandidates();
    if(!wzCandidatesList.length && $('#wzSource') && $('#wzSource').value !== 'all' && loadWizardCandidates.retry < 8){
      loadWizardCandidates.retry = (loadWizardCandidates.retry || 0) + 1;
      setTimeout(loadWizardCandidates, 1600);
    } else {
      loadWizardCandidates.retry = 0;
    }
  }catch(e){
    listEl.innerHTML = '<div style="padding:12px;text-align:center;color:var(--warn);font-size:12px">读取候选失败: ' + esc(e.message) + '</div>';
  }
}

function renderWizardCandidates(){
  const listEl = $('#wzCandidatesList');
  if(!listEl) return;
  if(!wzCandidatesList.length){
    listEl.innerHTML = '<div style="padding:12px;text-align:center;color:var(--dim);font-size:12px">该来源/国家下暂无刚刚测活通过的节点</div>';
    return;
  }

  listEl.innerHTML = wzCandidatesList.map((n, idx) => {
    const isChecked = selectedWzHosts.has(n.hostname);
    const flag = getFlagEmoji(n.country_code);
    const ping = n.ping || 0;
    const pingColor = ping > 0 && ping < 100 ? 'var(--ok)' : (ping > 0 && ping < 200 ? 'var(--accent)' : 'var(--warn)');
    const pingText = ping > 0 ? ping + ' ms' : '—';
    const proto = (n.proto || (n.config ? 'ovpn' : 'socks5')).toUpperCase();
    const isp = n.isp || n.country || '公网节点';
    const speed = n.speed_mbps ? n.speed_mbps.toFixed(1) + ' Mbps' : '';
    const inUseBadge = n.in_use ? '<span style="color:var(--warn);font-size:10px;padding:1px 4px;border:1px solid var(--warn);border-radius:3px;margin-left:4px">已占用</span>' : '';

    return '<label style="display:flex;align-items:center;gap:8px;padding:6px 8px;border-radius:4px;background:' + (isChecked ? 'var(--subtle)' : 'transparent') + ';cursor:pointer;border-bottom:1px solid rgba(255,255,255,0.04)">'
      + '<input type="checkbox" class="wz-cand-chk" data-host="' + esc(n.hostname) + '" ' + (isChecked ? 'checked' : '') + (n.in_use ? ' disabled' : '') + '>'
      + '<span style="font-size:11px;color:var(--dim);width:24px;text-align:right">#' + (idx + 1) + '</span>'
      + '<span style="font-size:14px">' + flag + '</span>'
      + '<span style="flex:1;font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="' + esc(n.ip + ' ' + isp) + '">'
      + '<b style="color:var(--text)">' + esc(n.ip || n.hostname) + '</b> '
      + '<span style="color:var(--dim);font-size:11px">' + esc(isp) + '</span>' + inUseBadge
      + '</span>'
      + (speed ? '<span style="font-size:11px;color:var(--dim)">' + speed + '</span>' : '')
      + '<span style="font-size:10px;padding:2px 6px;border-radius:3px;background:rgba(255,255,255,0.06);color:var(--dim)">' + proto + '</span>'
      + '<span style="font-size:10px;color:var(--ok)">● 实测</span>'
      + '<span style="font-size:11px;font-weight:600;color:' + pingColor + ';min-width:55px;text-align:right">' + pingText + '</span>'
      + '</label>';
  }).join('');

  updateWzSelectedCount();
}

function updateWzSelectedCount(){
  const el = $('#wzSelCount');
  if(el) el.textContent = selectedWzHosts.size;
  const goBtn = $('#go');
  if(selectedWzHosts.size > 0){
    if(goBtn) goBtn.textContent = '⚡ 启动选中 (' + selectedWzHosts.size + ' 个)';
  } else {
    if(goBtn) goBtn.textContent = '开始';
  }
}

if($('#toggleWzCandidates')){
  $('#toggleWzCandidates').onclick = () => {
    wzCandidatesExpanded = !wzCandidatesExpanded;
    const wrap = $('#wzCandidatesWrap');
    const arrow = $('#wzCandArrow');
    const status = $('#wzCandStatus');
    if(wrap) wrap.style.display = wzCandidatesExpanded ? 'block' : 'none';
    if(arrow) arrow.textContent = wzCandidatesExpanded ? '▼' : '▶';
    if(status) status.textContent = wzCandidatesExpanded ? '点击收起' : '点击展开';
    if(wzCandidatesExpanded){
      loadWizardCandidates();
    }
  };
}

if($('#wzSelectTop3')){
  $('#wzSelectTop3').onclick = () => {
    selectedWzHosts.clear();
    const available = wzCandidatesList.filter(n => !n.in_use);
    available.slice(0, 3).forEach(n => selectedWzHosts.add(n.hostname));
    renderWizardCandidates();
  };
}

if($('#wzSelectTop10')){
  $('#wzSelectTop10').onclick = () => {
    selectedWzHosts.clear();
    const available = wzCandidatesList.filter(n => !n.in_use);
    available.slice(0, 10).forEach(n => selectedWzHosts.add(n.hostname));
    renderWizardCandidates();
  };
}

if($('#wzSelectAll')){
  $('#wzSelectAll').onclick = () => {
    const available = wzCandidatesList.filter(n => !n.in_use);
    available.forEach(n => selectedWzHosts.add(n.hostname));
    renderWizardCandidates();
  };
}

if($('#wzClearSel')){
  $('#wzClearSel').onclick = () => {
    selectedWzHosts.clear();
    renderWizardCandidates();
  };
}

document.addEventListener('change', e => {
  if(e.target.matches('.wz-cand-chk')){
    const host = e.target.dataset.host;
    if(host){
      if(e.target.checked) selectedWzHosts.add(host);
      else selectedWzHosts.delete(host);
      updateWzSelectedCount();
    }
  }
});

$('#go').onclick = async e => {
  const tpl = $('#tpl').value || '0';
  const src = $('#wzSource') ? $('#wzSource').value : 'all';
  const btn = $('#go');
  const oldText = btn.textContent;
  btn.disabled = true;
  btn.textContent = '启动中...';
  $('#wzhint').textContent = '正在开辟出口隧道并对接节点链接...';
  try{
    if(selectedWzHosts.size > 0){
      const hosts = Array.from(selectedWzHosts).join(',');
      await api('/api/provision?hosts=' + encodeURIComponent(hosts) + '&template=' + tpl, {method:'POST', timeout: 60000});
      selectedWzHosts.clear();
    } else {
      const policies = readCountryPolicies();
      if(!policies.length){ throw new Error('请至少添加一个国家及出口数量'); }
      for(const p of policies){ if(p.count < 1 || p.count > 100) throw new Error('国家出口数量必须为 1-100'); }
      await api('/api/provision?policies=' + encodeURIComponent(JSON.stringify(policies.map(p => ({region:p.region,count:p.count,mode:p.mode,source:src})))) + '&template=' + tpl, {method:'POST', timeout: 60000});
    }
    closeModal('wizard');
    $('#wzhint').textContent = '';
    poll();
  }catch(err){
    toast(err.message, true);
    $('#wzhint').textContent = err.message;
  }finally{
    btn.disabled = false;
    btn.textContent = oldText;
  }
};

// ---- 出口操作 ----
document.addEventListener('click', async e => {
  const clearAllJobs = e.target.closest('#clearAllDoneJobs');
  if(clearAllJobs){
    try{ await api('/api/jobs/clear', {method:'POST'}); toast('已清理所有任务卡片'); }catch(err){}
    poll();
    return;
  }
  const toggleAll = e.target.closest('#toggleAllFailedBtn');
  if(toggleAll){
    showAllFailed = !showAllFailed;
    poll();
    return;
  }
  const toggleJob = e.target.closest('[data-togglejobfailed]');
  if(toggleJob){
    const wrap = $('#failed_wrap_' + toggleJob.dataset.togglejobfailed);
    if(wrap){
      wrap.classList.toggle('open');
      toggleJob.textContent = wrap.classList.contains('open') ? '▲ 收起候选' : '▼ 查看候选';
    }
    return;
  }
  const cleanJob = e.target.closest('[data-cleanjobfailed]');
  if(cleanJob){
    try{
      await api('/api/jobs/clean_failed?id=' + encodeURIComponent(cleanJob.dataset.cleanjobfailed), {method:'POST'});
      toast('已清理该任务中的爆红失败项');
    }catch(err){}
    poll();
    return;
  }
  const stop = e.target.closest('[data-stop]');
  if(stop){
    stop.disabled = true;
    try{ await api('/api/stop?slot=' + stop.dataset.stop, {method:'POST'}); }
    catch(err){ toast(err.message, true); }
    poll();
    return;
  }
  const swap = e.target.closest('[data-swap]');
  if(swap){
    swap.disabled = true;
    try{
      await api('/api/swap?slot=' + swap.dataset.swap, {method:'POST'});
      toast('正在换节点');
    }catch(err){ toast(err.message, true); }
    poll();
    return;
  }
  const cred = e.target.closest('[data-cred]');
  if(cred){ openCred(Number(cred.dataset.cred)); return; }
  const job = e.target.closest('[data-job]');
  if(job){
    try{ await api('/api/jobs/dismiss?id=' + job.dataset.job, {method:'POST'}); }catch(err){}
    poll();
    return;
  }
  const del = e.target.closest('[data-delorphans]');
  if(del){
    const list = view.direct || [];
    if(!confirm('删除这 ' + list.length + ' 个未绑定节点？此操作不可撤销。')) return;
    del.disabled = true;
    try{
      await api('/api/xui/delete?ids=' + list.map(i => i.id).join(','), {method:'POST'});
      toast('已清理 ' + list.length + ' 个入站');
    }catch(err){ toast(err.message, true); }
    poll();
    return;
  }

  const one = e.target.closest('[data-delone]');
  if(one){
    if(!confirm('删除入站 ' + one.dataset.name + '？此操作不可撤销。')) return;
    one.disabled = true;
    try{
      await api('/api/xui/delete?ids=' + one.dataset.delone, {method:'POST'});
      toast('已删除 ' + one.dataset.name);
    }catch(err){ toast(err.message, true); }
    poll();
  }
});

$('#stopall').onclick = async e => {
  if(!confirm('停止全部 ' + view.exits.length + ' 个出口？')) return;
  e.target.disabled = true;
  for(const x of view.exits){
    try{ await api('/api/stop?slot=' + x.slot, {method:'POST'}); }catch(err){}
  }
  poll();
};

const pruneBtn = $('#prunefailed');
if(pruneBtn){
  pruneBtn.onclick = async e => {
    pruneBtn.disabled = true;
    try{
      const res = await api('/api/exits/prune_failed', {method:'POST'});
      try{ await api('/api/jobs/clear', {method:'POST'}); }catch(_){}
      toast('已清除 ' + (res.count || 0) + ' 个失效出口并清理所有爆红记录');
    }catch(err){ toast(err.message, true); }
    poll();
    pruneBtn.disabled = false;
  };
}

// ---- 节点详情 ----
let curDetail = null;

// 详情弹窗的重绘要跟轮询解耦：正在编辑时被 poll 刷掉输入会很烦
async function openDetail(id){
  $('#dbody').innerHTML = '<div class="empty">读取中…</div>';
  curDetail = null;
  $('#ddel').disabled = true;
  openModal('detail');
  try{
    const d = await api('/api/xui/detail?id=' + id);
    curDetail = d;
    renderDetail(d);
    $('#ddel').hidden = isXCL();
    $('#ddel').disabled = isXCL();
  }catch(err){
    $('#dbody').innerHTML = '<div class="empty">读取失败: ' + esc(err.message) + '</div>';
  }
}

// 出口下拉：列出所有已连通的隧道，外加"直连"。绑定按 Xray 的 inboundTag 走。
function exitOptions(currentHost){
  const up = view.exits.filter(e => e.status === 'up');
  return '<option value=""' + (currentHost ? '' : ' selected') + '>直连（不走代理）</option>'
    + up.map(e => {
        const flag = getFlagEmoji(e.region || e.country_code);
        const name = flag + ' ' + (e.exit_ip || e.host) + (e.region ? ' · ' + e.region : '');
        return '<option value="' + esc(e.host) + '"'
          + (e.host === currentHost ? ' selected' : '') + '>'
          + esc(name) + '</option>';
      }).join('');
}

function renderDetail(d){
  const owner = view.exits.find(x => (x.inbounds || []).some(i => i.id === d.id));

  const clients = (d.clients || []).map((c, i) => {
    const link = (d.links || [])[i] || '';
    return '<div class="client">'
      + '<div class="crow">'
      +   '<span class="cemail">' + esc(c.email) + '</span>'
      +   '<span class="cid">' + esc(c.id) + '</span>'
      +   '<span class="spacer"></span>'
      +   (link ? '<button class="icon" data-copy="' + esc(link) + '" title="复制链接">' + ICON.copy + '</button>' : '')
      +   '<button class="icon" data-creset="' + esc(c.email) + '" title="换一套凭据，旧链接立即失效">' + ICON.redo + '</button>'
      +   '<button class="icon" data-cdel="' + esc(c.email) + '" title="删除这个客户端">' + ICON.trash + '</button>'
      + '</div>'
      + (link ? '<div class="share">' + esc(link) + '</div>' : '')
      + '</div>';
  }).join('');

  $('#dtitle').textContent = (d.remark || '节点') + '　:' + d.port;
  // xray-cf-lite 的节点归它自己管，这里只留出口选择，改端口/备注/客户端都不给
  const editable = !isXCL();
  $('#dbody').innerHTML = '<dl class="kv">'
    + '<dt>出口</dt><dd><select id="dbind" data-tag="' + esc(d.tag) + '">'
    +   exitOptions(owner ? owner.host : (d.bound_to || '')) + '</select></dd>'
    + '<dt>协议</dt><dd>' + esc(d.protocol) + '　' + esc(d.network || '')
    +   (d.tls && d.tls !== 'none' ? '　' + esc(d.tls) : '') + '</dd>'
    + '<dt>监听</dt><dd>' + esc(d.listen || '0.0.0.0') + '</dd>'
    + '</dl>'
    + (editable ? ('<div style="display:flex;gap:7px;flex-wrap:wrap;margin:10px 0 4px">'
    + '<button id="dresetall" data-inbound-id="' + esc(d.id) + '" title="重置本入站及克隆入站的全部客户端 UUID/密码">🔄 一键重置本节点全部 UUID</button>'
    + '</div>'
    + '<div class="editbar">'
    +   '<label class="ef"><span>备注</span>'
    +     '<input id="dremark" type="text" value="' + esc(d.remark || '') + '"></label>'
    +   '<label class="ef"><span>端口</span>'
    +     '<input id="dport" type="text" inputmode="numeric" value="' + d.port + '"></label>'
    +   '<label class="chk"><input type="checkbox" id="denable"'
    +     (d.enable === false ? '' : ' checked') + '> 启用</label>'
    +   '<span class="spacer"></span>'
    +   '<button class="primary" id="dsave">保存</button>'
    + '</div>'
    + '<div class="chead"><h3>客户端</h3><span class="count">'
    +   (d.clients || []).length + ' 个</span><span class="spacer"></span>'
    +   '<button id="dcadd">' + ICON.plus + '添加</button></div>'
    + (clients || '<div class="empty">没有客户端</div>'))
    : '<div class="hint">这个节点由 xray-cf-lite 管，端口、UUID 和分享链接都去它那边改。这里只决定它走哪条出口。</div>');
}

// 出口下拉改动即生效（支持自由切换任意出口或恢复直连）
document.addEventListener('change', async e => {
  const sel = e.target.closest('.obind');
  if(!sel) return;
  sel.disabled = true;
  try{
    await api('/api/xui/bind?tag=' + encodeURIComponent(sel.dataset.tag)
      + '&host=' + encodeURIComponent(sel.value), {method:'POST'});
    toast(sel.value ? '已切换至指定出口' : '已恢复直连');
    poll();
  }catch(err){ toast(err.message, true); }
  sel.disabled = false;
});

// 出口下拉改动即生效。绑定按 inboundTag 走，host 传空表示解绑回直连。
document.addEventListener('change', async e => {
  const sel = e.target.closest('#dbind');
  if(!sel) return;
  sel.disabled = true;
  try{
    await api('/api/xui/bind?tag=' + encodeURIComponent(sel.dataset.tag)
      + '&host=' + encodeURIComponent(sel.value), {method:'POST'});
    toast(sel.value ? '已绑定' : '已解绑');
    poll();
  }catch(err){ toast(err.message, true); }
  sel.disabled = false;
});

document.addEventListener('click', async e => {
  const link = e.target.closest('[data-detail]');
  if(link) return openDetail(link.dataset.detail);

  if(e.target.closest('#dsave')){
    const btn = e.target.closest('#dsave');
    btn.disabled = true;
    const q = new URLSearchParams({
      id: curDetail.id,
      port: ($('#dport').value || '').trim(),
      remark: ($('#dremark').value || '').trim(),
      enable: $('#denable').checked ? '1' : '0',
    });
    try{
      await api('/api/panel/inbound/update?' + q, {method:'POST'});
      toast('已保存');
      await openDetail(curDetail.id);
      poll();
    }catch(err){ toast(err.message, true); btn.disabled = false; }
    return;
  }

  const add = e.target.closest('#dcadd');
  if(add){
    add.disabled = true;
    try{
      await api('/api/panel/client/add?id=' + curDetail.id, {method:'POST'});
      toast('已添加客户端');
      await openDetail(curDetail.id);
    }catch(err){ toast(err.message, true); add.disabled = false; }
    return;
  }

  const del = e.target.closest('[data-cdel]');
  if(del){
    if(!confirm('删除客户端 ' + del.dataset.cdel + '？它的链接会立即失效。')) return;
    del.disabled = true;
    try{
      await api('/api/panel/client/del?id=' + curDetail.id
        + '&email=' + encodeURIComponent(del.dataset.cdel), {method:'POST'});
      toast('已删除');
      await openDetail(curDetail.id);
    }catch(err){ toast(err.message, true); del.disabled = false; }
    return;
  }

  const reset = e.target.closest('[data-creset]');
  if(reset){
    if(!confirm('重置 ' + reset.dataset.creset + ' 的凭据？已分发的旧链接会立即失效。')) return;
    reset.disabled = true;
    try{
      await api('/api/panel/client/reset?id=' + curDetail.id
        + '&email=' + encodeURIComponent(reset.dataset.creset), {method:'POST'});
      toast('已重置');
      await openDetail(curDetail.id);
    }catch(err){ toast(err.message, true); reset.disabled = false; }
    return;
  }

  // 详情弹窗里删掉当前这个入站
  const dd = e.target.closest('#ddel');
  if(dd && curDetail){
    const name = (curDetail.remark || curDetail.protocol || '节点') + ' :' + curDetail.port;
    if(!confirm('删除入站 ' + name + '？它的所有客户端链接都会失效，且不可撤销。')) return;
    dd.disabled = true;
    try{
      await api('/api/xui/delete?ids=' + curDetail.id, {method:'POST'});
      toast('已删除 ' + name);
      curDetail = null;
      closeModal('detail');
      poll();
    }catch(err){ toast(err.message, true); dd.disabled = false; }
  }
});

document.addEventListener('click', e => {
  const c = e.target.closest('[data-copy]');
  if(c) copy(c.dataset.copy);
});

// ---- SOCKS5 凭据 ----
let curCred = null;

function socksURL(host, port, user, pass){
  if(!user) return 'socks5://' + host + ':' + port;
  return 'socks5://' + user + ':' + pass + '@' + host + ':' + port;
}

// SOCKS5 端口监听在母机（跑 fanout 的这台服务器）上，客户端要连的是母机的
// 公网 IPv4，流量再从出口 IP 出去。出口 IP 是"出去以后"的地址，不能当连接地址。
// public_ip 是后端探测到的母机公网地址；探测不到才退回访问面板用的主机名。
function credHost(e){
  return view.public_ip || location.hostname || e.host;
}

function openCred(slot){
  const e = view.exits.find(x => x.slot === slot);
  if(!e){ toast('这个出口不在了', true); return; }
  curCred = {slot: slot, port: e.port, host: credHost(e)};
  $('#crtitle').textContent = e.region + ' · :' + e.port;
  $('#cruser').value = e.socks_user || '';
  $('#crpass').value = e.socks_pass || '';
  refreshCredURL();
  openModal('credbox');
}

function refreshCredURL(){
  if(!curCred) return;
  $('#crurl').textContent = socksURL(curCred.host, curCred.port,
    $('#cruser').value.trim(), $('#crpass').value.trim());
}
$('#cruser').oninput = refreshCredURL;
$('#crpass').oninput = refreshCredURL;

$('#crrand').onclick = () => {
  // 客户端和服务端都要能识别，只用无歧义、无需转义的字符
  const abc = 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  const gen = n => Array.from(crypto.getRandomValues(new Uint8Array(n)))
    .map(v => abc[v % abc.length]).join('');
  $('#cruser').value = 'fo' + gen(6);
  $('#crpass').value = gen(14);
  refreshCredURL();
};

$('#crcopy').onclick = () => { copy($('#crurl').textContent); };

$('#crsave').onclick = async e => {
  if(!curCred) return;
  const btn = e.target; btn.disabled = true;
  const q = new URLSearchParams({
    slot: curCred.slot,
    user: $('#cruser').value.trim(),
    pass: $('#crpass').value.trim(),
  });
  try{
    const r = await api('/api/cred?' + q, {method:'POST'});
    $('#cruser').value = r.user;
    $('#crpass').value = r.pass;
    refreshCredURL();
    toast('已保存，立即生效');
    poll();
  }catch(err){ toast(err.message, true); }
  btn.disabled = false;
};

// ---- 导出 ----
$('#exportAll').onclick = async () => {
  const ids = (view.exits || []).flatMap(x => (x.inbounds || []).map(i => i.id));
  const upExits = (view.exits || []).filter(e => e.status === 'up');
  const socksLinks = upExits.map(e => {
    const host = view.public_ip || window.location.hostname;
    const flag = getFlagEmoji(e.region || e.country_code);
    let ispName = (e.isp || '').trim();
    if (!ispName || ispName.toLowerCase() === 'public proxy' || ispName.toLowerCase() === 'public pool' || ispName.toLowerCase() === 'vpn gate') {
      if ((e.region || e.country_code || '').toUpperCase() === 'JP' || ispName.toLowerCase().includes('tsukuba')) {
        ispName = '筑波大学 VPN Gate';
      } else {
        ispName = '优质网络';
      }
    }
    return 'socks5://' + encodeURIComponent(e.socks_user || '') + ':' + encodeURIComponent(e.socks_pass || '')
      + '@' + host + ':' + e.port + '#' + encodeURIComponent(flag + ' ' + ispName);
  });

  if(!ids.length && !socksLinks.length){
    toast('还没有节点或运行中的出口可导出', true);
    return;
  }
  $('#exbox').value = '读取中…';
  $('#excount').textContent = '';
  openModal('export');
  let links = [...socksLinks];
  if(ids.length){
    try{
      const d = await api('/api/xui/links?ids=' + ids.join(','));
      if(d && d.links) links = links.concat(d.links);
    }catch(err){}
  }
  $('#exbox').value = links.join('\n');
  $('#excount').textContent = links.length + ' 条';
};
$('#copyall').onclick = () => { const v = $('#exbox').value; if(v) copy(v); };

// ---- 设置：改密码 / 改路径 / 改端口 / 改本地监听 ----
let curSettings = null;
let curBackend = null;

// 后端切换：把本机能用的模式列出来，装了的可选，没装的置灰并说明原因
async function loadBackendModes(){
  const sel = $('#setBackend');
  const hint = $('#setBackendHint');
  try{
    const m = await api('/api/panel/mode');
    curBackend = m.mode || '';
    sel.innerHTML = '<option value="">自动（按本机装了什么挑）</option>'
      + (m.modes || []).map(x =>
          '<option value="' + esc(x.mode) + '"' + (x.available ? '' : ' disabled')
          + '>' + esc(x.label) + (x.available ? '' : '（没装）') + '</option>').join('');
    sel.value = curBackend;
    const bad = (m.modes || []).filter(x => !x.available);
    hint.textContent = m.describe
      ? ('当前：' + m.describe + (bad.length ? '。灰掉的是本机没装的。' : ''))
      : '节点从哪来。装了 3x-ui 或 xray-cf-lite 就能直接接管，都没有就用自建。';
  }catch(err){
    sel.innerHTML = '<option value="">3x-ui (联动中)</option>';
    hint.textContent = '节点从哪来。装了 3x-ui 或 xray-cf-lite 就能直接接管，都没有就用自建。';
  }
}

$('#settingsBtn').onclick = async () => {
  openModal('settings');
  $('#setPw').value = '';
  $('#setPath').value = '';
  $('#setPathHint').textContent = '读取设置中…';

  const pModes = loadBackendModes().catch(()=>{});
  const pSettings = api('/api/settings').then(s => {
    curSettings = s;
    $('#setPath').value = (s.base_path || '').replace(/^\//, '');
    $('#setPort').value = s.port || '';
    $('#setListen').value = s.listen_addr || '0.0.0.0';
    $('#setPathHint').textContent = '界面挂在这个路径下，扫端口的探不到。只能用字母数字和 - _。';
    $('#updCur').textContent = s.version || '-';
    $('#updLatest').textContent = '';
    $('#updNotes').hidden = true;
    $('#updApply').hidden = true;
    $('#updCheck').disabled = false;
    $('#updCheck').textContent = '检查更新';
  }).catch(err => {
    $('#setPathHint').textContent = '界面挂在当前路径下。' + (err.message ? '提示: ' + err.message : '');
    $('#updCur').textContent = 'v0.3.3-enhanced';
    $('#updCheck').disabled = false;
  });

  await Promise.allSettled([pModes, pSettings]);
};

// 检查更新：问后端 GitHub 最新版，有新版就亮出更新按钮和更新内容
$('#updCheck').onclick = async e => {
  e.target.disabled = true;
  e.target.textContent = '检查中…';
  try{
    const u = await api('/api/update/check');
    $('#updCur').textContent = u.current || 'v3.0.0-Jesee-Mod';
    if(u.has_update){
      $('#updLatest').textContent = '有新版本 ' + u.latest;
      $('#updApplyVer').textContent = u.latest;
      $('#updApply').hidden = false;
      $('#updNotes').textContent = u.notes || '（检测到版本更新可用）';
      $('#updNotes').hidden = false;
    } else {
      $('#updLatest').textContent = '（已是最新版本）';
      $('#updApply').hidden = true;
      $('#updNotes').textContent = u.notes || '当前已是 Jesee 深度魔改最新旗舰版，所有自愈编排与专网系统稳定运行中。';
      $('#updNotes').hidden = false;
    }
  }catch(err){ toast('检查更新: ' + err.message, false); }
  e.target.disabled = false;
  e.target.textContent = '检查更新';
};

// 一键更新：后端下载替换二进制并重启服务，进程重启期间界面会短暂断连
$('#updApply').onclick = async e => {
  if(!confirm('更新到 ' + $('#updApplyVer').textContent + '？服务会重启，界面会短暂断开。')) return;
  e.target.disabled = true;
  e.target.textContent = '更新中…';
  try{
    const r = await api('/api/update/apply', {method:'POST'});
    if(r.restarting){
      $('#updNotes').textContent = '已下载新版本，服务正在重启，几秒后刷新页面即可。';
      $('#updNotes').hidden = false;
      toast('更新中，服务重启后刷新页面');
      // 给服务重启留点时间再自动刷新
      setTimeout(() => location.reload(), 6000);
    } else {
      toast(r.message || '已是最新版');
      e.target.disabled = false;
      e.target.textContent = '更新到 ' + $('#updApplyVer').textContent;
    }
  }catch(err){
    toast(err.message, true);
    e.target.disabled = false;
    e.target.textContent = '更新到 ' + $('#updApplyVer').textContent;
  }
};

// 端口/监听地址变了要提示用户之后从新地址进；密码/路径可原地生效
function nextURL(port, listen, path){
  const host = (listen && listen !== '0.0.0.0') ? listen : location.hostname;
  return location.protocol + '//' + host + ':' + port + (path ? '/' + path : '') + '/';
}

$('#setSave').onclick = async e => {
  e.target.disabled = true;
  const body = {};
  const pw = $('#setPw').value.trim();
  if(pw) body.password = pw;
  body.base_path = $('#setPath').value.trim();
  const port = parseInt($('#setPort').value.trim(), 10);
  if(port) body.port = port;
  body.listen_addr = $('#setListen').value;

  const portChanged = curSettings && (port !== curSettings.port
    || body.listen_addr !== (curSettings.listen_addr || '0.0.0.0'));

  try{
    // 后端和其它设置分属两个接口，先切后端：切失败就别继续，免得用户以为整单都生效了
    const backend = $('#setBackend').value;
    if(curBackend !== null && backend !== curBackend){
      const r = await api('/api/panel/mode', {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body: JSON.stringify({mode: backend}),
      });
      curBackend = r.mode || '';
      $('#setBackendHint').textContent = '当前：' + (r.describe || r.kind || '已切换');
    }
    await api('/api/settings', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify(body),
    });
    if(portChanged){
      const url = nextURL(port, body.listen_addr, body.base_path);
      $('#setPortHint').innerHTML = '监听已切换，请从新地址打开：<a href="' + esc(url) + '">' + esc(url) + '</a>';
      toast('监听已切换，用新地址重新打开');
      // 端口变了当前连接会断，不自动跳转，让用户看清新地址
    } else {
      toast('已保存');
      // 路径可能变了，重新加载到新路径下
      const np = body.base_path;
      const cur = (curSettings && curSettings.base_path || '').replace(/^\//, '');
      if(np !== cur){
        location.href = location.protocol + '//' + location.host
          + (np ? '/' + np : '') + '/';
        return;
      }
      closeModal('settings');
      poll();
    }
  }catch(err){ toast(err.message, true); }
  e.target.disabled = false;
};

// ---- 节点源管理 ----
let srcInfo = null;

function fmtTime(iso){
  if(!iso || iso.startsWith('0001')) return '尚未拉取';
  const d = new Date(iso);
  const now = new Date();
  const diffSec = Math.floor((now - d) / 1000);
  if(diffSec < 0) return '刚刚';
  if(diffSec < 60) return diffSec + ' 秒前';
  if(diffSec < 3600) return Math.floor(diffSec / 60) + ' 分钟前';
  return d.toLocaleTimeString();
}

async function loadSources(){
  try{
    srcInfo = await api('/api/sources');
    $('#srcActive').textContent = srcInfo.active_source || '默认官方直连';
    $('#srcTotal').textContent = (srcInfo.total_nodes || 0) + ' 个节点';
    $('#srcCustomCount').textContent = (srcInfo.custom_nodes || 0) + ' 个本地节点';
    $('#srcCachedCount').textContent = (srcInfo.cached_nodes || 0) + ' 个离线缓存';
    $('#srcLastFetch').textContent = fmtTime(srcInfo.last_fetch);
    $('#customSrcUrl').value = srcInfo.custom_url || '';
    $('#srcCustomDir').textContent = srcInfo.custom_dir || '/var/lib/fanout/custom_nodes';
    const errBox = $('#srcErr');
    if(srcInfo.last_error){
      errBox.hidden = false;
      errBox.textContent = '提示: ' + srcInfo.last_error;
    } else {
      errBox.hidden = true;
    }
  }catch(e){
    toast('加载节点源失败: ' + e.message, true);
  }
}

async function refreshSources(btn, src = 'all'){
  if(btn) btn.disabled = true;
  const nameMap = {
    all: '官方与高防镜像优质源（筑波大学+海外高校学术+住宅家宽原生）',
    vpngate: '日本筑波大学官方与高防镜像源',
    edu: '海外高校学术科研源（日本筑波/韩国/欧美名校）',
    residential: '住宅家宽原生优质节点源'
  };
  toast('正在拉取 ' + (nameMap[src] || src) + '，请稍候...');
  try{
    const res = await api('/api/sources/refresh?source=' + encodeURIComponent(src || 'all'), {method:'POST', timeout:60000});
    toast('拉取成功！已获取 ' + (res.count || 0) + ' 个节点');
    await loadSources();
    const currentWzSrc = $('#wzSource') ? $('#wzSource').value : 'all';
    regions = await api('/api/regions?source=' + encodeURIComponent(currentWzSrc)) || [];
    renderRegions();
    poll();
  }catch(e){
    toast('拉取失败: ' + e.message, true);
  }finally{
    if(btn) btn.disabled = false;
  }
}

async function scanCustomOvpn(btn){
  if(btn) btn.disabled = true;
  toast('正在扫描本地 .ovpn 目录...');
  try{
    const res = await api('/api/sources/scan', {method:'POST'});
    toast('扫描完成！新增/更新 ' + (res.count || 0) + ' 个自定义节点');
    await loadSources();
    regions = await api('/api/regions') || [];
    renderRegions();
    poll();
  }catch(e){
    toast('扫描失败: ' + e.message, true);
  }finally{
    if(btn) btn.disabled = false;
  }
}

// ---- 全平台聚合订阅 ----
async function openSubModal(){
  openModal('submodal');
  try{
    const s = await api('/api/subscription');
    const path = window.location.pathname.endsWith('/')
      ? window.location.pathname.slice(0,-1)
      : window.location.pathname.replace(/\/[^/]*$/, '');
    const prefix = path === '/' ? '' : path;
    const base64Url = window.location.origin + prefix + '/sub?token=' + encodeURIComponent(s.token || '');
    const clashUrl = window.location.origin + prefix + '/sub?format=clash&token=' + encodeURIComponent(s.token || '');
    const quanxUrl = window.location.origin + prefix + '/sub?format=quanx&token=' + encodeURIComponent(s.token || '');
    $('#subUrlBase64').value = base64Url;
    $('#subUrlClash').value = clashUrl;
    $('#subUrlQuanX').value = quanxUrl;
    $('#subQuotaGB').value = Number(s.quota_gb || 0);
    const days = s.expire_at ? Math.max(1, Math.ceil((Number(s.expire_at) - Date.now()) / 86400000)) : 0;
    $('#subExpireDays').value = String([0,1,7,30,90,365].includes(days) ? days : 0);
    $('#subPolicyStatus').textContent = s.quota_unlimited
      ? '流量：不限'
      : ('流量：' + Number(s.quota_gb).toFixed(1) + ' GB') + (s.expire_unlimited ? ' · 永久' : ' · 已设置到期时间');
  }catch(e){
    $('#subPolicyStatus').textContent = '读取订阅策略失败：' + e.message;
  }
}

document.addEventListener('click', async e => {
  if(e.target.closest('#subBtn')){
    await openSubModal();
    return;
  }
  if(e.target.closest('#copySubBase64')){
    const val = ($('#subUrlBase64').value || '').trim();
    if(val) copy(val);
    return;
  }
  if(e.target.closest('#copySubClash')){
    const val = ($('#subUrlClash').value || '').trim();
    if(val) copy(val);
    return;
  }
  if(e.target.closest('#copySubQuanX')){
    const val = ($('#subUrlQuanX').value || '').trim();
    if(val) copy(val);
    return;
  }
  if(e.target.closest('#importClashBtn')){
    const val = ($('#subUrlClash').value || '').trim();
    if(val){
      window.location.href = 'clash://install-config?url=' + encodeURIComponent(val) + '&name=FanoutGateway';
    }
    return;
  }
  if(e.target.closest('#dresetall')){
    const btn = e.target.closest('#dresetall');
    if(!confirm('确定重置这个节点的全部客户端 UUID/密码吗？旧链接会立即失效。')) return;
    btn.disabled = true;
    try{
      const id = Number(btn.dataset.inboundId || 0);
      await api('/api/panel/client/reset_all?id=' + encodeURIComponent(id), {method:'POST'});
      toast('已一键重置全部 UUID/密码');
      closeModal('detail');
      poll();
    }catch(err){ toast('重置失败：' + err.message, true); }
    finally{ btn.disabled = false; }
    return;
  }
  if(e.target.closest('#saveSubPolicy')){
    const btn = e.target.closest('#saveSubPolicy'); btn.disabled = true;
    try{
      const quota = Math.max(0, Math.min(100000, Number($('#subQuotaGB').value || 0)));
      const days = Math.max(0, parseInt($('#subExpireDays').value || '0', 10));
      await api('/api/subscription', {method:'POST', headers:{'Content-Type':'application/json'},
        body:JSON.stringify({quota_gb: quota, expire_days: days})});
      toast('订阅限制已保存');
      await openSubModal();
    }catch(err){ toast('保存订阅限制失败：' + err.message, true); }
    finally{ btn.disabled = false; }
    return;
  }
  if(e.target.closest('#resetSubToken')){
    if(!confirm('重置后旧订阅链接会立即失效，确定继续吗？')) return;
    const btn = e.target.closest('#resetSubToken'); btn.disabled = true;
    try{
      await api('/api/subscription', {method:'POST', headers:{'Content-Type':'application/json'},
        body:JSON.stringify({reset_token:true})});
      toast('订阅链接已重置，旧链接已失效');
      await openSubModal();
    }catch(err){ toast('重置订阅链接失败：' + err.message, true); }
    finally{ btn.disabled = false; }
    return;
  }
  if(e.target.closest('#autoOrchestrateBtn')){
    openModal('orchestrateModal');
    return;
  }
  if(e.target.closest('#orchAll')){
    const all = e.target.closest('#orchAll').checked;
    document.querySelectorAll('.orch-src').forEach(x => { x.checked = all; });
    return;
  }
  if(e.target.closest('#runOrchestrate')){
    const btn = e.target.closest('#runOrchestrate');
    const sources = Array.from(document.querySelectorAll('.orch-src:checked')).map(x => x.value);
    const maxStarts = Math.max(1, Math.min(150, parseInt($('#orchMax').value || '40', 10)));
    const hot = 1;
    const cold = 1;
    if(!sources.length){ toast('至少选择一个节点源', true); return; }
    btn.disabled = true;
    $('#orchProgress').style.display = 'block';
    $('#orchProgressStage').textContent = '启动中';
    $('#orchProgressMsg').textContent = '正在启动智能编排…';
    let progressTimer = null;
    const renderOrchProgress = (p) => {
      if(!p) return;
      const tested = Number(p.candidates_tested || 0), total = Number(p.candidates_total || 0);
      const pct = total ? Math.min(100, Math.round(tested * 100 / total)) : (p.running ? 8 : 100);
      $('#orchProgressBar').style.width = pct + '%';
      const stageMap = {prepare:'准备',collect:'整理候选',probe:'测活',start:'建立隧道',verify:'真实出网验证',bind:'绑定节点',done:'完成',busy:'已有任务'};
      $('#orchProgressStage').textContent = stageMap[p.stage] || p.stage || '处理中';
      $('#orchProgressCount').textContent = total ? (tested + ' / ' + total + ' 候选') : '候选统计中';
      $('#orchProgressMsg').textContent = p.message || '处理中…';
      $('#orchProgressStats').textContent = '测活通过 ' + (p.live_passed||0) + ' · 已启动 ' + (p.started||0) + ' · 验证通过 ' + (p.verified||0) + ' · 新增 ' + (p.added||0) + ' · 失败 ' + (p.failed||0);
      if(p.current_node) $('#orchProgressMsg').textContent += '（当前：' + p.current_node + '）';
      if(!p.running && progressTimer){ clearInterval(progressTimer); progressTimer = null; }
    };
    const pollOrch = async () => {
      try { renderOrchProgress(await api('/api/auto/orchestrate/status')); } catch(err) {}
    };
    try {
      const res = await api('/api/auto/orchestrate', {
        method:'POST', headers:{'Content-Type':'application/json'},
        body:JSON.stringify({sources:sources,max_starts:maxStarts})
      });
      closeModal('orchestrateModal');
      toast(res.message || '智能编排已启动');
      openModal('orchestrateModal');
      $('#orchProgress').style.display = 'block';
      progressTimer = setInterval(pollOrch, 1000);
      await pollOrch();
      setTimeout(poll, 1200); setTimeout(poll, 5000); setTimeout(poll, 12000);
    } catch(err) {
      toast('智能编排失败: ' + err.message, true);
      $('#orchProgressMsg').textContent = '启动失败：' + err.message;
    } finally {
      setTimeout(() => { btn.disabled = false; }, 1200);
    }
    return;
  }
  if(e.target.closest('#sourcesBtn') || e.target.closest('#wzSourcesBtn')){
    openModal('sourcesModal');
    loadSources();
    return;
  }
  if(e.target.closest('#wzRefreshBtn') || e.target.closest('#refreshMirrors') || e.target.closest('#refreshVpnGate') || e.target.closest('#refreshEdu') || e.target.closest('#refreshResidential') || e.target.closest('#refreshProxy')){
    const btn = e.target.closest('#wzRefreshBtn') || e.target.closest('#refreshMirrors') || e.target.closest('#refreshVpnGate') || e.target.closest('#refreshEdu') || e.target.closest('#refreshResidential') || e.target.closest('#refreshProxy');
    const src = btn.dataset.src || ($('#wzSource') ? $('#wzSource').value : 'all');
    await refreshSources(btn, src);
    return;
  }
  if(e.target.closest('#scanLocalOvpn')){
    await scanCustomOvpn(e.target.closest('#scanLocalOvpn'));
    return;
  }
  if(e.target.closest('#doImportNodes')){
    const btn = e.target.closest('#doImportNodes');
    const text = ($('#importNodesText').value || '').trim();
    if(!text){
      toast('请先粘贴节点 CSV、.ovpn 块或 IP 列表', true);
      return;
    }
    btn.disabled = true;
    try{
      const res = await api('/api/sources/import', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({text: text}),
      });
      toast('成功导入并去重 ' + (res.added || 0) + ' 个节点，当前节点池共 ' + (res.total || 0) + ' 节点');
      const rEl = $('#importResult');
      if(rEl){
        rEl.textContent = '✔ 成功导入 ' + res.added + ' 个节点，节点池总计 ' + res.total + ' 节点';
        rEl.hidden = false;
      }
      $('#importNodesText').value = '';
      await loadSources();
      regions = await api('/api/regions') || [];
      renderRegions();
      poll();
    }catch(err){
      toast('导入失败: ' + err.message, true);
    }finally{
      btn.disabled = false;
    }
    return;
  }
  if(e.target.closest('#saveCustomSrc')){
    const btn = e.target.closest('#saveCustomSrc');
    btn.disabled = true;
    const url = ($('#customSrcUrl').value || '').trim();
    try{
      const res = await api('/api/sources', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({custom_url: url}),
      });
      toast('已保存并重新拉取：共 ' + (res.count || 0) + ' 个节点');
      await loadSources();
      regions = await api('/api/regions') || [];
      renderRegions();
      poll();
    }catch(err){
      toast('设置失败: ' + err.message, true);
    }finally{
      btn.disabled = false;
    }
    return;
  }
});

// ---- 测活扫描器前端控制器 ----
let liveScanPollTimer = null;
let currentLiveNodes = [];
let selectedLiveHosts = new Set();

async function openLiveScanModal(){
  openModal('liveScanModal');
  loadLiveScanTemplates();
  await fetchAndRenderLiveNodes();
}

async function loadLiveScanTemplates(){
  const sel = $('#lsTpl');
  if(!sel) return;
  try{
    const v = await api('/api/exits');
    const free = v.direct || [];
    const bound = (v.exits || []).flatMap(e => e.inbounds || []);
    const inbounds = free.concat(bound);
    if(!inbounds.length){
      sel.innerHTML = '<option value="0">自动匹配活跃节点链接</option>';
      return;
    }
    const opt = i => '<option value="' + i.id + '">'
      + esc(i.remark || ('端口 ' + i.port)) + ' · ' + esc(i.protocol)
      + ' :' + i.port + '</option>';
    sel.innerHTML =
      '<option value="0">⚡ 自动匹配活跃节点链接 (推荐)</option>'
      + (free.length ? '<optgroup label="未绑定出口">' + free.map(opt).join('') + '</optgroup>' : '')
      + (bound.length ? '<optgroup label="已挂在出口上">' + bound.map(opt).join('') + '</optgroup>' : '');
  }catch(e){
    sel.innerHTML = '<option value="0">自动匹配活跃节点链接</option>';
  }
}

async function fetchAndRenderLiveNodes(isBackgroundPoll = false){
  const source = $('#lsSource') ? $('#lsSource').value : 'all';
  const region = $('#lsRegion') ? $('#lsRegion').value : 'all';
  const search = $('#lsSearch') ? ($('#lsSearch').value || '').trim() : '';
  const url = '/api/nodes/live?source=' + encodeURIComponent(source) + 
              '&region=' + encodeURIComponent(region) + 
              '&search=' + encodeURIComponent(search);
  try{
    const data = await api(url);
    if(!data) return;
    
    const isScanning = !!data.scanning;
    const dot = $('#lsDot');
    const statusText = $('#lsStatusText');
    const startBtn = $('#lsStartScanBtn');
    
    if(dot) dot.className = 'dot ' + (isScanning ? 'starting' : 'up');
    if(startBtn){
      startBtn.disabled = isScanning;
      startBtn.innerHTML = isScanning 
        ? (ICON.run + ' 正在并发测活 (' + (data.progress || 0) + '/' + (data.total || 0) + ')...')
        : '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><polygon points="10 8 16 12 10 16 10 8"/></svg> 开始测活扫描';
    }
    
    if(statusText){
      if(isScanning){
        statusText.textContent = '母机发包实测中: 进度 ' + (data.progress || 0) + ' / ' + (data.total || 0) + '，已验证 ' + (data.verified_count || 0) + ' 个有效节点';
      } else if(data.last_scan && data.last_scan !== '0001-01-01 00:00:00'){
        statusText.textContent = '探测完成！已筛选出 ' + (data.verified_count || 0) + ' 个母机 100% 实测通畅出网节点';
      } else {
        statusText.textContent = '就绪：点击【开始测活扫描】从 VPS 真实发包探测';
      }
    }

    if($('#lsVerifiedCount')) $('#lsVerifiedCount').textContent = data.verified_count || 0;
    if($('#lsLastScanTime')) $('#lsLastScanTime').textContent = (data.last_scan && data.last_scan !== '0001-01-01 00:00:00') ? ('上次: ' + data.last_scan) : '';

    currentLiveNodes = data.nodes || [];
    renderLiveNodesTable(currentLiveNodes);

    if(isScanning){
      if(!liveScanPollTimer){
        liveScanPollTimer = setInterval(() => fetchAndRenderLiveNodes(true), 1200);
      }
    } else {
      if(liveScanPollTimer){
        clearInterval(liveScanPollTimer);
        liveScanPollTimer = null;
      }
    }
  }catch(e){
    if(!isBackgroundPoll){
      toast('获取测活数据失败: ' + e.message, true);
    }
  }
}

async function triggerStartLiveScan(){
  const source = $('#lsSource') ? $('#lsSource').value : 'all';
  const region = $('#lsRegion') ? $('#lsRegion').value : 'all';
  const max = $('#lsMax') ? $('#lsMax').value : '0';
  const startBtn = $('#lsStartScanBtn');
  if(startBtn) startBtn.disabled = true;

  toast('正在启动母机并发真实测活 (100协程并发拉满)...');
  try{
    await api('/api/nodes/live_scan?source=' + encodeURIComponent(source) + '&region=' + encodeURIComponent(region) + '&max=' + encodeURIComponent(max), {
      method: 'POST'
    });
    fetchAndRenderLiveNodes();
    if(!liveScanPollTimer){
      liveScanPollTimer = setInterval(() => fetchAndRenderLiveNodes(true), 1200);
    }
  }catch(e){
    toast('启动测活失败: ' + e.message, true);
    if(startBtn) startBtn.disabled = false;
  }
}

function renderLiveNodesTable(nodes){
  const tbody = $('#lsTableBody');
  if(!tbody) return;
  if(!nodes || nodes.length === 0){
    tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;padding:36px;color:var(--dim)">暂无实测可用节点。请点击上方【开始测活扫描】从 VPS 发起全网真实探测</td></tr>';
    updateLiveBatchState();
    return;
  }

  // 严格按网络质量最优排在最前面 (延迟最低优先，同延迟则带宽最高优先)
  nodes.sort((a, b) => {
    const pa = a.ping && a.ping > 0 ? a.ping : 9999;
    const pb = b.ping && b.ping > 0 ? b.ping : 9999;
    if (pa !== pb) return pa - pb;
    return (b.speed_mbps || 0) - (a.speed_mbps || 0);
  });

  const rows = nodes.map(n => {
    const isChecked = selectedLiveHosts.has(n.hostname);
    const flag = getFlagEmoji(n.country_code);
    const cc = esc(n.country_code || 'GLOBAL');
    const countryName = esc(n.country || '');
    const ping = n.ping || 50;
    const pingColor = ping < 80 ? 'var(--ok)' : (ping < 180 ? 'var(--accent)' : 'var(--warn)');
    
    let protoBadge = '<span class="tag-host" style="font-size:10px">OpenVPN</span>';
    if(n.proto === 'socks5'){
      protoBadge = '<span class="tag-res" style="font-size:10px">SOCKS5</span>';
    } else if(n.proto === 'http' || n.proto === 'https'){
      protoBadge = '<span class="tag-mob" style="font-size:10px">HTTP</span>';
    }

    let eduBadge = '';
    if(n.source === 'edu' || n.ip_type === 'edu' || (n.hostname && n.hostname.toLowerCase().includes('tsukuba'))){
      eduBadge = '<span style="background:rgba(74,158,218,.2);color:#4a9eda;border:1px solid rgba(74,158,218,.4);border-radius:3px;padding:1px 4px;font-size:10px;margin-left:4px">🎓 海外学术</span>';
    }

    const hostName = esc(n.hostname);
    const ip = esc(n.ip);
    const isp = esc(n.isp || '公共骨干网');

    return '<tr style="border-bottom:1px solid var(--line);transition:background .15s" onmouseover="this.style.background=\'rgba(255,255,255,0.02)\'" onmouseout="this.style.background=\'transparent\'">' +
      '<td style="padding:7px 10px"><input type="checkbox" class="ls-chk" data-host="' + hostName + '" ' + (isChecked ? 'checked' : '') + '></td>' +
      '<td style="padding:7px 10px;white-space:nowrap"><span style="font-size:14px;margin-right:4px">' + flag + '</span><b>' + cc + '</b></td>' +
      '<td style="padding:7px 10px">' + protoBadge + '</td>' +
      '<td style="padding:7px 10px">' +
        '<div style="font-weight:600;font-family:monospace;color:var(--text);display:flex;align-items:center">' + ip + eduBadge + '</div>' +
        '<div style="font-size:11px;color:var(--dim);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:320px">' + isp + (countryName ? ' · ' + countryName : '') + '</div>' +
      '</td>' +
      '<td style="padding:7px 10px;font-family:monospace;font-weight:600;color:' + pingColor + '">' + ping + ' ms</td>' +
      '<td style="padding:7px 10px"><span style="color:var(--ok);font-size:11px;font-weight:600;display:inline-flex;align-items:center;gap:3px">✔ 实测通畅</span></td>' +
      '<td style="padding:7px 10px;text-align:right">' +
        '<button class="btn-xs primary ls-start-one" data-host="' + hostName + '" style="padding:3px 10px;font-weight:600">' +
          '<svg viewBox="0 0 24 24" style="width:12px;height:12px"><polygon points="5 3 19 12 5 21 5 3"/></svg> 启动' +
        '</button>' +
      '</td>' +
    '</tr>';
  }).join('');

  tbody.innerHTML = rows;
  updateLiveBatchState();
}

function updateLiveBatchState(){
  const count = selectedLiveHosts.size;
  if($('#lsSelCount')) $('#lsSelCount').textContent = count;
  const batchBtn = $('#lsBatchStartBtn');
  if(batchBtn) batchBtn.disabled = count === 0;
  
  const allChk = $('#lsSelectAll');
  if(allChk){
    if(currentLiveNodes.length > 0 && count === currentLiveNodes.length){
      allChk.checked = true;
      allChk.indeterminate = false;
    } else if(count > 0){
      allChk.checked = false;
      allChk.indeterminate = true;
    } else {
      allChk.checked = false;
      allChk.indeterminate = false;
    }
  }
}

async function startSingleLiveNode(host, btn){
  if(btn) btn.disabled = true;
  toast('正在启动该实测节点并对接节点链接...');
  const tpl = $('#lsTpl') ? ($('#lsTpl').value || '0') : '0';
  try{
    const res = await api('/api/start?host=' + encodeURIComponent(host) + '&template=' + encodeURIComponent(tpl));
    toast('节点启动成功！SOCKS5 端口: ' + (res.port || '已分配') + '，出口IP: ' + (res.exit_ip || '协商中'));
    closeModal('liveScanModal');
    poll();
  }catch(e){
    toast('启动失败: ' + e.message, true);
    if(btn) btn.disabled = false;
  }
}

async function startBatchLiveNodes(){
  const hosts = Array.from(selectedLiveHosts);
  if(!hosts.length){
    toast('请先勾选要启动的节点', true);
    return;
  }
  const btn = $('#lsBatchStartBtn');
  if(btn) btn.disabled = true;
  const tpl = $('#lsTpl') ? (parseInt($('#lsTpl').value, 10) || 0) : 0;
  toast('正在批量启动 ' + hosts.length + ' 个实测节点并对接节点链接...');
  try{
    const res = await api('/api/nodes/batch_start', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({hosts: hosts, template: tpl})
    });
    const started = (res.started || []).length;
    const errCount = (res.errors || []).length;
    if(started > 0){
      toast('成功启动 ' + started + ' 个出口节点并对接节点链接！' + (errCount ? ' (' + errCount + ' 个冲突/失败)' : ''));
      selectedLiveHosts.clear();
      closeModal('liveScanModal');
      poll();
    } else {
      toast('启动失败: ' + (res.errors || []).join('; '), true);
    }
  }catch(e){
    toast('批量启动失败: ' + e.message, true);
  }finally{
    if(btn) btn.disabled = false;
  }
}

document.addEventListener('click', async e => {
  if(e.target.closest('#scanNodesBtn')){
    openLiveScanModal();
    return;
  }
  if(e.target.closest('#lsStartScanBtn')){
    await triggerStartLiveScan();
    return;
  }
  if(e.target.closest('#lsBatchStartBtn')){
    await startBatchLiveNodes();
    return;
  }
  const startOne = e.target.closest('.ls-start-one');
  if(startOne){
    const host = startOne.dataset.host;
    if(host) await startSingleLiveNode(host, startOne);
    return;
  }
  if(e.target.matches('#lsSelectAll')){
    const chk = e.target;
    if(chk.checked){
      currentLiveNodes.forEach(n => selectedLiveHosts.add(n.hostname));
    } else {
      selectedLiveHosts.clear();
    }
    renderLiveNodesTable(currentLiveNodes);
    return;
  }
  const itemChk = e.target.closest('.ls-chk');
  if(itemChk){
    const host = itemChk.dataset.host;
    if(itemChk.checked){
      selectedLiveHosts.add(host);
    } else {
      selectedLiveHosts.delete(host);
    }
    updateLiveBatchState();
    return;
  }
});

const scanNodesBtn = $('#scanNodesBtn');
if(scanNodesBtn) scanNodesBtn.onclick = openLiveScanModal;
const lsSrcEl = $('#lsSource');
if(lsSrcEl) lsSrcEl.onchange = () => fetchAndRenderLiveNodes();
const lsRgEl = $('#lsRegion');
if(lsRgEl) lsRgEl.onchange = () => fetchAndRenderLiveNodes();
const lsSearchEl = $('#lsSearch');
if(lsSearchEl) lsSearchEl.oninput = () => fetchAndRenderLiveNodes();

const subBtn = $('#subBtn');
if(subBtn) subBtn.onclick = openSubModal;
const srcBtn = $('#sourcesBtn');
if(srcBtn) srcBtn.onclick = () => { openModal('sourcesModal'); loadSources(); };
const nExitBtn = $('#newexit');
if(nExitBtn) nExitBtn.onclick = () => { openModal('wizard'); if(!regionsLoaded) loadWizard(); else { renderRegions(); loadWizard(); } };

loadGlobalExitLimit();
poll();
setInterval(poll, 7000);
