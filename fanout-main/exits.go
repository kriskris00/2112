package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ExitInbound 是挂在某个出口上的一个 3x-ui 入站。
type ExitInbound struct {
	ID       int    `json:"id"`
	Port     int    `json:"port"`
	Remark   string `json:"remark"`
	Protocol string `json:"protocol"`
	Enable   bool   `json:"enable"`
	Tag      string `json:"tag"`
}

// Exit 是界面上的一行：一条隧道加上挂在它出口的所有入站。
// 用户脑子里的单位是"一个出口"，不是"一条隧道"和"一个入站"两样东西。
type Exit struct {
	Slot    int       `json:"slot"`
	Port    int       `json:"port"` // SOCKS5 端口
	Host    string    `json:"host"`
	Region  string    `json:"region"`
	Country string    `json:"country"`
	ExitIP  string    `json:"exit_ip"`
	Status  string    `json:"status"`
	Err     string    `json:"err,omitempty"`
	Since   time.Time `json:"since"`
	// SOCKS5 凭据：界面要能看、能复制、能改
	SocksUser string        `json:"socks_user"`
	SocksPass string        `json:"socks_pass"`
	Inbounds  []ExitInbound `json:"inbounds"`

	IPType      string `json:"ip_type,omitempty"`
	PurityScore int    `json:"purity_score,omitempty"`
	ISP         string `json:"isp,omitempty"`
}

// ExitsView 是主界面需要的全部数据。
type ExitsView struct {
	Exits []Exit `json:"exits"`
	// Direct 是没绑到任何出口的入站，仍然要能看见，否则用户会以为它们不见了
	Direct []ExitInbound `json:"direct"`
	Panel  string        `json:"panel"` // 面板不可用时的原因，空表示正常
	// Backend 是 "3x-ui" 或 "native"。界面据此决定是否提供新建入站入口：
	// 接管面板时入站归面板管，自建模式才由 fanout 自己建。
	Backend string `json:"backend"`
	// PanelInfo 是后端的一行说明，显示在标题旁
	PanelInfo string `json:"panel_info"`
	// PublicIP 是母机公网 IPv4，前端用它当 SOCKS5/分享链接的连接地址
	PublicIP string `json:"public_ip"`
}

// inboundCache 给入站列表做很短的缓存。界面每几秒轮询一次，
// 而每次读入站都要顺带解析一遍完整的 Xray 配置，没必要每次都真的去问面板。
type inboundCache struct {
	mu   sync.Mutex
	at   time.Time
	list []Inbound
	err  error
}

const inboundCacheTTL = 2500 * time.Millisecond

var ibCache inboundCache

func cachedInbounds(live map[string]bool) ([]Inbound, error) {
	ibCache.mu.Lock()
	defer ibCache.mu.Unlock()
	if time.Since(ibCache.at) < inboundCacheTTL && len(ibCache.list) > 0 && ibCache.err == nil {
		return ibCache.list, ibCache.err
	}

	var list []Inbound
	x, err := openPanel()
	if err == nil {
		list, err = x.Inbounds(live)
	}
	if err != nil && len(ibCache.list) > 0 {
		// 面板调用发生瞬时错误时，优先复用上一次成功的入站缓存，防止界面闪烁或清空入站
		return ibCache.list, nil
	}
	ibCache.at, ibCache.list, ibCache.err = time.Now(), list, err
	return list, err
}

// invalidateInbounds 在写操作之后调用，让下一次读立刻反映改动。
func invalidateInbounds() {
	ibCache.mu.Lock()
	ibCache.at = time.Time{}
	ibCache.mu.Unlock()
	clearInboundDetailCache()
}

