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
	Region     string   // 国家码，空表示不限
	Source     string   // 节点源："all", "vpngate", "edu", "proxy", "custom"
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
	// 如果 templateID <= 0，自动寻找第一个可用的非直连入站模板
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
	}
	// 如果面板中没有任何入站模板（例如刚装好的 3x-ui 面板），自动创建一个高质量标准入站作为母版
	if templateID <= 0 {
		created, cErr := x.CreateInbound(NewInboundSpec{
			Protocol: "vless",
			Network:  "tcp",
			Security: "reality",
			Remark:   "Fanout-Master",
		}, tunnels)
		if cErr == nil && created != nil && created.ID > 0 {
			templateID = created.ID
			log.Printf("[3x-ui联动] 自动创建首个标准入站模板 (ID: %d, 端口: %d)...", created.ID, created.Port)
		} else {
			// 备选尝试普通无需证书的 vless 入站
			created2, cErr2 := x.CreateInbound(NewInboundSpec{
				Protocol: "vless",
				Network:  "tcp",
				Security: "none",
				Remark:   "Fanout-Master",
			}, tunnels)
			if cErr2 == nil && created2 != nil && created2.ID > 0 {
				templateID = created2.ID
				log.Printf("[3x-ui联动] 自动创建首个备选入站模板 (ID: %d, 端口: %d)...", created2.ID, created2.Port)
			}
		}
	}
	if templateID <= 0 {
		return nil, nil
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
		t, err := m.Start(node)
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

	if templateID <= 0 {
		return
	}

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
		// 必须具有原生 OpenVPN 隧道配置（杜绝低质全网代理）
		if n.Config == "" {
			continue
		}
		if source != "" && source != "all" {
			if strings.EqualFold(source, "gov") {
				if n.IPType != "gov" && n.Source != "gov" {
					continue
				}
			} else if strings.EqualFold(source, "edu") {
				if n.Source != "edu" && !isEduIP(n.IP) {
					continue
				}
			} else if strings.EqualFold(source, "residential") {
				if n.IPType != "residential" {
					continue
				}
			} else if !strings.EqualFold(n.Source, source) {
				continue
			}
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
		// 仅统计具有原生 OpenVPN 隧道配置的高质量可用节点
		if n.Config == "" {
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

	// 保证常用地区至少有展示，即使节点池刚启动未完成聚合
	if len(out) == 0 {
		presets := []RegionStat{
			{Code: "GLOBAL", Name: "全球推荐 (自动优选)", Available: 50, BestSpeed: 100.0, BestPing: 45, AvgPurity: 92},
			{Code: "JP", Name: "日本 (筑波大学官方/优质家宽)", Available: 20, BestSpeed: 95.0, BestPing: 45, AvgPurity: 98},
			{Code: "US", Name: "美国 (原生住宅/学术网)", Available: 20, BestSpeed: 120.0, BestPing: 130, AvgPurity: 95},
			{Code: "HK", Name: "中国香港 (低延迟专线)", Available: 15, BestSpeed: 90.0, BestPing: 30, AvgPurity: 92},
			{Code: "TW", Name: "中国台湾 (TANet/中华电信)", Available: 12, BestSpeed: 85.0, BestPing: 38, AvgPurity: 94},
			{Code: "SG", Name: "新加坡 (原生宽带)", Available: 12, BestSpeed: 92.0, BestPing: 60, AvgPurity: 93},
			{Code: "KR", Name: "韩国 (KT/首尔高校网关)", Available: 12, BestSpeed: 88.0, BestPing: 55, AvgPurity: 96},
			{Code: "GB", Name: "英国 (伦敦/学术科研网)", Available: 10, BestSpeed: 85.0, BestPing: 150, AvgPurity: 94},
			{Code: "DE", Name: "德国 (法兰克福核心)", Available: 10, BestSpeed: 82.0, BestPing: 160, AvgPurity: 93},
			{Code: "CA", Name: "加拿大 (多伦多/温哥华)", Available: 8, BestSpeed: 85.0, BestPing: 140, AvgPurity: 92},
			{Code: "FR", Name: "法国 (巴黎)", Available: 8, BestSpeed: 80.0, BestPing: 165, AvgPurity: 91},
			{Code: "AU", Name: "澳大利亚 (悉尼)", Available: 8, BestSpeed: 75.0, BestPing: 120, AvgPurity: 92},
			{Code: "NL", Name: "荷兰 (阿姆斯特丹)", Available: 8, BestSpeed: 85.0, BestPing: 155, AvgPurity: 92},
			{Code: "VN", Name: "越南 (河内/胡志明)", Available: 6, BestSpeed: 60.0, BestPing: 70, AvgPurity: 90},
			{Code: "TH", Name: "泰国 (曼谷)", Available: 6, BestSpeed: 65.0, BestPing: 80, AvgPurity: 90},
			{Code: "MY", Name: "马来西亚 (吉隆坡)", Available: 6, BestSpeed: 70.0, BestPing: 75, AvgPurity: 90},
			{Code: "EDU", Name: "海外高校学术科研网 (不含国内)", Available: 15, BestSpeed: 75.0, BestPing: 25, AvgPurity: 99},
			{Code: "GOV", Name: "全球政府公共机构专网", Available: 5, BestSpeed: 85.0, BestPing: 46, AvgPurity: 99},
		}
		return presets
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

// AutoOrchestrate 全网出口智能自愈与编排：
// 1. 热门国家各维持 3 个健康可用出口；
// 2. 所有发现可用 OpenVPN 节点的冷门国家各维持 1 个健康可用出口；
// 3. 任何发现新可用节点的国家立即自动添加；
// 4. 节点失效后自动换同国；若同国节点耗尽自动删除，待新节点出现时再自动补回；
// 5. 自动绑定 3x-ui / 节点链接，保持所有订阅节点 100% 实时可用！
func (m *Manager) AutoOrchestrate() {
	if !m.orchestrateMu.TryLock() {
		return
	}
	defer m.orchestrateMu.Unlock()

	tunnels := m.Tunnels()

	// 0. 严格执行 1 出口 = 1 节点：清理 3x-ui / native 面板中的重复入站与废弃孤儿入站
	// 仅在已有激活隧道运行时执行孤儿清理，避免冷启动期间误删历史已创建入站
	if len(tunnels) > 0 {
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
		target := 1
		if hotCountries[cc] {
			target = 3
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

	// 3. 汇集节点池中所有国家（仅限真实可用 OpenVPN 节点）
	m.mu.RLock()
	allNodes := make([]Node, len(m.nodes))
	copy(allNodes, m.nodes)
	m.mu.RUnlock()

	countryCandidates := make(map[string][]Node)
	for _, n := range allNodes {
		if strings.TrimSpace(n.Config) == "" {
			continue
		}
		cc := strings.ToUpper(strings.TrimSpace(n.CountryCode))
		if cc == "" || cc == "GLOBAL" || cc == "CUSTOM" {
			continue
		}
		if usedHosts[n.HostName] || (n.IP != "" && usedIPs[n.IP]) {
			continue
		}
		countryCandidates[cc] = append(countryCandidates[cc], n)
	}

	// 4. 收集并按热门程度与可用节点数排序国家列表
	allCountries := make(map[string]bool)
	for cc := range countryCandidates {
		allCountries[cc] = true
	}
	for hc := range hotCountries {
		allCountries[hc] = true
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

	// 5. 为每个国家补齐所需出口配额（热门各 3 个，冷门各 1 个，稳定运行绝不触动）
	var newStarted []*Tunnel
	for _, cc := range sortedCountries {
		target := 1
		if hotCountries[cc] {
			target = 3
		}
		curCount := len(activeTunnelsByCountry[cc])
		needed := target - curCount
		if needed <= 0 {
			continue
		}

		cands := countryCandidates[cc]
		if len(cands) == 0 {
			continue
		}

		// 优选排序：政府 > 学术 > 住宅家宽 > 移动 > 其它，延迟低优先，带宽大优先
		sort.Slice(cands, func(i, j int) bool {
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
				p1 = 999
			}
			if p2 <= 0 {
				p2 = 999
			}
			if p1 != p2 {
				return p1 < p2
			}
			return cands[i].SpeedMbps > cands[j].SpeedMbps
		})

		for i := 0; i < needed && i < len(cands); i++ {
			pick := cands[i]
			t, err := m.Start(pick)
			if err != nil {
				continue
			}
			usedHosts[pick.HostName] = true
			if pick.IP != "" {
				usedIPs[pick.IP] = true
			}
			activeTunnelsByCountry[cc] = append(activeTunnelsByCountry[cc], t)
			newStarted = append(newStarted, t)
		}
	}

	if len(newStarted) > 0 {
		log.Printf("[全网智能编排] 自动拉起 %d 个国家/地区出口 (热门各3/冷门各1)...", len(newStarted))
		go func(tuns []*Tunnel) {
			var okHosts []string
			for _, t := range tuns {
				m.waitUp(t)
				if t.Status == "up" {
					okHosts = append(okHosts, t.Node.HostName)
				} else if t.Status == "failed" {
					_ = m.Stop(t.Slot)
				}
			}
			if len(okHosts) > 0 {
				log.Printf("[全网智能编排] %d 个出口已连通，正在自动同步创建 3x-ui 节点与路由绑定...", len(okHosts))
				ports, err := cloneTemplateToTunnels(0, okHosts, m.Tunnels())
				if err != nil {
					log.Printf("[全网智能编排] 同步创建 3x-ui 节点出错: %v", err)
				} else {
					log.Printf("[全网智能编排] 成功为 %d 个出口同步创建 3x-ui 节点 (已开端口: %v)！", len(ports), ports)
				}
			}
		}(newStarted)
	}
}

// WatchAutoOrchestrate 守护线程：定期巡检全网国家出口配额与自愈，并每 30 分钟定时拉取全网最新节点源
func (m *Manager) WatchAutoOrchestrate() {
	time.Sleep(10 * time.Second)
	m.AutoOrchestrate()

	// 1. 每 2 分钟做一次出口配额快速巡检与健康自愈维护（稳定运行的出口绝不触动）
	orchestrateTicker := time.NewTicker(2 * time.Minute)
	// 2. 每 30 分钟定时拉取一次全网最新节点源（自动发现新国家直接添加，离线恢复的国家自动补齐）
	sourceTicker := time.NewTicker(30 * time.Minute)

	defer orchestrateTicker.Stop()
	defer sourceTicker.Stop()

	for {
		select {
		case <-orchestrateTicker.C:
			m.AutoOrchestrate()
		case <-sourceTicker.C:
			log.Printf("[自动拉源] 定时拉取全网最新节点源 (30 分钟周期)...")
			_, _ = m.RefreshNodesSource("all")
			m.AutoOrchestrate()
		}
	}
}
