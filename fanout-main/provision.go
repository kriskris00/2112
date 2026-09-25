package main

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProvisionRequest 是"给我 N 个某地区的出口"这个意图。
type ProvisionRequest struct {
	Region     string // 国家码，空表示不限
	Source     string // 节点源："all", "vpngate", "edu", "proxy", "custom"
	Count      int
	TemplateID int      // 3x-ui 入站模板；0 表示只开隧道不建入站
	Hosts      []string // 指定要启动的主机名列表；非空时直接选用
}

// Provision 异步执行一次批量开出口，立刻返回作业句柄供界面轮询。
//
// 隧道并行拉起（每条都要等 openvpn 握手，串行会线性累加等待），
// 面板侧的入站创建则统一放到最后串行做一次，因为每次改路由都要重启 Xray。
func (m *Manager) Provision(req ProvisionRequest) (*Job, error) {
	var picks []Node
	if len(req.Hosts) > 0 {
		m.mu.RLock()
		nodeMap := make(map[string]Node, len(m.nodes))
		for _, n := range m.nodes {
			nodeMap[n.HostName] = n
		}
		m.mu.RUnlock()
		for _, h := range req.Hosts {
			if n, ok := nodeMap[h]; ok {
				picks = append(picks, n)
			} else if globalScanner != nil {
				if n, ok := globalScanner.GetNodeByHost(h); ok {
					picks = append(picks, n)
				}
			}
		}
		if len(picks) == 0 {
			return nil, fmt.Errorf("指定的候选节点已不存在或已下线")
		}
	} else {
		if req.Count < 1 {
			return nil, fmt.Errorf("数量至少为 1")
		}
		var err error
		picks, err = m.pickNodesSource(req.Region, req.Source, req.Count)
		if err != nil {
			return nil, err
		}
	}

	labels := make([]string, 0, len(picks)+1)
	for _, n := range picks {
		labels = append(labels, regionLabel(n)+" 出口")
	}
	if req.TemplateID > 0 {
		labels = append(labels, "创建节点链接")
	}

	where := req.Region
	if len(req.Hosts) > 0 {
		where = "自选节点"
	} else if where == "" {
		where = "任意地区"
	}
	job := m.jobs.New(fmt.Sprintf("开 %d 个 %s 出口", len(picks), where), labels)

	go m.runProvision(job, picks, req.TemplateID)
	return job, nil
}

// cloneTemplateToTunnels 为指定主机名列表的隧道复制并绑定入站模板（与新建出口完全一致）
func cloneTemplateToTunnels(templateID int, hosts []string, tunnels []*Tunnel) ([]int, error) {
	if len(hosts) == 0 {
		return nil, nil
	}
	x, err := openPanel()
	if err != nil {
		return nil, err
	}
	// 如果 templateID <= 0，自动寻找第一个可用的非直连入站模板；若没有入站则自动创建优质模板
	if templateID <= 0 {
		inbounds, _ := x.Inbounds(nil)
		for _, ib := range inbounds {
			if ib.Enable && ib.ID > 0 {
				templateID = ib.ID
				break
			}
		}
		if templateID <= 0 && len(inbounds) > 0 {
			templateID = inbounds[0].ID
		}
		if templateID <= 0 {
			spec := NewInboundSpec{
				Protocol: "vmess",
				Network:  "tcp",
				Remark:   "fanout-template",
			}
			created, err := x.CreateInbound(spec, tunnels)
			if err != nil {
				spec.Protocol = "vless"
				created, err = x.CreateInbound(spec, tunnels)
			}
			if err == nil && created != nil {
				templateID = created.ID
			}
		}
	}
	ports, err := x.CloneToTunnels(templateID, hosts, tunnels)
	invalidateInbounds()
	return ports, err
}

