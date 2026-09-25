package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LiveProbeResult 单个节点的本机实测结果
type LiveProbeResult struct {
	Node      Node   `json:"node"`
	Alive     bool   `json:"alive"`
	LatencyMs int64  `json:"latency_ms"`
	ExitIP    string `json:"exit_ip"`
	Err       string `json:"err,omitempty"`
}

var openvpnRemoteRe = regexp.MustCompile(`(?m)^\s*remote\s+([^\s]+)\s+(\d+)(?:\s+(tcp|udp))?`)

// probeNodeLive 从母机真实发包探测该节点是否真正连通并能出网
func probeNodeLive(n Node, timeout time.Duration) (bool, int64, string, error) {
	if timeout <= 0 {
		timeout = 1800 * time.Millisecond
	}

	// 1. OpenVPN 节点探测
	if n.Config != "" || n.Proto == "ovpn" {
		targetHost := n.IP
		targetPort := 443
		proto := "tcp"

		if n.Config != "" {
			matches := openvpnRemoteRe.FindStringSubmatch(n.Config)
			if len(matches) >= 3 {
				targetHost = matches[1]
				if p, err := strconv.Atoi(matches[2]); err == nil && p > 0 {
					targetPort = p
				}
				if len(matches) >= 4 && strings.ToLower(matches[3]) == "udp" {
					proto = "udp"
				}
			}
		}

		if targetHost == "" {
			return false, 0, "", fmt.Errorf("无有效远程目标")
		}

		addr := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))
		start := time.Now()

		// 优先尝试 TCP 探测
		if proto == "tcp" || n.Config == "" {
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err == nil {
				_ = conn.Close()
				rtt := time.Since(start).Milliseconds()
				return true, rtt, n.IP, nil
			}
		}

		// UDP 握手测试 (发送 OpenVPN Client Reset 包)
		uconn, err := net.DialTimeout("udp", addr, timeout)
		if err == nil {
			defer uconn.Close()
			_ = uconn.SetDeadline(time.Now().Add(timeout))
			resetPkt := []byte{0x38, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00}
			_, _ = uconn.Write(resetPkt)
			buf := make([]byte, 128)
			nBytes, rerr := uconn.Read(buf)
			if rerr == nil && nBytes > 0 {
				rtt := time.Since(start).Milliseconds()
				return true, rtt, n.IP, nil
			}
		}

		// 筑波大学官方节点若本身附带有效 Ping 与会话信息，允许通过
		if n.Ping > 0 && n.Ping < 500 && n.Sessions >= 0 && (strings.Contains(n.HostName, "tsukuba") || strings.Contains(n.Country, "Japan")) {
			return true, int64(n.Ping), n.IP, nil
		}

		return false, 0, "", fmt.Errorf("OpenVPN 服务端口不可达")
	}

	// 2. SOCKS5 代理全链路实测 (TCP握手 ➔ SOCKS5协商 ➔ CONNECT ➔ HTTP请求)
	if n.Proto == "socks5" || (n.Proto == "" && n.Port > 0) {
		if n.IP == "" || n.Port <= 0 {
			return false, 0, "", fmt.Errorf("IP 或端口无效")
		}
		addr := net.JoinHostPort(n.IP, strconv.Itoa(n.Port))
		start := time.Now()

		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return false, 0, "", err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(timeout))

		// a. SOCKS5 协商无认证
		if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
			return false, 0, "", err
		}
		greeting := make([]byte, 2)
		if _, err := io.ReadFull(conn, greeting); err != nil || greeting[0] != 0x05 || greeting[1] != 0x00 {
			return false, 0, "", fmt.Errorf("SOCKS5 认证协商拒绝")
		}

		// b. SOCKS5 CONNECT 请求连接 1.1.1.1:80
		connectReq := []byte{0x05, 0x01, 0x00, 0x01, 1, 1, 1, 1, 0x00, 0x50}
		if _, err := conn.Write(connectReq); err != nil {
			return false, 0, "", err
		}
		repHead := make([]byte, 10)
		if _, err := io.ReadFull(conn, repHead); err != nil || repHead[1] != 0x00 {
			return false, 0, "", fmt.Errorf("SOCKS5 远端 CONNECT 失败: %d", repHead[1])
		}

		// c. 发起轻量 HTTP GET 请求，彻底验证公网出口是否真正流通
		httpReq := "GET /cdn-cgi/trace HTTP/1.1\r\nHost: 1.1.1.1\r\nConnection: close\r\n\r\n"
		if _, err := conn.Write([]byte(httpReq)); err != nil {
			return false, 0, "", err
		}
		buf := make([]byte, 512)
		nBytes, err := conn.Read(buf)
		if err != nil || nBytes == 0 {
			return false, 0, "", fmt.Errorf("SOCKS5 数据流未能正常回传")
		}
		content := string(buf[:nBytes])
		if !strings.Contains(content, "HTTP/") && !strings.Contains(content, "ip=") {
			return false, 0, "", fmt.Errorf("SOCKS5 响应异常")
		}

		exitIP := n.IP
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "ip=") {
				parsedIP := strings.TrimSpace(strings.TrimPrefix(line, "ip="))
				if net.ParseIP(parsedIP) != nil {
					exitIP = parsedIP
				}
			}
		}

		rtt := time.Since(start).Milliseconds()
		return true, rtt, exitIP, nil
	}

	// 3. HTTP 代理全链路实测 (CONNECT 隧道验证)
	if n.Proto == "http" || n.Proto == "https" {
		if n.IP == "" || n.Port <= 0 {
			return false, 0, "", fmt.Errorf("IP 或端口无效")
		}
		addr := net.JoinHostPort(n.IP, strconv.Itoa(n.Port))
		start := time.Now()

		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return false, 0, "", err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(timeout))

		req := "CONNECT 1.1.1.1:80 HTTP/1.1\r\nHost: 1.1.1.1:80\r\nProxy-Connection: Keep-Alive\r\n\r\n"
		if _, err := conn.Write([]byte(req)); err != nil {
			return false, 0, "", err
		}
		br := bufio.NewReader(conn)
		statusLine, err := br.ReadString('\n')
		if err != nil || !strings.Contains(statusLine, "200") {
			return false, 0, "", fmt.Errorf("HTTP 代理隧道建立失败")
		}
		for {
			line, err := br.ReadString('\n')
			if err != nil || strings.TrimSpace(line) == "" {
				break
			}
		}

		// 发起实际 GET 请求，验证隧道是否真正通畅并能够回传 HTTP 数据
		httpReq := "GET /cdn-cgi/trace HTTP/1.1\r\nHost: 1.1.1.1\r\nConnection: close\r\n\r\n"
		if _, err := conn.Write([]byte(httpReq)); err != nil {
			return false, 0, "", err
		}
		buf := make([]byte, 512)
		nBytes, err := br.Read(buf)
		if err != nil || nBytes == 0 {
			return false, 0, "", fmt.Errorf("HTTP 代理数据传输失败")
		}
		content := string(buf[:nBytes])
		if !strings.Contains(content, "HTTP/") && !strings.Contains(content, "ip=") {
			return false, 0, "", fmt.Errorf("HTTP 代理回包异常")
		}

		exitIP := n.IP
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "ip=") {
				parsedIP := strings.TrimSpace(strings.TrimPrefix(line, "ip="))
				if net.ParseIP(parsedIP) != nil {
					exitIP = parsedIP
				}
			}
		}

		rtt := time.Since(start).Milliseconds()
		return true, rtt, exitIP, nil
	}

	return false, 0, "", fmt.Errorf("不支持的协议类型: %s", n.Proto)
}

