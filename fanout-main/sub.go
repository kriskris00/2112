package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

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

		// 收集所有 3x-ui 入站及其分享链接
		var details []*InboundDetail
		var allLinks []string

		p, err := openPanel()
		if err == nil && p != nil {
			inbounds, _ := p.Inbounds(nil)
			for _, ib := range inbounds {
				d, dErr := p.InboundDetail(ib.ID, host)
				if dErr == nil && d != nil {
					details = append(details, d)
					allLinks = append(allLinks, d.Links...)
				}
			}
		}

		// 收集运行中的出口隧道（SOCKS5 形式补充）
		tunnels := m.Tunnels()
		var upTunnels []*Tunnel
		for _, t := range tunnels {
			if t.Status == "up" {
				upTunnels = append(upTunnels, t)
				// 生成 SOCKS5 分享链接
				cred := t.credential()
				flag := getFlagEmoji(t.Node.CountryCode)
				label := fmt.Sprintf("%s 出口-%d (%s)", flag, t.Slot, t.Node.CountryCode)
				s5Link := fmt.Sprintf("socks5://%s:%s@%s:%d#%s", cred.User, cred.Pass, host, t.Port, label)
				allLinks = append(allLinks, s5Link)
			}
		}

		// 统一流量头信息（展示 10TB 配额，永不过期）
		w.Header().Set("Subscription-Userinfo", "upload=0; download=0; total=10737418240000; expire=0")
		w.Header().Set("Profile-Update-Interval", "12")

		if isClash {
			w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=\"fanout-clash.yaml\"")
			yamlContent := generateClashConfig(details, upTunnels, host)
			_, _ = w.Write([]byte(yamlContent))
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

	// 1. 处理 3x-ui 入站
	for _, d := range details {
		proto := strings.ToLower(d.Protocol)
		for idx, c := range d.Clients {
			clientEmail := c.Email
			if clientEmail == "" {
				clientEmail = fmt.Sprintf("client-%d", idx+1)
			}
			pName := fmt.Sprintf("%s :%d (%s)", strings.ToUpper(proto), d.Port, clientEmail)
			if d.Remark != "" {
				pName = fmt.Sprintf("%s - %s :%d", d.Remark, strings.ToUpper(proto), d.Port)
			}

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

	// 2. 处理运行中的出口隧道 (SOCKS5 出口)
	for _, t := range tunnels {
		cred := t.credential()
		flag := getFlagEmoji(t.Node.CountryCode)
		pName := fmt.Sprintf("%s 出口-%d [%s %s]", flag, t.Slot, t.Node.CountryCode, t.ExitIP)
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