func (m *Manager) runProvision(job *Job, picks []Node, templateID int) {
	defer job.Finish()

	var wg sync.WaitGroup
	started := make([]*Tunnel, len(picks))

	for i, node := range picks {
		var t *Tunnel
		var err error
		if len(picks) > 0 {
			// Hosts 是用户明确选择的实时节点，必须严格启动这个节点，不允许静默换节点。
			t, err = m.StartExact(node)
		} else {
			t, err = m.Start(node)
		}
		if err != nil {
			job.Set(i, "failed", err.Error())
			continue
		}
		started[i] = t
		job.Set(i, "running", "正在连接 "+node.HostName)

		wg.Add(1)
		go func(i int, t *Tunnel) {
			defer wg.Done()
			m.waitUp(t)
			if t.Status == "up" {
				job.Set(i, "ok", t.ExitIP)
				return
			}
			job.Set(i, "failed", firstLine(t.Err))
		}(i, t)
	}
	wg.Wait()

	step := len(picks)
	var hosts []string
	for _, t := range started {
		if t != nil && t.Status == "up" {
			hosts = append(hosts, t.Node.HostName)
		}
	}
	if len(hosts) == 0 {
		job.Set(step, "failed", "没有连通的出口，跳过")
		return
	}

	job.Set(step, "running", fmt.Sprintf("为 %d 个出口建入站", len(hosts)))
	ports, err := cloneTemplateToTunnels(templateID, hosts, m.Tunnels())
	if err != nil {
		job.Set(step, "failed", firstLine(err.Error()))
		return
	}
	job.Set(step, "ok", fmt.Sprintf("已创建 %d 个入站", len(ports)))
}

// waitUp 等一条隧道跑完 bringUp。bringUp 最多试 6 个候选节点，
// 每个节点等 tun0 最长 40 秒，所以这里给足余量。
func (m *Manager) waitUp(t *Tunnel) {
	const maxWait = 5 * time.Minute
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if t.Status == "up" || t.Status == "failed" || t.Status == "stopped" {
			return
		}
		time.Sleep(time.Second)
	}
}

// pickNodes 按地区挑 count 个还没被占用的节点（向前兼容，供重连与测试使用）。
func (m *Manager) pickNodes(region string, count int) ([]Node, error) {
	return m.pickNodesSource(region, "", count)
}

// pickNodesSource 按地区和节点源挑 count 个还没被占用的优质节点，纯净度与速度优先。
func (m *Manager) pickNodesSource(region string, source string, count int) ([]Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	usedHosts := map[string]bool{}
	usedIPs := map[string]bool{}
	for _, t := range m.tunnels {
		usedHosts[t.Node.HostName] = true
		if t.Node.IP != "" {
			usedIPs[t.Node.IP] = true
		}
	}

	var pool []Node
	for _, n := range m.nodes {
		if usedHosts[n.HostName] || (n.IP != "" && usedIPs[n.IP]) {
			continue
		}
		// 必须具有原生 OpenVPN 隧道配置或 SOCKS5/HTTP 代理能力
		if n.Config == "" && n.Proto != "socks5" && n.Proto != "http" {
			continue
		}
		if !nodeMatchesSource(n, source) {
			continue
		}
		if region != "" && !strings.EqualFold(n.CountryCode, region) && !strings.EqualFold(n.Country, region) {
			continue
		}
		pool = append(pool, n)
	}

	if len(pool) == 0 {
		if region != "" {
			return nil, fmt.Errorf("%s 在所选节点源下暂无可用空闲节点", region)
		}
		return nil, fmt.Errorf("所选节点源下暂无可用空闲节点，建议重新拉取节点")
	}

	// 纯净度与低延迟排序 (政府 > 学术 > 住宅家宽 > 移动 > 其它)
	sort.Slice(pool, func(i, j int) bool {
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
		if pool[i].PurityScore != pool[j].PurityScore {
			return pool[i].PurityScore > pool[j].PurityScore
		}
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
		return pool[i].SpeedMbps > pool[j].SpeedMbps
	})

	if len(pool) > count {
		pool = pool[:count]
	}
	return pool, nil
}

// RegionStat 是某个地区的可用节点概况，用于新建向导里的地区选择。
type RegionStat struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Available   int     `json:"available"`
	BestPing    int     `json:"best_ping"`
	BestSpeed   float64 `json:"best_speed_mbps"`
	Residential int     `json:"residential"`
	AvgPurity   int     `json:"avg_purity"`
}

var popularCountryRank = map[string]int{
	"GLOBAL": 1,
	"JP":     2,  // 日本 (筑波大学官方/优质家宽)
	"US":     3,  // 美国 (原生住宅/学术网)
	"HK":     4,  // 中国香港
	"TW":     5,  // 中国台湾 (TANet/中华电信)
	"SG":     6,  // 新加坡
	"KR":     7,  // 韩国 (KT/高校学术网)
	"GB":     8,  // 英国
	"DE":     9,  // 德国
	"CA":     10, // 加拿大
	"FR":     11, // 法国
	"AU":     12, // 澳大利亚
	"NL":     13, // 荷兰
	"VN":     14, // 越南
	"TH":     15, // 泰国
	"MY":     16, // 马来西亚
	"PH":     17, // 菲律宾
	"ID":     18, // 印尼
	"IN":     19, // 印度
	"RU":     20, // 俄罗斯
	"BR":     21, // 巴西
	"TR":     22, // 土耳其
	"IT":     23, // 意大利
	"ES":     24, // 西班牙
	"CH":     25, // 瑞士
	"SE":     26, // 瑞典
	"NO":     27, // 挪威
	"FI":     28, // 芬兰
	"PL":     29, // 波兰
	"CZ":     30, // 捷克
	"AT":     31, // 奥地利
	"GOV":    32, // 政府公共网络
	"EDU":    33, // 海外高校学术
}