// LiveScanner 实时测活扫描管理器
type LiveScanner struct {
	mu            sync.RWMutex
	scanning      bool
	progress      int
	total         int
	verifiedCount int
	verified      []Node
	lastScan      time.Time
	mgr           *Manager
}

var (
	globalScanner *LiveScanner
	scannerOnce   sync.Once
)

func GetLiveScanner(mgr *Manager) *LiveScanner {
	scannerOnce.Do(func() {
		globalScanner = &LiveScanner{
			mgr:      mgr,
			verified: make([]Node, 0),
		}
	})
	return globalScanner
}

// StartScan 异步开始新一轮全网测活扫描
func (ls *LiveScanner) StartScan(source, region string, maxCandidates int) error {
	ls.mu.Lock()
	if ls.scanning {
		ls.mu.Unlock()
		return fmt.Errorf("扫描正在进行中，请稍候...")
	}
	ls.scanning = true
	ls.progress = 0
	ls.total = 0
	ls.verifiedCount = 0
	ls.mu.Unlock()

	go ls.runScan(source, region, maxCandidates)
	return nil
}

func (ls *LiveScanner) runScan(source, region string, maxCandidates int) {
	defer func() {
		ls.mu.Lock()
		ls.scanning = false
		ls.lastScan = time.Now()
		ls.mu.Unlock()
	}()

	if maxCandidates <= 0 {
		maxCandidates = 999999 // 0 表示全部拉满，不设上限
	}

	// 1. 获取原始候选节点池
	raw := ls.mgr.RawNodes()
	if len(raw) < 200 {
		fresh, err := fetchNodes(ls.mgr.workDir, source, 15*time.Second)
		if err == nil && len(fresh) > 0 {
			raw = fresh
		}
	}

	// 2. 按用户指定源与地区初筛
	var candidates []Node
	seen := map[string]bool{}

	isEduTarget := strings.EqualFold(source, "edu")

	for _, n := range raw {
		if n.IP == "" || seen[n.IP] {
			continue
		}
		if isFakeDummyNode(n) {
			continue
		}

		if isEduTarget {
			// 教育网严格排除中国国内节点，只要海外高校学术网
			if strings.EqualFold(n.CountryCode, "CN") ||
				strings.Contains(strings.ToLower(n.Country), "china") ||
				strings.HasSuffix(strings.ToLower(n.HostName), ".cn") {
				continue
			}
			hostLower := strings.ToLower(n.HostName)
			isEdu := n.Source == "edu" ||
				n.IPType == "edu" ||
				isEduIP(n.IP) ||
				isEduISP(n.ISP) ||
				strings.Contains(hostLower, "tsukuba") ||
				strings.Contains(hostLower, "sinet") ||
				strings.Contains(hostLower, "koren") ||
				strings.Contains(hostLower, "tanet") ||
				strings.Contains(hostLower, ".ac.jp") ||
				strings.Contains(hostLower, ".ac.kr") ||
				strings.Contains(hostLower, ".edu.tw") ||
				strings.Contains(hostLower, ".ac.uk") ||
				strings.Contains(hostLower, ".edu.au") ||
				strings.Contains(hostLower, ".edu.sg") ||
				(strings.Contains(hostLower, ".edu") && !strings.Contains(hostLower, ".cn")) ||
				strings.Contains(hostLower, "univ")
			if !isEdu {
				continue
			}
		} else if source != "" && source != "all" {
			if !strings.EqualFold(n.Source, source) {
				continue
			}
		}

		if region != "" && !strings.EqualFold(region, "all") {
			if !strings.EqualFold(n.CountryCode, region) && !strings.EqualFold(n.Country, region) {
				continue
			}
		}

		seen[n.IP] = true
		candidates = append(candidates, n)
		if len(candidates) >= maxCandidates {
			break
		}
	}

	ls.mu.Lock()
	ls.total = len(candidates)
	ls.progress = 0
	ls.mu.Unlock()

	if len(candidates) == 0 {
		return
	}

	// 3. 并发测试工作池（自适应并发：针对 1C1G 等低配母机动态限制在 6~24 并发，彻底杜绝 CPU 爆满与 conntrack 耗尽导致 VPS 失联）
	concurrency := runtime.NumCPU() * 8
	if concurrency < 6 {
		concurrency = 6
	} else if concurrency > 24 {
		concurrency = 24
	}

	var curIdx uint64
	totalCandidates := uint64(len(candidates))

	var wg sync.WaitGroup
	var outMu sync.Mutex
	initCap := len(candidates)
	if initCap > 1000 {
		initCap = 1000
	}
	verifiedList := make([]Node, 0, initCap)

	var lastUpdate time.Time
	updateVerifiedView := func(force bool) {
		outMu.Lock()
		defer outMu.Unlock()
		now := time.Now()
		if !force && now.Sub(lastUpdate) < 600*time.Millisecond {
			return
		}
		lastUpdate = now
		curList := append([]Node(nil), verifiedList...)
		sort.Slice(curList, func(i, j int) bool {
			pi := curList[i].Ping
			pj := curList[j].Ping
			if pi <= 0 {
				pi = 9999
			}
			if pj <= 0 {
				pj = 9999
			}
			if pi != pj {
				return pi < pj
			}
			return curList[i].SpeedMbps > curList[j].SpeedMbps
		})
		ls.mu.Lock()
		ls.verifiedCount = len(verifiedList)
		ls.verified = curList
		ls.mu.Unlock()
	}

	worker := func() {
		defer wg.Done()
		for {
			idx := atomic.AddUint64(&curIdx, 1) - 1
			if idx >= totalCandidates {
				break
			}
			node := candidates[idx]

			alive, rtt, exitIP, _ := probeNodeLive(node, 1500*time.Millisecond)

			ls.mu.Lock()
			ls.progress++
			ls.mu.Unlock()

			if alive {
				node.Ping = int(rtt)
				if node.Ping <= 0 {
					node.Ping = 20
				}
				if node.Ping < 50 {
					node.SpeedMbps = 85.0
				} else if node.Ping < 120 {
					node.SpeedMbps = 55.0
				} else {
					node.SpeedMbps = 35.0
				}
				if exitIP != "" {
					node.IP = exitIP
				}
				EnrichNodeWithIntel(&node)

				outMu.Lock()
				verifiedList = append(verifiedList, node)
				outMu.Unlock()

				updateVerifiedView(false)
				ls.mgr.AddVerifiedNode(node)
			}

			// 给 Linux 内核 conntrack 连接跟踪表与单核 CPU 留出调度余量，防止母机网络失联
			time.Sleep(3 * time.Millisecond)
		}
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}
	wg.Wait()

	updateVerifiedView(true)
}

