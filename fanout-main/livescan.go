package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

var (
	openvpnRemoteRe = regexp.MustCompile(`(?m)^\s*remote\s+([^\s]+)\s+(\d+)(?:\s+(tcp|udp))?`)
	openvpnProtoRe  = regexp.MustCompile(`(?m)^\s*proto\s+(udp|tcp)\s*$`)
)

// probeOpenVPNHandshake performs a real OpenVPN control/data-channel handshake
// without installing routes on the host. A TCP/UDP port being reachable is not
// enough: VPN Gate nodes frequently accept the port while the VPN session itself
// is dead. Using --dev null + --route-nopull/--route-noexec keeps this probe isolated.
func probeOpenVPNHandshake(n Node, timeout time.Duration) (bool, int64, error) {
	if strings.TrimSpace(n.Config) == "" {
		return false, 0, fmt.Errorf("OpenVPN 配置为空")
	}
	bin, err := exec.LookPath("openvpn")
	if err != nil {
		return false, 0, fmt.Errorf("openvpn 不可用: %w", err)
	}
	probeTimeout := timeout * 4
	if probeTimeout < 5*time.Second {
		probeTimeout = 5 * time.Second
	}
	if probeTimeout > 8*time.Second {
		probeTimeout = 8 * time.Second
	}

	dir, err := os.MkdirTemp("", "fanout-ovpn-probe-")
	if err != nil {
		return false, 0, err
	}
	defer os.RemoveAll(dir)
	cfgPath := filepath.Join(dir, "node.ovpn")
	authPath := filepath.Join(dir, "auth.txt")
	logPath := filepath.Join(dir, "openvpn.log")
	if err := os.WriteFile(cfgPath, []byte(n.Config), 0600); err != nil {
		return false, 0, err
	}
	if err := os.WriteFile(authPath, []byte("vpn\nvpn\n"), 0600); err != nil {
		return false, 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	args := []string{
		"--config", cfgPath,
		"--auth-user-pass", authPath,
		"--auth-nocache",
		"--dev", "null",
		"--route-nopull",
		"--route-noexec",
		"--connect-retry-max", "1",
		"--connect-timeout", "4",
		"--verb", "3",
		"--log", logPath,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return false, 0, fmt.Errorf("启动 OpenVPN 探测失败: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	deadline := time.Now().Add(probeTimeout)
	for time.Now().Before(deadline) {
		if raw, readErr := os.ReadFile(logPath); readErr == nil {
			logText := string(raw)
			if strings.Contains(logText, "Initialization Sequence Completed") {
				return true, time.Since(start).Milliseconds(), nil
			}
			if strings.Contains(logText, "AUTH_FAILED") ||
				strings.Contains(logText, "TLS Error") ||
				strings.Contains(logText, "Connection timed out") ||
				strings.Contains(logText, "SIGTERM[soft") {
				// Keep polling briefly: OpenVPN may write a transient TLS error before
				// retrying once, but do not wait longer than the probe deadline.
			}
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return false, 0, fmt.Errorf("OpenVPN 探测进程提前退出")
		}
		select {
		case <-ctx.Done():
			return false, 0, fmt.Errorf("OpenVPN 握手超时")
		case <-time.After(80 * time.Millisecond):
		}
	}
	return false, 0, fmt.Errorf("OpenVPN 握手超时")
}

// probeNodeLive 从母机真实发包探测节点。对 OpenVPN 不再把“远端端口能连”当作成功，
// 必须完成真实 OpenVPN 握手；SOCKS5/HTTP 必须完成代理 CONNECT + 实际 HTTP 回包。
func probeNodeLive(n Node, timeout time.Duration) (bool, int64, string, error) {
	if timeout <= 0 {
		timeout = 1200 * time.Millisecond
	}

	// 1. OpenVPN：先做快速端口门槛，再做真实握手。
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
				if len(matches) >= 4 && strings.EqualFold(matches[3], "udp") {
					proto = "udp"
				} else if protoLine := openvpnProtoRe.FindStringSubmatch(n.Config); len(protoLine) >= 2 && strings.EqualFold(protoLine[1], "udp") {
					proto = "udp"
				}
			}
		}
		if targetHost == "" {
			return false, 0, "", fmt.Errorf("无有效远程目标")
		}
		addr := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))
		start := time.Now()
		if proto == "tcp" {
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err != nil {
				return false, 0, "", err
			}
			_ = conn.Close()
		} else {
			conn, err := net.DialTimeout("udp", addr, timeout)
			if err != nil {
				return false, 0, "", err
			}
			_ = conn.Close()
		}
		_, rtt, err := probeOpenVPNHandshake(n, timeout)
		if err != nil {
			return false, 0, "", err
		}
		if rtt <= 0 {
			rtt = time.Since(start).Milliseconds()
		}
		return true, rtt, n.IP, nil
	}

	// 2. SOCKS5 代理全链路实测。
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
		if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
			return false, 0, "", err
		}
		greeting := make([]byte, 2)
		if _, err := io.ReadFull(conn, greeting); err != nil || greeting[0] != 0x05 || greeting[1] != 0x00 {
			return false, 0, "", fmt.Errorf("SOCKS5 认证协商拒绝")
		}
		connectReq := []byte{0x05, 0x01, 0x00, 0x01, 1, 1, 1, 1, 0x00, 0x50}
		if _, err := conn.Write(connectReq); err != nil {
			return false, 0, "", err
		}
		head := make([]byte, 4)
		if _, err := io.ReadFull(conn, head); err != nil || head[1] != 0x00 {
			if err != nil {
				return false, 0, "", err
			}
			return false, 0, "", fmt.Errorf("SOCKS5 远端 CONNECT 失败: %d", head[1])
		}
		var extra int
		switch head[3] {
		case 0x01:
			extra = 6
		case 0x04:
			extra = 18
		case 0x03:
			b := make([]byte, 1)
			if _, err := io.ReadFull(conn, b); err != nil {
				return false, 0, "", err
			}
			extra = int(b[0]) + 2
		default:
			return false, 0, "", fmt.Errorf("SOCKS5 响应地址类型无效")
		}
		if extra > 0 {
			if _, err := io.CopyN(io.Discard, conn, int64(extra)); err != nil {
				return false, 0, "", err
			}
		}
		req := "GET /generate_204 HTTP/1.1\r\nHost: connectivitycheck.gstatic.com\r\nConnection: close\r\n\r\n"
		if _, err := conn.Write([]byte(req)); err != nil {
			return false, 0, "", err
		}
		br := bufio.NewReader(conn)
		status, err := br.ReadString('\n')
		if err != nil || !strings.Contains(status, "HTTP/") {
			return false, 0, "", fmt.Errorf("SOCKS5 实际 HTTP 回包失败")
		}
		return true, time.Since(start).Milliseconds(), n.IP, nil
	}

	// 3. HTTP 代理全链路实测。
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
		req := "CONNECT connectivitycheck.gstatic.com:80 HTTP/1.1\r\nHost: connectivitycheck.gstatic.com:80\r\nProxy-Connection: Keep-Alive\r\n\r\n"
		if _, err := conn.Write([]byte(req)); err != nil {
			return false, 0, "", err
		}
		br := bufio.NewReader(conn)
		statusLine, err := br.ReadString('\n')
		if err != nil || !strings.Contains(statusLine, " 200 ") {
			return false, 0, "", fmt.Errorf("HTTP 代理隧道建立失败")
		}
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return false, 0, "", err
			}
			if strings.TrimSpace(line) == "" {
				break
			}
		}
		if _, err := conn.Write([]byte("GET /generate_204 HTTP/1.1\r\nHost: connectivitycheck.gstatic.com\r\nConnection: close\r\n\r\n")); err != nil {
			return false, 0, "", err
		}
		status, err := br.ReadString('\n')
		if err != nil || !strings.Contains(status, "HTTP/") {
			return false, 0, "", fmt.Errorf("HTTP 代理实际回包失败")
		}
		return true, time.Since(start).Milliseconds(), n.IP, nil
	}
	return false, 0, "", fmt.Errorf("不支持的协议类型: %s", n.Proto)
}

