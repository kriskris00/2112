package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// version 由构建时通过 -ldflags 注入。
var version = "v3.1.0-live-engine"

func main() {
	var (
		webPort  = flag.Int("web", 8899, "Web 管理端口")
		maxSlots = flag.Int("max", 150, "最多同时运行的隧道数")
		workDir  = flag.String("dir", "/var/lib/fanout", "工作目录")
	)
	panelMode := flag.String("panel", "", "节点链接后端: 留空按界面设置/自动探测, 3x-ui, native, xray-cf-lite")
	publicIP := flag.String("ip", "", "母机公网 IPv4，用于分享链接/SOCKS5 地址；留空则自动探测")
	showVersion := flag.Bool("version", false, "显示版本后退出")
	flag.Parse()

	if *publicIP == "" {
		*publicIP = os.Getenv("FANOUT_PUBLIC_IP")
	}

	if *showVersion {
		fmt.Println("fanout", version)
		return
	}

	if os.Geteuid() != 0 {
		log.Fatal("需要 root 权限（要创建 netns 和改 iptables）")
	}
	if err := os.MkdirAll(*workDir, 0700); err != nil {
		log.Fatalf("创建工作目录失败: %v", err)
	}
	initIPIntel(*workDir)
	setPublicIPOverride(*publicIP)
	go hostPublicIP() // 预热探测，别让首个请求阻塞
	if err := prepareHost(); err != nil {
		log.Fatal(err)
	}

	configurePanel(*workDir, *panelMode)
	if p, err := openPanel(); err != nil {
		log.Printf("节点链接后端暂不可用（可在 Web 界面查看原因）: %v", err)
	} else {
		log.Printf("节点链接后端: %s", p.Describe())
	}

	mgr := NewManager(*maxSlots, *workDir)
	// 服务刚启动就保证存在一个真正不走任何出口的“服务器直连 · 本机”节点，
	// 不必等智能编排跑完几十个候选后才出现。
	go mgr.reconcilePanelBindings()
	initNodes, _ := mgr.Nodes()
	log.Printf("节点底池已就绪: %d 个节点 (动态多源节点池，含 VPN Gate/筑波大学及其他公开 OpenVPN 源)", len(initNodes))

	// 后台并发拉取全网最新节点与多源 OpenVPN/代理源，不阻塞服务极速启动
	go func() {
		log.Printf("后台开始并发拉取全网最新节点与多源 OpenVPN/代理源...")
		if n, err := mgr.RefreshNodes(); err != nil {
			log.Printf("后台拉取提示（底池正常运作）: %v", err)
		} else {
			log.Printf("全网节点池已聚合扩展至 %d 个节点", n)
		}
		// 节点池拉取完毕后立即触发全网自动编排（热门国家各3节点，冷门国家各1节点）
		mgr.AutoOrchestrate()
	}()

	if n, err := mgr.restoreState(); err != nil {
		log.Printf("恢复上次状态失败: %v", err)
	} else if n > 0 {
		log.Printf("正在恢复上次的 %d 条隧道", n)
		// 3x-ui 模式重启不会自动重写面板出站，旧版本升上来时面板里的 socks
		// 出站没有认证字段，端口一旦要认证就连不上，这里恢复后对账一次
		go mgr.ReconcileOutbounds()
	}
	go mgr.reconcilePanelBindings()

	go mgr.WatchHealth()
	go mgr.WatchAutoOrchestrate()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		log.Println("正在清理所有隧道...")
		mgr.Shutdown()
		closePanel()
		os.Exit(0)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	liveScanner := GetLiveScanner(mgr)
	mux.HandleFunc("/api/nodes/live", apiLiveNodes(liveScanner))
	mux.HandleFunc("/api/nodes/live_scan", apiLiveScan(liveScanner, mgr))
	mux.HandleFunc("/api/nodes/batch_start", apiBatchStart(mgr, liveScanner))
	mux.HandleFunc("/api/nodes", apiNodes(mgr))
	mux.HandleFunc("/api/tunnels", apiTunnels(mgr))
	mux.HandleFunc("/api/start", apiStart(mgr))
	mux.HandleFunc("/api/stop", apiStop(mgr))
	mux.HandleFunc("/api/swap", apiSwap(mgr))
	mux.HandleFunc("/api/cred", apiCred(mgr))
	mux.HandleFunc("/api/refresh", apiRefresh(mgr))
	mux.HandleFunc("/api/sources", apiSources(mgr))
	mux.HandleFunc("/api/sources/refresh", apiSourcesRefresh(mgr))
	mux.HandleFunc("/api/sources/scan", apiSourcesScan(mgr))
	mux.HandleFunc("/api/regions", apiRegions(mgr))
	mux.HandleFunc("/api/auto/orchestrate", apiAutoOrchestrate(mgr))
	mux.HandleFunc("/api/auto/orchestrate/status", apiAutoOrchestrateStatus(mgr))
	mux.HandleFunc("/api/provision", apiProvision(mgr))
	mux.HandleFunc("/api/jobs", apiJobs(mgr))
	mux.HandleFunc("/api/jobs/dismiss", apiJobDismiss(mgr))
	mux.HandleFunc("/api/jobs/clear", apiJobsClear(mgr))
	mux.HandleFunc("/api/jobs/clean_failed", apiJobsCleanFailed(mgr))
	mux.HandleFunc("/api/exits", apiExits(mgr))
	mux.HandleFunc("/api/exits/prune_failed", apiExitsPruneFailed(mgr))
	mux.HandleFunc("/api/sources/import", apiSourcesImport(mgr))
	mux.HandleFunc("/api/xui", apiXUIStatus)
	mux.HandleFunc("/api/xui/inbounds", apiXUIInbounds(mgr))
	mux.HandleFunc("/api/xui/bind", apiXUIBind(mgr))
	mux.HandleFunc("/api/xui/clone", apiXUIClone(mgr))
	mux.HandleFunc("/api/xui/detail", apiXUIDetail(mgr))
	mux.HandleFunc("/api/xui/links", apiXUILinks)
	mux.HandleFunc("/api/xui/delete", apiXUIDelete(mgr))
	mux.HandleFunc("/api/panel/inbound/new", apiInboundCreate(mgr))
	mux.HandleFunc("/api/panel/inbound/update", apiInboundUpdate(mgr))
	mux.HandleFunc("/api/panel/client/add", apiClientAdd(mgr))
	mux.HandleFunc("/api/panel/client/del", apiClientDelete(mgr))
	mux.HandleFunc("/api/panel/client/reset", apiClientReset(mgr))
	mux.HandleFunc("/api/panel/mode", apiPanelMode(*workDir))

	auth, created, err := NewAuth(*workDir)
	if err != nil {
		log.Fatalf("初始化访问口令失败: %v", err)
	}
	if created {
		log.Printf("已生成访问口令，见 %s", filepath.Join(*workDir, "password"))
	}

	bpCreated, err := initBasePath(*workDir)
	if err != nil {
		log.Fatalf("初始化访问路径失败: %v", err)
	}
	if bpCreated {
		log.Printf("已生成访问路径，见 %s", filepath.Join(*workDir, "basepath"))
	}

	mux.HandleFunc("/sub", apiSubscription(mgr, auth))
	mux.HandleFunc("/api/cred/token", apiCredToken(auth))

	// 用户显式给了 -web 就以命令行为准，否则沿用界面上存过的端口
	portExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "web" {
			portExplicit = true
		}
	})
	webCfg, err := loadWebSettings(*workDir, *webPort, portExplicit)
	if err != nil {
		log.Fatalf("加载 Web 设置失败: %v", err)
	}

	srv := newWebServer(StripBasePath(auth.Wrap(mux)))
	// 设置面板：改密码 / 改路径 / 改端口 / 改本地监听。
	mux.HandleFunc("/api/settings", apiSettings(auth, srv))
	mux.HandleFunc("/api/update/check", apiUpdateCheck)
	mux.HandleFunc("/api/update/apply", apiUpdateApply)

	log.Printf("管理界面: http://<本机IP>%s%s/", webCfg.listenAddrString(), currentBasePath())
	log.Printf("SOCKS5 端口在 %d-%d 之间随机分配", randPortMin, randPortMax)
	if err := srv.serve(); err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func apiNodes(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		region := strings.ToUpper(strings.TrimSpace(q.Get("region")))
		source := strings.ToLower(strings.TrimSpace(q.Get("source")))
		search := strings.ToLower(strings.TrimSpace(q.Get("search")))
		liveOnly := q.Get("live") == "1" || strings.EqualFold(q.Get("live"), "true")
		limit := 300
		if limitStr := q.Get("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}

		allNodes, fetched := m.Nodes()
		tunnels := m.Tunnels()
		usedHosts := make(map[string]bool, len(tunnels))
		for _, t := range tunnels {
			usedHosts[t.Node.HostName] = true
		}

		var filtered []Node
		for _, n := range allNodes {
			// 仅展示具备有效 OpenVPN 配置或原生代理协议的高质量真实节点，过滤失效及不可用代理
			if strings.TrimSpace(n.Config) == "" && n.Proto != "socks5" && n.Proto != "http" {
				continue
			}
			if region != "" && region != "ALL" {
				if strings.ToUpper(n.CountryCode) != region && !strings.Contains(strings.ToUpper(n.Country), region) {
					continue
				}
			}
			if !nodeMatchesSource(n, source) {
				continue
			}
			if search != "" {
				s := strings.ToLower(n.HostName + " " + n.IP + " " + n.ISP + " " + n.Country + " " + n.CountryCode)
				if !strings.Contains(s, search) {
					continue
				}
			}
			filtered = append(filtered, n)
		}

		// 排序优选推荐：学术/家宽纯净度优先，低延迟高带宽优先
		sort.Slice(filtered, func(i, j int) bool {
			typeRank := func(t string) int {
				switch strings.ToLower(t) {
				case "gov":
					return 4
				case "edu":
					return 3
				case "residential":
					return 2
				case "mobile":
					return 1
				default:
					return 0
				}
			}
			r1, r2 := typeRank(filtered[i].IPType), typeRank(filtered[j].IPType)
			if r1 != r2 {
				return r1 > r2
			}
			if filtered[i].PurityScore != filtered[j].PurityScore {
				return filtered[i].PurityScore > filtered[j].PurityScore
			}
			pi := filtered[i].Ping
			pj := filtered[j].Ping
			if pi <= 0 {
				pi = 9999
			}
			if pj <= 0 {
				pj = 9999
			}
			if pi != pj {
				return pi < pj
			}
			return filtered[i].SpeedMbps > filtered[j].SpeedMbps
		})

		if liveOnly {
			// 不要只测前 120 个就结束：前排可能全部是死节点。扩大到 2x 候选，
			// 但设置硬上限，保证手机点国家时不会拖成几十秒。
			probeLimit := limit * 2
			if probeLimit < 120 {
				probeLimit = 120
			}
			if probeLimit > 300 {
				probeLimit = 300
			}
			if len(filtered) > probeLimit {
				filtered = filtered[:probeLimit]
			}
			filtered = probeNodesLive(filtered, 1200*time.Millisecond)
			// live 模式只返回本机刚刚真实探测通过的节点；没有成功探测到的绝不展示。
		}

		total := len(filtered)
		if len(filtered) > limit {
			filtered = filtered[:limit]
		}

		type NodeView struct {
			Node
			InUse bool `json:"in_use"`
		}
		views := make([]NodeView, len(filtered))
		for i, n := range filtered {
			views[i] = NodeView{
				Node:  n,
				InUse: usedHosts[n.HostName],
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"nodes":   views,
			"total":   total,
			"fetched": fetched,
		})
	}
}