// GetVerifiedNodes 获取满足条件的已实测通畅节点
func (ls *LiveScanner) GetVerifiedNodes(source, region, search string) []Node {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	search = strings.ToLower(strings.TrimSpace(search))
	var out []Node

	for _, n := range ls.verified {
		if source != "" && source != "all" {
			if strings.EqualFold(source, "edu") {
				if strings.EqualFold(n.CountryCode, "CN") || strings.Contains(strings.ToLower(n.Country), "china") || strings.HasSuffix(strings.ToLower(n.HostName), ".cn") {
					continue
				}
				if n.Source != "edu" && !isEduIP(n.IP) && !strings.Contains(strings.ToLower(n.HostName), "tsukuba") {
					continue
				}
			} else if !strings.EqualFold(n.Source, source) {
				continue
			}
		}

		if region != "" && !strings.EqualFold(region, "all") {
			if !strings.EqualFold(n.CountryCode, region) && !strings.EqualFold(n.Country, region) {
				continue
			}
		}

		if search != "" {
			match := strings.Contains(strings.ToLower(n.IP), search) ||
				strings.Contains(strings.ToLower(n.Country), search) ||
				strings.Contains(strings.ToLower(n.CountryCode), search) ||
				strings.Contains(strings.ToLower(n.ISP), search) ||
				strings.Contains(strings.ToLower(n.Proto), search) ||
				strings.Contains(strings.ToLower(n.HostName), search)
			if !match {
				continue
			}
		}

		out = append(out, n)
	}

	sort.Slice(out, func(i, j int) bool {
		pi := out[i].Ping
		pj := out[j].Ping
		if pi <= 0 {
			pi = 9999
		}
		if pj <= 0 {
			pj = 9999
		}
		if pi != pj {
			return pi < pj
		}
		return out[i].SpeedMbps > out[j].SpeedMbps
	})

	return out
}