// Regions 汇总各地区还剩多少空闲节点，涵盖全球所有国家，热门地区严格优先前置
func (m *Manager) Regions(source ...string) []RegionStat {
	src := ""
	if len(source) > 0 {
		src = strings.TrimSpace(source[0])
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	used := map[string]bool{}
	for _, t := range m.tunnels {
		used[t.Node.HostName] = true
	}

	type regAccum struct {
		stat      RegionStat
		puritySum int
	}

	byCode := map[string]*regAccum{}
	for _, n := range m.nodes {
		if used[n.HostName] {
			continue
		}
		// 仅统计具有原生 OpenVPN 隧道配置或代理协议的高质量可用节点
		if n.Config == "" && n.Proto != "socks5" && n.Proto != "http" {
			continue
		}
		if src != "" && src != "all" {
			if strings.EqualFold(src, "gov") {
				if n.IPType != "gov" && n.Source != "gov" {
					continue
				}
			} else if strings.EqualFold(src, "edu") {
				if n.Source != "edu" && !isEduIP(n.IP) {
					continue
				}
			} else if strings.EqualFold(src, "residential") {
				if n.IPType != "residential" {
					continue
				}
			} else if !strings.EqualFold(n.Source, src) {
				continue
			}
		}
		cc := strings.ToUpper(strings.TrimSpace(n.CountryCode))
		if cc == "" && n.Country != "" {
			cc = strings.ToUpper(strings.TrimSpace(n.Country))
		}
		if cc == "" {
			cc = "GLOBAL"
		}
		name := n.Country
		if zh, ok := countryNameZH[cc]; ok && zh != "" {
			name = zh
		}
		if name == "" {
			name = cc
		}
		s := byCode[cc]
		if s == nil {
			s = &regAccum{stat: RegionStat{Code: cc, Name: name, BestPing: n.Ping}}
			byCode[cc] = s
		}
		s.stat.Available++
		if n.SpeedMbps > s.stat.BestSpeed {
			s.stat.BestSpeed = n.SpeedMbps
		}
		if n.Ping > 0 && (s.stat.BestPing == 0 || n.Ping < s.stat.BestPing) {
			s.stat.BestPing = n.Ping
		}
		if n.IPType == "residential" {
			s.stat.Residential++
		}
		purity := n.PurityScore
		if purity <= 0 {
			purity = 90
		}
		s.puritySum += purity
	}

	out := make([]RegionStat, 0, len(byCode))
	for _, a := range byCode {
		if a.stat.Available > 0 {
			a.stat.AvgPurity = a.puritySum / a.stat.Available
		}
		out = append(out, a.stat)
	}

	if len(out) == 0 {
		return nil
	}

	sort.Slice(out, func(i, j int) bool {
		r1 := popularCountryRank[out[i].Code]
		r2 := popularCountryRank[out[j].Code]
		if r1 > 0 && r2 > 0 {
			return r1 < r2
		}
		if r1 > 0 {
			return true
		}
		if r2 > 0 {
			return false
		}
		if out[i].Available != out[j].Available {
			return out[i].Available > out[j].Available
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// regionLabel 给出口起一个人能读的名字。
func regionLabel(n Node) string {
	if n.CountryCode != "" {
		return n.CountryCode
	}
	return n.HostName
}

// firstLine 截取错误的第一行，界面里放得下。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// hotCountries 定义核心热门国家列表：各维持 3 个健康可用节点
var hotCountries = map[string]bool{
	"JP": true, "US": true, "HK": true, "TW": true,
	"SG": true, "KR": true, "GB": true, "DE": true,
	"CA": true, "FR": true, "AU": true, "NL": true,
}

func orchestrateSourceAllowed(n Node, sources []string) bool {
	if len(sources) == 0 {
		return true
	}
	for _, raw := range sources {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" || s == "all" {
			return true
		}
		switch s {
		case "vpngate":
			if strings.EqualFold(n.Source, "vpngate") {
				return true
			}
		case "edu":
			if strings.EqualFold(n.Source, "edu") || strings.EqualFold(n.IPType, "edu") || isEduIP(n.IP) {
				return true
			}
		case "gov":
			if strings.EqualFold(n.Source, "gov") || strings.EqualFold(n.IPType, "gov") {
				return true
			}
		case "residential":
			if strings.EqualFold(n.Source, "residential") || strings.EqualFold(n.IPType, "residential") {
				return true
			}
		case "proxy":
			if strings.EqualFold(n.Source, "proxy") || n.Proto == "socks5" || n.Proto == "http" {
				return true
			}
		case "ipspeed", "openvpn":
			if strings.EqualFold(n.Source, "ipspeed") || strings.EqualFold(n.Source, "vpngate") {
				return true
			}
		case "custom":
			if strings.EqualFold(n.Source, "custom") || strings.EqualFold(n.CountryCode, "CUSTOM") {
				return true
			}
		default:
			if strings.EqualFold(n.Source, s) {
				return true
			}
		}
	}
	return false
}

// AutoOrchestrate 全网出口智能自愈与编排：
// 1. 热门国家各维持 3 个健康可用出口；
// 2. 所有发现可用 OpenVPN 节点的冷门国家各维持 1 个健康可用出口；
// 3. 任何发现新可用节点的国家立即自动添加；
// 4. 节点失效后自动换同国；若同国节点耗尽自动删除，待新节点出现时再自动补回；
// 5. 自动绑定 3x-ui / 节点链接，保持所有订阅节点 100% 实时可用！
func (m *Manager) AutoOrchestrate() {
	m.AutoOrchestrateWithOptions(DefaultAutoOrchestrateOptions())
}

// AutoOrchestrateWithOptions 先筛选候选源，再逐个真实建立隧道并等待出网验证。
// 只有 Status=up 的节点才会被绑定入站并进入正式出口池；失败节点进入冷却。
func (m *Manager) AutoOrchestrateWithOptions(opts AutoOrchestrateOptions) {
	if opts.HotTarget < 1 {
		opts.HotTarget = 3
	}
	if opts.ColdTarget < 1 {
		opts.ColdTarget = 1
	}
	if opts.MaxStarts < 1 {
		opts.MaxStarts = 6
	}
	if opts.MaxStarts > 20 {
		opts.MaxStarts = 20
	}
	if opts.VerifyTimeout <= 0 {
		opts.VerifyTimeout = 25 * time.Second
	}
	if len(opts.Sources) == 0 {
		opts.Sources = []string{"all"}
	}
	if !m.orchestrateMu.TryLock() {
		m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
			p.Running = true
			p.Stage = "busy"
			p.Message = "已有智能编排正在运行，当前请求排队跳过"
		})
		return
	}
	m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
		*p = AutoOrchestrateProgress{Running: true, Stage: "prepare", Message: "正在整理出口、节点池与候选源…", StartedAt: time.Now(), UpdatedAt: time.Now()}
	})
	defer func() {
		m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
			p.Running = false
			p.Stage = "done"
			p.FinishedAt = time.Now()
			p.Message = fmt.Sprintf("完成：验证通过 %d 个，新增正式出口 %d 个，失败 %d 个", p.Verified, p.Added, p.Failed)
		})
		m.orchestrateMu.Unlock()
	}()

	tunnels := m.Tunnels()

	// 0. 严格执行 1 出口 = 1 节点：清理 3x-ui / native 面板中的重复入站与废弃孤儿入站
	if p, err := openPanel(); err == nil && p != nil {
		inbounds, _ := p.Inbounds(nil)
		byExitInbounds := make(map[string][]int) // host -> []inboundID
		existingTunnelHosts := make(map[string]bool)
		for _, t := range tunnels {
			existingTunnelHosts[t.Node.HostName] = true
			existingTunnelHosts[sanitizeTag(t.Node.HostName)] = true
			if t.Node.IP != "" {
				existingTunnelHosts[t.Node.IP] = true
			}
			if t.ExitIP != "" {
				existingTunnelHosts[t.ExitIP] = true
			}
		}

		var orphanIDs []int
		for _, ib := range inbounds {
			bTo := strings.TrimSpace(ib.BoundTo)
			if bTo == "" || strings.EqualFold(bTo, "direct") || strings.EqualFold(bTo, "none") {
				continue
			}
			matchedHost := ""
			for _, t := range tunnels {
				if bTo == t.Node.HostName || sanitizeTag(bTo) == sanitizeTag(t.Node.HostName) ||
					(t.Node.IP != "" && bTo == t.Node.IP) || (t.ExitIP != "" && bTo == t.ExitIP) ||
					bTo == fmt.Sprintf("exit-%d", t.Slot) || bTo == fmt.Sprintf("slot-%d", t.Slot) {
					matchedHost = t.Node.HostName
					break
				}
			}
			if matchedHost != "" {
				byExitInbounds[matchedHost] = append(byExitInbounds[matchedHost], ib.ID)
			} else {
				orphanIDs = append(orphanIDs, ib.ID)
			}
		}

		var duplicateIDs []int
		for _, ids := range byExitInbounds {
			if len(ids) > 1 {
				// 严格 1:1，只保留第 1 个正常入站，多余的全部删除
				duplicateIDs = append(duplicateIDs, ids[1:]...)
			}
		}
		toDelete := append(duplicateIDs, orphanIDs...)
		if len(toDelete) > 0 {
			log.Printf("[1出口1节点] 正在清理面板中 %d 个冗余/孤儿入站...", len(toDelete))
			_ = p.DeleteInbounds(toDelete, tunnels)
			invalidateInbounds()
		}
	}

	// 1. 统计当前运行中或启动中的各国家出口数量并收集按国家分组的隧道
	activeTunnelsByCountry := make(map[string][]*Tunnel)
	usedHosts := make(map[string]bool, len(tunnels))
	usedIPs := make(map[string]bool, len(tunnels))
	for _, t := range tunnels {
		if t.Status == "up" || t.Status == "starting" {
			c := strings.ToUpper(strings.TrimSpace(t.TargetRegion))
			if c == "" {
				c = strings.ToUpper(strings.TrimSpace(t.Node.CountryCode))
			}
			if c != "" {
				activeTunnelsByCountry[c] = append(activeTunnelsByCountry[c], t)
			}
			usedHosts[t.Node.HostName] = true
			if t.Node.IP != "" {
				usedIPs[t.Node.IP] = true
			}
		}
	}

	// 2. 超额出口自动精简：热门国家严格限制最多 3 个，冷门国家严格限制最多 1 个
	for cc, tList := range activeTunnelsByCountry {
		target := opts.ColdTarget
		if hotCountries[cc] {
			target = opts.HotTarget
		}
		if len(tList) > target {
			// 排序保留最优质/最稳定运行的出口，剔除超额出口
			sort.Slice(tList, func(i, j int) bool {
				if (tList[i].Status == "up") != (tList[j].Status == "up") {
					return tList[i].Status == "up"
				}
				return tList[i].Since.Before(tList[j].Since)
			})
			for idx := target; idx < len(tList); idx++ {
				excess := tList[idx]
				log.Printf("[配额精简] 国家 %s 运行出口数 (%d) 超过目标配额 (%d)，自动移除超额出口 槽位 %d (%s)...", cc, len(tList), target, excess.Slot, excess.Node.HostName)
				_ = m.Stop(excess.Slot)
				delete(usedHosts, excess.Node.HostName)
				if excess.Node.IP != "" {
					delete(usedIPs, excess.Node.IP)
				}
			}
			activeTunnelsByCountry[cc] = tList[:target]
		}
	}

	m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
		p.Stage = "collect"
		p.Message = "正在整理候选节点，只处理实际存在的国家…"
	})

	// 3. 汇集节点池中所有国家（仅限真实可用 OpenVPN 节点）
	m.mu.RLock()
	allNodes := make([]Node, len(m.nodes))
	copy(allNodes, m.nodes)
	m.mu.RUnlock()

	countryCandidates := make(map[string][]Node)
	for _, n := range allNodes {
		if strings.TrimSpace(n.Config) == "" && n.Proto != "socks5" && n.Proto != "http" {
			continue
		}
		cc := strings.ToUpper(strings.TrimSpace(n.CountryCode))
		if cc == "" || cc == "GLOBAL" || cc == "CUSTOM" {
			continue
		}
		if !orchestrateSourceAllowed(n, opts.Sources) {
			continue
		}
		if usedHosts[n.HostName] || (n.IP != "" && usedIPs[n.IP]) {
			continue
		}
		countryCandidates[cc] = append(countryCandidates[cc], n)
	}

	candidateTotal := 0
	for _, list := range countryCandidates {
		candidateTotal += len(list)
	}
	m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
		p.CountriesTotal = len(countryCandidates)
		p.CandidatesTotal = candidateTotal
		p.Stage = "probe"
		p.Message = fmt.Sprintf("已找到 %d 个国家、%d 个候选，开始逐个测活…", len(countryCandidates), candidateTotal)
	})

	// 4. 收集并按热门程度与可用节点数排序国家列表
	allCountries := make(map[string]bool)
	for cc := range countryCandidates {
		allCountries[cc] = true
	}

	var sortedCountries []string
	for cc := range allCountries {
		sortedCountries = append(sortedCountries, cc)
	}
	sort.Slice(sortedCountries, func(i, j int) bool {
		r1 := popularCountryRank[sortedCountries[i]]
		r2 := popularCountryRank[sortedCountries[j]]
		if r1 > 0 && r2 > 0 {
			return r1 < r2
		}
		if r1 > 0 {
			return true
		}
		if r2 > 0 {
			return false
		}
		return len(countryCandidates[sortedCountries[i]]) > len(countryCandidates[sortedCountries[j]])
	})

	// 5. 为节点池中实际存在的国家补齐配额。
	// 关键规则：候选节点不会因为“看起来可用”就直接进入正式出口，
	// 必须逐个启动并完成真实出网验证；失败立即停止并冷却。
	// 5. 为节点池中实际存在的国家补齐配额。
	// 先并发做一轮真实测活，再对通过的少量候选建立隧道。这样“测活”不再
	// 一个节点一个节点串行等待，尤其是 VPN Gate 大量死节点时速度差异很大。
	var verifiedStarted []*Tunnel
	startsUsed := 0
	for _, cc := range sortedCountries {
		m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
			p.CurrentCountry = cc
			p.Stage = "probe"
			p.Message = fmt.Sprintf("正在处理 %s：目标 %d 个", cc, func() int {
				if hotCountries[cc] {
					return opts.HotTarget
				}
				return opts.ColdTarget
			}())
		})
		target := opts.ColdTarget
		if hotCountries[cc] {
			target = opts.HotTarget
		}
		curCount := len(activeTunnelsByCountry[cc])
		needed := target - curCount
		if needed <= 0 || startsUsed >= opts.MaxStarts {
			continue
		}

		cands := append([]Node(nil), countryCandidates[cc]...)
		filtered := cands[:0]
		for _, c := range cands {
			if !m.nodeCooling(c) {
				filtered = append(filtered, c)
			}
		}
		cands = filtered
		if len(cands) == 0 {
			continue
		}

		sort.SliceStable(cands, func(i, j int) bool {
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
			r1, r2 := typeRank(cands[i].IPType), typeRank(cands[j].IPType)
			if r1 != r2 {
				return r1 > r2
			}
			if cands[i].PurityScore != cands[j].PurityScore {
				return cands[i].PurityScore > cands[j].PurityScore
			}
			p1, p2 := cands[i].Ping, cands[j].Ping
			if p1 <= 0 {
				p1 = 9999
			}
			if p2 <= 0 {
				p2 = 9999
			}
			if p1 != p2 {
				return p1 < p2
			}
			return cands[i].SpeedMbps > cands[j].SpeedMbps
		})

		probeCount := needed * 4
		if probeCount < 8 {
			probeCount = 8
		}
		if probeCount > 24 {
			probeCount = 24
		}
		if len(cands) > probeCount {
			cands = cands[:probeCount]
		}
		m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
			p.CandidatesTested += len(cands)
			p.Message = fmt.Sprintf("%s 并发实测 %d 个候选，剔除无握手/无回包节点…", cc, len(cands))
		})

		live := probeNodesLive(cands, 1200*time.Millisecond)
		liveSet := make(map[string]bool, len(live))
		for _, n := range live {
			liveSet[nodeKey(n)] = true
		}
		for _, n := range cands {
			if !liveSet[nodeKey(n)] {
				m.markNodeFailed(n)
			}
		}
		if len(cands) > len(live) {
			failed := len(cands) - len(live)
			m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
				p.Failed += failed
				p.LastError = fmt.Sprintf("%s：%d 个候选未通过真实测活", cc, failed)
			})
		}
		m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
			p.LivePassed += len(live)
			p.Stage = "start"
			p.Message = fmt.Sprintf("%s 测活通过 %d 个，开始建立正式隧道…", cc, len(live))
		})

		for _, pick := range live {
			if needed <= 0 || startsUsed >= opts.MaxStarts {
				break
			}
			pick.Ping = int(pick.Ping)
			m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
				p.CurrentNode = pick.HostName
				p.Stage = "start"
				p.Message = fmt.Sprintf("%s 测活通过，正在建立真实隧道…", pick.HostName)
			})
			t, err := m.StartExact(pick)
			if err != nil {
				m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) { p.Failed++; p.LastError = firstLine(fmt.Sprint(err)) })
				m.markNodeFailed(pick)
				log.Printf("[智能编排] 启动失败，跳过 %s (%s): %v", pick.HostName, cc, err)
				continue
			}
			startsUsed++
			m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
				p.Started++
				p.Stage = "verify"
				p.Message = fmt.Sprintf("%s 隧道已启动，等待真实出口 IP…", pick.HostName)
			})

			deadline := time.Now().Add(opts.VerifyTimeout)
			for time.Now().Before(deadline) && t.Status == "starting" {
				time.Sleep(250 * time.Millisecond)
			}
			if t.Status != "up" || strings.TrimSpace(t.ExitIP) == "" {
				m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
					p.Failed++
					p.LastError = firstLine(t.Err)
					p.Message = fmt.Sprintf("%s 最终出网验证失败，已移除", pick.HostName)
				})
				m.markNodeFailed(pick)
				log.Printf("[智能编排] 最终出网验证失败，移除 %s (%s): %s", pick.HostName, cc, firstLine(t.Err))
				_ = m.Stop(t.Slot)
				continue
			}

			activeTunnelsByCountry[cc] = append(activeTunnelsByCountry[cc], t)
			usedHosts[t.Node.HostName] = true
			if t.Node.IP != "" {
				usedIPs[t.Node.IP] = true
			}
			verifiedStarted = append(verifiedStarted, t)
			needed--
			m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
				p.Verified++
				p.Added++
				p.Message = fmt.Sprintf("%s 验证通过：%s，加入正式出口并准备绑定节点", cc, t.ExitIP)
			})
			log.Printf("[智能编排] %s 验证通过并加入正式出口: %s -> %s (%dms)", cc, t.Node.HostName, t.ExitIP, pick.Ping)
		}
	}

	if len(verifiedStarted) == 0 {
		m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
			p.Stage = "done"
			if p.CandidatesTotal == 0 {
				p.Message = "本轮没有符合当前源筛选条件的候选节点，请先刷新节点源或放宽源筛选。"
			} else if p.LivePassed == 0 {
				p.Message = fmt.Sprintf("本轮验证了 %d 个候选，但没有一个通过测活。失败节点已进入冷却，不会加入出口。", p.CandidatesTested)
			} else {
				p.Message = fmt.Sprintf("有 %d 个候选测活通过，但真实出网验证没有形成正式出口。", p.LivePassed)
			}
		})
	}

	if len(verifiedStarted) > 0 {
		log.Printf("[全网智能编排] 本轮仅加入 %d 个已实测出网的正式出口 (热门%d/冷门%d，最大补位%d)", len(verifiedStarted), opts.HotTarget, opts.ColdTarget, opts.MaxStarts)
	}

	m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
		p.Stage = "bind"
		p.Message = "出口验证完成，正在逐个绑定节点并检查服务器直连节点…"
	})
	// 出口启动与入站绑定严格分离：先确保所有 UP 出口都有节点，再保证本机直连节点存在。
	m.reconcilePanelBindings()

}