func apiTunnels(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, m.Tunnels())
	}
}

func apiAutoOrchestrateStatus(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, m.OrchestrateProgress())
	}
}

func apiAutoOrchestrate(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		opts := DefaultAutoOrchestrateOptions()
		if r.Method == http.MethodPost {
			var body struct {
				Sources    []string `json:"sources"`
				HotTarget  int      `json:"hot_target"`
				ColdTarget int      `json:"cold_target"`
				MaxStarts  int      `json:"max_starts"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Sources) > 0 {
				opts.Sources = body.Sources
			}
			if body.HotTarget > 0 {
				opts.HotTarget = body.HotTarget
			}
			if body.ColdTarget > 0 {
				opts.ColdTarget = body.ColdTarget
			}
			if body.MaxStarts > 0 {
				opts.MaxStarts = body.MaxStarts
			}
		}
		if m.IsOrchestrating() {
			writeJSON(w, http.StatusConflict, map[string]any{"status": "busy", "message": "已有智能编排正在运行，请查看实时进度", "progress": m.OrchestrateProgress()})
			return
		}
		go m.AutoOrchestrateWithOptions(opts)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"message": fmt.Sprintf("已启动智能编排：热门 %d / 冷门 %d；最多验证 %d 个候选", opts.HotTarget, opts.ColdTarget, opts.MaxStarts),
			"options": opts,
		})
	}
}

func apiStart(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.URL.Query().Get("host")
		if host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 host 参数"})
			return
		}
		tpl := 0
		if s := r.URL.Query().Get("template"); s != "" {
			tpl, _ = strconv.Atoi(s)
		}

		startNode := func(vn Node) {
			t, err := m.StartExact(vn)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			// 自动克隆绑定节点链接（与新建出口一致）
			go func(tun *Tunnel, tplID int) {
				m.waitUp(tun)
				if tun.Status == "up" {
					_, _ = cloneTemplateToTunnels(tplID, []string{tun.Node.HostName}, m.Tunnels())
				}
			}(t, tpl)
			writeJSON(w, http.StatusOK, t)
		}

		if globalScanner != nil {
			if vn, ok := globalScanner.GetNodeByHost(host); ok {
				startNode(vn)
				return
			}
		}
		nodes, _ := m.Nodes()
		for _, n := range nodes {
			if n.HostName == host {
				startNode(n)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "节点不存在，可能列表已过期"})
	}
}

func apiStop(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slot, err := strconv.Atoi(r.URL.Query().Get("slot"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slot 参数无效"})
			return
		}
		if err := m.Stop(slot); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "已停止"})
	}
}

func apiRefresh(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		n, err := m.RefreshNodesSource(source)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		nodes, _ := m.Nodes()
		go BatchEnrichNodes(nodes)
		writeJSON(w, http.StatusOK, map[string]int{"count": n})
	}
}

// apiSwap 就地把一个出口换到别的节点，端口不变。
func apiSwap(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slot, err := strconv.Atoi(r.URL.Query().Get("slot"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slot 参数无效"})
			return
		}
		if err := m.Swap(slot); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "正在换节点"})
	}
}

// apiRegions 给新建向导用：各地区还剩多少空闲节点，支持 ?source= 过滤。
func apiRegions(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		if r.URL.Query().Get("live") == "1" || strings.EqualFold(r.URL.Query().Get("live"), "true") {
			// 选定单一来源时，如果当前内存池还没有该来源，后台立即补拉；前端不被长时间请求卡住。
			nodes, _ := m.Nodes()
			hasSource := false
			for _, n := range nodes {
				if nodeMatchesSource(n, source) {
					hasSource = true
					break
				}
			}
			if !hasSource && strings.TrimSpace(source) != "" && !strings.EqualFold(source, "all") {
				go func(src string) {
					if _, err := m.RefreshNodesSource(src); err != nil {
						log.Printf("[节点源] 后台补拉 %s 失败: %v", src, err)
					}
				}(source)
			}
			writeJSON(w, http.StatusOK, liveRegions(m, source))
			return
		}
		writeJSON(w, http.StatusOK, m.Regions(source))
	}
}

// apiSources 返回节点源状态与配置，POST 用于设置自定义源
func apiSources(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				CustomURL string `json:"custom_url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
				return
			}
			SetCustomSourceURL(body.CustomURL)
			n, err := m.RefreshNodes()
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "info": GetSourceInfo()})
				return
			}
			nodes, _ := m.Nodes()
			go BatchEnrichNodes(nodes)
			writeJSON(w, http.StatusOK, map[string]any{"count": n, "info": GetSourceInfo()})
			return
		}
		writeJSON(w, http.StatusOK, GetSourceInfo())
	}
}

