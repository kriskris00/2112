package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Auth 给管理界面加一层登录。
// 口令存在工作目录下，首次启动自动生成，避免公网上裸奔。
type Auth struct {
	dir      string
	password string
	mu       sync.RWMutex
	sessions map[string]time.Time
	// 按来源 IP 记录登录失败，挡低速凭据喷洒
	fails map[string]*loginFails
}

// loginFails 跟踪单个来源 IP 的连续失败。
type loginFails struct {
	count   int
	last    time.Time
	blocked time.Time
}

const sessionCookie = "fanout_session"
const sessionTTL = 12 * time.Hour

// 登录失败节流：同一 IP 连续错 loginMaxFails 次后，锁 loginBlockFor。
// 阈值给得宽松，正常用户偶尔输错不受影响；成功登录会清零。
const (
	loginMaxFails  = 8
	loginBlockFor  = 2 * time.Minute
	loginFailReset = 10 * time.Minute
)

// NewAuth 载入或生成访问口令。返回口令是否为本次新建。
func NewAuth(dir string) (*Auth, bool, error) {
	path := filepath.Join(dir, "password")
	created := false

	blob, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		pw, gerr := randomToken(9)
		if gerr != nil {
			return nil, false, gerr
		}
		if werr := os.WriteFile(path, []byte(pw+"\n"), 0600); werr != nil {
			return nil, false, fmt.Errorf("写口令文件失败: %w", werr)
		}
		blob = []byte(pw)
		created = true
	} else if err != nil {
		return nil, false, err
	}

	return &Auth{
		dir:      dir,
		password: strings.TrimSpace(string(blob)),
		sessions: map[string]time.Time{},
		fails:    map[string]*loginFails{},
	}, created, nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// check 比对口令，用恒定时间比较避免时序泄漏。
func (a *Auth) check(pw string) bool {
	a.mu.RLock()
	cur := a.password
	a.mu.RUnlock()
	want := sha256.Sum256([]byte(cur))
	got := sha256.Sum256([]byte(pw))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}

// SetPassword 改访问口令并落盘。空口令拒绝，避免误改成无密码裸奔。
// 改完不动已有会话：当前登录的浏览器不会被踢，新登录才用新口令。
func (a *Auth) SetPassword(pw string) error {
	pw = strings.TrimSpace(pw)
	if pw == "" {
		return fmt.Errorf("口令不能为空")
	}
	if len(pw) < 4 {
		return fmt.Errorf("口令至少 4 位")
	}
	path := filepath.Join(a.dir, "password")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(pw+"\n"), 0600); err != nil {
		return fmt.Errorf("写口令文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("保存口令失败: %w", err)
	}
	a.mu.Lock()
	a.password = pw
	a.mu.Unlock()
	return nil
}

// issue 发一个会话 token。
func (a *Auth) issue() (string, error) {
	tok, err := randomToken(16)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.sessions[tok] = time.Now().Add(sessionTTL)
	// 顺手清掉过期会话
	for k, exp := range a.sessions {
		if time.Now().After(exp) {
			delete(a.sessions, k)
		}
	}
	a.mu.Unlock()
	return tok, nil
}

func (a *Auth) valid(tok string) bool {
	a.mu.RLock()
	exp, ok := a.sessions[tok]
	a.mu.RUnlock()
	return ok && time.Now().Before(exp)
}

func (a *Auth) Password() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.password
}

// Wrap 保护一个 handler，未登录时 API 返回 401、页面跳登录。
func (a *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			a.handleLogin(w, r)
			return
		}
		// 聚合订阅接口：允许通过 ?token= 或 ?key= 或 Basic Auth 口令鉴权，兼容各种代理客户端
		if r.URL.Path == "/sub" || r.URL.Path == "/sub/" {
			token := r.URL.Query().Get("token")
			if token == "" {
				token = r.URL.Query().Get("key")
			}
			if token == "" {
				_, pass, ok := r.BasicAuth()
				if ok {
					token = pass
				}
			}
			if token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(a.Password())) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			if c, err := r.Cookie(sessionCookie); err == nil && a.valid(c.Value) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "未授权的订阅访问，请在链接后附带 ?token=您的访问口令", http.StatusUnauthorized)
			return
		}
		if c, err := r.Cookie(sessionCookie); err == nil && a.valid(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loginHTML))
	})
}

