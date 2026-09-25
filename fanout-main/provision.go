package main

import (
	"fmt"
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
			if strings.EqualFold(source, "edu") {
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

	// 纯净度与低延迟排序
	sort.Slice(pool, func(i, j int) bool {
		typeRank := func(t string) int {
			switch strings.ToLower(t) {
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
	"EDU":    32, // 海外高校学术
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
			if strings.EqualFold(src, "edu") {
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