// apiSourcesRefresh 强制重新拉取节点，支持 ?source=all|vpngate|edu|proxy
func apiSourcesRefresh(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		n, err := m.RefreshNodesSource(source)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "info": GetSourceInfo()})
			return
		}
		nodes, _ := m.Nodes()
		go BatchEnrichNodes(nodes)
		writeJSON(w, http.StatusOK, map[string]any{"count": n, "info": GetSourceInfo()})
	}
}

// apiSourcesScan 扫描本地 .ovpn 目录
func apiSourcesScan(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, err := m.ScanLocalNodes()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"count": n, "info": GetSourceInfo()})
	}
}

// apiCred 改一个出口的 SOCKS5 用户名口令。两个参数都留空表示随机重置。
func apiCred(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		slot, err := strconv.Atoi(q.Get("slot"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slot 参数无效"})
			return
		}
		cred, err := m.SetCred(slot, SocksCred{
			User: strings.TrimSpace(q.Get("user")),
			Pass: strings.TrimSpace(q.Get("pass")),
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"user": cred.User,
			"pass": cred.Pass,
		})
	}
}

// apiSettings 管理界面自身的设置：改密码 / 改路径 / 改端口 / 改本地监听。
// GET 返回当前值（不含明文口令）；POST 按传入的字段逐项应用，任一项失败即整体回报。
func apiSettings(auth *Auth, srv *webServer) http.HandlerFunc {
	type settingsReq struct {
		Password   *string `json:"password"`    // 非空则改口令
		BasePath   *string `json:"base_path"`   // 提供即改访问路径（空串=去掉前缀）
		Port       *int    `json:"port"`        // 提供即改监听端口
		ListenAddr *string `json:"listen_addr"` // 提供即改监听地址
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in settingsReq
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
				return
			}

			// 改口令
			if in.Password != nil && *in.Password != "" {
				if err := auth.SetPassword(*in.Password); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
			}
			// 改访问路径
			if in.BasePath != nil {
				if _, err := setBasePath(*in.BasePath); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
			}
			// 改端口 / 监听地址：合成一份新的 WebSettings 一起应用，避免绑两次
			if in.Port != nil || in.ListenAddr != nil {
				next := getWebSettings()
				if in.Port != nil {
					next.Port = *in.Port
				}
				if in.ListenAddr != nil {
					next.ListenAddr = *in.ListenAddr
				}
				if err := srv.applyWebSettings(next); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
			}
		}

		cfg := getWebSettings()
		listen := cfg.ListenAddr
		if listen == "" {
			listen = "0.0.0.0"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"base_path":    currentBasePath(),
			"port":         cfg.Port,
			"listen_addr":  listen,
			"has_password": true,
			"version":      version,
		})
	}
}

