package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var countryNameZH = map[string]string{
	"JP": "日本", "KR": "韩国", "US": "美国", "HK": "中国香港", "TW": "中国台湾",
	"SG": "新加坡", "GB": "英国", "DE": "德国", "FR": "法国", "CA": "加拿大",
	"AU": "澳大利亚", "NL": "荷兰", "MY": "马来西亚", "PH": "菲律宾", "ID": "印尼",
	"TH": "泰国", "VN": "越南", "IN": "印度", "RU": "俄罗斯", "BR": "巴西",
	"TR": "土耳其", "IT": "意大利", "ES": "西班牙", "SE": "瑞典", "CH": "瑞士",
	"NO": "挪威", "FI": "芬兰", "PL": "波兰", "CZ": "捷克", "AT": "奥地利",
	"GLOBAL": "全球", "EDU": "海外高校学术网",
}

// formatProxyName 构造规范的订阅节点名称：[国旗Emoji] [企业/ISP名称] (仅国旗表情，不含文字国家，不含"出口"字样)
func formatProxyName(countryCode, country, isp string, suffix string) string {
	cc := strings.ToUpper(strings.TrimSpace(countryCode))
	flag := getFlagEmoji(cc)

	comp := strings.TrimSpace(isp)
	if comp == "" || strings.EqualFold(comp, "Public Proxy") || strings.EqualFold(comp, "Public Pool") || strings.EqualFold(comp, "VPN Gate") {
		if strings.Contains(strings.ToLower(isp), "tsukuba") || strings.EqualFold(cc, "JP") {
			comp = "筑波大学 VPN Gate"
		} else {
			comp = "优质网络"
		}
	}
	runes := []rune(comp)
	if len(runes) > 24 {
		comp = string(runes[:22]) + "..."
	}

	cleanSuffix := strings.TrimSpace(suffix)
	if strings.Contains(cleanSuffix, "出口") {
		cleanSuffix = strings.TrimSpace(strings.ReplaceAll(cleanSuffix, "出口", ""))
		cleanSuffix = strings.TrimPrefix(cleanSuffix, "-")
		cleanSuffix = strings.TrimSpace(cleanSuffix)
	}

	if cleanSuffix != "" {
		return fmt.Sprintf("%s %s (%s)", flag, comp, cleanSuffix)
	}
	return fmt.Sprintf("%s %s", flag, comp)
}

// apiCredToken 供已登录 Web 界面的管理员获取口令，方便一键生成带 token 的聚合订阅链接
func apiCredToken(a *Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"token": a.Password(),
		})
	}
}

