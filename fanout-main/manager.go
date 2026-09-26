package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Manager 维护所有隧道，负责分配槽位与端口。
type Manager struct {
	mu            sync.RWMutex
	tunnels       map[int]*Tunnel
	nodes         []Node
	fetched       time.Time
	workDir       string
	maxSlots      int
	jobs          JobStore
	refreshing    bool
	orchestrateMu sync.Mutex
	bindingMu     sync.Mutex
	stateMu       sync.Mutex
	progressMu    sync.RWMutex
	orchProgress  AutoOrchestrateProgress
	cooldownMu    sync.RWMutex
	cooldowns     map[string]time.Time
}

func NewManager(maxSlots int, workDir string) *Manager {
	if maxSlots < 100 {
		maxSlots = 150
	}
	initial := dedupNodesByIP(loadInitialNodes(workDir))
	return &Manager{
		tunnels:   map[int]*Tunnel{},
		nodes:     initial,
		fetched:   time.Now(),
		workDir:   workDir,
		maxSlots:  maxSlots,
		cooldowns: map[string]time.Time{},
	}
}

const failedNodeCooldown = 10 * time.Minute

// AutoOrchestrateOptions 控制智能编排。只有通过真实出口验证的隧道
// 才会进入正式出口池；Sources 为空或包含 all 表示使用全部候选源。
type AutoOrchestrateProgress struct {
	Running          bool      `json:"running"`
	Stage            string    `json:"stage"`
	Message          string    `json:"message"`
	StartedAt        time.Time `json:"started_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	FinishedAt       time.Time `json:"finished_at,omitempty"`
	CountriesTotal   int       `json:"countries_total"`
	CandidatesTotal  int       `json:"candidates_total"`
	CandidatesTested int       `json:"candidates_tested"`
	LivePassed       int       `json:"live_passed"`
	Started          int       `json:"started"`
	Verified         int       `json:"verified"`
	Added            int       `json:"added"`
	Failed           int       `json:"failed"`
	CurrentCountry   string    `json:"current_country"`
	CurrentNode      string    `json:"current_node"`
	LastError        string    `json:"last_error,omitempty"`
}

func (m *Manager) setOrchestrateProgress(fn func(*AutoOrchestrateProgress)) {
	m.progressMu.Lock()
	fn(&m.orchProgress)
	m.orchProgress.UpdatedAt = time.Now()
	m.progressMu.Unlock()
}

func (m *Manager) OrchestrateProgress() AutoOrchestrateProgress {
	m.progressMu.RLock()
	p := m.orchProgress
	m.progressMu.RUnlock()
	return p
}

func (m *Manager) IsOrchestrating() bool {
	m.progressMu.RLock()
	v := m.orchProgress.Running
	m.progressMu.RUnlock()
	return v
}

type AutoOrchestrateOptions struct {
	Sources       []string      `json:"sources"`
	HotTarget     int           `json:"hot_target"`
	ColdTarget    int           `json:"cold_target"`
	MaxStarts     int           `json:"max_starts"`
	VerifyTimeout time.Duration `json:"-"`
}

func DefaultAutoOrchestrateOptions() AutoOrchestrateOptions {
	return AutoOrchestrateOptions{
		Sources:       []string{"vpngate", "ipspeed", "edu", "custom"},
		HotTarget:     3,
		ColdTarget:    1,
		MaxStarts:     8,
		VerifyTimeout: 30 * time.Second,
	}
}

// nodeMatchesSource 严格限定节点来源，避免“选择一个源却混入其它源”的问题。
func nodeMatchesSource(n Node, source string) bool {
	s := strings.ToLower(strings.TrimSpace(source))
	if s == "" || s == "all" {
		return true
	}
	switch s {
	case "vpngate":
		return strings.EqualFold(n.Source, "vpngate")
	case "ipspeed", "openvpn":
		return strings.EqualFold(n.Source, "ipspeed")
	case "proxy":
		return strings.EqualFold(n.Source, "proxy") || n.Proto == "socks5" || n.Proto == "http" || n.Proto == "https"
	case "edu":
		return strings.EqualFold(n.Source, "edu") || strings.EqualFold(n.IPType, "edu") || isEduIP(n.IP)
	case "gov":
		return strings.EqualFold(n.Source, "gov") || strings.EqualFold(n.IPType, "gov")
	case "residential":
		return strings.EqualFold(n.Source, "residential") || strings.EqualFold(n.IPType, "residential")
	case "custom":
		return strings.EqualFold(n.Source, "custom") || strings.HasPrefix(strings.ToLower(n.HostName), "custom_")
	default:
		return strings.EqualFold(n.Source, s)
	}
}

func nodeKey(n Node) string {
	if n.IP != "" {
		return strings.ToLower(strings.TrimSpace(n.IP)) + ":" + fmt.Sprint(n.Port)
	}
	return strings.ToLower(strings.TrimSpace(n.HostName)) + ":" + fmt.Sprint(n.Port)
}

func (m *Manager) markNodeFailed(n Node) {
	key := nodeKey(n)
	if key == ":0" || key == "" {
		return
	}
	m.cooldownMu.Lock()
	m.cooldowns[key] = time.Now().Add(failedNodeCooldown)
	m.cooldownMu.Unlock()
}

func (m *Manager) nodeCooling(n Node) bool {
	key := nodeKey(n)
	m.cooldownMu.RLock()
	until, ok := m.cooldowns[key]
	m.cooldownMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	m.cooldownMu.Lock()
	delete(m.cooldowns, key)
	m.cooldownMu.Unlock()
	return false
}

func dedupNodesByIP(nodes []Node) []Node {
	seen := make(map[string]bool, len(nodes))
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		ip := normalizeIP(n.IP)
		if ip == "" {
			out = append(out, n)
			continue
		}
		if seen[ip] {
			continue
		}
		seen[ip] = true
		out = append(out, n)
	}
	return out
}

// RefreshNodes 重新拉取所有节点源。
func (m *Manager) RefreshNodes() (int, error) {
	return m.RefreshNodesSource("all")
}

// RefreshNodesSource 重新拉取指定节点源（all, vpngate, edu, proxy）。
func (m *Manager) RefreshNodesSource(source string) (int, error) {
	return m.refreshNodesSource(source, true)
}

func (m *Manager) refreshNodesSource(source string, triggerOrchestrate bool) (int, error) {
	if source == "" {
		source = "all"
	}
	m.mu.Lock()
	if m.refreshing {
		m.mu.Unlock()
		m.mu.RLock()
		defer m.mu.RUnlock()
		return len(m.nodes), nil
	}
	m.refreshing = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.refreshing = false
		m.mu.Unlock()
	}()

	customDir := filepath.Join(m.workDir, "custom_nodes")
	if envDir := os.Getenv("FANOUT_CUSTOM_NODES_DIR"); envDir != "" {
		customDir = envDir
	}
	_ = os.MkdirAll(customDir, 0755)

	nodes, err := fetchNodes(m.workDir, source, 35*time.Second)
	// 扫描加载本地自定义 .ovpn 节点并合并
	customNodes := loadLocalOvpnNodes(customDir)
	if len(customNodes) > 0 {
		nodes = append(customNodes, nodes...)
	}

	sourceInfoMu.Lock()
	globalSourceInfo.CustomDir = customDir
	globalSourceInfo.CustomNodes = len(customNodes)
	globalSourceInfo.TotalNodes = len(nodes)
	sourceInfoMu.Unlock()

	if err != nil && len(nodes) == 0 {
		return 0, err
	}
	nodes = dedupNodesByIP(nodes)
	m.mu.Lock()
	m.nodes = nodes
	m.fetched = time.Now()
	m.mu.Unlock()

	// 手动刷新才触发全网智能编排；故障自愈刷新只更新节点池，避免递归启动另一套编排。
	if triggerOrchestrate {
		go m.AutoOrchestrate()
	}

	return len(nodes), nil
}

// ScanLocalNodes 仅扫描本地自定义 .ovpn 目录并合并到现有节点中
func (m *Manager) ScanLocalNodes() (int, error) {
	customDir := filepath.Join(m.workDir, "custom_nodes")
	if envDir := os.Getenv("FANOUT_CUSTOM_NODES_DIR"); envDir != "" {
		customDir = envDir
	}
	_ = os.MkdirAll(customDir, 0755)
	customNodes := loadLocalOvpnNodes(customDir)

	m.mu.Lock()
	var remoteNodes []Node
	for _, n := range m.nodes {
		if n.CountryCode != "CUSTOM" && !strings.HasPrefix(n.HostName, "custom_") {
			remoteNodes = append(remoteNodes, n)
		}
	}
	m.nodes = append(customNodes, remoteNodes...)
	m.fetched = time.Now()
	total := len(m.nodes)
	m.mu.Unlock()

	sourceInfoMu.Lock()
	globalSourceInfo.CustomDir = customDir
	globalSourceInfo.CustomNodes = len(customNodes)
	globalSourceInfo.TotalNodes = total
	sourceInfoMu.Unlock()

	if len(customNodes) > 0 {
		go BatchEnrichNodes(customNodes)
	}
	return len(customNodes), nil
}

// ImportNodes 批量导入用户粘贴的节点文本（支持 CSV、.ovpn 块或 IP 列表），去重后合并入节点池
func (m *Manager) ImportNodes(rawText string) (int, error) {
	newNodes, err := parseImportedNodes(rawText)
	if err != nil {
		return 0, err
	}
	if len(newNodes) == 0 {
		return 0, fmt.Errorf("未从文本中识别出有效节点或 IP")
	}
	newNodes = dedupNodesByIP(newNodes)

	m.mu.Lock()
	existing := map[string]bool{}
	for _, n := range m.nodes {
		if n.IP != "" {
			existing[n.IP] = true
		}
		if n.HostName != "" {
			existing[n.HostName] = true
		}
	}
	var added int
	for _, n := range newNodes {
		if !existing[n.IP] && !existing[n.HostName] {
			m.nodes = append(m.nodes, n)
			if n.IP != "" {
				existing[n.IP] = true
			}
			if n.HostName != "" {
				existing[n.HostName] = true
			}
			added++
		}
	}
	m.fetched = time.Now()
	total := len(m.nodes)
	m.mu.Unlock()

	// 保存持久化离线缓存
	if m.workDir != "" {
		saveNodesToCache(m.workDir, m.nodes)
	}

	sourceInfoMu.Lock()
	globalSourceInfo.TotalNodes = total
	globalSourceInfo.CustomNodes += added
	sourceInfoMu.Unlock()

	go BatchEnrichNodes(newNodes)
	return added, nil
}

// PruneFailed 停止并清理所有连接失败或中断的出口
func (m *Manager) PruneFailed() int {
	m.mu.Lock()
	var slots []int
	for slot, t := range m.tunnels {
		if t.Status == "failed" || t.Status == "stopped" {
			slots = append(slots, slot)
		}
	}
	m.mu.Unlock()
	for _, slot := range slots {
		_ = m.Stop(slot)
	}
	return len(slots)
}

func (m *Manager) Nodes() ([]Node, time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Node, len(m.nodes))
	globalIPIntel.mu.RLock()
	for i, n := range m.nodes {
		out[i] = n
		if intel, ok := globalIPIntel.cache[n.IP]; ok {
			out[i].IPType = intel.IPType
			out[i].PurityScore = intel.PurityScore
			out[i].ISP = intel.ISP
		}
	}
	globalIPIntel.mu.RUnlock()
	return out, m.fetched
}

// AddVerifiedNode 将经本机实测通畅的优质节点注入节点池前列
func (m *Manager) AddVerifiedNode(n Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	nip := normalizeIP(n.IP)
	for i, cur := range m.nodes {
		if cur.HostName == n.HostName {
			// 同一 Host 更新元数据；若新 IP 与池中其它节点冲突，则保留已有唯一 IP。
			for j, other := range m.nodes {
				if j != i && nip != "" && normalizeIP(other.IP) == nip {
					return
				}
			}
			m.nodes[i] = n
			return
		}
		if nip != "" && normalizeIP(cur.IP) == nip {
			return
		}
	}
	m.nodes = append([]Node{n}, m.nodes...)
}

// RawNodes 获取当前所有原始候选节点
func (m *Manager) RawNodes() []Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Node, len(m.nodes))
	copy(out, m.nodes)
	return out
}

func (m *Manager) Tunnels() []*Tunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Tunnel, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

// freeSlot 找一个未占用的槽位。槽位同时决定端口与网段。
func (m *Manager) freeSlot() (int, error) {
	for i := 1; i <= m.maxSlots; i++ {
		if _, used := m.tunnels[i]; !used {
			return i, nil
		}
	}
	return 0, fmt.Errorf("槽位已满（上限 %d）", m.maxSlots)
}

// Start 为指定节点开一条隧道，返回分配到的本地端口。
func (m *Manager) exitIPInUse(ip string, exceptSlot int) bool {
	ip = normalizeIP(ip)
	if ip == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for slot, other := range m.tunnels {
		if slot == exceptSlot || other.Status == "stopped" || other.Status == "failed" {
			continue
		}
		if normalizeIP(other.ExitIP) == ip {
			return true
		}
	}
	return false
}

func (m *Manager) startTunnel(node Node, jpMode string, exact bool) (*Tunnel, error) {
	m.mu.Lock()
	for _, other := range m.tunnels {
		if other.Status != "stopped" && other.Status != "failed" &&
			(other.Node.HostName == node.HostName || (node.IP != "" && normalizeIP(other.Node.IP) == normalizeIP(node.IP))) {
			m.mu.Unlock()
			return nil, fmt.Errorf("节点 %s (IP: %s) 已在运行中，请勿重复添加", node.HostName, node.IP)
		}
	}
	slot, err := m.freeSlot()
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	taken := map[int]bool{}
	for _, other := range m.tunnels {
		taken[other.Port] = true
	}
	port, err := freeRandomPort(taken)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	cred, err := newSocksCred()
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	t := &Tunnel{
		Slot: slot, Port: port, Node: node,
		TargetRegion:     strings.ToUpper(strings.TrimSpace(node.CountryCode)),
		TargetPolicyMode: normalizeJapanMode(jpMode),
		Status:           "starting", Since: time.Now(), Cred: cred,
	}
	m.tunnels[slot] = t
	m.mu.Unlock()
	if exact {
		go m.bringUpExact(t, true)
	} else {
		go m.bringUp(t, true)
	}
	return t, nil
}

// Start 为指定节点开一条隧道。普通自动启动允许故障时按策略寻找替代节点。
func (m *Manager) Start(node Node) (*Tunnel, error) {
	return m.startTunnel(node, "", false)
}

// StartExact 严格启动用户已经选中的节点，不自动偷偷换成同国其它节点。
func (m *Manager) StartExact(node Node) (*Tunnel, error) {
	return m.startTunnel(node, "", true)
}

// StartWithPolicy 用于策略型批量出口：策略在隧道启动前就写入，避免并发启动时丢失日本运营商限制。
func (m *Manager) StartWithPolicy(node Node, jpMode string) (*Tunnel, error) {
	return m.startTunnel(node, jpMode, false)
}

func (m *Manager) bringUpExact(t *Tunnel, notify bool) {
	if err := m.tryNode(t); err != nil {
		t.Err = firstLine(err.Error())
		t.stop()
		t.Status = "failed"
		m.markNodeFailed(t.Node)
		if serr := m.saveState(); serr != nil {
			log.Printf("保存状态失败: %v", serr)
		}
		return
	}
	if m.exitIPInUse(t.ExitIP, t.Slot) {
		t.Err = fmt.Sprintf("出口 IP %s 与现有出口重复，已拒绝加入", t.ExitIP)
		t.stop()
		t.Status = "failed"
		m.markNodeFailed(t.Node)
		if serr := m.saveState(); serr != nil {
			log.Printf("保存状态失败: %v", serr)
		}
		return
	}
	t.Status = "up"
	t.Err = ""
	if serr := m.saveState(); serr != nil {
		log.Printf("保存状态失败: %v", serr)
	}
	if notify {
		m.notifyPanel()
	}
}

// bringUp 把一条隧道拉起来。
//
// notify 决定成功后是否立刻重建后端配置。换节点重连时要传 false：
// 那条路径随后会调 rebind/resync 把入站改绑到新节点，在那之前重建配置
// 会因为入站还指着旧节点名而把路由规则丢掉。
func (m *Manager) bringUp(t *Tunnel, notify bool) {
	m.bringUpPersist(t, notify, false)
}

// 自动重连的退避区间：一轮候选全挂后等一会儿再刷新节点列表重来，
// 别把死节点列表打爆，也别让恢复拖太久。
const (
	reconnectBackoffMin = 5 * time.Second
	reconnectBackoffMax = 60 * time.Second
)

// bringUpPersist 把一条隧道拉起来。
//
// persist=false（手动新建）：走一轮候选，全失败就标 failed，让用户能立刻看到并重试。
// persist=true（自动重连 / 重启恢复）：一轮全失败不放弃，退避后刷新节点列表再来一轮，
// 一直循环到连上或这条隧道被用户停掉。VPN Gate 死节点多，"当前都不可用"往往只是
// 这一批候选恰好都挂了，过一会儿就有新节点，不该让出口永久躺死。
func (m *Manager) bringUpPersist(t *Tunnel, notify bool, persist bool) {
	if m.tryCandidates(t, notify) {
		return
	}
	if persist {
		// 自动故障恢复：先等待瞬时抖动过去；随后刷新节点池再尝试新的候选。
		// 连续几轮都没有新的可用节点，才删除失效出口，避免短暂源波动导致出口立刻消失。
		for round := 0; round < 3; round++ {
			if !m.tunnelActive(t) {
				return
			}
			time.Sleep(time.Duration(3+round*5) * time.Second)
			if !m.tunnelActive(t) {
				return
			}
			_, _ = m.refreshNodesSource("all", false)
			if m.tryCandidates(t, notify) {
				return
			}
		}
		log.Printf("隧道 %d (国家: %s) 连续多轮均无新的可用候选，已自动删除失效出口", t.Slot, t.TargetRegion)
		_ = m.Stop(t.Slot)
		return
	}

	t.Status = "failed"
	if serr := m.saveState(); serr != nil {
		log.Printf("保存状态失败: %v", serr)
	}
}

// tryCandidates 走一轮候选节点，成功返回 true。失败不改 Status（留给调用方决定）。
func (m *Manager) tryCandidates(t *Tunnel, notify bool) bool {
	// 连不上就顺着同国家最优候选列表换下一个，优先速度与纯净度
	candidates := m.candidatesFor(t)
	// 如果这是一次故障恢复，且首个候选节点正好是刚才挂掉的旧节点，直接跳过它尝试下一个同国候选
	if len(candidates) > 1 && t.Status == "starting" && t.Err != "" && candidates[0].HostName == t.Node.HostName {
		candidates = candidates[1:]
	}
	for i, node := range candidates {
		if !m.tunnelActive(t) {
			return false
		}
		// 其他隧道可能在重试期间占用了这个节点，跳过以免多个端口撞同一出口 IP
		if i > 0 && m.nodeInUse(node.HostName, t.Slot) {
			continue
		}
		t.Node = node
		t.Status = "starting"
		if i > 0 {
			t.Err = fmt.Sprintf("已自动换至同国第 %d 个优质候选节点 (%s)", i+1, node.IP)
		}

		err := m.tryNode(t)
		if err == nil && m.exitIPInUse(t.ExitIP, t.Slot) {
			err = fmt.Errorf("出口 IP %s 与现有出口重复", t.ExitIP)
		}
		if err == nil {
			t.Status = "up"
			t.Err = ""
			if serr := m.saveState(); serr != nil {
				log.Printf("保存状态失败: %v", serr)
			}
			if notify {
				m.notifyPanel()
			}
			return true
		}
		t.stop()
		m.markNodeFailed(node)
	}
	return false
}

// tunnelActive 判断这条隧道是否还归管理器所有且未被用户停掉。
// 用指针比对：Stop 会从 map 里删除并把 Status 置 stopped，
// 重连循环据此退出，避免对着一条已经不存在的隧道空转。
func (m *Manager) tunnelActive(t *Tunnel) bool {
	if t.Status == "stopped" {
		return false
	}
	m.mu.RLock()
	cur, ok := m.tunnels[t.Slot]
	m.mu.RUnlock()
	return ok && cur == t
}

// tryNode 尝试用当前节点把隧道拉起来。
func (m *Manager) tryNode(t *Tunnel) error {
	if t.Node.Proto == "socks5" || t.Node.Proto == "http" || (t.Node.Config == "" && t.Node.Port > 0) {
		proto := t.Node.Proto
		if proto == "" {
			proto = "socks5"
		}
		upstreamAddr := fmt.Sprintf("%s:%d", t.Node.IP, t.Node.Port)
		t.dialer = makeUpstreamDialer(proto, upstreamAddr, 6*time.Second)
		if t.listener == nil {
			if err := t.serve(); err != nil {
				return err
			}
		}
		ip, err := t.probeExitIP()
		if err != nil {
			return err
		}
		t.ExitIP = ip
		if err := enforceCleanExitIP(ip); err != nil {
			return err
		}
		enrichNodeFromPing0(&t.Node, ip)
		if t.Node.IP == "" {
			t.Node.IP = ip
		}
		EnrichNodeWithIntel(&t.Node)
		return nil
	}

	if err := t.setupNetns(); err != nil {
		return err
	}
	if err := t.startOpenVPN(m.workDir); err != nil {
		return err
	}
	if t.listener == nil {
		if err := t.serve(); err != nil {
			return err
		}
	}
	ip, err := t.probeExitIP()
	if err != nil {
		return err
	}
	t.ExitIP = ip
	if err := enforceCleanExitIP(ip); err != nil {
		return err
	}
	enrichNodeFromPing0(&t.Node, ip)
	if t.Node.IP == "" {
		t.Node.IP = ip
	}
	EnrichNodeWithIntel(&t.Node)
	return nil
}

// candidatesFor 寻找指定节点所属同一国家的其他优质候选节点。
// 严格按【纯净度(学术/家宽原生优先) ➔ 延迟(最低Ping优先) ➔ 带宽速度】智能排序。
func (m *Manager) candidatesFor(t *Tunnel) []Node {
	first := t.Node
	const maxTries = 8
	m.mu.RLock()
	defer m.mu.RUnlock()

	usedHosts := map[string]bool{first.HostName: true}
	usedIPs := map[string]bool{}
	usedISPs := map[string]bool{}
	if first.IP != "" {
		usedIPs[normalizeIP(first.IP)] = true
	}
	for _, t := range m.tunnels {
		usedHosts[t.Node.HostName] = true
		if t.Node.IP != "" {
			usedIPs[normalizeIP(t.Node.IP)] = true
		}
		if t.TargetPolicyMode == "isp" {
			tr := strings.ToUpper(strings.TrimSpace(t.TargetRegion))
			if tr == "" {
				tr = strings.ToUpper(strings.TrimSpace(t.Node.CountryCode))
			}
			if tr != "" && strings.EqualFold(tr, strings.ToUpper(strings.TrimSpace(first.CountryCode))) {
				if isp := normalizeISP(t.Node.ISP); isp != "" {
					usedISPs[isp] = true
				}
			}
		}
	}

	// 确定目标国家代码
	region := strings.ToUpper(strings.TrimSpace(first.CountryCode))
	if region == "" {
		for _, n := range m.nodes {
			if n.HostName == first.HostName {
				region = strings.ToUpper(strings.TrimSpace(n.CountryCode))
				break
			}
		}
	}

	// 收集同一国家的所有可用空闲节点
	var pool []Node
	for _, n := range m.nodes {
		if usedHosts[n.HostName] || (n.IP != "" && usedIPs[normalizeIP(n.IP)]) {
			continue
		}
		if t.TargetPolicyMode == "isp" {
			if isp := normalizeISP(n.ISP); isp == "" || usedISPs[isp] {
				continue
			}
		}
		if m.nodeCooling(n) {
			continue
		}
		// 必须具有合法的 OpenVPN 配置或原生代理协议（杜绝无法出网的坏节点）
		if n.Config == "" && n.Proto != "socks5" && n.Proto != "http" {
			continue
		}
		// 必须严格属于同一个国家
		if region != "" && !strings.EqualFold(n.CountryCode, region) && !strings.EqualFold(n.Country, region) {
			continue
		}
		pool = append(pool, n)
	}

	// 智能质量优选排序：纯净度优先、速度/低延迟优先
	sort.Slice(pool, func(i, j int) bool {
		// 1. 纯净度等级 (gov > edu/住宅家宽 > 移动 > 其它)
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
		r1, r2 := typeRank(pool[i].IPType), typeRank(pool[j].IPType)
		if r1 != r2 {
			return r1 > r2
		}
		// 2. 纯净度得分
		if pool[i].PurityScore != pool[j].PurityScore {
			return pool[i].PurityScore > pool[j].PurityScore
		}
		// 3. Ping 延迟优先 (越低越好)
		p1, p2 := pool[i].Ping, pool[j].Ping
		if p1 <= 0 {
			p1 = 999
		}
		if p2 <= 0 {
			p2 = 999
		}
		if p1 != p2 {
			return p1 < p2
		}
		// 4. 速度优先
		return pool[i].SpeedMbps > pool[j].SpeedMbps
	})

	var out []Node
	// 如果 first 本身配置非空或具有合法代理协议，保留在首位
	if first.Config != "" || (first.IP != "" && first.Port > 0) {
		out = append(out, first)
	}
	for _, n := range pool {
		if len(out) >= maxTries {
			break
		}
		out = append(out, n)
	}

	// 若当前国家可用节点较少，触发异步后台刷新补充
	if len(pool) < 2 {
		go func() {
			_, _ = m.RefreshNodes()
		}()
	}

	return out
}

// Stop 停掉一条隧道并释放槽位。
func (m *Manager) Stop(slot int) error {
	invalidateInbounds()
	m.mu.Lock()
	t, ok := m.tunnels[slot]
	if ok {
		delete(m.tunnels, slot)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("槽位 %d 没有运行中的隧道", slot)
	}
	t.stop()
	if err := m.saveState(); err != nil {
		log.Printf("保存状态失败: %v", err)
	}

	// 坏死彻底剔除：从 3x-ui 与面板中同步移除与此出口绑定的入站，绝不留任何失效死节点
	if p, err := openPanel(); err == nil && p != nil {
		inbounds, _ := p.Inbounds(nil)
		var toDel []int
		for _, ib := range inbounds {
			bTo := strings.TrimSpace(ib.BoundTo)
			if bTo == "" || strings.EqualFold(bTo, "direct") || strings.EqualFold(bTo, "none") {
				continue
			}
			if bTo == t.Node.HostName || sanitizeTag(bTo) == sanitizeTag(t.Node.HostName) ||
				(t.Node.IP != "" && bTo == t.Node.IP) || (t.ExitIP != "" && bTo == t.ExitIP) ||
				bTo == fmt.Sprintf("exit-%d", t.Slot) || bTo == fmt.Sprintf("slot-%d", t.Slot) {
				toDel = append(toDel, ib.ID)
			}
		}
		if len(toDel) > 0 {
			_ = p.DeleteInbounds(toDel, m.Tunnels())
			invalidateInbounds()
		}
	}

	m.notifyPanel()
	return nil
}

// Swap 把一条隧道换到同地区的另一个节点上，端口与已分发的客户端配置保持不变。
//
// 与健康检查的自动重连不同：那边优先重连原节点（目标是恢复），
// 这里用户是嫌当前出口 IP 不好用，必须真的换一个。
func (m *Manager) Swap(slot int) error {
	m.mu.RLock()
	t, ok := m.tunnels[slot]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("槽位 %d 没有运行中的隧道", slot)
	}
	if t.Status == "starting" {
		return fmt.Errorf("这个出口正在连接中，稍等一下")
	}

	// pickNodes 已排除所有在用节点，优先尝试原节点源，若无则尝试该地区任意源
	picks, err := m.pickNodesSource(t.Node.CountryCode, t.Node.Source, 1)
	if err != nil {
		picks, err = m.pickNodes(t.Node.CountryCode, 1)
	}
	if err != nil {
		return err
	}
	oldHost := t.Node.HostName
	t.Node = picks[0]
	m.reconnect(t, oldHost)
	return nil
}

// StopAll 停掉所有隧道并清空状态文件。
func (m *Manager) StopAll() {
	for _, t := range m.Tunnels() {
		_ = m.Stop(t.Slot)
	}
}

// SetCred 改一条出口的 SOCKS5 凭据。cred 两个字段都为空表示随机重置。
//
// 改完要通知后端：本机 Xray 的 socks 出站里带着这套凭据，
// 不同步的话面板侧的节点会立刻连不上自己的出口。
func (m *Manager) SetCred(slot int, cred SocksCred) (SocksCred, error) {
	m.mu.RLock()
	t, ok := m.tunnels[slot]
	m.mu.RUnlock()
	if !ok {
		return SocksCred{}, fmt.Errorf("槽位 %d 没有运行中的隧道", slot)
	}

	if cred.User == "" && cred.Pass == "" {
		gen, err := newSocksCred()
		if err != nil {
			return SocksCred{}, err
		}
		cred = gen
	}
	if err := validateCred(cred); err != nil {
		return SocksCred{}, err
	}

	t.setCredential(cred)
	if err := m.saveState(); err != nil {
		log.Printf("保存状态失败: %v", err)
	}
	m.syncCred(t)
	return cred, nil
}

// ReconcileOutbounds 在启动恢复隧道后跑一次，把后端出站对齐到当前隧道（含 SOCKS5 凭据）。
//
// 只为 3x-ui 模式而生：它的 OnTunnelsChanged 是空操作，重启不会重写面板出站，
// 而从旧版本升上来时面板里持久化的 socks 出站没有认证字段，端口一旦要认证就连不上。
// 自建模式恢复时每条隧道 up 都会重建配置，本就自洽，这里跳过免得多重启一次 Xray。
func (m *Manager) ReconcileOutbounds() {
	p, err := openPanel()
	if err != nil || p.Kind() != "3x-ui" {
		return
	}

	// 等隧道尽量都起完再重写一次，避免只覆盖到先 up 的那几条
	deadline := time.Now().Add(90 * time.Second)
	for {
		tunnels := m.Tunnels()
		if len(tunnels) == 0 {
			return
		}
		var up *Tunnel
		settled := true
		for _, t := range tunnels {
			if t.Status == "up" && up == nil {
				up = t
			}
			if t.Status == "starting" {
				settled = false
			}
		}
		if (settled || time.Now().After(deadline)) && up != nil {
			if err := m.resync(up); err != nil {
				log.Printf("启动对账面板出站失败: %v", err)
			}
			return
		}
		if settled || time.Now().After(deadline) {
			return // 全 failed，没有可写的出站
		}
		time.Sleep(2 * time.Second)
	}
}

// syncCred 把新凭据写进后端的 socks 出站。
//
// 两种后端的做法不同：自建模式整份重建配置，3x-ui 模式只改出站那一段。
// 都走 ResyncOutbound，接口语义正好是"重写这条隧道对应的出站"。
func (m *Manager) syncCred(t *Tunnel) {
	if err := m.resync(t); err != nil {
		log.Printf("同步 SOCKS5 凭据到节点链接后端失败: %v", err)
	}
}

// Shutdown 停掉运行态但保留状态文件，让下次启动能恢复同样的隧道。
func (m *Manager) Shutdown() {
	for _, t := range m.Tunnels() {
		t.stop()
	}
}

// prepareHost 打开转发开关。netns 出网依赖它。
func prepareHost() error {
	if err := exec.Command("sysctl", "-qw", "net.ipv4.ip_forward=1").Run(); err != nil {
		return fmt.Errorf("开启 ip_forward 失败: %w", err)
	}
	return nil
}

// nodeInUse 判断某节点是否已被别的隧道占用。
func (m *Manager) nodeInUse(host string, exceptSlot int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for slot, t := range m.tunnels {
		if slot != exceptSlot && t.Node.HostName == host {
			return true
		}
	}
	return false
}

// rebind 在隧道换节点后，把原先指向旧节点的 3x-ui 入站改绑到新节点。
// 面板不可用时静默跳过，健康检查本身不应因此失败。
func (m *Manager) rebind(oldHost string, t *Tunnel) error {
	x, err := openPanel()
	if err != nil {
		return nil
	}
	return x.Rebind(oldHost, t, m.Tunnels())
}

// resync 在节点没换但重连过之后，把 3x-ui 的出站配置刷新一遍。
// 面板不可用时静默跳过，健康检查本身不应因此失败。
func (m *Manager) resync(t *Tunnel) error {
	x, err := openPanel()
	if err != nil {
		return nil
	}
	return x.ResyncOutbound(t, m.Tunnels())
}

// notifyPanel 告诉后端隧道集合变了。
//
// 自建模式下出站是由隧道列表现算出来的，不通知的话新开的出口在 Xray 里
// 没有对应的 socks 出站，绑定会指向一个不存在的 tag。接管 3x-ui 时是空操作。
// 后端不可用不该让开关出口失败，所以只记日志。
func (m *Manager) notifyPanel() {
	p, err := openPanel()
	if err != nil {
		return
	}
	if err := p.OnTunnelsChanged(m.Tunnels()); err != nil {
		log.Printf("同步节点链接后端失败: %v", err)
	}
}