// apiUpdateCheck 问 GitHub 最新 release，回报当前/最新版本与更新内容。
func apiUpdateCheck(w http.ResponseWriter, r *http.Request) {
	st, err := checkUpdate()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "检查更新失败: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// apiUpdateApply 下载最新版替换二进制并重启服务。成功后进程会被拉起成新版本。
func apiUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "用 POST"})
		return
	}
	st, err := checkUpdate()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "检查更新失败: " + err.Error()})
		return
	}
	if !st.HasUpdate {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restarting": false, "message": "已经是最新版"})
		return
	}
	if err := applyUpdate(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// 先把响应发回去，restartSelf 已排在延迟后触发
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restarting": true, "latest": st.Latest})
}

// apiExits 返回主界面需要的一切：出口以及挂在它上面的入站。
func apiExits(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, m.ExitsOf())
	}
}

// apiProvision 接收"开 N 个某地区的出口"或批量自选节点启动这个意图，返回作业 id 供轮询。
func apiProvision(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var hosts []string
		if rawHosts := q.Get("hosts"); rawHosts != "" {
			for _, h := range strings.Split(rawHosts, ",") {
				if trimmed := strings.TrimSpace(h); trimmed != "" {
					hosts = append(hosts, trimmed)
				}
			}
		}

		tpl := 0
		if s := q.Get("template"); s != "" {
			if parsedTpl, err := strconv.Atoi(s); err == nil {
				tpl = parsedTpl
			}
		}

		count := len(hosts)
		if count == 0 {
			var err error
			count, err = strconv.Atoi(q.Get("count"))
			if err != nil || count < 1 {
				count = 3
			}
			if count > 100 {
				count = 100
			}
		}

		source := q.Get("source")
		job, err := m.Provision(ProvisionRequest{
			Region:     q.Get("region"),
			Source:     source,
			Count:      count,
			TemplateID: tpl,
			Hosts:      hosts,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"job": job.ID()})
	}
}