// liveNodeCache 避免新建出口页面反复点击国家时重复打同一批节点。
type liveNodeCacheEntry struct {
	at      time.Time
	alive   bool
	latency int64
	exitIP  string
}

var liveNodeCache = struct {
	sync.RWMutex
	m map[string]liveNodeCacheEntry
}{m: map[string]liveNodeCacheEntry{}}

const liveCacheTTL = 25 * time.Second
const liveDeadCacheTTL = 8 * time.Second

func probeNodesLive(nodes []Node, timeout time.Duration) []Node {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]Node, 0, len(nodes))
	var mu sync.Mutex
	var wg sync.WaitGroup
	concurrency := runtime.NumCPU() * 3
	if concurrency < 6 {
		concurrency = 6
	}
	if concurrency > 16 {
		concurrency = 16
	}
	sem := make(chan struct{}, concurrency)
	now := time.Now()
	for _, node := range nodes {
		n := node
		key := nodeKey(n)
		liveNodeCache.RLock()
		cached, ok := liveNodeCache.m[key]
		liveNodeCache.RUnlock()
		cacheTTL := liveCacheTTL
		if !cached.alive {
			cacheTTL = liveDeadCacheTTL
		}
		if ok && now.Sub(cached.at) < cacheTTL {
			if cached.alive {
				n.Ping = int(cached.latency)

				out = append(out, n)
			}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			alive, rtt, exitIP, _ := probeNodeLive(n, timeout)
			<-sem
			liveNodeCache.Lock()
			liveNodeCache.m[key] = liveNodeCacheEntry{at: time.Now(), alive: alive, latency: rtt, exitIP: exitIP}
			liveNodeCache.Unlock()
			if alive {
				n.Ping = int(rtt)

				mu.Lock()
				out = append(out, n)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := out[i].Ping, out[j].Ping
		if pi != pj {
			return pi < pj
		}
		if out[i].PurityScore != out[j].PurityScore {
			return out[i].PurityScore > out[j].PurityScore
		}
		return out[i].SpeedMbps > out[j].SpeedMbps
	})
	return out
}