// apiSubscription 生成聚合订阅链接，支持 Base64 通用订阅与 Clash / Mihomo 配置订阅
func apiSubscription(m *Manager, a *Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := publicHost(r)
		format := strings.ToLower(r.URL.Query().Get("format"))
		ua := strings.ToLower(r.UserAgent())
		isClash := format == "clash" ||
			strings.Contains(ua, "clash") ||
			strings.Contains(ua, "mihomo") ||
			strings.Contains(ua, "meta")
		isQuanX := format == "quanx" ||
			strings.Contains(ua, "quantumult") ||
			strings.Contains(ua, "quanx")

		// 建立已连接出站绑定的映射：支持 HostName, sanitizeTag, IP, slot 等全方位匹配
		tunnels := m.Tunnels()
		boundTunnel := make(map[string]*Tunnel)
		var upTunnels []*Tunnel
		for _, t := range tunnels {
			if t.Status == "up" {
				upTunnels = append(upTunnels, t)
				boundTunnel[t.Node.HostName] = t
				boundTunnel[sanitizeTag(t.Node.HostName)] = t
				if t.Node.IP != "" {
					boundTunnel[t.Node.IP] = t
				}
				boundTunnel[fmt.Sprintf("exit-%d", t.Slot)] = t
				boundTunnel[fmt.Sprintf("slot-%d", t.Slot)] = t
			}
		}

		findTunnel := func(boundTo string) *Tunnel {
			boundTo = strings.TrimSpace(boundTo)
			if boundTo == "" || strings.EqualFold(boundTo, "direct") || strings.EqualFold(boundTo, "none") {
				return nil
			}
			if t, ok := boundTunnel[boundTo]; ok && t != nil {
				return t
			}
			clean := sanitizeTag(boundTo)
			if t, ok := boundTunnel[clean]; ok && t != nil {
				return t
			}
			for _, t := range upTunnels {
				s := sanitizeTag(t.Node.HostName)
				if len(s) >= 3 && (strings.Contains(boundTo, s) || strings.Contains(s, boundTo)) {
					return t
				}
				if t.Node.IP != "" && strings.Contains(boundTo, t.Node.IP) {
					return t
				}
			}
			return nil
		}

		// 收集运行中的出口节点与绑定的 3x-ui 入站（严格 1:1 实时同步，绝不重复生成 20->40 个）
		coveredSlots := make(map[int]bool)
		var validDetails []*InboundDetail
		var allLinks []string

		p, err := openPanel()
		if err == nil && p != nil {
			inbounds, _ := p.Inbounds(nil)
			for _, ib := range inbounds {
				d, dErr := p.InboundDetail(ib.ID, host)
				if dErr == nil && d != nil {
					t := findTunnel(d.BoundTo)
					// 严格只同步当前正在运行(up)的出口节点，未绑定或已停止的不入订阅
					if t == nil {
						continue
					}
					coveredSlots[t.Slot] = true
					validDetails = append(validDetails, d)

					for _, rawLink := range d.Links {
						idx := strings.LastIndex(rawLink, "#")
						cleanName := formatProxyName(t.Node.CountryCode, t.Node.Country, t.Node.ISP, fmt.Sprintf("%s :%d", strings.ToUpper(d.Protocol), d.Port))
						if idx != -1 {
							rawLink = rawLink[:idx] + "#" + url.QueryEscape(cleanName)
						} else {
							rawLink = rawLink + "#" + url.QueryEscape(cleanName)
						}
						allLinks = append(allLinks, rawLink)
					}
				}
			}
		}

		// 仅对尚未绑定入站的独立出口隧道补充 SOCKS5 节点（仅国旗表情，无文字国家，无出口字样）
		for _, t := range upTunnels {
			if coveredSlots[t.Slot] {
				continue
			}
			cred := t.credential()
			label := formatProxyName(t.Node.CountryCode, t.Node.Country, t.Node.ISP, fmt.Sprintf(":%d", t.Port))
			s5Link := fmt.Sprintf("socks5://%s:%s@%s:%d#%s", cred.User, cred.Pass, host, t.Port, url.QueryEscape(label))
			allLinks = append(allLinks, s5Link)
		}

		// 统一流量头信息（展示 10TB 配额，永不过期）
		w.Header().Set("Subscription-Userinfo", "upload=0; download=0; total=10737418240000; expire=0")
		w.Header().Set("Profile-Update-Interval", "12")

		if isClash {
			w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=\"fanout-clash.yaml\"")
			yamlContent := generateClashConfig(validDetails, upTunnels, host)
			_, _ = w.Write([]byte(yamlContent))
			return
		}

		if isQuanX {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=\"fanout-quanx.txt\"")
			qxContent := generateQuanXConfig(validDetails, upTunnels, host)
			if r.URL.Query().Get("raw") == "1" {
				_, _ = w.Write([]byte(qxContent))
				return
			}
			b64 := base64.StdEncoding.EncodeToString([]byte(qxContent))
			_, _ = w.Write([]byte(b64))
			return
		}

		// 默认通用 Base64 订阅（适用于 Shadowrocket / V2RayN / Sing-box / NekoBox / Loon 等）
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"fanout.txt\"")
		body := strings.Join(allLinks, "\n")
		b64 := base64.StdEncoding.EncodeToString([]byte(body))
		_, _ = w.Write([]byte(b64))
	}
}