func apiJobs(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, m.jobs.Views())
	}
}

func apiJobDismiss(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.jobs.Dismiss(r.URL.Query().Get("id"))
		writeJSON(w, http.StatusOK, map[string]string{"ok": "已关闭"})
	}
}

func apiJobsClear(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.jobs.DismissAll()
		writeJSON(w, http.StatusOK, map[string]string{"ok": "已清理所有已完成任务"})
	}
}

func apiJobsCleanFailed(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		m.jobs.ClearFailedSteps(id)
		writeJSON(w, http.StatusOK, map[string]string{"ok": "已清理爆红记录"})
	}
}

func apiExitsPruneFailed(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count := m.PruneFailed()
		writeJSON(w, http.StatusOK, map[string]any{"count": count})
	}
}

func apiSourcesImport(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
			return
		}
		added, err := m.ImportNodes(req.Text)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		nodes, _ := m.Nodes()
		writeJSON(w, http.StatusOK, map[string]any{
			"added": added,
			"total": len(nodes),
			"info":  GetSourceInfo(),
		})
	}
}

// apiXUIStatus 报告当前的节点链接后端：接管的 3x-ui，或 fanout 自己跑的 Xray。
func apiXUIStatus(w http.ResponseWriter, r *http.Request) {
	p, err := openPanel()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false,
			"reason":    err.Error(),
		})
		return
	}
	// xray-cf-lite 模式下节点由 xray-cf-lite 管，fanout 只改路由，不提供新建入口。
	_, isXCL := p.(*XCL)
	resp := map[string]any{
		"available": true,
		"kind":      p.Kind(),
		"describe":  p.Describe(),
		// 自建/3x-ui 能建入站；xray-cf-lite 只能改路由
		"can_create": !isXCL,
	}
	if x, ok := p.(*XUI); ok {
		resp["port"] = x.Port
		resp["base_path"] = x.BasePath
		resp["scheme"] = x.Scheme
		resp["host"] = x.Host
	}
	writeJSON(w, http.StatusOK, resp)
}

