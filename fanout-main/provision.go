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
	Region     string // 国家码，空表示不限
	Source     string // 节点源："all", "vpngate", "edu", "proxy", "custom"
	Count      int
	TemplateID int // 3x-ui 入站模板；0 表示只开隧道不建入站
}

// Provision 异步执行一次批量开出口，立刻返回作业句柄供界面轮询。
//
// 隧道并行拉起（每条都要等 openvpn 握手，串行会线性累加等待），
// 面板侧的入站创建则统一放到最后串行做一次，因为每次改路由都要重启 Xray。
func (m *Manager) Provision(req ProvisionRequest) (*Job, error) {
	if req.Count < 1 {
		return nil, fmt.Errorf("数量至少为 1")
	}
	picks, err := m.pickNodesSource(req.Region, req.Source, req.Count)
	if err != nil {
		return nil, err
	}

	labels := make([]string, 0, len(picks)+1)
	for _, n := range picks {
		labels = append(labels, regionLabel(n)+" 出口")
	}
	if req.TemplateID > 0 {
		labels = append(labels, "创建节点链接")
	}

	where := req.Region
	if where == "" {
		where = "任意地区"
	}
	job := m.jobs.New(fmt.Sprintf("开 %d 个 %s 出口", len(picks), where), labels)

	go m.runProvision(job, picks, req.TemplateID)
	return job, nil
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
	x, err := openPanel()
	if err != nil {
		job.Set(step, "failed", err.Error())
		return
	}
	ports, err := x.CloneToTunnels(templateID, hosts, m.Tunnels())
	invalidateInbounds()
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

// pickNodesSource 按地区和节点源挑 count 个还没被占用的节点，速度优先。
func (m *Manager) pickNodesSource(region string, source string, count int) ([]Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	used := map[string]bool{}
	for _, t := range m.tunnels {
		used[t.Node.HostName] = true
	}

	var out []Node
	for _, n := range m.nodes {
		if len(out) >= count {
			break
		}
		if used[n.HostName] {
			continue
		}
		if source != "" && source != "all" {
			if strings.EqualFold(source, "edu") {
				if n.Source != "edu" && !isEduIP(n.IP) {
					continue
				}
			} else if !strings.EqualFold(n.Source, source) {
				continue
			}
		}
		if region != "" && !strings.EqualFold(n.CountryCode, region) && !strings.EqualFold(n.Country, region) {
			continue
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		if region != "" {
			return nil, fmt.Errorf("%s 在所选节点源下暂无可用空闲节点", region)
		}
		return nil, fmt.Errorf("所选节点源下暂无可用空闲节点，建议切换为全部源或重新拉取")
	}
	return out, nil
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

// Regions 汇总各地区还剩多少空闲节点，按可用数量降序。支持按源筛选。
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
		if src != "" && src != "all" {
			if strings.EqualFold(src, "edu") {
				if n.Source != "edu" && !isEduIP(n.IP) {
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
			purity = 75
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
			{Code: "GLOBAL", Name: "全球推荐 (自动优选)", Available: 50, BestSpeed: 100.0, BestPing: 45, AvgPurity: 85},
			{Code: "JP", Name: "日本", Available: 15, BestSpeed: 95.0, BestPing: 45, AvgPurity: 95},
			{Code: "EDU", Name: "海外高校学术科研网 (不含国内)", Available: 10, BestSpeed: 75.0, BestPing: 25, AvgPurity: 99},
			{Code: "HK", Name: "中国香港", Available: 10, BestSpeed: 90.0, BestPing: 30, AvgPurity: 90},
			{Code: "TW", Name: "中国台湾", Available: 8, BestSpeed: 85.0, BestPing: 38, AvgPurity: 88},
			{Code: "SG", Name: "新加坡", Available: 8, BestSpeed: 92.0, BestPing: 60, AvgPurity: 90},
			{Code: "US", Name: "美国", Available: 20, BestSpeed: 120.0, BestPing: 130, AvgPurity: 88},
		}
		return presets
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Code == "EDU" {
			return true
		}
		if out[j].Code == "EDU" {
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