// ExitsOf 把隧道和入站 join 成界面直接可用的形态。
func (m *Manager) ExitsOf() ExitsView {
	tunnels := m.Tunnels()
	view := ExitsView{Exits: make([]Exit, 0, len(tunnels)), PublicIP: hostPublicIP()}

	// 先填后端类型：入站读取失败时界面仍要知道当前是哪种模式
	if p, err := openPanel(); err == nil {
		view.Backend = p.Kind()
		view.PanelInfo = p.Describe()
	}

	live := map[string]bool{}
	for _, t := range tunnels {
		if t.Status == "up" {
			live[sanitizeTag(t.Node.HostName)] = true
		}
	}

	byHost := map[string]int{}
	for i, t := range tunnels {
		byHost[sanitizeTag(t.Node.HostName)] = i
		byHost[t.Node.HostName] = i
		if t.ExitIP != "" {
			byHost[t.ExitIP] = i
		}
		if t.Node.IP != "" {
			byHost[t.Node.IP] = i
		}
		byHost[fmt.Sprintf("exit-%d", t.Slot)] = i
		byHost[fmt.Sprintf("slot-%d", t.Slot)] = i
		cred := t.credential()
		intel := GetIPIntel(t.ExitIP)
		if (intel.IPType == "" || intel.IPType == "hosting" && intel.ISP == "Unknown") && t.Node.IP != "" {
			nodeIntel := GetIPIntel(t.Node.IP)
			if nodeIntel.ISP != "Unknown" {
				intel = nodeIntel
			}
		}
		region := t.Node.CountryCode
		country := t.Node.Country
		isp := t.Node.ISP
		if intel.ISP != "" && intel.ISP != "Unknown" && !strings.EqualFold(intel.ISP, "Public Proxy") {
			isp = intel.ISP
		}
		if intel.CountryCode != "" && intel.CountryCode != "GLOBAL" {
			region = intel.CountryCode
			country = intel.Country
		}
		if isp == "" || strings.EqualFold(isp, "Public Proxy") {
			isp = "优质网络"
		}
		view.Exits = append(view.Exits, Exit{
			Slot: t.Slot, Port: t.Port, Host: t.Node.HostName,
			Region: region, Country: country,
			ExitIP: t.ExitIP, Status: t.Status, Err: t.Err, Since: t.Since,
			SocksUser: cred.User, SocksPass: cred.Pass,
			IPType: intel.IPType, PurityScore: intel.PurityScore, ISP: isp,
		})
	}

	list, err := cachedInbounds(live)
	if err != nil {
		view.Panel = err.Error()
		if len(list) == 0 {
			return view
		}
	}

	for _, ib := range list {
		row := ExitInbound{
			ID: ib.ID, Port: ib.Port, Remark: ib.Remark,
			Protocol: ib.Protocol, Enable: ib.Enable, Tag: ib.Tag,
		}
		matchedIdx := -1
		bTo := strings.TrimSpace(ib.BoundTo)
		// 只有真正绑定了有效出口目标的入站才做匹配；未绑定的直连入站绝不乱挂
		if bTo != "" && !strings.EqualFold(bTo, "direct") && !strings.EqualFold(bTo, "none") {
			// 1. 精确哈希匹配 O(1)
			if idx, ok := byHost[bTo]; ok {
				matchedIdx = idx
			} else if idx, ok := byHost[sanitizeTag(bTo)]; ok {
				matchedIdx = idx
			} else {
				// 2. 严格按隧道列表固定顺序进行确定性特征匹配（杜绝 map 随机迭代乱跳）
				sBTo := sanitizeTag(bTo)
				for i, t := range tunnels {
					sTag := sanitizeTag(t.Node.HostName)
					if len(sTag) >= 3 && (strings.Contains(sBTo, sTag) || strings.Contains(sTag, sBTo)) {
						matchedIdx = i
						break
					}
					if t.Node.IP != "" && strings.Contains(bTo, t.Node.IP) {
						matchedIdx = i
						break
					}
					if t.ExitIP != "" && strings.Contains(bTo, t.ExitIP) {
						matchedIdx = i
						break
					}
				}
			}
		}

		if matchedIdx >= 0 && matchedIdx < len(view.Exits) {
			view.Exits[matchedIdx].Inbounds = append(view.Exits[matchedIdx].Inbounds, row)
			continue
		}
		view.Direct = append(view.Direct, row)
	}
	return view
}