// apiPanelMode 读取/切换节点链接后端。
// GET 返回当前模式与本机可选模式；POST {"mode":"..."} 运行时切换，空 mode = 恢复自动探测。
func apiPanelMode(workDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in struct {
				Mode string `json:"mode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
				return
			}
			p, err := switchPanelMode(in.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			invalidateInbounds()
			writeJSON(w, http.StatusOK, map[string]any{
				"mode":     currentPanelMode(),
				"kind":     p.Kind(),
				"describe": p.Describe(),
			})
			return
		}
		resp := map[string]any{
			"mode":  currentPanelMode(),
			"modes": availablePanelModes(workDir),
		}
		if p, err := openPanel(); err == nil {
			resp["kind"] = p.Kind()
			resp["describe"] = p.Describe()
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// apiXUIInbounds 列出面板里已有的入站及其绑定状态。
func apiXUIInbounds(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := cachedInbounds(liveHosts(m))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// liveHosts 返回当前有连通隧道的节点标识集合。
func liveHosts(m *Manager) map[string]bool {
	live := map[string]bool{}
	for _, t := range m.Tunnels() {
		if t.Status == "up" {
			live[sanitizeTag(t.Node.HostName)] = true
		}
	}
	return live
}

// apiXUIBind 把某个入站绑定到某条隧道，slot=0 表示解绑。
func apiXUIBind(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tag := r.URL.Query().Get("tag")
		if tag == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 tag 参数"})
			return
		}
		host := r.URL.Query().Get("host")
		x, err := openPanel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		if err := x.Bind(tag, host, m.Tunnels()); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		invalidateInbounds()
		writeJSON(w, http.StatusOK, map[string]string{"ok": "已更新"})
	}
}

// apiXUIClone 以某个入站为模板，为所有已连通的隧道各复制一个入站并绑好出口。
func apiXUIClone(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id 参数无效"})
			return
		}

		tunnels := m.Tunnels()
		// 用节点主机名而非槽位号：槽位在重启后会重排，指代会错位
		var hosts []string
		if raw := r.URL.Query().Get("hosts"); raw != "" {
			for _, part := range strings.Split(raw, ",") {
				if h := strings.TrimSpace(part); h != "" {
					hosts = append(hosts, h)
				}
			}
		} else {
			for _, t := range tunnels {
				if t.Status == "up" {
					hosts = append(hosts, t.Node.HostName)
				}
			}
		}
		if len(hosts) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "没有可用的隧道"})
			return
		}

		x, err := openPanel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		ports, err := x.CloneToTunnels(id, hosts, tunnels)
		invalidateInbounds()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "created": ports})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"created": ports})
	}
}

// apiXUIDetail 返回某个入站的详情，含客户端与可直接复制的分享链接（带国旗表情与企业名称）。
func apiXUIDetail(mgr *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id 参数无效"})
			return
		}
		x, err := openPanel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		host := r.URL.Query().Get("host")
		if host == "" {
			host = publicHost(r)
		}
		detail, err := x.InboundDetail(id, host)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		// 格式化分享链接名称：国旗表情 + 企业名称
		if detail != nil && len(detail.Links) > 0 {
			tunnels := mgr.Tunnels()
			var matchedTunnel *Tunnel
			bTo := strings.TrimSpace(detail.BoundTo)
			if bTo != "" && !strings.EqualFold(bTo, "direct") && !strings.EqualFold(bTo, "none") {
				for _, t := range tunnels {
					if t.Status == "up" {
						if t.Node.HostName == bTo || sanitizeTag(t.Node.HostName) == bTo || t.Node.IP == bTo || t.ExitIP == bTo || fmt.Sprintf("exit-%d", t.Slot) == bTo {
							matchedTunnel = t
							break
						}
						sTag := sanitizeTag(t.Node.HostName)
						if len(sTag) >= 3 && (strings.Contains(bTo, sTag) || strings.Contains(sTag, bTo)) {
							matchedTunnel = t
							break
						}
					}
				}
			}
			if matchedTunnel != nil {
				cleanName := formatProxyName(matchedTunnel.Node.CountryCode, matchedTunnel.Node.Country, matchedTunnel.Node.ISP)
				for i, rawLink := range detail.Links {
					idx := strings.LastIndex(rawLink, "#")
					if idx != -1 {
						detail.Links[i] = rawLink[:idx] + "#" + url.QueryEscape(cleanName)
					} else {
						detail.Links[i] = rawLink + "#" + url.QueryEscape(cleanName)
					}
				}
			}
		}

		writeJSON(w, http.StatusOK, detail)
	}
}

// publicHost 决定分享链接里的连接地址。母机公网 IPv4 才是客户端真正能连上
// 的地址，所以优先用它；探测不到（比如纯内网）再退回访问 fanout 时用的主机名。
func publicHost(r *http.Request) string {
	if ip := hostPublicIP(); ip != "" {
		return ip
	}
	host := r.Host
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	if host == "" || host == "127.0.0.1" || host == "localhost" {
		return "<服务器IP>"
	}
	return host
}

// apiXUILinks 批量导出多个入站的分享链接。
func apiXUILinks(w http.ResponseWriter, r *http.Request) {
	x, err := openPanel()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	var ids []int
	if raw := r.URL.Query().Get("ids"); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
				ids = append(ids, n)
			}
		}
	} else {
		list, err := x.Inbounds(nil)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		for _, ib := range list {
			ids = append(ids, ib.ID)
		}
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "没有可导出的入站"})
		return
	}

	host := r.URL.Query().Get("host")
	if host == "" {
		host = publicHost(r)
	}
	links, err := x.InboundLinks(ids, host)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "links": links})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

// apiXUIDelete 删除入站。停掉出口后它的入站会留下来，用户需要一个清理入口。
func apiXUIDelete(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var ids []int
		for _, part := range strings.Split(r.URL.Query().Get("ids"), ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
				ids = append(ids, n)
			}
		}
		if len(ids) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "没有指定要删除的入站"})
			return
		}
		x, err := openPanel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		err = x.DeleteInbounds(ids, m.Tunnels())
		invalidateInbounds()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"deleted": len(ids)})
	}
}

// apiInboundCreate 新建一个入站。两种后端都支持：自建模式写自己的库，
// 接管 3x-ui 时走面板的 inbounds/add API。
// apiInboundUpdate 改入站的端口、备注与启停。两种后端都支持。
func apiInboundUpdate(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := openPanel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		q := r.URL.Query()
		id, err := strconv.Atoi(q.Get("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id 参数无效"})
			return
		}

		// 只有真正传了的参数才改，没传的保持原样
		var patch InboundPatch
		if v := q.Get("port"); v != "" {
			port, err := strconv.Atoi(v)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "端口无效"})
				return
			}
			patch.Port = &port
		}
		if q.Has("remark") {
			remark := q.Get("remark")
			patch.Remark = &remark
		}
		if v := q.Get("enable"); v != "" {
			enable := v == "1"
			patch.Enable = &enable
		}

		err = p.UpdateInbound(id, patch, m.Tunnels())
		invalidateInbounds()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "已保存"})
	}
}

// clientAction 把三个客户端操作的公共部分收拢：解析 id/email 再调后端。
func clientAction(m *Manager, what string,
	do func(p Panel, id int, email string, tunnels []*Tunnel) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := openPanel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id 参数无效"})
			return
		}
		err = do(p, id, r.URL.Query().Get("email"), m.Tunnels())
		invalidateInbounds()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": what})
	}
}

func apiClientAdd(m *Manager) http.HandlerFunc {
	return clientAction(m, "已添加", func(p Panel, id int, email string, t []*Tunnel) error {
		return p.AddClient(id, email, t)
	})
}

func apiClientDelete(m *Manager) http.HandlerFunc {
	return clientAction(m, "已删除", func(p Panel, id int, email string, t []*Tunnel) error {
		return p.DeleteClient(id, email, t)
	})
}

func apiClientReset(m *Manager) http.HandlerFunc {
	return clientAction(m, "已重置", func(p Panel, id int, email string, t []*Tunnel) error {
		return p.ResetClient(id, email, t)
	})
}

func apiInboundCreate(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := openPanel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		q := r.URL.Query()
		port, _ := strconv.Atoi(q.Get("port"))
		ib, err := p.CreateInbound(NewInboundSpec{
			Protocol: q.Get("protocol"),
			Network:  q.Get("network"),
			Port:     port,
			Remark:   q.Get("remark"),
			Path:     q.Get("path"),
			Host:     q.Get("host"),
			Security: q.Get("security"),
			Vision:   q.Get("vision") == "1",

			ServerName: q.Get("sni"),
			CertFile:   q.Get("cert"),
			KeyFile:    q.Get("key"),

			Dest:        q.Get("dest"),
			ServerNames: q.Get("server_names"),
			ShortID:     q.Get("sid"),
			Fingerprint: q.Get("fp"),
		}, m.Tunnels())
		invalidateInbounds()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       ib.ID,
			"port":     ib.Port,
			"protocol": ib.Protocol,
			"remark":   ib.Remark,
			"network":  ib.Network,
			"security": ib.Security,
		})
	}
}
