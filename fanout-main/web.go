package main

import "net/http"

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>fanout</title>
<style>
:root{
  --bg:#12151a; --panel:#181c23; --line:#262c36; --text:#dde3ec;
  --dim:#8b95a5; --accent:#4a9eda; --ok:#3fa66b; --warn:#c9903a; --bad:#c25450;
}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);
  font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
header{display:flex;align-items:center;gap:16px;padding:10px 16px;
  border-bottom:1px solid var(--line);background:var(--panel)}
h1{font-size:13px;font-weight:600;margin:0;letter-spacing:0}
.spacer{flex:1}
button{font:inherit;color:var(--text);background:#222833;border:1px solid var(--line);
  border-radius:4px;padding:4px 10px;cursor:pointer;display:inline-flex;
  align-items:center;gap:5px;white-space:nowrap}
button:hover:not(:disabled){border-color:var(--accent)}
button:disabled{opacity:.45;cursor:default}
button.primary{background:var(--accent);border-color:var(--accent);color:#0b0e12;font-weight:600}
button.icon{padding:3px 6px;background:transparent;border-color:transparent;color:var(--dim)}
button.icon:hover:not(:disabled){color:var(--accent);border-color:var(--line)}
button.icon.danger:hover:not(:disabled){color:var(--bad);border-color:rgba(194,84,80,.35)}
svg{width:14px;height:14px;stroke:currentColor;fill:none;stroke-width:1.8;
  stroke-linecap:round;stroke-linejoin:round;flex:none}
main{padding:14px 16px 40px;max-width:1180px;margin:0 auto}
.bar{display:flex;align-items:center;gap:10px;margin-bottom:12px}
.bar h2{font-size:12px;margin:0;font-weight:600;color:var(--dim)}
.exit{border:1px solid var(--line);border-radius:6px;margin-bottom:8px;
  background:var(--panel);overflow:hidden}
.exit>.row{display:grid;gap:6px 12px;align-items:center;padding:9px 12px;
  grid-template-columns:14px auto 1fr auto auto auto;
  grid-template-areas:"dot ip meta chips socks acts"}
.exit .dot{grid-area:dot}
.exit .ip{grid-area:ip;display:flex;align-items:center;flex-wrap:wrap;gap:4px}
.exit .meta{grid-area:meta}
.exit .chips{grid-area:chips}
.exit .socks{grid-area:socks}
.exit .acts{grid-area:acts}
.tag-res{background:rgba(63,166,107,.18);color:#3fa66b;border:1px solid rgba(63,166,107,.4);border-radius:3px;padding:1px 5px;font-size:11px;font-weight:600}
.tag-host{background:rgba(74,158,218,.18);color:#4a9eda;border:1px solid rgba(74,158,218,.4);border-radius:3px;padding:1px 5px;font-size:11px;font-weight:600}
.tag-mob{background:rgba(201,144,58,.18);color:#c9903a;border:1px solid rgba(201,144,58,.4);border-radius:3px;padding:1px 5px;font-size:11px;font-weight:600}
.tag-purity{font-size:11px;font-weight:600;padding:1px 5px;border-radius:3px;background:#0e1116;border:1px solid var(--line)}
.stats-summary{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:12px}
.stat-pill{background:var(--panel);border:1px solid var(--line);border-radius:4px;padding:3px 9px;font-size:12px;color:var(--dim)}
.stat-pill b{color:var(--text);margin-left:4px}
.dot{width:8px;height:8px;border-radius:50%;background:var(--dim);justify-self:center}
.dot.up{background:var(--ok)}
.dot.starting{background:var(--warn);animation:pulse 1.2s ease-in-out infinite}
.dot.failed{background:var(--bad)}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.3}}
.ip{font-weight:600;font-variant-numeric:tabular-nums}
.meta{color:var(--dim);font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.chips{display:flex;gap:6px;flex-wrap:wrap}
.chip{border:1px solid var(--line);border-radius:3px;padding:1px 7px;font-size:11px;
  color:var(--dim);cursor:pointer;background:#0e1116}
.chip:hover{border-color:var(--accent);color:var(--text)}
.chip-item{display:inline-flex;align-items:center;background:#202632;border:1px solid var(--line);border-radius:4px;overflow:hidden}
.chip-item .chip{border:none;border-radius:0;background:transparent;padding:1px 6px}
.chip-item .chip:hover{background:#2a3242}
.chip-item .chip-select{border:none;border-left:1px solid var(--line);background:#161a22;color:var(--dim);font-size:11px;padding:2px 5px;cursor:pointer;outline:none}
.chip-item .chip-select:hover{color:var(--text);background:#222834}
.chip.none{border-style:dashed;cursor:default}
.chip.none:hover{border-color:var(--line);color:var(--dim)}
.orphan{margin-top:18px;border:1px solid var(--line);border-radius:6px;
  background:var(--panel);padding:10px 12px}
.orphan .top{display:flex;align-items:center;gap:10px;margin-bottom:8px}
.orphan .top h3{font-size:12px;margin:0;font-weight:600;color:var(--dim)}
.socks{color:var(--dim);font-size:12px;font-variant-numeric:tabular-nums}
.socks button{background:transparent;border-color:transparent;color:var(--dim);
  font-size:12px;padding:2px 6px;font-variant-numeric:tabular-nums}
.socks button:hover:not(:disabled){color:var(--accent);border-color:var(--line)}
.socks button .lock{width:11px;height:11px;stroke-width:2}
.acts{display:flex;gap:2px;justify-self:end}
.errline{padding:0 12px 9px 38px;color:var(--bad);font-size:11px;
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.empty{border:1px dashed var(--line);border-radius:6px;padding:40px 20px;
  text-align:center;color:var(--dim)}
.empty button{margin-top:14px}
.jobs{margin-bottom:12px}
.job{border:1px solid var(--line);border-radius:6px;background:var(--panel);
  padding:10px 12px;margin-bottom:8px}
.job .top{display:flex;align-items:center;gap:10px;margin-bottom:8px}
.job .top strong{font-weight:600;font-size:12px}
.steps{display:flex;flex-wrap:wrap;gap:6px}
.step{display:flex;align-items:center;gap:5px;font-size:11px;color:var(--dim);
  border:1px solid var(--line);border-radius:3px;padding:2px 7px;background:#0e1116}
.step.ok{color:var(--ok);border-color:rgba(63,166,107,.35)}
.step.failed{color:var(--bad);border-color:rgba(194,84,80,.35)}
.step.running{color:var(--warn);border-color:rgba(201,144,58,.35)}
.spin{animation:rot 1s linear infinite;transform-origin:center}
@keyframes rot{to{transform:rotate(360deg)}}
.jobs-bar{display:flex;align-items:center;gap:10px;margin-bottom:8px;font-size:12px;color:var(--dim)}
.btn-xs{font-size:11px;padding:3px 8px;border-radius:4px;background:#141820;border:1px solid var(--line);color:var(--dim);cursor:pointer;display:inline-flex;align-items:center;gap:4px}
.btn-xs:hover{color:var(--text);border-color:var(--accent)}
.btn-xs.danger{color:var(--bad)}
.btn-xs.danger:hover{border-color:var(--bad)}
.step.failed-summary{cursor:pointer;border-style:dashed}
.step.failed-summary:hover{background:rgba(194,84,80,.1)}
.failed-steps-wrap{width:100%;margin-top:6px;padding-top:6px;border-top:1px dashed var(--line);display:none}
.failed-steps-wrap.open{display:flex;flex-wrap:wrap;gap:6px}
.links{display:flex;gap:14px;margin-right:4px}
.links a{color:var(--dim);text-decoration:none;font-size:12px}
.links a:hover{color:var(--accent)}
@media(max-width:820px){.links{display:none}
  main{padding:12px 12px 40px}
  .exit>.row{grid-template-columns:14px 1fr auto;
    grid-template-areas:"dot ip acts" ". meta meta" ". socks socks" ". chips chips"}
  .exit .chips{margin-top:2px}
  .bar{flex-wrap:wrap}}
.modal{position:fixed;inset:0;background:rgba(8,10,14,.72);display:none;
  align-items:center;justify-content:center;z-index:50;padding:20px}
.modal.open{display:flex}
.sheet{background:var(--bg);border:1px solid var(--line);border-radius:6px;
  width:min(680px,100%);max-height:86vh;display:flex;flex-direction:column}
.sheet .head{display:flex;align-items:center;gap:10px;padding:10px 14px;
  border-bottom:1px solid var(--line);background:var(--panel);border-radius:6px 6px 0 0}
.sheet .head h2{font-size:12px;margin:0;font-weight:600}
.sheet .body{overflow:auto;padding:14px}
.sheet .foot{display:flex;align-items:center;gap:10px;padding:10px 14px;
  border-top:1px solid var(--line);background:var(--panel);border-radius:0 0 6px 6px}
.count{color:var(--dim);font-size:11px}
label.f{display:block;margin-bottom:16px}
label.f[hidden]{display:none}
label.f>span{display:block;color:var(--dim);font-size:11px;margin-bottom:6px}
.regions{display:grid;grid-template-columns:repeat(auto-fill,minmax(148px,1fr));
  gap:6px;max-height:224px;overflow:auto}
.rg{border:1px solid var(--line);background:#0e1116;border-radius:4px;padding:7px 9px;
  cursor:pointer;text-align:left;display:block;width:100%}
.rg:hover{border-color:var(--accent)}
.rg.sel{border-color:var(--accent);background:rgba(74,158,218,.1)}
.rg b{font-weight:600;font-size:12px;display:block;overflow:hidden;
  text-overflow:ellipsis;white-space:nowrap}
.rg em{display:block;font-style:normal;color:var(--dim);font-size:11px;margin-top:2px}
.stepper{display:flex;align-items:center;gap:0;width:fit-content;
  border:1px solid var(--line);border-radius:4px;overflow:hidden;background:#0e1116}
.stepper button{border:0;border-radius:0;background:transparent;padding:5px 11px}
select,input[type=search],input[type=text]{font:inherit;background:#0e1116;
  border:1px solid var(--line);color:var(--text);border-radius:4px;
  padding:5px 8px;width:100%}
select:focus,input[type=search]:focus,input[type=text]:focus{outline:none;border-color:var(--accent)}
.stepper input[type=text]{width:56px;text-align:center;font:inherit;background:transparent;
  border:0;border-left:1px solid var(--line);border-right:1px solid var(--line);
  color:var(--text);padding:5px 0;font-variant-numeric:tabular-nums}
.stepper input:focus{outline:none}
.hint{color:var(--dim);font-size:11px;margin-top:6px}
.setrow{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:16px}
.setrow input,.setrow select{width:100%}
.updsec{margin-top:18px;padding-top:14px;border-top:1px solid var(--line)}
.updrow{display:flex;align-items:center;gap:10px}
.updver{font-size:12px;color:var(--text)}
.updver b{font-weight:600}
.updver span{color:var(--dim);margin-left:8px}
.updnotes{margin-top:10px;padding:10px;background:#0e1116;border:1px solid var(--line);
  border-radius:4px;font-size:12px;line-height:1.6;color:var(--dim);white-space:pre-wrap;
  max-height:180px;overflow:auto}
label.chk{display:flex;align-items:center;gap:7px;color:var(--text);font-size:12px;
  cursor:pointer;margin:0}
label.chk input{margin:0}
.hint.bad{color:var(--bad)}
.kv{display:grid;grid-template-columns:76px 1fr;gap:5px 12px;margin:0 0 14px}
.kv dt{color:var(--dim)}
.kv dd{margin:0;word-break:break-all}
.share{padding:10px;background:#0e1116;border:1px solid var(--line);
  border-radius:4px;word-break:break-all;font-size:12px;line-height:1.7;margin-bottom:8px}
.editbar{display:flex;align-items:flex-end;gap:12px;flex-wrap:wrap;
  padding:12px 0;border-top:1px solid var(--line);margin-top:4px}
.ef{display:block}
.ef>span{display:block;color:var(--dim);font-size:11px;margin-bottom:4px}
.ef input{width:150px}
.credrow{display:flex;align-items:flex-end;gap:12px;flex-wrap:wrap;margin-bottom:10px}
.credrow .ef input{width:190px}
.chead{display:flex;align-items:center;gap:10px;margin:14px 0 8px;
  padding-top:12px;border-top:1px solid var(--line)}
.chead h3{font-size:12px;margin:0;font-weight:600;color:var(--dim)}
.client{border:1px solid var(--line);border-radius:4px;padding:8px 10px;margin-bottom:8px}
.orow{display:flex;align-items:center;gap:10px;padding:6px 0}
.orow select{width:200px}
.crow{display:flex;align-items:center;gap:10px}
.cemail{font-weight:600;font-size:12px}
.cid{color:var(--dim);font-size:11px;overflow:hidden;text-overflow:ellipsis;
  white-space:nowrap;max-width:280px}
.client .share{margin:8px 0 0}
.share button{margin-top:8px}
textarea{width:100%;min-height:300px;background:#0e1116;border:1px solid var(--line);
  color:var(--text);border-radius:4px;
  font:12px/1.8 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
  padding:10px 12px;resize:vertical}
textarea:focus{outline:none;border-color:var(--accent)}
.toast{position:fixed;left:50%;bottom:24px;transform:translateX(-50%);
  background:var(--panel);border:1px solid var(--line);border-radius:4px;
  padding:8px 14px;font-size:12px;z-index:80;opacity:0;pointer-events:none;
  transition:opacity .18s}
.toast.show{opacity:1}
.toast.bad{border-color:rgba(194,84,80,.5);color:var(--bad)}
</style>
</head>
<body>
<header>
  <h1>fanout</h1>
  <span class="count" id="panel"></span>
  <span class="spacer"></span>
  <button class="icon" id="settingsBtn" title="设置">
    <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
  </button>
  <nav class="links">
    <a href="https://t.me/+ft-zI76oovgwNmRh" target="_blank" rel="noopener">交流群</a>
    <a href="https://youtube.com/@joeyblog" target="_blank" rel="noopener">油管</a>
    <a href="https://joeyblog.net" target="_blank" rel="noopener">博客</a>
    <a href="https://github.com/byJoey/fanout" target="_blank" rel="noopener">GitHub</a>
  </nav>
</header>

<main>
  <div class="jobs" id="jobs"></div>

  <div class="bar">
    <h2>出口</h2>
    <span class="count" id="ecount"></span>
    <span class="spacer"></span>
    <button class="primary" id="subBtn" title="全平台通用聚合订阅 (V2RayN / Shadowrocket / Sing-box / Clash / Mihomo)" style="background:linear-gradient(135deg,#2e8555,#3fa66b);border-color:#3fa66b;color:#fff">
      <svg viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
      聚合订阅
    </button>
    <button id="exportAll" title="导出全部节点链接">
      <svg viewBox="0 0 24 24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M7 10l5 5 5-5"/><path d="M12 15V3"/></svg>
      导出链接
    </button>
    <button id="stopall" title="停止所有出口">
      <svg viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>
      全部停止
    </button>
    <button id="prunefailed" title="清除所有连接失败或中断的出口">
      <svg viewBox="0 0 24 24"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><line x1="10" y1="11" x2="10" y2="17"/><line x1="14" y1="11" x2="14" y2="17"/></svg>
      清除失效
    </button>
    <button id="sourcesBtn" title="管理节点源与抓取镜像">
      <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg>
      节点源
    </button>
    <button id="newnode" title="新建一个节点（协议与端口）">
      <svg viewBox="0 0 24 24"><path d="M4 7h16"/><path d="M4 12h16"/><path d="M4 17h10"/></svg>
      新建节点
    </button>
    <button class="primary" id="newexit">
      <svg viewBox="0 0 24 24"><path d="M12 5v14"/><path d="M5 12h14"/></svg>
      新建出口
    </button>
  </div>

  <div id="list"></div>

  <div id="orphans"></div>
</main>

<div class="modal" id="wizard">
  <div class="sheet">
    <div class="head">
      <h2>新建出口</h2>
      <span class="spacer"></span>
      <button class="icon" data-close="wizard" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <label class="f">
        <span>地区</span>
        <input type="search" id="rgfilter" placeholder="筛选地区">
        <div class="regions" id="regions" style="margin-top:6px"></div>
      </label>
      <label class="f">
        <span>数量</span>
        <div class="stepper">
          <button id="minus" title="减少">
            <svg viewBox="0 0 24 24"><path d="M5 12h14"/></svg>
          </button>
          <input id="count" type="text" inputmode="numeric" value="3">
          <button id="plus" title="增加">
            <svg viewBox="0 0 24 24"><path d="M12 5v14"/><path d="M5 12h14"/></svg>
          </button>
        </div>
        <div class="hint" id="availhint"></div>
      </label>
      <label class="f" id="tplwrap">
        <span>节点链接</span>
        <select id="tpl"></select>
        <div class="hint" id="tplhint"></div>
      </label>
    </div>
    <div class="foot">
      <span class="count" id="wzhint"></span>
      <span class="spacer"></span>
      <button data-close="wizard">取消</button>
      <button class="primary" id="go">开始</button>
    </div>
  </div>
</div>

<div class="modal" id="newnodebox">
  <div class="sheet">
    <div class="head">
      <h2>新建节点</h2>
      <span class="spacer"></span>
      <button class="icon" data-close="newnodebox" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <label class="f">
        <span>协议</span>
        <select id="nproto">
          <option value="vless">VLESS</option>
          <option value="vmess">VMess</option>
          <option value="trojan">Trojan</option>
          <option value="shadowsocks">Shadowsocks</option>
          <option value="socks">Socks5</option>
          <option value="http">HTTP</option>
          <option value="wireguard">WireGuard</option>
        </select>
      </label>
      <label class="f">
        <span>传输</span>
        <select id="nnet">
          <option value="tcp">TCP</option>
          <option value="ws">WebSocket</option>
          <option value="grpc">gRPC</option>
          <option value="httpupgrade">HTTPUpgrade</option>
          <option value="xhttp">XHTTP</option>
        </select>
      </label>
      <label class="f">
        <span>安全</span>
        <select id="nsec">
          <option value="none">无</option>
          <option value="tls">TLS</option>
          <option value="reality">REALITY</option>
        </select>
        <div class="hint" id="nsechint"></div>
      </label>
      <label class="f" id="nvisionwrap" hidden>
        <span>流控</span>
        <label class="chk"><input type="checkbox" id="nvision"> xtls-rprx-vision</label>
      </label>
      <label class="f" id="nsniwrap" hidden>
        <span>域名 SNI</span>
        <input id="nsni" type="text" placeholder="留空用 localhost，将生成自签证书">
      </label>
      <label class="f" id="ncertwrap" hidden>
        <span>证书路径</span>
        <input id="ncert" type="text" placeholder="留空生成自签证书，如 /etc/ssl/x.crt">
      </label>
      <label class="f" id="nkeywrap" hidden>
        <span>私钥路径</span>
        <input id="nkey" type="text" placeholder="与证书成对填写，如 /etc/ssl/x.key">
      </label>
      <label class="f" id="ndestwrap" hidden>
        <span>借用站点</span>
        <input id="ndest" type="text" placeholder="留空用 www.tesla.com:443">
      </label>
      <label class="f" id="npathwrap" hidden>
        <span id="npathlabel">路径</span>
        <input id="npath" type="text" placeholder="留空自动生成">
      </label>
      <label class="f">
        <span>端口</span>
        <input id="nport" type="text" inputmode="numeric" placeholder="留空随机分配">
      </label>
      <label class="f">
        <span>备注</span>
        <input id="nremark" type="text" placeholder="留空自动命名">
      </label>
    </div>
    <div class="foot">
      <span class="count" id="nnhint"></span>
      <span class="spacer"></span>
      <button data-close="newnodebox">取消</button>
      <button class="primary" id="ncreate">创建</button>
    </div>
  </div>
</div>

<div class="modal" id="detail">
  <div class="sheet">
    <div class="head">
      <h2 id="dtitle">节点</h2>
      <span class="spacer"></span>
      <button class="icon danger" id="ddel" title="删除这个入站">
        <svg viewBox="0 0 24 24"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
      </button>
      <button class="icon" data-close="detail" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body" id="dbody"></div>
  </div>
</div>

<div class="modal" id="credbox">
  <div class="sheet">
    <div class="head">
      <h2>SOCKS5 访问凭据</h2>
      <span class="count" id="crtitle"></span>
      <span class="spacer"></span>
      <button class="icon" data-close="credbox" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <div class="share" id="crurl"></div>
      <div class="credrow">
        <label class="ef"><span>用户名</span>
          <input id="cruser" type="text" spellcheck="false"></label>
        <label class="ef"><span>口令</span>
          <input id="crpass" type="text" spellcheck="false"></label>
        <button id="crrand" title="随机生成一套">
          <svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/></svg>
          随机
        </button>
      </div>
      <div class="hint">改完立即生效，已连上的会话不断；用旧凭据的客户端要改配置。</div>
    </div>
    <div class="foot">
      <button id="crcopy">
        <svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
        复制地址
      </button>
      <span class="spacer"></span>
      <button data-close="credbox">取消</button>
      <button class="primary" id="crsave">保存</button>
    </div>
  </div>
</div>

<div class="modal" id="export">
  <div class="sheet">
    <div class="head">
      <h2>节点链接</h2>
      <span class="count" id="excount"></span>
      <span class="spacer"></span>
      <button id="copyall">
        <svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
        全部复制
      </button>
      <button class="icon" data-close="export" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body"><textarea id="exbox" spellcheck="false" readonly></textarea></div>
  </div>
</div>

<div class="modal" id="settings">
  <div class="sheet">
    <div class="head">
      <h2>设置</h2>
      <span class="spacer"></span>
      <button class="icon" data-close="settings" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <label class="f"><span>访问口令</span>
        <input id="setPw" type="password" spellcheck="false" autocomplete="new-password" placeholder="留空则不改"></label>
      <div class="hint">改完只影响新登录，当前这个浏览器不会被踢下线。</div>

      <label class="f" style="margin-top:16px"><span>访问路径</span>
        <input id="setPath" type="text" spellcheck="false" placeholder="留空则去掉路径前缀"></label>
      <div class="hint" id="setPathHint">界面挂在这个路径下，扫端口的探不到。只能用字母数字和 - _。</div>

      <label class="f" style="margin-top:16px"><span>节点后端</span>
        <select id="setBackend"></select></label>
      <div class="hint" id="setBackendHint">节点从哪来。装了 3x-ui 或 xray-cf-lite 就能直接接管，都没有就用自建。</div>

      <div class="setrow">
        <label class="f" style="margin:0"><span>监听端口</span>
          <input id="setPort" type="text" inputmode="numeric" spellcheck="false"></label>
        <label class="f" style="margin:0"><span>本地监听地址</span>
          <select id="setListen">
            <option value="0.0.0.0">所有网卡（0.0.0.0）</option>
            <option value="127.0.0.1">仅本机（127.0.0.1）</option>
          </select></label>
      </div>
      <div class="hint bad" id="setPortHint">改端口或监听地址会切换监听，保存后要用新地址重新打开界面。</div>

      <div class="updsec">
        <div class="updrow">
          <div class="updver">版本 <b id="updCur">-</b><span id="updLatest"></span></div>
          <span class="spacer"></span>
          <button id="updCheck">检查更新</button>
          <button class="primary" id="updApply" hidden>更新到 <span id="updApplyVer"></span></button>
        </div>
        <div class="updnotes" id="updNotes" hidden></div>
      </div>
    </div>
    <div class="foot">
      <span class="spacer"></span>
      <button data-close="settings">取消</button>
      <button class="primary" id="setSave">保存</button>
    </div>
  </div>
</div>

<div class="modal" id="sourcesModal">
  <div class="sheet">
    <div class="head">
      <h2>节点源与镜像管理</h2>
      <span class="spacer"></span>
      <button class="icon" data-close="sourcesModal" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <div style="background:var(--subtle);padding:14px;border-radius:8px;margin-bottom:16px;border:1px solid var(--border)">
        <div style="display:flex;align-items:center;margin-bottom:8px">
          <span style="font-weight:600">当前活跃源：</span>
          <span id="srcActive" style="color:var(--accent);font-family:monospace;margin-left:6px">-</span>
        </div>
        <div style="display:flex;gap:12px;font-size:12px;color:var(--dim);flex-wrap:wrap">
          <span>总节点：<b id="srcTotal" style="color:var(--text)">0</b></span>
          <span>本地 .ovpn：<b id="srcCustomCount" style="color:var(--text)">0</b></span>
          <span>离线缓存：<b id="srcCachedCount" style="color:var(--text)">0</b></span>
          <span>上次刷新：<span id="srcLastFetch">-</span></span>
        </div>
        <div id="srcErr" class="hint bad" style="margin-top:8px" hidden></div>
      </div>

      <div style="margin-bottom:16px">
        <h3 style="margin-bottom:8px;font-size:14px">自定义在线节点源（支持填写多个订阅）</h3>
        <div style="display:flex;flex-direction:column;gap:8px">
          <textarea id="customSrcUrl" style="min-height:60px;font-size:11px;font-family:monospace;padding:6px 8px" placeholder="https://... 或 http://... (支持填写多个源，每行一个或逗号分隔)"></textarea>
          <div><button class="primary" id="saveCustomSrc">保存并拉取所有源</button></div>
        </div>
        <div class="hint">可输入您自己的 VPN Gate 镜像、反代或自建 API。留空则恢复默认官方与内置镜像。</div>
      </div>

      <div style="margin-bottom:16px">
        <h3 style="margin-bottom:8px;font-size:14px">全网多源节点池（全网数万节点 + 日本筑波大学 + 教育网高校）</h3>
        <div style="font-size:12px;color:var(--dim);line-height:1.6;background:var(--card-bg);padding:10px;border-radius:6px;border:1px solid var(--border);margin-bottom:8px">
          <div>• <b>日本筑波大学官方及全量容灾镜像池</b>：150.40.105.19 / 119.195.163.98 等 11+ 镜像</div>
          <div>• <b>全网开源公开代理库 (100,000+ 节点)</b>：Zevtyardt / Proxifly / Hookzof / TheSpeedX 等并发聚合</div>
          <div>• <b>教育网与高校学术网专项</b>：CERNET / SINET / 清华 / 北大 / 中科大 / 浙大等高校 IP 专项识别与归类</div>
          <div>• <b>全自动容灾与并发测速</b>：自动并发拉取、剔除不可用死节点、按纯净度与网络速度降序排序</div>
        </div>
        <button id="refreshMirrors">🔄 立即轮询并发聚合全网所有源 (数万节点)</button>
      </div>

      <div style="margin-bottom:16px;border-top:1px solid var(--border);padding-top:14px">
        <h3 style="margin-bottom:6px;font-size:14px">📋 批量导入节点 / IP 文本 (扩充数万节点)</h3>
        <div class="hint" style="margin-bottom:8px">
          支持直接粘贴 <b>VPN Gate CSV 全量文本</b>、多个 <b>.ovpn 配置文本</b>，或<b>每行一个 IP / IP:Port</b>。<br>
          系统将自动提取 IP、测速、定位国家并去重合并入当前节点池：
        </div>
        <textarea id="importNodesText" style="min-height:90px;font-size:11px;font-family:monospace;padding:6px 8px" placeholder="在此粘贴 CSV 内容、.ovpn 块或 IP 列表...&#10;示例 1: 150.40.105.19:1194&#10;示例 2: 126.79.197.198&#10;示例 3: *vpn_servers 格式的 CSV 全文"></textarea>
        <div style="display:flex;gap:8px;margin-top:8px">
          <button class="primary" id="doImportNodes">📥 立即批量导入到节点池</button>
          <span id="importResult" style="font-size:12px;color:var(--ok);align-self:center" hidden></span>
        </div>
      </div>

      <div>
        <h3 style="margin-bottom:8px;font-size:14px">本地自定义 .ovpn 节点</h3>
        <div class="hint" style="margin-bottom:8px">
          您可以把任意商业/私有 VPN 的 <code>.ovpn</code> 配置文件上传到服务器目录：<br>
          <code id="srcCustomDir" style="color:var(--accent)">/var/lib/fanout/custom_nodes</code><br>
          （支持以国家码命名如 <code>US_server1.ovpn</code>、<code>JP_fast.ovpn</code>，扫描后将直接加入节点池，自动识别纯净度与机房/住宅）
        </div>
        <button id="scanLocalOvpn">📁 扫描本地 .ovpn 目录</button>
      </div>
    </div>
    <div class="foot">
      <span class="spacer"></span>
      <button data-close="sourcesModal">关闭</button>
    </div>
  </div>
</div>

<div class="modal" id="submodal">
  <div class="sheet" style="max-width:580px">
    <div class="head">
      <h2>⚡ 全平台聚合订阅</h2>
      <span class="spacer"></span>
      <button class="icon" data-close="submodal" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <div style="font-size:12px;color:var(--dim);margin-bottom:12px">
        支持所有主流客户端导入订阅。节点在服务端自由切换出口后，客户端无需重新导入或刷新，流量实时无缝生效！
      </div>

      <label class="f">
        <span style="font-weight:600;display:flex;align-items:center;gap:6px">
          📱 通用聚合订阅 (Base64)
          <em style="color:var(--dim);font-weight:normal;font-style:normal">V2RayN / Shadowrocket / Sing-box / NekoBox / Loon</em>
        </span>
        <div style="display:flex;gap:6px;margin-top:4px">
          <input type="text" id="subUrlBase64" readonly style="flex:1;background:#12151a;color:var(--accent);font-family:monospace;font-size:11px">
          <button class="primary" id="copySubBase64" style="flex:none">📋 复制</button>
        </div>
      </label>

      <label class="f" style="margin-top:14px">
        <span style="font-weight:600;display:flex;align-items:center;gap:6px">
          🐱 Clash / Mihomo 配置订阅
          <em style="color:var(--dim);font-weight:normal;font-style:normal">Clash Verge / Clash Meta / Mihomo Party / ShellClash</em>
        </span>
        <div style="display:flex;gap:6px;margin-top:4px">
          <input type="text" id="subUrlClash" readonly style="flex:1;background:#12151a;color:#c9903a;font-family:monospace;font-size:11px">
          <button class="primary" id="copySubClash" style="flex:none">📋 复制</button>
          <button id="importClashBtn" style="flex:none">⚡ 一键导入</button>
        </div>
      </label>

      <div style="margin-top:14px;padding:10px;background:rgba(74,158,218,.1);border:1px solid rgba(74,158,218,.3);border-radius:4px;font-size:11px;line-height:1.6">
        💡 <b>智能出口路由特性</b>：<br>
        1. 订阅链接自动聚合当前所有 3x-ui 入站及正在运行的 VPN Gate 出口隧道。<br>
        2. 在主界面随时将节点切换到任意出口，客户端立即生效，实现真正的自由多国跳板。
      </div>
    </div>
    <div class="foot">
      <span class="spacer"></span>
      <button data-close="submodal">完成</button>
    </div>
  </div>
</div>

<div class="toast" id="toast"></div>

<script>
const $ = s => document.querySelector(s);
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

// 界面挂在随机前缀下，请求一律走相对路径
async function api(path, opts){
  const r = await fetch(path.replace(/^\//, ''), opts);
  const d = await r.json().catch(()=>({}));
  if(!r.ok) throw new Error(d.error || ('HTTP '+r.status));
  return d;
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
  EDU: '教育网高校', GLOBAL: '全球公网',
  JP: '日本', KR: '韩国', US: '美国', RU: '俄罗斯', VN: '越南',
  IN: '印度', MV: '马尔代夫', TH: '泰国', HK: '中国香港', TW: '中国台湾',
  SG: '新加坡', GB: '英国', DE: '德国', FR: '法国', CA: '加拿大',
  AU: '澳大利亚', NL: '荷兰', MY: '马来西亚', PH: '菲律宾', ID: '印尼',
  UA: '乌克兰', BR: '巴西', TR: '土耳其', PL: '波兰', SE: '瑞典',
  NO: '挪威', FI: '芬兰', ES: '西班牙', IT: '意大利', CH: '瑞士',
  RO: '罗马尼亚', BG: '保加利亚', CZ: '捷克', HU: '匈牙利', AT: '奥地利',
  CUSTOM: '自定义'
};

function formatCountry(code, name) {
  if (!code || code === 'CUSTOM') return '🌐 ' + (name || '自定义');
  if (code === 'EDU') return '🎓 EDU 教育网高校' + (name && name !== 'EDU' && name !== '教育网高校' ? ' · ' + name : '');
  const flag = getFlagEmoji(code);
  const zh = COUNTRY_ZH[code.toUpperCase()] || '';
  if (zh) {
    return flag + ' ' + code + ' ' + zh + (name && name !== code && name !== zh ? ' · ' + name : '');
  }
  return flag + ' ' + code + (name ? ' · ' + name : '');
}

function renderExits(){
  const list = $('#list');
  const n = view.exits.length;
  $('#ecount').textContent = n ? n + ' 个' : '';
  $('#exportAll').disabled = !view.exits.some(e => e.inbounds && e.inbounds.length);
  $('#stopall').disabled = !n;

  const resCount = view.exits.filter(e => e.ip_type === 'residential').length;
  const purities = view.exits.map(e => e.purity_score || 55);
  const avgPurity = purities.length ? Math.round(purities.reduce((a, b) => a + b, 0) / purities.length) : 0;

  const summaryBar = '<div class="stats-summary">'
    + '<span class="stat-pill">运行出口<b>' + n + '</b></span>'
    + '<span class="stat-pill">🏡 住宅出口<b style="color:#3fa66b">' + resCount + '</b></span>'
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
    const isRes = e.ip_type === 'residential';
    const typeTag = isRes ? '<span class="tag-res" title="家庭宽带住宅 IP">🏡 住宅</span>'
      : (e.ip_type === 'mobile' ? '<span class="tag-mob" title="移动蜂窝网络 IP">📱 移动</span>'
      : '<span class="tag-host" title="数据中心机房 IP">🏢 机房</span>');

    const purity = e.purity_score || 55;
    let pColor = '#c25450';
    if(purity >= 85) pColor = '#3fa66b';
    else if(purity >= 70) pColor = '#4a9eda';
    else if(purity >= 50) pColor = '#c9903a';
    const purityTag = '<span class="tag-purity" style="color:' + pColor + '" title="IP 纯净度评分">' + purity + '%</span>';

    const chips = (e.inbounds || []).length
      ? e.inbounds.map(i =>
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

async function poll(){
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

function renderRegions(){
  const kw = $('#rgfilter').value.trim().toLowerCase();
  const list = regions.filter(r => {
    if(!kw) return true;
    const zh = (COUNTRY_ZH[r.code.toUpperCase()] || '').toLowerCase();
    return r.code.toLowerCase().includes(kw)
      || r.name.toLowerCase().includes(kw)
      || zh.includes(kw);
  });

  if(!regions.length){
    $('#regions').innerHTML = '<div style="padding:14px;background:var(--subtle);border-radius:8px;border:1px dashed var(--border);margin-top:6px">'
      + '<div style="font-weight:600;color:var(--bad);margin-bottom:6px">⚠️ 暂未获取到在线节点列表</div>'
      + '<div style="color:var(--dim);font-size:12px;line-height:1.5;margin-bottom:10px">可能由于网络延迟或官方源受阻，点击下方按钮可立即自动探测筑波大学镜像并获取节点。</div>'
      + '<div style="display:flex;gap:8px;flex-wrap:wrap">'
      + '<button class="primary" id="wzRefreshBtn" type="button" style="font-size:12px;padding:6px 12px">🔄 立即拉取节点源</button>'
      + '<button id="wzSourcesBtn" type="button" style="font-size:12px;padding:6px 12px">🌐 节点源管理</button>'
      + '</div></div>';
    updateAvail();
    return;
  }

  $('#regions').innerHTML = ['<button class="rg' + (region === '' ? ' sel' : '')
      + '" data-rg=""><b>🌐 不限地区</b><em>速度优先</em></button>']
    .concat(list.map(r => {
      const resHint = r.residential ? ' · 🏡 ' + r.residential + ' 住宅' : '';
      const purityHint = r.avg_purity ? ' · ' + r.avg_purity + '% 纯净' : '';
      const title = formatCountry(r.code, r.name);
      return '<button class="rg' + (region === r.code ? ' sel' : '')
        + '" data-rg="' + esc(r.code) + '"><b>' + esc(title) + '</b>'
        + '<em>' + r.available + ' 个空闲' + resHint + purityHint + '</em></button>';
    }))
    .join('');
  updateAvail();
}

function availOf(code){
  if(code === '') return regions.reduce((a, r) => a + r.available, 0);
  const r = regions.find(x => x.code === code);
  return r ? r.available : 0;
}

function updateAvail(){
  if(!regions.length){
    const hint = $('#availhint');
    hint.innerHTML = '<span style="color:var(--bad)">请点击上方「立即拉取节点源」获取可用节点</span>';
    hint.className = 'hint bad';
    $('#go').disabled = true;
    return;
  }
  const avail = availOf(region);
  const want = Number($('#count').value) || 0;
  const hint = $('#availhint');
  hint.textContent = avail ? '可用 ' + avail + ' 个节点' : '这个地区没有空闲节点';
  hint.className = 'hint' + (want > avail ? ' bad' : '');
  if(want > avail && avail) hint.textContent = '只剩 ' + avail + ' 个，将全部使用';
  $('#go').disabled = !avail;
}

async function loadWizard(){
  try{
    regions = await api('/api/regions') || [];
    regionsLoaded = true;
    renderRegions();
  }catch(e){ toast('读取地区失败: ' + e.message, true); }

  const sel = $('#tpl');
  // xray-cf-lite 模式不能复制节点，向导退化成"只开出口"，之后在节点详情里挑出口
  if(isXCL()){
    $('#tplwrap').hidden = true;
    sel.innerHTML = '<option value="0">只开出口，不建节点</option>';
    return;
  }
  $('#tplwrap').hidden = false;
  try{
    // 已经挂在出口上的多半是上一批复制出来的，拿它当模板会套娃，
    // 所以把没绑出口的排在前面并默认选中
    const v = await api('/api/exits');
    const free = v.direct || [];
    const bound = (v.exits || []).flatMap(e => e.inbounds || []);
    inbounds = free.concat(bound);
    if(!inbounds.length){
      sel.innerHTML = '<option value="0">还没有节点</option>';
      $('#tplhint').textContent = '先用上面的「新建节点」建一个，之后这里可以按它批量生成';
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
  }catch(e){
    sel.innerHTML = '<option value="0">' + backendName() + '不可用</option>';
    $('#tplhint').textContent = e.message;
  }
}

document.addEventListener('click', e => {
  if(e.target.closest('#newexit') || e.target.closest('#newexit2')){
    openModal('wizard');
    if(!regionsLoaded) loadWizard(); else { renderRegions(); loadWizard(); }
  }
  const rg = e.target.closest('[data-rg]');
  if(rg){ region = rg.dataset.rg; renderRegions(); }
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
  e.target.disabled = true;
  try{
    const r = await api('/api/panel/inbound/new?' + q.toString(), {method:'POST'});
    toast('已创建 ' + r.protocol + ' 节点，端口 ' + r.port);
    closeModal('newnodebox');
    $('#nport').value = '';
    $('#nremark').value = '';
    poll();
  }catch(err){ toast(err.message, true); }
  e.target.disabled = false;
};

$('#rgfilter').oninput = renderRegions;
$('#minus').onclick = () => { step(-1); };
$('#plus').onclick = () => { step(1); };
function step(d){
  const el = $('#count');
  el.value = Math.min(20, Math.max(1, (Number(el.value) || 1) + d));
  updateAvail();
}
$('#count').oninput = updateAvail;

$('#go').onclick = async e => {
  const want = Math.min(Number($('#count').value) || 1, availOf(region) || 1);
  const tpl = $('#tpl').value || '0';
  e.target.disabled = true;
  try{
    await api('/api/provision?count=' + want + '&region=' + encodeURIComponent(region)
      + '&template=' + tpl, {method:'POST'});
    closeModal('wizard');
    poll();
  }catch(err){ toast(err.message, true); }
  e.target.disabled = false;
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
    if(!confirm('清理所有连接失败或已断开的失效出口？')) return;
    pruneBtn.disabled = true;
    try{
      const res = await api('/api/exits/prune_failed', {method:'POST'});
      toast('已清理 ' + (res.count || 0) + ' 个失效出口');
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
    +   exitOptions(owner ? owner.host : '') + '</select></dd>'
    + '<dt>协议</dt><dd>' + esc(d.protocol) + '　' + esc(d.network || '')
    +   (d.tls && d.tls !== 'none' ? '　' + esc(d.tls) : '') + '</dd>'
    + '<dt>监听</dt><dd>' + esc(d.listen || '0.0.0.0') + '</dd>'
    + '</dl>'
    + (editable ? ('<div class="editbar">'
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

async function openSubModal(){
  const host = location.hostname;
  const port = location.port ? ':' + location.port : '';
  const proto = location.protocol;
  let token = '';
  try {
    const tokenRes = await api('/api/cred/token');
    if (tokenRes && tokenRes.token) token = tokenRes.token;
  } catch(e) {}

  const tokenQuery = token ? '?token=' + encodeURIComponent(token) : '';
  const subBase = proto + '//' + host + port + '/sub' + tokenQuery;
  const clashSep = tokenQuery ? '&format=clash' : '?format=clash';
  const subClash = proto + '//' + host + port + '/sub' + tokenQuery + clashSep;

  $('#subUrlBase64').value = subBase;
  $('#subUrlClash').value = subClash;
  $('#copySubBase64').onclick = () => copy(subBase);
  $('#copySubClash').onclick = () => copy(subClash);
  $('#importClashBtn').onclick = () => {
    window.location.href = 'clash://install-config?url=' + encodeURIComponent(subClash) + '&name=fanout';
  };
  openModal('submodal');
}

document.addEventListener('click', async e => {
  if(e.target.closest('#subBtn')){
    openSubModal();
    return;
  }

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
  const ids = view.exits.flatMap(x => (x.inbounds || []).map(i => i.id));
  if(!ids.length){ toast('还没有节点可导出', true); return; }
  $('#exbox').value = '读取中…';
  $('#excount').textContent = '';
  openModal('export');
  try{
    const d = await api('/api/xui/links?ids=' + ids.join(','));
    $('#exbox').value = (d.links || []).join('\n');
    $('#excount').textContent = (d.links || []).length + ' 条';
  }catch(err){ $('#exbox').value = '导出失败: ' + err.message; }
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
    sel.innerHTML = '<option value="">读取失败</option>';
    hint.textContent = err.message;
  }
}

$('#settingsBtn').onclick = async () => {
  $('#setPw').value = '';
  $('#setPath').value = '';
  $('#setPathHint').textContent = '读取中…';
  openModal('settings');
  loadBackendModes();
  try{
    const s = await api('/api/settings');
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
  }catch(err){ $('#setPathHint').textContent = '读取失败: ' + err.message; }
};

// 检查更新：问后端 GitHub 最新版，有新版就亮出更新按钮和更新内容
$('#updCheck').onclick = async e => {
  e.target.disabled = true;
  e.target.textContent = '检查中…';
  try{
    const u = await api('/api/update/check');
    $('#updCur').textContent = u.current || '-';
    if(u.has_update){
      $('#updLatest').textContent = '有新版本 ' + u.latest;
      $('#updApplyVer').textContent = u.latest;
      $('#updApply').hidden = false;
      $('#updNotes').textContent = u.notes || '（这个版本没写更新说明）';
      $('#updNotes').hidden = false;
    } else {
      $('#updLatest').textContent = '已是最新';
      $('#updApply').hidden = true;
      $('#updNotes').hidden = true;
    }
  }catch(err){ toast(err.message, true); }
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

async function refreshSources(btn){
  if(btn) btn.disabled = true;
  toast('正在轮询探测筑波大学镜像及官方源...');
  try{
    const res = await api('/api/sources/refresh', {method:'POST'});
    toast('拉取成功！已获取 ' + (res.count || 0) + ' 个可用节点');
    await loadSources();
    regions = await api('/api/regions') || [];
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
// ---- 全平台聚合订阅 ----
async function openSubModal(){
  openModal('submodal');
  let token = '';
  try{
    const cred = await api('/api/cred/token');
    if(cred && cred.token) token = cred.token;
  }catch(e){}

  const origin = window.location.origin;
  const tokenQuery = token ? '?token=' + encodeURIComponent(token) : '';
  const tokenQueryClash = token ? '&token=' + encodeURIComponent(token) : '';
  const base64Url = origin + '/sub' + tokenQuery;
  const clashUrl = origin + '/sub?format=clash' + tokenQueryClash;

  const bEl = $('#subUrlBase64');
  if(bEl) bEl.value = base64Url;
  const cEl = $('#subUrlClash');
  if(cEl) cEl.value = clashUrl;
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
  if(e.target.closest('#importClashBtn')){
    const val = ($('#subUrlClash').value || '').trim();
    if(val){
      window.location.href = 'clash://install-config?url=' + encodeURIComponent(val) + '&name=FanoutGateway';
    }
    return;
  }
  if(e.target.closest('#sourcesBtn') || e.target.closest('#wzSourcesBtn')){
    openModal('sourcesModal');
    loadSources();
    return;
  }
  if(e.target.closest('#wzRefreshBtn') || e.target.closest('#refreshMirrors')){
    const btn = e.target.closest('#wzRefreshBtn') || e.target.closest('#refreshMirrors');
    await refreshSources(btn);
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

poll();
setInterval(poll, 3000);
</script>
</body>
</html>`