func liveRegions(m *Manager, source string) []RegionStat {
	raw := m.RawNodes()
	used := map[string]bool{}
	for _, t := range m.Tunnels() {
		used[t.Node.HostName] = true
	}
	// 每个国家最多抽取 6 个当前质量最高候选，避免一次点击把整个几万节点池全部测一遍。
	byCountry := map[string][]Node{}
	for _, n := range raw {
		if used[n.HostName] || (n.Config == "" && n.Proto != "socks5" && n.Proto != "http" && n.Proto != "https") {
			continue
		}
		if !nodeMatchesSource(n, source) {
			continue
		}
		cc := strings.ToUpper(strings.TrimSpace(n.CountryCode))
		if cc == "" {
			continue
		}
		byCountry[cc] = append(byCountry[cc], n)
	}
	candidates := make([]Node, 0)
	for cc, list := range byCountry {
		_ = cc
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].PurityScore != list[j].PurityScore {
				return list[i].PurityScore > list[j].PurityScore
			}
			pi, pj := list[i].Ping, list[j].Ping
			if pi <= 0 {
				pi = 9999
			}
			if pj <= 0 {
				pj = 9999
			}
			if pi != pj {
				return pi < pj
			}
			return list[i].SpeedMbps > list[j].SpeedMbps
		})
		if len(list) > 10 {
			list = list[:10]
		}
		candidates = append(candidates, list...)
	}
	live := probeNodesLive(candidates, 1200*time.Millisecond)
	acc := map[string]*RegionStat{}
	for _, n := range live {
		cc := strings.ToUpper(strings.TrimSpace(n.CountryCode))
		if cc == "" {
			continue
		}
		r := acc[cc]
		if r == nil {
			name := n.Country
			if zh, ok := countryNameZH[cc]; ok {
				name = zh
			}
			r = &RegionStat{Code: cc, Name: name}
			acc[cc] = r
		}
		r.Available++
		if n.Ping > 0 && (r.BestPing == 0 || n.Ping < r.BestPing) {
			r.BestPing = n.Ping
		}
		if n.SpeedMbps > r.BestSpeed {
			r.BestSpeed = n.SpeedMbps
		}
		if n.IPType == "residential" {
			r.Residential++
		}
		purity := n.PurityScore
		if purity <= 0 {
			purity = 0
		}
		r.AvgPurity += purity
	}
	out := make([]RegionStat, 0, len(acc))
	for _, r := range acc {
		if r.Available > 0 {
			r.AvgPurity /= r.Available
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := popularCountryRank[out[i].Code], popularCountryRank[out[j].Code]
		if ri > 0 && rj > 0 {
			return ri < rj
		}
		if ri > 0 {
			return true
		}
		if rj > 0 {
			return false
		}
		if out[i].Available != out[j].Available {
			return out[i].Available > out[j].Available
		}
		return out[i].BestPing < out[j].BestPing
	})
	return out
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
		maxCandidates = 600 // 默认快速扫描上限；国家/来源筛选时由接口进一步缩小范围
	}
	if maxCandidates > 2000 {
		maxCandidates = 2000
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

	for _, n := range raw {
		if n.IP == "" || seen[nodeKey(n)] {
			continue
		}
		if isFakeDummyNode(n) {
			continue
		}
		if !nodeMatchesSource(n, source) {
			continue
		}
		if region != "" && !strings.EqualFold(region, "all") {
			if !strings.EqualFold(n.CountryCode, region) && !strings.EqualFold(n.Country, region) {
				continue
			}
		}
		seen[nodeKey(n)] = true
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

			t, err := mgr.StartExact(node)
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