// generateClashConfig 生成标准的 Clash / Mihomo YAML 订阅配置
func generateClashConfig(details []*InboundDetail, tunnels []*Tunnel, host string) string {
	var sb strings.Builder
	sb.WriteString("# Fanout 聚合代理订阅 - 自动路由出口\r\n")
	sb.WriteString(fmt.Sprintf("# 生成时间: %s\r\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("port: 7890\r\nsocks-port: 7891\r\nallow-lan: true\r\nmode: rule\r\nlog-level: info\r\nipv6: false\r\n\r\n")

	type proxyItem struct {
		name string
		yaml string
	}
	var proxies []proxyItem

	// 建立 boundTo 对应关系：支持 HostName, sanitizeTag, IP, slot 等全方位匹配
	boundTunnel := make(map[string]*Tunnel)
	for _, t := range tunnels {
		boundTunnel[t.Node.HostName] = t
		boundTunnel[sanitizeTag(t.Node.HostName)] = t
		if t.Node.IP != "" {
			boundTunnel[t.Node.IP] = t
		}
		boundTunnel[fmt.Sprintf("exit-%d", t.Slot)] = t
		boundTunnel[fmt.Sprintf("%d", t.Slot)] = t
	}
	findTunnel := func(boundTo string) *Tunnel {
		if boundTo == "" {
			if len(tunnels) == 1 {
				return tunnels[0]
			}
			return nil
		}
		if t, ok := boundTunnel[boundTo]; ok && t != nil {
			return t
		}
		clean := sanitizeTag(boundTo)
		if t, ok := boundTunnel[clean]; ok && t != nil {
			return t
		}
		for _, t := range tunnels {
			s := sanitizeTag(t.Node.HostName)
			if strings.Contains(boundTo, s) || strings.Contains(s, boundTo) {
				return t
			}
			if t.Node.IP != "" && strings.Contains(boundTo, t.Node.IP) {
				return t
			}
		}
		if len(tunnels) == 1 {
			return tunnels[0]
		}
		return nil
	}

	seenNames := make(map[string]int)
	makeUniqueName := func(raw string) string {
		count := seenNames[raw]
		seenNames[raw] = count + 1
		if count > 0 {
			return fmt.Sprintf("%s (%d)", raw, count+1)
		}
		return raw
	}

	// 1. 处理 3x-ui 入站（严格 1:1 绑定到运行中的出口）
	coveredSlots := make(map[int]bool)
	for _, d := range details {
		proto := strings.ToLower(d.Protocol)
		t := findTunnel(d.BoundTo)
		if t == nil {
			continue
		}
		coveredSlots[t.Slot] = true

		for idx, c := range d.Clients {
			clientEmail := c.Email
			if clientEmail == "" {
				clientEmail = fmt.Sprintf("client-%d", idx+1)
			}
			suffix := fmt.Sprintf("%s :%d", strings.ToUpper(proto), d.Port)
			if len(d.Clients) > 1 {
				suffix += fmt.Sprintf(" - %s", clientEmail)
			}
			pName := makeUniqueName(formatProxyName(t.Node.CountryCode, t.Node.Country, t.Node.ISP, suffix))

			switch proto {
			case "vless":
				tlsBool := d.TLS == "tls" || d.TLS == "reality"
				netType := d.Network
				if netType == "" {
					netType = "tcp"
				}
				py := fmt.Sprintf("  - name: %q\r\n    type: vless\r\n    server: %q\r\n    port: %d\r\n    uuid: %q\r\n    udp: true\r\n    network: %q\r\n    tls: %v\r\n",
					pName, host, d.Port, c.ID, netType, tlsBool)
				if netType == "ws" {
					py += "    ws-opts:\r\n      path: \"/\"\r\n"
				}
				proxies = append(proxies, proxyItem{name: pName, yaml: py})

			case "vmess":
				tlsBool := d.TLS == "tls"
				netType := d.Network
				if netType == "" {
					netType = "tcp"
				}
				py := fmt.Sprintf("  - name: %q\r\n    type: vmess\r\n    server: %q\r\n    port: %d\r\n    uuid: %q\r\n    alterId: 0\r\n    cipher: auto\r\n    udp: true\r\n    network: %q\r\n    tls: %v\r\n",
					pName, host, d.Port, c.ID, netType, tlsBool)
				if netType == "ws" {
					py += "    ws-opts:\r\n      path: \"/\"\r\n"
				}
				proxies = append(proxies, proxyItem{name: pName, yaml: py})

			case "trojan":
				py := fmt.Sprintf("  - name: %q\r\n    type: trojan\r\n    server: %q\r\n    port: %d\r\n    password: %q\r\n    udp: true\r\n    sni: %q\r\n",
					pName, host, d.Port, c.ID, host)
				proxies = append(proxies, proxyItem{name: pName, yaml: py})

			case "shadowsocks":
				parts := strings.SplitN(c.ID, ":", 2)
				method := "aes-256-gcm"
				pwd := c.ID
				if len(parts) == 2 {
					method = strings.TrimSpace(parts[0])
					pwd = strings.TrimSpace(parts[1])
				}
				py := fmt.Sprintf("  - name: %q\r\n    type: ss\r\n    server: %q\r\n    port: %d\r\n    cipher: %q\r\n    password: %q\r\n    udp: true\r\n",
					pName, host, d.Port, method, pwd)
				proxies = append(proxies, proxyItem{name: pName, yaml: py})

			case "socks":
				py := fmt.Sprintf("  - name: %q\r\n    type: socks5\r\n    server: %q\r\n    port: %d\r\n    username: %q\r\n    password: %q\r\n    udp: true\r\n",
					pName, host, d.Port, c.Email, c.ID)
				proxies = append(proxies, proxyItem{name: pName, yaml: py})
			}
		}
	}

	// 2. 仅对未绑定入站的独立出口隧道补充 SOCKS5 节点（仅国旗表情，无文字国家，无出口字样）
	for _, t := range tunnels {
		if coveredSlots[t.Slot] {
			continue
		}
		cred := t.credential()
		pName := makeUniqueName(formatProxyName(t.Node.CountryCode, t.Node.Country, t.Node.ISP, fmt.Sprintf(":%d", t.Port)))
		py := fmt.Sprintf("  - name: %q\r\n    type: socks5\r\n    server: %q\r\n    port: %d\r\n    username: %q\r\n    password: %q\r\n    udp: true\r\n",
			pName, host, t.Port, cred.User, cred.Pass)
		proxies = append(proxies, proxyItem{name: pName, yaml: py})
	}

	if len(proxies) == 0 {
		py := fmt.Sprintf("  - name: %q\r\n    type: socks5\r\n    server: %q\r\n    port: 1080\r\n", "暂无可用节点，请在面板开启出口", host)
		proxies = append(proxies, proxyItem{name: "暂无可用节点，请在面板开启出口", yaml: py})
	}

	sb.WriteString("proxies:\r\n")
	for _, p := range proxies {
		sb.WriteString(p.yaml)
	}
	sb.WriteString("\r\n")

	sb.WriteString("proxy-groups:\r\n")
	sb.WriteString("  - name: \"⚡ 节点选择\"\r\n    type: select\r\n    proxies:\r\n      - \"🚀 自动优选\"\r\n")
	for _, p := range proxies {
		sb.WriteString(fmt.Sprintf("      - %q\r\n", p.name))
	}

	sb.WriteString("  - name: \"🚀 自动优选\"\r\n    type: url-test\r\n    url: http://www.gstatic.com/generate_204\r\n    interval: 300\r\n    tolerance: 50\r\n    proxies:\r\n")
	for _, p := range proxies {
		sb.WriteString(fmt.Sprintf("      - %q\r\n", p.name))
	}
	sb.WriteString("\r\n")

	sb.WriteString("rules:\r\n  - MATCH,⚡ 节点选择\r\n")
	return sb.String()
}

// generateQuanXConfig 生成标准的 Quantumult X 节点订阅配置 (原生 server_remote 规范)
func generateQuanXConfig(details []*InboundDetail, tunnels []*Tunnel, host string) string {
	var sb strings.Builder
	sb.WriteString("# Quantumult X 节点订阅 - 聚合出海出口\r\n")
	sb.WriteString(fmt.Sprintf("# 生成时间: %s\r\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("# 格式说明: Quantumult X 原生支持 SOCKS5 / Trojan / VMess / Shadowsocks；由于 QuanX 内核不支持 VLESS，VLESS 节点已自动标注\r\n\r\n")

	boundTunnel := make(map[string]*Tunnel)
	for _, t := range tunnels {
		boundTunnel[t.Node.HostName] = t
		boundTunnel[sanitizeTag(t.Node.HostName)] = t
		if t.Node.IP != "" {
			boundTunnel[t.Node.IP] = t
		}
		boundTunnel[fmt.Sprintf("exit-%d", t.Slot)] = t
		boundTunnel[fmt.Sprintf("%d", t.Slot)] = t
	}
	findTunnel := func(boundTo string) *Tunnel {
		if boundTo == "" {
			if len(tunnels) == 1 {
				return tunnels[0]
			}
			return nil
		}
		if t, ok := boundTunnel[boundTo]; ok && t != nil {
			return t
		}
		clean := sanitizeTag(boundTo)
		if t, ok := boundTunnel[clean]; ok && t != nil {
			return t
		}
		for _, t := range tunnels {
			s := sanitizeTag(t.Node.HostName)
			if strings.Contains(boundTo, s) || strings.Contains(s, boundTo) {
				return t
			}
			if t.Node.IP != "" && strings.Contains(boundTo, t.Node.IP) {
				return t
			}
		}
		if len(tunnels) == 1 {
			return tunnels[0]
		}
		return nil
	}

	seenNames := make(map[string]int)
	makeUniqueName := func(raw string) string {
		count := seenNames[raw]
		seenNames[raw] = count + 1
		if count > 0 {
			return fmt.Sprintf("%s (%d)", raw, count+1)
		}
		return raw
	}

	// 1. 处理 3x-ui 入站（严格 1:1 绑定到运行中的出口）
	coveredSlots := make(map[int]bool)
	for _, d := range details {
		proto := strings.ToLower(d.Protocol)
		t := findTunnel(d.BoundTo)
		if t == nil {
			continue
		}
		coveredSlots[t.Slot] = true

		for idx, c := range d.Clients {
			clientEmail := c.Email
			if clientEmail == "" {
				clientEmail = fmt.Sprintf("client-%d", idx+1)
			}
			suffix := fmt.Sprintf("%s :%d", strings.ToUpper(proto), d.Port)
			if len(d.Clients) > 1 {
				suffix += fmt.Sprintf(" - %s", clientEmail)
			}
			pName := makeUniqueName(formatProxyName(t.Node.CountryCode, t.Node.Country, t.Node.ISP, suffix))

			switch proto {
			case "trojan":
				sb.WriteString(fmt.Sprintf("trojan = %s:%d, password=%s, over-tls=true, tls-verification=false, fast-open=false, udp-relay=true, tag=%s\r\n",
					host, d.Port, c.ID, pName))

			case "vmess":
				netType := strings.ToLower(d.Network)
				tlsOpt := ""
				if d.TLS == "tls" {
					tlsOpt = ", over-tls=true, tls-verification=false"
				}
				obfsOpt := ""
				if netType == "ws" {
					obfsOpt = ", obfs=ws, obfs-uri=/"
				}
				sb.WriteString(fmt.Sprintf("vmess = %s:%d, method=chacha20-ietf-poly1305, password=%s%s%s, fast-open=false, udp-relay=true, tag=%s\r\n",
					host, d.Port, c.ID, obfsOpt, tlsOpt, pName))

			case "shadowsocks":
				parts := strings.SplitN(c.ID, ":", 2)
				method := "aes-256-gcm"
				pwd := c.ID
				if len(parts) == 2 {
					method = strings.TrimSpace(parts[0])
					pwd = strings.TrimSpace(parts[1])
				}
				sb.WriteString(fmt.Sprintf("shadowsocks = %s:%d, method=%s, password=%s, fast-open=false, udp-relay=true, tag=%s\r\n",
					host, d.Port, method, pwd, pName))

			case "socks":
				if c.Email != "" && c.ID != "" {
					sb.WriteString(fmt.Sprintf("socks5 = %s:%d, username=%s, password=%s, fast-open=false, udp-relay=true, tag=%s\r\n",
						host, d.Port, c.Email, c.ID, pName))
				} else {
					sb.WriteString(fmt.Sprintf("socks5 = %s:%d, fast-open=false, udp-relay=true, tag=%s\r\n",
						host, d.Port, pName))
				}

			case "vless":
				sb.WriteString(fmt.Sprintf("; [QuanX 不支持 VLESS] %s (端口:%d) 采用 VLESS 协议。由于 Quantumult X 官方内核不支持 VLESS，建议在 3x-ui 面板中改用 Trojan 或 VMess。\r\n",
					pName, d.Port))
			}
		}
	}

	// 2. 仅对未绑定入站的独立出口隧道补充 SOCKS5 节点（仅国旗表情，无文字国家，无出口字样）
	for _, t := range tunnels {
		if coveredSlots[t.Slot] {
			continue
		}
		cred := t.credential()
		pName := makeUniqueName(formatProxyName(t.Node.CountryCode, t.Node.Country, t.Node.ISP, fmt.Sprintf(":%d", t.Port)))
		if cred.User != "" && cred.Pass != "" {
			sb.WriteString(fmt.Sprintf("socks5 = %s:%d, username=%s, password=%s, fast-open=false, udp-relay=true, tag=%s\r\n",
				host, t.Port, cred.User, cred.Pass, pName))
		} else {
			sb.WriteString(fmt.Sprintf("socks5 = %s:%d, fast-open=false, udp-relay=true, tag=%s\r\n",
				host, t.Port, pName))
		}
	}

	return sb.String()
}

// getFlagEmoji 把两位国家码（ISO 3166-1 alpha-2）转换为对应的国旗 Emoji
func getFlagEmoji(countryCode string) string {
	cc := strings.ToUpper(strings.TrimSpace(countryCode))
	if cc == "EDU" {
		return "🎓"
	}
	if len(cc) != 2 {
		return "🌐"
	}
	r1 := rune(cc[0]) - 'A' + 0x1F1E6
	r2 := rune(cc[1]) - 'A' + 0x1F1E6
	if r1 < 0x1F1E6 || r1 > 0x1F1FF || r2 < 0x1F1E6 || r2 > 0x1F1FF {
		return "🌐"
	}
	return string([]rune{r1, r2})
}