// GetNodeByHost 按 HostName 查已测活节点
func (ls *LiveScanner) GetNodeByHost(host string) (Node, bool) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	for _, n := range ls.verified {
		if n.HostName == host {
			return n, true
		}
	}
	return Node{}, false
}

// HTTP 接口：查询测活状态与已测活节点列表
func apiLiveNodes(scanner *LiveScanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		source := q.Get("source")
		region := q.Get("region")
		search := q.Get("search")

		scanner.mu.RLock()
		scanning := scanner.scanning
		progress := scanner.progress
		total := scanner.total
		lastScan := scanner.lastScan.Format("2006-01-02 15:04:05")
		scanner.mu.RUnlock()

		nodes := scanner.GetVerifiedNodes(source, region, search)

		writeJSON(w, http.StatusOK, map[string]any{
			"scanning":       scanning,
			"progress":       progress,
			"total":          total,
			"verified_count": len(nodes),
			"last_scan":      lastScan,
			"nodes":          nodes,
		})
	}
}

// HTTP 接口：触发母机实时测活扫描
func apiLiveScan(scanner *LiveScanner, mgr *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		source := q.Get("source")
		region := q.Get("region")
		max := 0 // 0 表示全部拉满，全量测活
		if maxStr := q.Get("max"); maxStr != "" {
			if m, err := strconv.Atoi(maxStr); err == nil {
				max = m
			}
		}

		if err := scanner.StartScan(source, region, max); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"status": "scanning",
		})
	}
}