// blocked 判断某来源 IP 是否处于登录冷却期。
func (a *Auth) blocked(ip string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	f, ok := a.fails[ip]
	return ok && time.Now().Before(f.blocked)
}

// recordFail 记一次失败，达到阈值就进入冷却。
func (a *Auth) recordFail(ip string) {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	f, ok := a.fails[ip]
	// 距上次失败太久就重新计数，避免长期累积误伤
	if !ok || (f.blocked.IsZero() && now.Sub(f.last) > loginFailReset) {
		f = &loginFails{}
		a.fails[ip] = f
	}
	f.count++
	f.last = now
	if f.count >= loginMaxFails {
		f.blocked = now.Add(loginBlockFor)
		f.count = 0
	}
	// 顺手清掉早已过期的记录，别让 map 无限增长
	for k, v := range a.fails {
		if now.Sub(v.last) > loginFailReset && now.After(v.blocked) {
			delete(a.fails, k)
		}
	}
}

// clearFails 登录成功后清掉该 IP 的失败记录。
func (a *Auth) clearFails(ip string) {
	a.mu.Lock()
	delete(a.fails, ip)
	a.mu.Unlock()
}

// clientIP 从 RemoteAddr 取来源 IP。服务直接监听公网端口、不在反代后，
// 所以不采信 X-Forwarded-For 之类可伪造的头。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *Auth) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loginHTML))
		return
	}
	ip := clientIP(r)
	if a.blocked(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "登录失败次数过多，请稍后再试"})
		return
	}
	if !a.check(r.FormValue("password")) {
		a.recordFail(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "口令不对"})
		return
	}
	a.clearFails(ip)
	tok, err := a.issue()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"ok": "已登录"})
}

const loginHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>fanout</title>
<style>
:root{
  --accent:#0284c7;
  --text:#0f172a;
  --dim:#475569;
  --apple-font:-apple-system, BlinkMacSystemFont, "SF Pro Display", "SF Pro Text", "Helvetica Neue", sans-serif;
}
*{box-sizing:border-box}
body{
  margin:0;min-height:100vh;display:flex;flex-direction:column;gap:18px;
  align-items:center;justify-content:center;
  color:var(--text);font:13px/1.5 var(--apple-font);-webkit-font-smoothing:antialiased;
  background:radial-gradient(circle at 50% 50%, #f8fafc 0%, #e2e8f0 100%);position:relative;overflow-x:hidden;
}

.fluid-aura-container{
  position:fixed;inset:0;width:100vw;height:100vh;overflow:hidden;pointer-events:none;z-index:0;
  background:radial-gradient(circle at 50% 50%, #f8fafc 0%, #e2e8f0 100%);filter:blur(48px);-webkit-filter:blur(48px);transform:translateZ(0);
}
.aura-blob{
  position:absolute;border-radius:45% 55% 65% 35% / 40% 50% 60% 50%;
  opacity:0.96;will-change:transform, background;
}
.blob-1{
  width:70vw;height:70vw;top:-12%;left:-8%;
  animation:fluidOrbit1 11s ease-in-out infinite alternate, colorCycleBlob1 22s ease-in-out infinite;
}
.blob-2{
  width:68vw;height:68vw;bottom:-12%;right:-8%;
  animation:fluidOrbit2 13s ease-in-out infinite alternate, colorCycleBlob2 22s ease-in-out infinite;
}
.blob-3{
  width:62vw;height:62vw;top:18%;left:22%;
  animation:fluidOrbit3 9s ease-in-out infinite alternate, colorCycleBlob3 22s ease-in-out infinite;
}
.blob-4{
  width:58vw;height:58vw;bottom:8%;left:-4%;
  animation:fluidOrbit4 12s ease-in-out infinite alternate, colorCycleBlob4 22s ease-in-out infinite;
}
.blob-5{
  width:54vw;height:54vw;top:8%;right:-4%;
  animation:fluidOrbit5 10s ease-in-out infinite alternate, colorCycleBlob5 22s ease-in-out infinite;
}

@keyframes fluidOrbit1{
  0%{transform:translate(-8%, -12%) rotate(0deg) scale(1);}
  33%{transform:translate(32%, 18%) rotate(120deg) scale(1.18);}
  66%{transform:translate(18%, 38%) rotate(240deg) scale(0.90);}
  100%{transform:translate(-18%, 22%) rotate(360deg) scale(1.10);}
}
@keyframes fluidOrbit2{
  0%{transform:translate(12%, 18%) rotate(0deg) scale(1.12);}
  33%{transform:translate(-28%, -12%) rotate(-120deg) scale(0.92);}
  66%{transform:translate(-12%, -32%) rotate(-240deg) scale(1.20);}
  100%{transform:translate(28%, -18%) rotate(-360deg) scale(1);}
}
@keyframes fluidOrbit3{
  0%{transform:translate(0%, 0%) scale(0.92) rotate(0deg);}
  50%{transform:translate(-24%, 28%) scale(1.25) rotate(180deg);}
  100%{transform:translate(28%, -18%) scale(1.08) rotate(360deg);}
}
@keyframes fluidOrbit4{
  0%{transform:translate(18%, -18%) scale(1.08) rotate(0deg);}
  50%{transform:translate(-22%, 24%) scale(1.22) rotate(180deg);}
  100%{transform:translate(22%, 8%) scale(0.90) rotate(360deg);}
}
@keyframes fluidOrbit5{
  0%{transform:translate(-12%, 12%) scale(1);}
  50%{transform:translate(18%, -24%) scale(1.28);}
  100%{transform:translate(-8%, 18%) scale(1.08);}
}

/* 6 大色系高速互流，无任何黑灰色，全部为明朗高亮纯净色与纯白晶莹高光 */
@keyframes colorCycleBlob1{
  0%, 100%{background:#38bdf8;}
  16.66%{background:#0284c7;}
  33.33%{background:#a3e635;}
  50.00%{background:#0ea5e9;}
  66.66%{background:#34d399;}
  83.33%{background:#67e8f9;}
}
@keyframes colorCycleBlob2{
  0%, 100%{background:#f472b6;}
  16.66%{background:#fb7185;}
  33.33%{background:#f43f5e;}
  50.00%{background:#ec4899;}
  66.66%{background:#fb923c;}
  83.33%{background:#fda4af;}
}
@keyframes colorCycleBlob3{
  0%, 100%{background:#ffffff;}
  16.66%{background:#f0fdf4;}
  33.33%{background:#ffffff;}
  50.00%{background:#e0f2fe;}
  66.66%{background:#ffffff;}
  83.33%{background:#fdf4ff;}
}
@keyframes colorCycleBlob4{
  0%, 100%{background:#fde047;}
  16.66%{background:#fb923c;}
  33.33%{background:#f43f5e;}
  50.00%{background:#fbbf24;}
  66.66%{background:#fef08a;}
  83.33%{background:#fed7aa;}
}
@keyframes colorCycleBlob5{
  0%, 100%{background:#e879f9;}
  16.66%{background:#ffffff;}
  33.33%{background:#38bdf8;}
  50.00%{background:#ffffff;}
  66.66%{background:#4ade80;}
  83.33%{background:#c084fc;}
}

.noise-overlay{
  position:fixed;inset:0;width:100%;height:100%;pointer-events:none;z-index:2;opacity:0.18;
  mix-blend-mode:hard-light;
  background-image:url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noiseFilter'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.8' numOctaves='3' stitchTiles='stitch'/%3E%3CfeColorMatrix type='matrix' values='1 0 0 0 0  0 1 0 0 0  0 0 1 0 0  0 0 0 0.85 0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noiseFilter)'/%3E%3C/svg%3E");
  background-repeat:repeat;
}

/* 2026 Apple Liquid Glass 纯白/高透质感 */
.card{background:rgba(255, 255, 255, 0.58);border:1px solid rgba(255, 255, 255, 0.85);
  border-top:1.5px solid #ffffff;border-radius:28px;
  backdrop-filter:blur(42px) saturate(210%) brightness(110%);-webkit-backdrop-filter:blur(42px) saturate(210%) brightness(110%);
  box-shadow:0 30px 70px rgba(0,0,0,0.10), inset 0 1px 2px rgba(255,255,255,0.95);
  padding:34px 38px;width:340px;position:relative;z-index:10}
.brand{display:flex;align-items:center;gap:8px;margin-bottom:18px}
h1{font-size:18px;font-weight:700;margin:0;letter-spacing:-0.4px;
  background:linear-gradient(135deg, #0f172a 0%, #0369a1 60%, #0d9488 100%);
  -webkit-background-clip:text;-webkit-text-fill-color:transparent}
.badge{font-size:10px;font-weight:600;padding:2px 8px;border-radius:9999px;
  background:rgba(2, 132, 199, 0.12);color:#0284c7;border:1px solid rgba(2, 132, 199, 0.3);
  box-shadow:0 2px 6px rgba(2, 132, 199, 0.1)}
label{display:block;color:var(--dim);font-size:11px;margin-bottom:7px;font-weight:600}
input{width:100%;box-sizing:border-box;background:rgba(255, 255, 255, 0.70);
  border:1px solid rgba(255, 255, 255, 0.90);color:var(--text);border-radius:12px;
  padding:9px 12px;font:inherit;backdrop-filter:blur(10px);box-shadow:inset 0 1px 2px rgba(0,0,0,0.04);transition:all .15s}
input:focus{outline:none;border-color:var(--accent);background:#ffffff;box-shadow:0 0 0 3px rgba(2, 132, 199, 0.25)}
button{width:100%;margin-top:16px;background:linear-gradient(135deg, #0284c7 0%, #2563eb 100%);
  border:1px solid rgba(255, 255, 255, 0.4);color:#fff;font:inherit;font-weight:600;border-radius:9999px;padding:9px;
  cursor:pointer;box-shadow:0 4px 16px rgba(2, 132, 199, 0.35), inset 0 1px 1px rgba(255,255,255,0.4);
  transition:all .18s ease}
button:hover{background:linear-gradient(135deg, #0ea5e9 0%, #3b82f6 100%);transform:translateY(-1px);
  box-shadow:0 6px 22px rgba(2, 132, 199, 0.48)}
.err{color:#dc2626;font-size:11px;margin-top:10px;min-height:14px;text-align:center}
.credits{font-size:11px;color:var(--dim);display:flex;align-items:center;gap:6px;position:relative;z-index:10}
.credits a{color:var(--text);text-decoration:none;font-weight:600}
.credits a:hover{color:var(--accent)}
.links{display:flex;gap:14px;position:relative;z-index:10}
.links a{color:var(--dim);text-decoration:none;font-size:12px;transition:color .15s}
.links a:hover{color:var(--accent)}
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
<form class="card" id="f">
  <div class="brand">
    <h1>fanout</h1>
    <span class="badge">Jesee 魔改旗舰版</span>
  </div>
  <label for="pw">访问口令</label>
  <input type="password" id="pw" autofocus autocomplete="current-password" placeholder="请输入口令">
  <button type="submit">进入控制台</button>
  <div class="err" id="err"></div>
</form>
<div class="credits">
  <span>原作者: <a href="https://github.com/byJoey/fanout" target="_blank" rel="noopener">Joey</a></span>
  <span style="opacity:0.3">·</span>
  <span style="color:#70b8ff">魔改升级: Jesee</span>
</div>
<div class="links">
  <a href="https://t.me/+ft-zI76oovgwNmRh" target="_blank" rel="noopener">交流群</a>
  <a href="https://youtube.com/@joeyblog" target="_blank" rel="noopener">Joey油管</a>
  <a href="https://joeyblog.net" target="_blank" rel="noopener">Joey博客</a>
  <a href="https://github.com/byJoey/fanout" target="_blank" rel="noopener">GitHub</a>
</div>
<script>
document.getElementById('f').onsubmit = async e => {
  e.preventDefault();
  const body = new URLSearchParams({password: document.getElementById('pw').value});
  const r = await fetch('login', {method:'POST', body});
  if(r.ok){ location.reload(); return; }
  const d = await r.json().catch(()=>({}));
  document.getElementById('err').textContent = d.error || '登录失败';
};
</script>
</body>
</html>`