// reconcilePanelBindings 保证两件事始终成立：
// 1. 至少保留一个“服务器直连”入站，它不绑定任何出口，流量直接走本机公网；
// 2. 每一条已真实验证为 UP 的出口，都必须有且只有一个对应入站。
//
// 这里故意按单出口串行补绑，而不是一次性批量调用，避免自建 Xray 在重建配置时
// 与多个异步 CloneToTunnels 互相覆盖，造成“出口已经 UP、面板却显示无节点”。
func (m *Manager) reconcilePanelBindings() {
	m.bindingMu.Lock()
	defer m.bindingMu.Unlock()

	p, err := openPanel()
	if err != nil || p == nil {
		if err != nil {
			log.Printf("[节点绑定] 打开面板后端失败: %v", err)
		}
		return
	}

	tunnels := m.Tunnels()
	inbounds, err := p.Inbounds(nil)
	if err != nil {
		log.Printf("[节点绑定] 读取入站失败: %v", err)
		return
	}

	// 没有任何未绑定入站时，自动创建一个“服务器直连 · 本机”节点。
	// 已存在的未绑定入站则直接把其中一个明确标记为服务器直连，不重复制造垃圾节点。
	directFound := false
	for _, ib := range inbounds {
		if strings.TrimSpace(ib.BoundTo) != "" && !strings.EqualFold(strings.TrimSpace(ib.BoundTo), "direct") && !strings.EqualFold(strings.TrimSpace(ib.BoundTo), "none") {
			continue
		}
		if strings.Contains(strings.ToLower(ib.Remark), "服务器直连") || strings.Contains(strings.ToLower(ib.Remark), "server-direct") {
			directFound = true
			break
		}
	}
	if !directFound {
		var candidate *Inbound
		for i := range inbounds {
			ib := &inbounds[i]
			b := strings.TrimSpace(ib.BoundTo)
			if b == "" || strings.EqualFold(b, "direct") || strings.EqualFold(b, "none") {
				candidate = ib
				break
			}
		}
		if candidate != nil {
			remark := "服务器直连 · 本机"
			if err := p.UpdateInbound(candidate.ID, InboundPatch{Remark: &remark}, tunnels); err != nil {
				log.Printf("[节点绑定] 标记服务器直连节点失败: %v", err)
			} else {
				directFound = true
				log.Printf("[节点绑定] 已将入站 %d 标记为服务器直连 · 本机", candidate.ID)
			}
		} else {
			// 使用 VLESS + TCP + REALITY，作为真正的“本机直连”节点：不绑定任何 VPN 出口。
			ib, err := p.CreateInbound(NewInboundSpec{
				Protocol:    "vless",
				Network:     "tcp",
				Security:    "reality",
				Remark:      "服务器直连 · 本机",
				Dest:        "www.microsoft.com:443",
				ServerNames: "www.microsoft.com",
				Fingerprint: "chrome",
			}, tunnels)
			if err != nil {
				log.Printf("[节点绑定] 创建服务器直连节点失败: %v", err)
			} else if ib != nil {
				directFound = true
				log.Printf("[节点绑定] 已创建服务器直连 · 本机 :%d（不走任何出口）", ib.Port)
			}
		}
	}

	// 重新读取一次，确保刚创建/改名的直连节点已进入最新状态。
	inbounds, _ = p.Inbounds(nil)
	bound := make(map[string]bool)
	for _, ib := range inbounds {
		b := strings.TrimSpace(ib.BoundTo)
		if b == "" || strings.EqualFold(b, "direct") || strings.EqualFold(b, "none") {
			continue
		}
		bound[b] = true
		bound[sanitizeTag(b)] = true
	}

	for _, t := range tunnels {
		if t == nil || t.Status != "up" {
			continue
		}
		host := t.Node.HostName
		if bound[host] || bound[sanitizeTag(host)] {
			continue
		}
		var lastErr error
		for attempt := 1; attempt <= 3; attempt++ {
			if _, err := cloneTemplateToTunnels(0, []string{host}, tunnels); err == nil {
				bound[host] = true
				bound[sanitizeTag(host)] = true
				log.Printf("[节点绑定] 出口 %s 已自动创建并绑定入站（第 %d 次尝试）", host, attempt)
				break
			} else {
				lastErr = err
				time.Sleep(time.Second)
			}
		}
		if lastErr != nil && !bound[host] && !bound[sanitizeTag(host)] {
			log.Printf("[节点绑定] 出口 %s 自动绑定失败，稍后继续重试: %v", host, lastErr)
		}
	}
	invalidateInbounds()
}

