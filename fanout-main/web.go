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
  --bg:#a9aeb7;
  --panel:rgba(255, 255, 255, 0.52);
  --line:rgba(255, 255, 255, 0.70);
  --text:#0f172a;
  --dim:#475569;
  --accent:#0284c7;
  --accent-glow:0 0 20px rgba(2, 132, 199, 0.25);
  --ok:#059669;
  --warn:#d97706;
  --bad:#dc2626;
  --glass-shadow:0 16px 40px rgba(0, 0, 0, 0.07), 0 1px 3px rgba(0, 0, 0, 0.04), inset 0 1px 2px rgba(255, 255, 255, 0.95), inset 0 -1px 2px rgba(255, 255, 255, 0.35);
  --glass-blur:blur(42px) saturate(210%) brightness(110%);
  --apple-font:-apple-system, BlinkMacSystemFont, "SF Pro Display", "SF Pro Text", "Helvetica Neue", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
  --mono-font:ui-monospace, "SF Mono", Menlo, Consolas, monospace;
}
*{box-sizing:border-box}
body{
  margin:0;color:var(--text);font:13px/1.55 var(--apple-font);-webkit-font-smoothing:antialiased;min-height:100vh;
  background:#a9aeb7;position:relative;overflow-x:hidden;
}

/* 2026 Apple Liquid Aura: 5 个高饱和能量流体球在画面中自主多轨运动，同色系内的多种色彩互相流动交融 */
.fluid-aura-container{
  position:fixed;inset:0;width:100vw;height:100vh;overflow:hidden;pointer-events:none;z-index:0;
  background:#a9aeb7;filter:blur(75px);-webkit-filter:blur(75px);transform:translateZ(0);
}
.aura-blob{
  position:absolute;border-radius:45% 55% 65% 35% / 40% 50% 60% 50%;
  opacity:0.92;will-change:transform, background;
}
.blob-1{
  width:75vw;height:75vw;top:-15%;left:-10%;
  animation:fluidOrbit1 22s ease-in-out infinite alternate, colorCycleBlob1 48s ease-in-out infinite;
}
.blob-2{
  width:70vw;height:70vw;bottom:-15%;right:-10%;
  animation:fluidOrbit2 26s ease-in-out infinite alternate, colorCycleBlob2 48s ease-in-out infinite;
}
.blob-3{
  width:65vw;height:65vw;top:20%;left:20%;
  animation:fluidOrbit3 18s ease-in-out infinite alternate, colorCycleBlob3 48s ease-in-out infinite;
}
.blob-4{
  width:60vw;height:60vw;bottom:10%;left:-5%;
  animation:fluidOrbit4 24s ease-in-out infinite alternate, colorCycleBlob4 48s ease-in-out infinite;
}
.blob-5{
  width:55vw;height:55vw;top:10%;right:-5%;
  animation:fluidOrbit5 20s ease-in-out infinite alternate, colorCycleBlob5 48s ease-in-out infinite;
}

@keyframes fluidOrbit1{
  0%{transform:translate(-5%, -10%) rotate(0deg) scale(1);}
  33%{transform:translate(30%, 15%) rotate(120deg) scale(1.15);}
  66%{transform:translate(15%, 35%) rotate(240deg) scale(0.92);}
  100%{transform:translate(-15%, 20%) rotate(360deg) scale(1.08);}
}
@keyframes fluidOrbit2{
  0%{transform:translate(10%, 15%) rotate(0deg) scale(1.1);}
  33%{transform:translate(-25%, -10%) rotate(-120deg) scale(0.95);}
  66%{transform:translate(-10%, -30%) rotate(-240deg) scale(1.18);}
  100%{transform:translate(25%, -15%) rotate(-360deg) scale(1);}
}
@keyframes fluidOrbit3{
  0%{transform:translate(0%, 0%) scale(0.95) rotate(0deg);}
  50%{transform:translate(-20%, 25%) scale(1.22) rotate(180deg);}
  100%{transform:translate(25%, -15%) scale(1.05) rotate(360deg);}
}
@keyframes fluidOrbit4{
  0%{transform:translate(15%, -15%) scale(1.05) rotate(0deg);}
  50%{transform:translate(-20%, 20%) scale(1.2) rotate(180deg);}
  100%{transform:translate(20%, 5%) scale(0.92) rotate(360deg);}
}
@keyframes fluidOrbit5{
  0%{transform:translate(-10%, 10%) scale(1);}
  50%{transform:translate(15%, -20%) scale(1.25);}
  100%{transform:translate(-5%, 15%) scale(1.05);}
}

