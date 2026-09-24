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
  --accent:#38bdf8;
  --text:#f8fafc;
  --dim:#94a3b8;
  --apple-font:-apple-system, BlinkMacSystemFont, "SF Pro Display", "SF Pro Text", "Helvetica Neue", sans-serif;
}
body{
  margin:0;min-height:100vh;display:flex;flex-direction:column;gap:18px;
  align-items:center;justify-content:center;
  color:var(--text);font:13px/1.5 var(--apple-font);-webkit-font-smoothing:antialiased;
  background-attachment:fixed;
  animation:energy6ClusterFlow 36s cubic-bezier(0.4, 0, 0.2, 1) infinite alternate;
}
.noise-overlay{
  position:fixed;inset:0;width:100%;height:100%;pointer-events:none;z-index:99;opacity:0.085;
  mix-blend-mode:overlay;
  background-image:url("data:image/svg+xml,%3Csvg viewBox='0 0 400 400' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noiseFilter'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noiseFilter)'/%3E%3C/svg%3E");
  background-repeat:repeat;
}
@keyframes energy6ClusterFlow {
  /* 1. INFINITE: 珍珠白核心 + 腮红粉体 + 冰青水蓝光晕 + 金斑闪烁 */
  0%, 100% {
    background:
      radial-gradient(circle at 50% 42%, rgba(255, 255, 255, 0.96) 0%, rgba(254, 240, 138, 0.85) 15%, rgba(244, 114, 182, 0.82) 32%, transparent 58%),
      radial-gradient(circle at 55% 38%, rgba(244, 114, 182, 0.90) 0%, rgba(251, 207, 232, 0.65) 28%, transparent 55%),
      radial-gradient(circle at 45% 46%, rgba(103, 232, 249, 0.92) 0%, rgba(56, 189, 248, 0.78) 35%, transparent 68%),
      radial-gradient(circle at 48% 54%, rgba(253, 224, 71, 0.85) 0%, transparent 35%),
      radial-gradient(ellipse at 50% 45%, rgba(224, 242, 254, 0.75) 0%, transparent 75%),
      linear-gradient(135deg, #a3a8b0 0%, #b8bdc5 50%, #9ba0a9 100%);
  }
  /* 2. WARMTH: 炽烈红宝石/草莓红 + 黑暗樱桃心 + 炽焰橘橙 + 外围天青色寒雾 */
  16.66% {
    background:
      radial-gradient(circle at 52% 43%, rgba(255, 247, 237, 0.95) 0%, rgba(251, 146, 60, 0.85) 18%, transparent 32%),
      radial-gradient(circle at 50% 50%, rgba(92, 8, 30, 0.98) 0%, rgba(220, 38, 38, 0.92) 28%, transparent 58%),
      radial-gradient(circle at 48% 46%, rgba(236, 33, 89, 0.95) 0%, rgba(225, 29, 72, 0.88) 36%, transparent 68%),
      radial-gradient(circle at 54% 45%, rgba(249, 115, 22, 0.90) 0%, rgba(251, 146, 60, 0.75) 42%, transparent 70%),
      radial-gradient(circle at 50% 30%, rgba(125, 211, 252, 0.88) 0%, rgba(56, 189, 248, 0.65) 42%, transparent 72%),
      linear-gradient(135deg, #a3a8b0 0%, #b0b5be 50%, #9ea3ac 100%);
  }
  /* 3. UPLIFTING: 电光洋红粉 + 嫩青柠/薄荷绿顶冠 + 顶端深紫李 + 纵向能量光柱 */
  33.33% {
    background:
      radial-gradient(ellipse at 50% 45%, rgba(255, 255, 255, 0.98) 0%, rgba(253, 231, 243, 0.75) 16%, transparent 38%),
      radial-gradient(circle at 50% 48%, rgba(237, 29, 115, 0.96) 0%, rgba(219, 39, 119, 0.90) 36%, transparent 70%),
      radial-gradient(circle at 50% 26%, rgba(190, 242, 100, 0.92) 0%, rgba(134, 239, 172, 0.78) 32%, transparent 58%),
      radial-gradient(circle at 50% 21%, rgba(112, 26, 117, 0.92) 0%, transparent 32%),
      radial-gradient(circle at 53% 53%, rgba(244, 63, 94, 0.82) 0%, transparent 66%),
      linear-gradient(135deg, #a2a7b0 0%, #b2b7c0 50%, #9ca1aa 100%);
  }
  /* 4. UNIVERSAL: 广袤电光蔚蓝天宇 + 核心深邃桑葚紫/天鹅绒李 + 散落金砂星光 */
  50.00% {
    background:
      radial-gradient(circle at 51% 49%, rgba(255, 255, 255, 0.95) 0%, transparent 18%),
      radial-gradient(circle at 47% 42%, rgba(251, 191, 36, 0.90) 0%, transparent 26%),
      radial-gradient(circle at 50% 48%, rgba(131, 59, 96, 0.98) 0%, rgba(112, 26, 117, 0.90) 35%, transparent 60%),
      radial-gradient(circle at 50% 45%, rgba(14, 165, 233, 0.96) 0%, rgba(56, 189, 248, 0.88) 38%, rgba(2, 132, 199, 0.7) 65%, transparent 82%),
      radial-gradient(circle at 50% 55%, rgba(74, 4, 78, 0.88) 0%, transparent 48%),
      linear-gradient(135deg, #9da2ab 0%, #aeb4be 50%, #9aa0a9 100%);
  }
  /* 5. RECIPROCATED: 晨露薄荷翡翠青海 + 嵌套双生红宝石/玫瑰心 + 环绕金黄柠檬光环 */
  66.66% {
    background:
      radial-gradient(circle at 50% 47%, rgba(255, 255, 255, 0.96) 0%, transparent 18%),
      radial-gradient(circle at 46% 38%, rgba(236, 44, 104, 0.98) 0%, rgba(244, 63, 94, 0.88) 22%, rgba(254, 240, 138, 0.75) 32%, transparent 52%),
      radial-gradient(circle at 54% 56%, rgba(236, 44, 104, 0.98) 0%, rgba(225, 29, 72, 0.88) 22%, rgba(254, 240, 138, 0.75) 32%, transparent 52%),
      radial-gradient(circle at 50% 46%, rgba(110, 231, 183, 0.95) 0%, rgba(190, 242, 100, 0.85) 36%, rgba(52, 211, 153, 0.7) 62%, transparent 80%),
      linear-gradient(135deg, #a1a6af 0%, #b0b6bf 50%, #9ca1aa 100%);
  }
  /* 6. VITAL: 盛大落日珊瑚粉霞 + 幻彩欧珀彩虹光环(金桃紫青) + 雾银石青中央茧 */
  83.33% {
    background:
      radial-gradient(ellipse at 50% 36%, rgba(255, 255, 255, 0.96) 0%, transparent 22%),
      radial-gradient(ellipse at 50% 56%, rgba(255, 255, 255, 0.96) 0%, transparent 22%),
      radial-gradient(ellipse at 50% 46%, rgba(144, 148, 157, 0.95) 0%, rgba(203, 213, 225, 0.82) 28%, transparent 55%),
      radial-gradient(circle at 53% 42%, rgba(254, 215, 170, 0.90) 0%, rgba(254, 240, 138, 0.80) 28%, transparent 54%),
      radial-gradient(circle at 47% 50%, rgba(196, 181, 253, 0.88) 0%, rgba(147, 197, 253, 0.78) 32%, transparent 58%),
      radial-gradient(circle at 50% 46%, rgba(251, 113, 133, 0.95) 0%, rgba(244, 114, 182, 0.86) 38%, rgba(253, 164, 175, 0.65) 66%, transparent 84%),
      linear-gradient(135deg, #a4a9b2 0%, #b4b9c2 50%, #9ea3ac 100%);
  }
}
.card{background:rgba(15, 23, 42, 0.78);border:1px solid rgba(255, 255, 255, 0.20);
  border-top:1px solid rgba(255, 255, 255, 0.45);border-radius:26px;
  backdrop-filter:blur(36px) saturate(200%);-webkit-backdrop-filter:blur(36px) saturate(200%);
  box-shadow:0 30px 70px rgba(0,0,0,0.45), inset 0 1px 1px rgba(255,255,255,0.30);
  padding:32px 34px;width:340px}
.brand{display:flex;align-items:center;gap:8px;margin-bottom:18px}
h1{font-size:18px;font-weight:700;margin:0;letter-spacing:-0.4px;
  background:linear-gradient(135deg, #ffffff 0%, #a7f3d0 60%, #67e8f9 100%);
  -webkit-background-clip:text;-webkit-text-fill-color:transparent}
.badge{font-size:10px;font-weight:600;padding:2px 7px;border-radius:9999px;
  background:linear-gradient(135deg, rgba(56,189,248,0.24), rgba(168,85,247,0.24));
  color:#7dd3fc;border:1px solid rgba(56,189,248,0.45);box-shadow:0 0 12px rgba(56,189,248,0.2)}
label{display:block;color:var(--dim);font-size:11px;margin-bottom:7px}
input{width:100%;box-sizing:border-box;background:rgba(8, 14, 24, 0.75);
  border:1px solid rgba(255, 255, 255, 0.14);color:var(--text);border-radius:12px;
  padding:9px 12px;font:inherit;backdrop-filter:blur(10px);transition:border-color .15s}
input:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 3px rgba(56,189,248,0.3)}
button{width:100%;margin-top:16px;background:linear-gradient(135deg, #0284c7 0%, #2563eb 100%);
  border:0;color:#fff;font:inherit;font-weight:600;border-radius:9999px;padding:9px;
  cursor:pointer;box-shadow:0 4px 18px rgba(37,99,235,0.42), inset 0 1px 0 rgba(255,255,255,0.35);
  transition:all .18s ease}
button:hover{background:linear-gradient(135deg, #38bdf8 0%, #3b82f6 100%);transform:translateY(-1px);
  box-shadow:0 6px 24px rgba(56,189,248,0.6)}
.err{color:#f43f5e;font-size:11px;margin-top:10px;min-height:14px;text-align:center}
.credits{font-size:11px;color:var(--dim);display:flex;align-items:center;gap:6px}
.credits a{color:var(--text);text-decoration:none;font-weight:600}
.credits a:hover{color:var(--accent)}
.links{display:flex;gap:14px}
.links a{color:var(--dim);text-decoration:none;font-size:12px;transition:color .15s}
.links a:hover{color:var(--accent)}
</style>
</head>
<body>
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