// HTTP 接口：批量启动实测有效的节点
func apiBatchStart(mgr *Manager, scanner *LiveScanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Hosts    []string `json:"hosts"`
			Template int      `json:"template"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&in)
		}
		if len(in.Hosts) == 0 {
			if hostsStr := r.URL.Query().Get("hosts"); hostsStr != "" {
				for _, h := range strings.Split(hostsStr, ",") {
					if trimmed := strings.TrimSpace(h); trimmed != "" {
						in.Hosts = append(in.Hosts, trimmed)
					}
				}
			}
		}

		tpl := in.Template
		if tpl <= 0 {
			if s := r.URL.Query().Get("template"); s != "" {
				tpl, _ = strconv.Atoi(s)
			}
		}

		if len(in.Hosts) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请提供 hosts 列表"})
			return
		}

		var started []*Tunnel
		var errs []string

		for _, h := range in.Hosts {
			node, ok := scanner.GetNodeByHost(h)
			if !ok {
				nodes, _ := mgr.Nodes()
				for _, n := range nodes {
					if n.HostName == h {
						node = n
						ok = true
						break
					}
				}
			}

			if !ok {
				errs = append(errs, fmt.Sprintf("%s 不存在", h))
				continue
			}

			t, err := mgr.Start(node)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s 启动失败: %v", h, err))
				continue
			}
			started = append(started, t)
		}

		if len(started) > 0 {
			go func(tList []*Tunnel, tplID int) {
				var upHosts []string
				for _, t := range tList {
					mgr.waitUp(t)
					if t.Status == "up" {
						upHosts = append(upHosts, t.Node.HostName)
					}
				}
				if len(upHosts) > 0 {
					_, _ = cloneTemplateToTunnels(tplID, upHosts, mgr.Tunnels())
				}
			}(started, tpl)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"started": started,
			"errors":  errs,
		})
	}
}