/* 6 大色系按顺序推进，球体运动带动内部各颜色真实流动交融 */
@keyframes colorCycleBlob1{
  0%, 100%{background:#67e8f9;}
  16.66%{background:#7dd3fc;}
  33.33%{background:#bef264;}
  50.00%{background:#0ea5e9;}
  66.66%{background:#6ee7b7;}
  83.33%{background:#a5f3fc;}
}
@keyframes colorCycleBlob2{
  0%, 100%{background:#f472b6;}
  16.66%{background:#ec2159;}
  33.33%{background:#ed1d73;}
  50.00%{background:#833b60;}
  66.66%{background:#ec2c68;}
  83.33%{background:#fb7185;}
}
@keyframes colorCycleBlob3{
  0%, 100%{background:#ffffff;}
  16.66%{background:#5c081e;}
  33.33%{background:#ffffff;}
  50.00%{background:#4a044e;}
  66.66%{background:#ffffff;}
  83.33%{background:#90949d;}
}
@keyframes colorCycleBlob4{
  0%, 100%{background:#fde047;}
  16.66%{background:#f97316;}
  33.33%{background:#701a75;}
  50.00%{background:#fbbf24;}
  66.66%{background:#fef08a;}
  83.33%{background:#fed7aa;}
}
@keyframes colorCycleBlob5{
  0%, 100%{background:#e879f9;}
  16.66%{background:#fff7ed;}
  33.33%{background:#f43f5e;}
  50.00%{background:#ffffff;}
  66.66%{background:#34d399;}
  83.33%{background:#c4b5fd;}
}

/* 磨砂噪点质感 */
.noise-overlay{
  position:fixed;inset:0;width:100%;height:100%;pointer-events:none;z-index:2;opacity:0.085;
  mix-blend-mode:overlay;
  background-image:url("data:image/svg+xml,%3Csvg viewBox='0 0 400 400' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noiseFilter'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noiseFilter)'/%3E%3C/svg%3E");
  background-repeat:repeat;
}

header{display:flex;align-items:center;gap:14px;padding:12px 20px;
  position:sticky;top:0;z-index:40;
  border-bottom:1px solid rgba(255, 255, 255, 0.75);background:rgba(255, 255, 255, 0.60);
  backdrop-filter:var(--glass-blur);-webkit-backdrop-filter:var(--glass-blur);
  box-shadow:0 4px 20px rgba(0,0,0,0.04)}
.brand-wrap{display:flex;align-items:center;gap:8px}
h1{font-size:16px;font-weight:700;margin:0;letter-spacing:-0.3px;
  background:linear-gradient(135deg, #0f172a 0%, #0369a1 60%, #0d9488 100%);
  -webkit-background-clip:text;-webkit-text-fill-color:transparent}
.badge-jesee{font-size:11px;font-weight:600;padding:2px 8px;border-radius:9999px;
  background:rgba(2, 132, 199, 0.12);color:#0284c7;border:1px solid rgba(2, 132, 199, 0.3);
  box-shadow:0 2px 6px rgba(2, 132, 199, 0.1);letter-spacing:0.3px}
.author-banner{display:flex;align-items:center;gap:6px;font-size:11px;color:var(--dim);
  padding:3px 10px;border-radius:9999px;background:rgba(255,255,255,0.55);
  border:1px solid rgba(255,255,255,0.85);backdrop-filter:blur(10px)}
.author-banner a{color:var(--text);text-decoration:none;font-weight:600}
.author-banner a:hover{color:var(--accent)}
.dot-sep{opacity:0.3}
.spacer{flex:1}
button{font:inherit;color:var(--text);background:rgba(255, 255, 255, 0.65);
  border:1px solid rgba(255, 255, 255, 0.95);backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);
  border-radius:9999px;padding:5px 12px;cursor:pointer;display:inline-flex;
  align-items:center;gap:6px;white-space:nowrap;touch-action:manipulation;
  box-shadow:0 2px 8px rgba(0,0,0,0.05), inset 0 1px 1px #fff;
  transition:all .18s cubic-bezier(0.16, 1, 0.3, 1)}
button, a, input, select, textarea, [data-rg], [data-close], [data-detail], [data-cred],
[data-stop], [data-swap], [data-job], [data-del], [data-delone], [data-delclient],
[data-resetclient], [data-togglejobfailed], [data-cleanjobfailed], .chip, .rg, .step, .btn-xs {
  cursor:pointer;-webkit-tap-highlight-color:transparent;touch-action:manipulation}
button:hover:not(:disabled){background:#ffffff;border-color:rgba(2, 132, 199, 0.4);
  color:var(--accent);transform:translateY(-1px);box-shadow:0 4px 16px rgba(0,0,0,0.08)}
button:active:not(:disabled){transform:translateY(0);background:rgba(255,255,255,0.85)}
button:disabled{opacity:.4;cursor:default}
button.primary{background:linear-gradient(135deg, #0284c7 0%, #2563eb 100%);
  border-color:rgba(255,255,255,0.4);color:#ffffff;font-weight:600;
  box-shadow:0 4px 16px rgba(2, 132, 199, 0.35), inset 0 1px 1px rgba(255,255,255,0.4)}
button.primary:hover:not(:disabled){background:linear-gradient(135deg, #0ea5e9 0%, #3b82f6 100%);
  box-shadow:0 6px 22px rgba(2, 132, 199, 0.48);border-color:rgba(255,255,255,0.6);color:#fff}
button.icon{padding:5px 8px;background:rgba(255,255,255,0.45);border-color:rgba(255,255,255,0.7);color:var(--dim)}
button.icon:hover:not(:disabled){color:var(--text);background:#ffffff;border-color:rgba(255,255,255,0.95)}
button.icon.danger:hover:not(:disabled){color:var(--bad);background:rgba(220,38,38,0.1);border-color:rgba(220,38,38,0.3)}
svg{width:14px;height:14px;stroke:currentColor;fill:none;stroke-width:1.8;
  stroke-linecap:round;stroke-linejoin:round;flex:none}
main{position:relative;z-index:10;padding:18px 20px 48px;max-width:1200px;margin:0 auto}
.bar{display:flex;align-items:center;gap:12px;margin-bottom:14px}
.bar h2{font-size:13px;margin:0;font-weight:700;color:var(--text);letter-spacing:0.2px}
.exit{border:1px solid rgba(255, 255, 255, 0.75);border-top:1.5px solid rgba(255, 255, 255, 0.98);
  border-radius:16px;margin-bottom:10px;background:rgba(255, 255, 255, 0.48);
  backdrop-filter:var(--glass-blur);-webkit-backdrop-filter:var(--glass-blur);
  box-shadow:var(--glass-shadow);overflow:hidden;
  transition:all .2s cubic-bezier(0.16, 1, 0.3, 1)}
.exit:hover{transform:translateY(-2px);background:rgba(255, 255, 255, 0.70);border-color:rgba(2, 132, 199, 0.4);
  box-shadow:0 20px 48px rgba(0, 0, 0, 0.09), inset 0 1px 2px #fff}
.exit>.row{display:grid;gap:8px 14px;align-items:center;padding:11px 16px;
  grid-template-columns:14px auto 1fr auto auto auto;
  grid-template-areas:"dot ip meta chips socks acts"}
.exit .dot{grid-area:dot}
.exit .ip{grid-area:ip;display:flex;align-items:center;flex-wrap:wrap;gap:6px}
.exit .meta{grid-area:meta}
.exit .chips{grid-area:chips}
.exit .socks{grid-area:socks}
.exit .acts{grid-area:acts}
.tag-res{background:rgba(5, 150, 105, 0.12);color:#059669;border:1px solid rgba(5, 150, 105, 0.3);
  border-radius:9999px;padding:2px 7px;font-size:11px;font-weight:600}
.tag-host{background:rgba(2, 132, 199, 0.12);color:#0284c7;border:1px solid rgba(2, 132, 199, 0.3);
  border-radius:9999px;padding:2px 7px;font-size:11px;font-weight:600}
.tag-purity{font-size:11px;font-weight:600;padding:2px 7px;border-radius:9999px;
  background:rgba(255,255,255,0.6);border:1px solid rgba(255,255,255,0.9);color:var(--dim)}
.tag-gov{background:rgba(217, 119, 6, 0.12);color:#d97706;border:1px solid rgba(217, 119, 6, 0.3);
  border-radius:9999px;padding:2px 7px;font-size:11px;font-weight:600}
.tag-edu{background:rgba(147, 51, 234, 0.12);color:#9333ea;border:1px solid rgba(147, 51, 234, 0.3);
  border-radius:9999px;padding:2px 7px;font-size:11px;font-weight:600}
.stats-summary{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:14px}
.stat-pill{background:rgba(255,255,255,0.55);border:1px solid rgba(255,255,255,0.85);
  border-radius:9999px;padding:4px 12px;font-size:12px;color:var(--dim);
  backdrop-filter:blur(10px);box-shadow:0 2px 8px rgba(0,0,0,0.04)}
.stat-pill b{color:var(--text);margin-left:5px}
.dot{width:9px;height:9px;border-radius:50%;background:var(--dim);justify-self:center;box-shadow:0 0 6px currentColor}
.dot.up{background:var(--ok);color:var(--ok);box-shadow:0 0 10px rgba(5,150,105,0.6)}
.dot.starting{background:var(--warn);color:var(--warn);box-shadow:0 0 10px rgba(217,119,6,0.6);animation:pulse 1.2s ease-in-out infinite}
.dot.failed{background:var(--bad);color:var(--bad);box-shadow:0 0 10px rgba(220,38,38,0.6)}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.3}}
.ip{font-weight:600;font-variant-numeric:tabular-nums;font-family:var(--mono-font);letter-spacing:-0.2px}
.meta{color:var(--dim);font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.chips{display:flex;gap:6px;flex-wrap:wrap}
.chip{border:1px solid rgba(255,255,255,0.85);border-radius:9999px;padding:2px 8px;font-size:11px;
  color:var(--dim);cursor:pointer;background:rgba(255,255,255,0.55);backdrop-filter:blur(6px);
  box-shadow:0 1px 4px rgba(0,0,0,0.03);transition:all .15s ease}
.chip:hover{border-color:var(--accent);color:var(--accent);background:#ffffff}
.chip-item{display:inline-flex;align-items:center;background:rgba(255,255,255,0.55);
  border:1px solid rgba(255,255,255,0.85);border-radius:9999px;overflow:hidden}
.chip-item .chip{border:none;border-radius:0;background:transparent;padding:2px 7px}
.chip-item .chip:hover{background:rgba(255,255,255,0.3)}
.chip-item .chip-select{border:none;border-left:1px solid rgba(255,255,255,0.5);background:transparent;
  color:var(--dim);font-size:11px;padding:2px 6px;cursor:pointer;outline:none}
.chip-item .chip-select:hover{color:var(--text);background:rgba(255,255,255,0.3)}
.chip.none{border-style:dashed;cursor:default}
.chip.none:hover{border-color:var(--line);color:var(--dim)}
.orphan{margin-top:20px;border:1px solid rgba(255,255,255,0.75);border-top:1.5px solid #fff;border-radius:14px;
  background:rgba(255, 255, 255, 0.50);padding:12px 16px;backdrop-filter:var(--glass-blur);box-shadow:0 8px 24px rgba(0,0,0,0.05)}
.orphan .top{display:flex;align-items:center;gap:10px;margin-bottom:8px}
.orphan .top h3{font-size:12px;margin:0;font-weight:700;color:var(--text)}
.socks{color:var(--dim);font-size:12px;font-variant-numeric:tabular-nums;font-family:var(--mono-font)}
.socks button{background:transparent;border-color:transparent;color:var(--dim);
  font-size:12px;padding:3px 8px;font-variant-numeric:tabular-nums;box-shadow:none}
.socks button:hover:not(:disabled){color:var(--text);background:rgba(255,255,255,0.6);border-color:rgba(255,255,255,0.85)}
.socks button .lock{width:11px;height:11px;stroke-width:2}
.acts{display:flex;gap:4px;justify-self:end}
.errline{padding:0 16px 10px 42px;color:var(--bad);font-size:11px;
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.empty{border:1px dashed rgba(255,255,255,0.8);border-radius:14px;padding:48px 24px;
  text-align:center;color:var(--dim);background:rgba(255,255,255,0.35)}
.empty button{margin-top:16px}
.jobs{margin-bottom:14px}
.job{border:1px solid rgba(255,255,255,0.75);border-top:1.5px solid #fff;border-radius:12px;background:rgba(255, 255, 255, 0.50);
  padding:12px 14px;margin-bottom:10px;backdrop-filter:var(--glass-blur);box-shadow:0 8px 24px rgba(0,0,0,0.05)}
.job .top{display:flex;align-items:center;gap:10px;margin-bottom:8px}
.job .top strong{font-weight:700;font-size:12px;color:var(--text)}
.steps{display:flex;flex-wrap:wrap;gap:6px}
.step{display:flex;align-items:center;gap:5px;font-size:11px;color:var(--dim);
  border:1px solid rgba(255,255,255,0.85);border-radius:9999px;padding:3px 8px;background:rgba(255,255,255,0.55)}
.step.ok{color:var(--ok);border-color:rgba(5,150,105,.4);background:rgba(5,150,105,.12)}
.step.failed{color:var(--bad);border-color:rgba(220,38,38,.4);background:rgba(220,38,38,.12)}
.step.running{color:var(--warn);border-color:rgba(217,119,6,.4);background:rgba(217,119,6,.12)}
.spin{animation:rot 1s linear infinite;transform-origin:center}
@keyframes rot{to{transform:rotate(360deg)}}
.jobs-bar{display:flex;align-items:center;gap:10px;margin-bottom:8px;font-size:12px;color:var(--dim)}
.btn-xs{font-size:11px;padding:3px 9px;border-radius:9999px;background:rgba(255,255,255,0.55);
  border:1px solid rgba(255,255,255,0.85);color:var(--dim);cursor:pointer;display:inline-flex;align-items:center;gap:4px}
.btn-xs:hover{color:var(--text);border-color:rgba(2,132,199,0.4);background:#ffffff}
.btn-xs.danger{color:var(--bad)}
.btn-xs.danger:hover{border-color:var(--bad);background:rgba(220,38,38,0.15)}
.step.failed-summary{cursor:pointer;border-style:dashed}
.step.failed-summary:hover{background:rgba(220,38,38,.15)}
.failed-steps-wrap{width:100%;margin-top:8px;padding-top:8px;border-top:1px dashed rgba(255,255,255,0.6);display:none}
.failed-steps-wrap.open{display:flex;flex-wrap:wrap;gap:6px}
.links{display:flex;gap:14px;margin-right:4px}
.links a{color:var(--dim);text-decoration:none;font-size:12px;transition:color .15s}
.links a:hover{color:var(--accent)}
@media(max-width:860px){.links, .author-banner{display:none}
  main{padding:12px 14px 40px}
  .exit>.row{grid-template-columns:14px 1fr auto;
    grid-template-areas:"dot ip acts" ". meta meta" ". socks socks" ". chips chips"}
  .exit .chips{margin-top:2px}
  .bar{flex-wrap:wrap}}
.modal{position:fixed;inset:0;background:rgba(15, 23, 42, 0.25);
  backdrop-filter:blur(20px);-webkit-backdrop-filter:blur(20px);
  display:none;align-items:center;justify-content:center;z-index:50;padding:20px}
.modal.open{display:flex}
.sheet{background:rgba(255, 255, 255, 0.85);
  backdrop-filter:blur(48px) saturate(220%);-webkit-backdrop-filter:blur(48px) saturate(220%);
  border:1px solid rgba(255, 255, 255, 0.95);border-top:1.5px solid #ffffff;
  border-radius:24px;width:min(700px,100%);max-height:86vh;display:flex;flex-direction:column;
  box-shadow:0 32px 80px rgba(0,0,0,0.15), inset 0 1px 2px #fff;
  animation:popIn .2s cubic-bezier(0.16, 1, 0.3, 1)}
@keyframes popIn{from{opacity:0;transform:scale(0.96) translateY(8px)}to{opacity:1;transform:scale(1) translateY(0)}}
.sheet .head{display:flex;align-items:center;gap:10px;padding:14px 18px;
  border-bottom:1px solid rgba(255,255,255,0.60);background:rgba(255,255,255,0.45);border-radius:24px 24px 0 0}
.sheet .head h2{font-size:13px;margin:0;font-weight:700;color:var(--text)}
.sheet .body{overflow:auto;padding:16px 18px}
.sheet .foot{display:flex;align-items:center;gap:10px;padding:12px 18px;
  border-top:1px solid rgba(255,255,255,0.60);background:rgba(255,255,255,0.45);border-radius:0 0 24px 24px}
.count{color:var(--dim);font-size:11px}
label.f{display:block;margin-bottom:16px}
label.f[hidden]{display:none}
label.f>span{display:block;color:var(--dim);font-size:11px;margin-bottom:6px}
.regions{display:grid;grid-template-columns:repeat(auto-fill,minmax(148px,1fr));
  gap:8px;max-height:230px;overflow:auto}
.rg{border:1px solid rgba(255,255,255,0.85);background:rgba(255,255,255,0.55);border-radius:10px;padding:8px 10px;
  cursor:pointer;text-align:left;display:block;width:100%;transition:all .15s ease}
.rg:hover{border-color:var(--accent);background:#ffffff}
.rg.sel{border-color:var(--accent);background:rgba(2,132,199,0.12);box-shadow:0 0 12px rgba(2,132,199,0.2)}
.rg b{font-weight:600;font-size:12px;display:block;overflow:hidden;
  text-overflow:ellipsis;white-space:nowrap;color:var(--text)}
.rg em{display:block;font-style:normal;color:var(--dim);font-size:11px;margin-top:3px}
.stepper{display:flex;align-items:center;gap:0;width:fit-content;
  border:1px solid rgba(255,255,255,0.85);border-radius:9999px;overflow:hidden;background:rgba(255,255,255,0.55)}
.stepper button{border:0;border-radius:0;background:transparent;padding:6px 12px;box-shadow:none}
select,input[type=search],input[type=text],input[type=password],textarea{font:inherit;background:rgba(255, 255, 255, 0.65);
  border:1px solid rgba(255, 255, 255, 0.90);color:var(--text);border-radius:12px;
  padding:7px 10px;backdrop-filter:blur(10px);box-shadow:inset 0 1px 2px rgba(0,0,0,0.04);transition:all .15s}
select:focus,input[type=search]:focus,input[type=text]:focus,input[type=password]:focus,textarea:focus{
  outline:none;border-color:var(--accent);background:#ffffff;box-shadow:0 0 0 3px rgba(2, 132, 199, 0.25)}
select{cursor:pointer}
.grid-2{display:grid;grid-template-columns:1fr 1fr;gap:12px}
.hint{color:var(--dim);font-size:11px;margin-top:4px}
.field-error{color:var(--bad);font-size:11px;margin-top:4px;display:none}
.chead{display:flex;align-items:center;justify-content:space-between;margin:16px 0 8px}
.chead h3{font-size:12px;margin:0;font-weight:700;color:var(--text)}
.client{border:1px solid rgba(255,255,255,0.85);border-radius:10px;padding:10px 12px;margin-bottom:8px;background:rgba(255,255,255,0.55)}
.orow{display:flex;align-items:center;gap:10px;padding:6px 0}
.orow select{width:200px}
.crow{display:flex;align-items:center;gap:10px}
.cemail{font-weight:600;font-size:12px;color:var(--text)}
.cid{color:var(--dim);font-size:11px;overflow:hidden;text-overflow:ellipsis;
  white-space:nowrap;max-width:280px}
.client .share{margin:8px 0 0}
.share button{margin-top:8px}
textarea{width:100%;min-height:300px;background:rgba(255,255,255,0.65);border:1px solid rgba(255,255,255,0.90);
  color:var(--text);border-radius:10px;
  font:12px/1.8 var(--mono-font);
  padding:10px 12px;resize:vertical}
textarea:focus{outline:none;border-color:var(--accent)}
.toast{position:fixed;left:50%;bottom:26px;transform:translateX(-50%);
  background:rgba(255,255,255,0.85);border:1px solid rgba(255,255,255,0.95);border-top:1.5px solid #fff;border-radius:9999px;
  color:var(--text);backdrop-filter:blur(24px);-webkit-backdrop-filter:blur(24px);box-shadow:0 12px 32px rgba(0,0,0,0.12), inset 0 1px 1px #fff;
  padding:9px 18px;font-size:12px;font-weight:600;z-index:80;opacity:0;pointer-events:none;
  transition:all .2s cubic-bezier(0.16, 1, 0.3, 1)}
.toast.show{opacity:1;transform:translateX(-50%) translateY(-4px)}
.toast.bad{border-color:rgba(220,38,38,.5);color:var(--bad)}
</style>
</head>
<body>
<div class="fluid-aura-container" aria-hidden="true">
  <div class="aura-blob blob-1"></div>
  <div class="aura-blob blob-2"></div>
  <div class="aura-blob blob-3"></div>
  <div class="aura-blob blob-4"></div>
  <div class="aura-blob blob-5"></div>
</div>
<div class="noise-overlay" aria-hidden="true"></div>
<header>
  <div class="brand-wrap">
    <h1>fanout</h1>
    <span class="badge-jesee">Jesee 魔改旗舰版</span>
  </div>
  <span class="count" id="panel"></span>
  <span class="spacer"></span>
  <div class="author-banner">
    <span>原作者: <a href="https://github.com/byJoey/fanout" target="_blank" rel="noopener">Joey</a></span>
    <span class="dot-sep">·</span>
    <span style="color:#70b8ff">魔改升级: Jesee</span>
  </div>
  <button class="icon" id="settingsBtn" title="设置">
    <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
  </button>
  <nav class="links">
    <a href="https://t.me/+ft-zI76oovgwNmRh" target="_blank" rel="noopener">交流群</a>
    <a href="https://youtube.com/@joeyblog" target="_blank" rel="noopener">Joey油管</a>
    <a href="https://joeyblog.net" target="_blank" rel="noopener">Joey博客</a>
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
    <button class="primary" id="scanNodesBtn" title="母机真实发包测活：验证能连通能出网才展示，可挑选单个或批量启动" style="background:linear-gradient(135deg,#1b3b6f,#212d40);border-color:#4a9eda;color:#5eb3ec;font-weight:600">
      <svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><path d="M11 8v6M8 11h6"/></svg>
      测活扫节点
    </button>
    <button class="primary" id="autoOrchestrateBtn" title="全网智能编排：热门国家各维持 3 个健康出口，冷门国家各维持 1 个，自动发现、失效同国轮换与自愈" style="background:linear-gradient(135deg,#6366f1,#8b5cf6);border-color:#818cf8;color:#fff;font-weight:600">
      <svg viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
      智能编排
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
        <div style="display:flex;justify-content:space-between;align-items:center">
          <span>节点来源 / 节点池</span>
          <span style="font-size:12px;color:var(--dim)">自由选择单独源或全部聚合</span>
        </div>
        <select id="wzSource" style="padding:8px 10px;border-radius:6px;border:1px solid var(--line);background:var(--card);color:var(--text);font-size:13px;width:100%">
          <option value="all">🌐 官方与镜像优质节点 (全量家宽/高校/政府 · 自动优选)</option>
          <option value="vpngate">🇯🇵 日本筑波大学 (官方与高防镜像源)</option>
          <option value="gov">🏛️ 全球政府公共机构网 (政府自治体/公共政务专网)</option>
          <option value="edu">🎓 海外高校学术科研网 (日本筑波/韩国/台湾/欧美名校)</option>
          <option value="residential">🏡 住宅家宽原生节点 (纯净高分 · 极速防封)</option>
          <option value="custom">📁 本地导入与自定义节点 (.ovpn / 自建)</option>
        </select>
      </label>
      <label class="f">
        <span>地区 / 国家</span>
        <input type="search" id="rgfilter" placeholder="搜索或筛选国家/地区，如 日本、JP、美国、海外学术...">
        <div class="regions" id="regions" style="margin-top:6px"></div>
      </label>
      <div id="wzCandidateSection" style="margin-bottom:14px">
        <div style="display:flex;justify-content:space-between;align-items:center;cursor:pointer;padding:8px 12px;background:var(--subtle);border-radius:6px;border:1px solid var(--border)" id="toggleWzCandidates">
          <span style="font-size:12px;font-weight:600;display:flex;align-items:center;gap:6px">
            <span id="wzCandArrow">▶</span> 展开候选节点详情 (<span id="wzCandCount">0</span> 个候选 · 优劣推荐 / 自由选择启动)
          </span>
          <span style="font-size:11px;color:var(--accent)" id="wzCandStatus">点击展开</span>
        </div>
        <div id="wzCandidatesWrap" style="display:none;margin-top:8px;border:1px solid var(--border);border-radius:6px;padding:10px;background:var(--card-bg)">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;font-size:12px;color:var(--dim)">
            <span>已选 <b id="wzSelCount" style="color:var(--primary)">0</b> 个节点</span>
            <div style="display:flex;gap:6px">
              <button type="button" id="wzSelectTop3" style="font-size:11px;padding:2px 8px">⚡ 优选前 3 个</button>
              <button type="button" id="wzSelectTop10" style="font-size:11px;padding:2px 8px">⚡ 优选前 10 个</button>
              <button type="button" id="wzSelectAll" style="font-size:11px;padding:2px 8px">全选</button>
              <button type="button" id="wzClearSel" style="font-size:11px;padding:2px 8px">清空</button>
            </div>
          </div>
          <div id="wzCandidatesList" style="max-height:220px;overflow-y:auto;display:flex;flex-direction:column;gap:4px"></div>
        </div>
      </div>
      <label class="f">
        <div style="display:flex;justify-content:space-between;align-items:center">
          <span>出口数量 (未勾选具体节点时自动开辟)</span>
          <span style="font-size:12px;color:var(--primary);font-weight:600">推荐 3 个 (最佳性能与稳定性)</span>
        </div>
        <div class="stepper">
          <button id="minus" type="button" title="减少">
            <svg viewBox="0 0 24 24"><path d="M5 12h14"/></svg>
          </button>
          <input id="count" type="number" min="1" max="100" step="1" value="3" placeholder="3" style="text-align:center;font-weight:700;font-size:15px">
          <button id="plus" type="button" title="增加">
            <svg viewBox="0 0 24 24"><path d="M12 5v14"/><path d="M5 12h14"/></svg>
          </button>
        </div>
        <div class="hint" style="color:var(--dim);font-size:12px;margin-top:4px">
          ⚙️ 默认推荐 <b>3</b> 个出口 · 允许范围: <b>1 ~ 100</b> 个 (自选节点不限数量)
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
        <div style="margin-top:12px;padding:12px 14px;background:rgba(255,255,255,0.04);border:1px solid rgba(255,255,255,0.1);border-radius:12px;font-size:12px;color:var(--dim);line-height:1.6">
          <div style="font-weight:600;color:var(--text);margin-bottom:4px">🌟 项目致谢与开发信息</div>
          <div>原版架构与作者：<b>Joey</b>（<a href="https://youtube.com/@joeyblog" target="_blank" rel="noopener" style="color:var(--accent)">@joeyblog</a> / <a href="https://github.com/byJoey/fanout" target="_blank" rel="noopener" style="color:var(--accent)">GitHub</a>）</div>
          <div>深度魔改升级版：<b style="color:#70b8ff">Jesee</b>（独家：10秒同国自愈轮换、全网智能编排、纯净住宅/政府专网原生支持、苹果液态玻璃 UI）</div>
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
        <h3 style="margin-bottom:8px;font-size:14px">优质节点池（日本筑波大学官方/镜像 + 海外高校学术科研网 + 住宅家宽原生）</h3>
        <div style="font-size:12px;color:var(--dim);line-height:1.6;background:var(--card-bg);padding:10px;border-radius:6px;border:1px solid var(--border);margin-bottom:8px">
          <div>• <b>日本筑波大学官方及全量高防镜像池</b>：150.40.105.19 / 119.195.163.98 等多镜像并发聚合</div>
          <div>• <b>海外高校学术科研网专项</b>：日本 (SINET/筑波) / 韩国 (KOREN) / 台湾 (TANet) / 欧美名校学术专网（不含国内）</div>
          <div>• <b>住宅家宽原生优质 IP</b>：纯净家庭宽带与移动网络原生节点，低风控、极速防封</div>
          <div>• <b>全自动容灾与并发测速</b>：自动并发拉取、剔除不可用死节点、按纯净度与网络速度降序排序</div>
        </div>
        <div style="display:flex;gap:8px;flex-wrap:wrap">
          <button class="primary" id="refreshMirrors" data-src="all">🔄 刷新全部优质源</button>
          <button id="refreshVpnGate" data-src="vpngate">🇯🇵 仅拉取筑波大学源</button>
          <button id="refreshEdu" data-src="edu">🎓 仅拉取海外学术网</button>
          <button id="refreshResidential" data-src="residential">🏡 仅拉取住宅家宽源</button>
        </div>
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

      <label class="f" style="margin-top:14px">
        <span style="font-weight:600;display:flex;align-items:center;gap:6px">
          🔴 Quantumult X 节点订阅
          <em style="color:var(--dim);font-weight:normal;font-style:normal">Quantumult X 官方专有格式 (原生解析 SOCKS5 / Trojan / VMess / SS)</em>
        </span>
        <div style="display:flex;gap:6px;margin-top:4px">
          <input type="text" id="subUrlQuanX" readonly style="flex:1;background:#12151a;color:#d87070;font-family:monospace;font-size:11px">
          <button class="primary" id="copySubQuanX" style="flex:none">📋 复制</button>
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

<div class="modal" id="liveScanModal">
  <div class="sheet" style="max-width:960px;width:95vw;max-height:90vh">
    <div class="head">
      <h2>🔍 母机测活扫描器 (全网/全国/高校真实发包测活)</h2>
      <span class="spacer"></span>
      <button class="icon" data-close="liveScanModal" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body" style="padding:14px">
      <div style="font-size:12px;color:var(--dim);margin-bottom:12px;line-height:1.6;background:#0e1116;border:1px solid var(--line);border-radius:6px;padding:10px 12px">
        ⚡ <b>真实出网验证标准</b>：直接从本机 VPS 发起真实 TCP+SOCKS5/OpenVPN 握手并向 <code>1.1.1.1/cdn-cgi/trace</code> 校验出口，<b>只有母机实测 100% 能连通、有回包的节点才会列出</b>！杜绝任何失效死节点与假节点。
      </div>

      <div style="display:flex;gap:10px;align-items:flex-end;flex-wrap:wrap;margin-bottom:12px;background:var(--panel);border:1px solid var(--line);padding:10px;border-radius:6px">
        <div style="display:flex;flex-direction:column;gap:4px">
          <span style="font-size:11px;color:var(--dim)">扫描来源</span>
          <select id="lsSource" style="padding:6px 8px;font-size:12px;min-width:140px;background:#0e1116;border:1px solid var(--line);color:var(--text);border-radius:4px">
            <option value="all">🌐 全部优质源 (全量并发实测)</option>
            <option value="gov">🏛️ 全球政府公共机构网 (政务专网)</option>
            <option value="edu" selected>🎓 海外高校学术网 (日本筑波/韩国/台湾/欧美)</option>
            <option value="vpngate">🇯🇵 日本筑波大学 (VPN Gate)</option>
            <option value="residential">🏡 住宅家宽原生节点</option>
            <option value="custom">📁 本地自定义节点</option>
          </select>
        </div>

        <div style="display:flex;flex-direction:column;gap:4px">
          <span style="font-size:11px;color:var(--dim)">筛选国家/地区</span>
          <select id="lsRegion" style="padding:6px 8px;font-size:12px;min-width:110px;background:#0e1116;border:1px solid var(--line);color:var(--text);border-radius:4px">
            <option value="all">全部国家/地区</option>
            <option value="JP">🇯🇵 日本 (筑波大学等)</option>
            <option value="US">🇺🇸 美国 (名校/骨干)</option>
            <option value="KR">🇰🇷 韩国 (KOREN)</option>
            <option value="TW">🇹🇼 台湾 (TANet)</option>
            <option value="HK">🇭🇰 香港</option>
            <option value="SG">🇸🇬 新加坡</option>
            <option value="DE">🇩🇪 德国</option>
            <option value="GB">🇬🇧 英国</option>
            <option value="CN">🇨🇳 中国 (国内节点)</option>
          </select>
        </div>

        <div style="display:flex;flex-direction:column;gap:4px">
          <span style="font-size:11px;color:var(--dim)">测活数量</span>
          <select id="lsMax" style="padding:6px 8px;font-size:12px;min-width:125px;background:#0e1116;border:1px solid var(--line);color:var(--text);border-radius:4px;font-weight:600">
            <option value="0" selected>⚡ 全部拉满 (全量测活)</option>
            <option value="5000">超大池 (5,000 个)</option>
            <option value="2000">深度池 (2,000 个)</option>
            <option value="1000">快速池 (1,000 个)</option>
            <option value="300">极速体验 (300 个)</option>
          </select>
        </div>

        <div style="display:flex;flex-direction:column;gap:4px;flex:1;min-width:160px">
          <span style="font-size:11px;color:var(--dim)">关键词过滤 (IP/学校/运营商)</span>
          <input type="search" id="lsSearch" placeholder="如 tsukuba、edu、sinet、150.40..." style="font-size:12px;padding:6px 8px;background:#0e1116;border:1px solid var(--line);color:var(--text);border-radius:4px">
        </div>

        <div style="display:flex;flex-direction:column;gap:4px">
          <span style="font-size:11px;color:var(--dim)">对接节点链接 (与新建出口一致)</span>
          <select id="lsTpl" style="padding:6px 8px;font-size:12px;min-width:160px;background:#0e1116;border:1px solid var(--line);color:var(--text);border-radius:4px">
            <option value="0">自动匹配活跃节点链接</option>
          </select>
        </div>

        <div style="display:flex;align-items:center;gap:8px">
          <button class="primary" id="lsStartScanBtn" style="height:32px;font-weight:600;padding:0 12px">
            <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><polygon points="10 8 16 12 10 16 10 8"/></svg>
            开始测活扫描
          </button>
          <button id="lsBatchStartBtn" disabled style="height:32px;border-color:var(--ok);color:var(--ok);padding:0 12px">
            ⚡ 批量启动选中 (<span id="lsSelCount">0</span>)
          </button>
        </div>
      </div>

      <div id="lsStatusBox" style="margin-bottom:12px;display:flex;align-items:center;gap:12px;font-size:12px;color:var(--dim);background:#0e1116;border:1px solid var(--line);border-radius:4px;padding:7px 12px">
        <span id="lsScanIndicator" style="display:inline-flex;align-items:center;gap:6px">
          <span class="dot" id="lsDot"></span>
          <span id="lsStatusText">准备就绪，点击上方按钮从 VPS 本机探测</span>
        </span>
        <span class="spacer"></span>
        <span>实测有效：<b id="lsVerifiedCount" style="color:var(--ok)">0</b> 个</span>
        <span id="lsLastScanTime" style="margin-left:8px"></span>
      </div>

      <div style="max-height:420px;overflow-y:auto;border:1px solid var(--line);border-radius:6px;background:var(--panel)">
        <table style="width:100%;border-collapse:collapse;font-size:12px;text-align:left">
          <thead style="position:sticky;top:0;background:#181c23;border-bottom:1px solid var(--line);z-index:2">
            <tr>
              <th style="padding:8px 10px;width:36px"><input type="checkbox" id="lsSelectAll" title="全选所有可用"></th>
              <th style="padding:8px 10px;width:80px">地区</th>
              <th style="padding:8px 10px;width:80px">协议</th>
              <th style="padding:8px 10px">节点地址 / 归属网络</th>
              <th style="padding:8px 10px;width:95px">实测延迟</th>
              <th style="padding:8px 10px;width:95px">真实状态</th>
              <th style="padding:8px 10px;text-align:right;width:90px">操作</th>
            </tr>
          </thead>
          <tbody id="lsTableBody">
            <tr>
              <td colspan="7" style="text-align:center;padding:36px;color:var(--dim)">
                暂无实测数据，点击【开始测活扫描】从本机 VPS 真实探测全网与高校节点
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div class="foot">
      <span class="count" id="lsFootInfo">实测通畅节点启动后自动挂接出网</span>
      <span class="spacer"></span>
      <button data-close="liveScanModal">关闭</button>
    </div>
  </div>
</div>

<div class="toast" id="toast"></div>

<script>
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
  const n = view.exits.length;
  $('#ecount').textContent = n ? n + ' 个' : '';
  const hasInboundsOrExits = view.exits.some(e => (e.inbounds && e.inbounds.length) || e.status === 'up');
  $('#exportAll').disabled = !hasInboundsOrExits;
  $('#stopall').disabled = !n;

  const govCount = view.exits.filter(e => e.ip_type === 'gov').length;
  const eduCount = view.exits.filter(e => e.ip_type === 'edu').length;
  const resCount = view.exits.filter(e => e.ip_type === 'residential').length;
  const purities = view.exits.map(e => e.purity_score || 55);
  const avgPurity = purities.length ? Math.round(purities.reduce((a, b) => a + b, 0) / purities.length) : 0;

  const summaryBar = '<div class="stats-summary">'
    + '<span class="stat-pill">运行出口<b>' + n + '</b></span>'
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

const DEFAULT_CORE_REGIONS = [
  {code: 'GLOBAL', name: '全球推荐 (自动优选)', available: 50, avg_purity: 92},
  {code: 'JP', name: '日本 (筑波大学官方/镜像)', available: 20, avg_purity: 95},
  {code: 'US', name: '美国 (家宽/科研)', available: 25, avg_purity: 90},
  {code: 'HK', name: '中国香港', available: 15, avg_purity: 92},
  {code: 'TW', name: '中国台湾', available: 12, avg_purity: 90},
  {code: 'SG', name: '新加坡', available: 12, avg_purity: 92},
  {code: 'KR', name: '韩国 (高校/家宽)', available: 10, avg_purity: 95},
  {code: 'GB', name: '英国', available: 10, avg_purity: 90},
  {code: 'DE', name: '德国', available: 10, avg_purity: 92},
  {code: 'CA', name: '加拿大', available: 8, avg_purity: 90},
  {code: 'FR', name: '法国', available: 8, avg_purity: 90},
  {code: 'AU', name: '澳大利亚', available: 8, avg_purity: 90},
  {code: 'NL', name: '荷兰', available: 8, avg_purity: 92},
  {code: 'GOV', name: '全球政府公共机构专网', available: 5, avg_purity: 99},
  {code: 'EDU', name: '海外高校学术科研网 (不含国内)', available: 15, avg_purity: 99}
];

function clampCount(val) {
  let n = parseInt(val, 10);
  if (isNaN(n) || n < 1) n = 3;
  if (n > 100) n = 100;
  return n;
}

function renderRegions(){
  const kw = ($('#rgfilter').value || '').trim().toLowerCase();
  const sourceList = regions.length ? regions : DEFAULT_CORE_REGIONS;
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
  updateAvail();
}

function availOf(code){
  const sourceList = regions.length ? regions : DEFAULT_CORE_REGIONS;
  if(code === '') return sourceList.reduce((a, r) => a + r.available, 0);
  const r = sourceList.find(x => x.code === code);
  return r ? r.available : 0;
}

function updateAvail(){
  const countEl = $('#count');
  countEl.value = clampCount(countEl.value);
  const want = Number(countEl.value) || 3;
  const avail = availOf(region);
  const hint = $('#availhint');
  const goBtn = $('#go');

  hint.textContent = avail ? '当前源可用 ' + avail + ' 个空闲节点' : (region ? '当前源此地区暂无空闲节点，将尝试全网池' : '全网可用');
  hint.className = 'hint' + (want > avail && avail ? ' bad' : '');
  if(want > avail && avail) hint.textContent = '只剩 ' + avail + ' 个，将全部使用';
  goBtn.disabled = false;
}

async function loadWizard(selectedSrc){
  const src = selectedSrc !== undefined ? selectedSrc : ($('#wzSource') ? $('#wzSource').value : 'all');
  const regionsBox = $('#regions');
  if (!regions.length) {
    regionsBox.innerHTML = '<div style="padding:16px;text-align:center;color:var(--dim);font-size:12px">'
      + '<div class="spin" style="display:inline-block;margin-bottom:8px">' + ICON.run + '</div>'
      + '<div>正在载入可用地区与节点列表…</div></div>';
  }

  // 1. 独立异步读取选定节点源的地区列表
  api('/api/regions?source=' + encodeURIComponent(src || 'all')).then(res => {
    regions = res || [];
    regionsLoaded = true;
    renderRegions();
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
    renderRegions();
    if(wzCandidatesExpanded) loadWizardCandidates();
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
    loadWizard($('#wzSource').value);
    if(wzCandidatesExpanded) loadWizardCandidates();
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
    const res = await api('/api/nodes?region=' + encodeURIComponent(region || '') + '&source=' + encodeURIComponent(src || 'all') + '&limit=300');
    wzCandidatesList = (res && res.nodes) ? res.nodes : [];
    if(countEl) countEl.textContent = wzCandidatesList.length;
    renderWizardCandidates();
  }catch(e){
    listEl.innerHTML = '<div style="padding:12px;text-align:center;color:var(--warn);font-size:12px">读取候选失败: ' + esc(e.message) + '</div>';
  }
}

function renderWizardCandidates(){
  const listEl = $('#wzCandidatesList');
  if(!listEl) return;
  if(!wzCandidatesList.length){
    listEl.innerHTML = '<div style="padding:12px;text-align:center;color:var(--dim);font-size:12px">该地区/筛选条件下暂无空闲候选节点</div>';
    return;
  }

  listEl.innerHTML = wzCandidatesList.map((n, idx) => {
    const isChecked = selectedWzHosts.has(n.hostname);
    const flag = getFlagEmoji(n.country_code);
    const ping = n.ping || 0;
    const pingColor = ping > 0 && ping < 100 ? 'var(--ok)' : (ping > 0 && ping < 200 ? 'var(--accent)' : 'var(--warn)');
    const pingText = ping > 0 ? ping + ' ms' : '就绪';
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
      const count = clampCount($('#count').value);
      const avail = availOf(region);
      const want = Math.min(count, avail > 0 ? avail : count);
      await api('/api/provision?count=' + want + '&region=' + encodeURIComponent(region)
        + '&source=' + encodeURIComponent(src) + '&template=' + tpl, {method:'POST', timeout: 60000});
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
  const quanxUrl = origin + '/sub?format=quanx' + tokenQueryClash;

  const bEl = $('#subUrlBase64');
  if(bEl) bEl.value = base64Url;
  const cEl = $('#subUrlClash');
  if(cEl) cEl.value = clashUrl;
  const qEl = $('#subUrlQuanX');
  if(qEl) qEl.value = quanxUrl;
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
  if(e.target.closest('#autoOrchestrateBtn')){
    const btn = e.target.closest('#autoOrchestrateBtn');
    btn.disabled = true;
    toast('⚡ 正在执行全网智能编排：热门国家各维持 3 个出口，冷门国家各维持 1 个...');
    try {
      await api('/api/auto/orchestrate', {method: 'POST'});
      toast('全网智能编排已触发！正在检查各国家配额并拉起可用节点...');
      setTimeout(poll, 2500);
    } catch(err) {
      toast('触发编排失败: ' + err.message, true);
    } finally {
      setTimeout(() => { btn.disabled = false; }, 3000);
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

poll();
setInterval(poll, 3000);
</script>
</body>
</html>`