// WatchAutoOrchestrate 守护线程：定期巡检全网国家出口配额与自愈，并每 30 分钟定时拉取全网最新节点源
func (m *Manager) WatchAutoOrchestrate() {
	time.Sleep(10 * time.Second)
	m.AutoOrchestrate()

	// 1. 每 30 秒补一次“UP 出口 -> 入站”绑定，避免异步启动时序导致面板短暂显示“无节点”。
	bindingTicker := time.NewTicker(30 * time.Second)
	// 2. 每 2 分钟做一次出口配额快速巡检与健康自愈维护（稳定运行的出口绝不触动）
	orchestrateTicker := time.NewTicker(2 * time.Minute)
	// 3. 每 15 分钟定时拉取一次全网最新节点源（自动发现新国家直接添加，离线恢复的国家自动补齐）
	sourceTicker := time.NewTicker(15 * time.Minute)

	defer bindingTicker.Stop()
	defer orchestrateTicker.Stop()
	defer sourceTicker.Stop()

	for {
		select {
		case <-bindingTicker.C:
			m.reconcilePanelBindings()
		case <-orchestrateTicker.C:
			m.AutoOrchestrate()
		case <-sourceTicker.C:
			log.Printf("[自动拉源] 定时拉取全网最新节点源 (15 分钟周期)...")
			_, _ = m.RefreshNodesSource("all")
			m.AutoOrchestrate()
		}
	}
}
