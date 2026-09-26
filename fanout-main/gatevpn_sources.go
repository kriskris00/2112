package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	vpnBookURL          = "https://www.vpnbook.com/freevpn"
	vpngateScraperURL   = "https://github.com/fdciabdul/Vpngate-Scraper-API/blob/main/README.md"
	publicVPNListAPIURL = "https://publicvpnlist.com/api/v1/servers"
)

// fetchGitHubOVPNArchiveNodes 从公开 GitHub 仓库的 codeload 压缩包中提取真实 .ovpn 配置。
// 这里只接入公开、无需账号的配置仓库；配置本身仍必须经过后续真实隧道/Exit-IP 检查。
func fetchGitHubOVPNArchiveNodes(timeout time.Duration, repo, branch, sourceName string) []Node {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	if branch == "" {
		branch = "main"
	}
	u := "https://codeload.github.com/" + strings.Trim(repo, "/") + "/zip/refs/heads/" + branch
	c := sourceHTTPClient(timeout)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "fanout/1.0")
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	resp.Body.Close()
	if err != nil || len(body) == 0 {
		return nil
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil
	}
	out := make([]Node, 0, 256)
	seen := map[string]bool{}
	for _, f := range zr.File {
		name := f.Name
		if f.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(name), ".ovpn") || len(out) >= 5000 {
			continue
		}
		r, err := f.Open()
		if err != nil {
			continue
		}
		cfg, err := io.ReadAll(io.LimitReader(r, 512<<10))
		r.Close()
		if err != nil || len(cfg) == 0 || !strings.Contains(strings.ToLower(string(cfg)), "client") {
			continue
		}
		host, port := firstRemote(string(cfg))
		if host == "" || port == 0 {
			continue
		}
		ip := resolveIPv4(host)
		if ip == "" {
			continue
		}
		key := ip + ":" + strconv.Itoa(port)
		if seen[key] {
			continue
		}
		seen[key] = true
		cc, cn := countryFromPath(name)
		if cc == "" {
			cc, cn = guessCountryByIP(ip)
		}
		out = append(out, Node{
			HostName: "github-" + strings.TrimSuffix(strings.ReplaceAll(filepath.Base(name), ".ovpn", ""), ".OVPN"),
			IP:       ip, Port: port, Proto: "ovpn", CountryCode: cc, Country: cn,
			Config: string(cfg), IPType: "unknown", PurityScore: 50, ISP: sourceName, Source: strings.ToLower(sourceName),
		})
	}
	return out
}

func countryFromPath(name string) (string, string) {
	parts := strings.FieldsFunc(strings.ReplaceAll(name, "\\", "/"), func(r rune) bool { return r == '/' || r == '_' || r == '-' })
	for i := len(parts) - 1; i >= 0; i-- {
		v := strings.ToLower(strings.TrimSpace(parts[i]))
		if v == "" {
			continue
		}
		if cc, ok := englishCountryToCode[v]; ok {
			cn := v
			if zh, ok := countryNameZH[cc]; ok {
				cn = zh
			}
			return cc, cn
		}
		if len(v) == 2 {
			cc := strings.ToUpper(v)
			if _, ok := countryNameZH[cc]; ok {
				return cc, countryNameZH[cc]
			}
		}
	}
	return "", ""
}

// fetchRiseupVPNNodes 动态读取 Riseup 的公开 LEAP 网关和临时客户端证书，
// 生成带内嵌 CA/证书/私钥的 OpenVPN 客户端配置。证书是短期的，刷新时重新获取。
func fetchRiseupVPNNodes(timeout time.Duration) []Node {
	c := sourceHTTPClient(timeout)
	get := func(url string, accept string) ([]byte, bool) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, false
		}
		req.Header.Set("User-Agent", "fanout/1.0")
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		resp, err := c.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			return nil, false
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		return b, err == nil
	}
	certData, ok := get("https://api.black.riseup.net/3/cert", "text/html")
	if !ok {
		return nil
	}
	caData, ok := get("https://black.riseup.net/ca.crt", "text/html")
	if !ok {
		return nil
	}
	gwData, ok := get("https://api.black.riseup.net/3/config/eip-service.json", "application/json")
	if !ok {
		return nil
	}
	var root struct {
		Gateways []struct {
			IP           string `json:"ip_address"`
			Host         string `json:"host"`
			Location     string `json:"location"`
			Capabilities struct {
				Transport []struct {
					Type  string `json:"type"`
					Ports []int  `json:"ports"`
				} `json:"transport"`
			} `json:"capabilities"`
		} `json:"gateways"`
	}
	if json.Unmarshal(gwData, &root) != nil {
		return nil
	}
	textCert := string(certData)
	key := extractPEM(textCert, "RSA PRIVATE KEY")
	if key == "" {
		key = extractPEM(textCert, "PRIVATE KEY")
	}
	cert := extractPEM(textCert, "CERTIFICATE")
	ca := string(caData)
	if key == "" || cert == "" || !strings.Contains(ca, "BEGIN CERTIFICATE") {
		return nil
	}
	var out []Node
	seen := map[string]bool{}
	for _, g := range root.Gateways {
		ip := net.ParseIP(strings.TrimSpace(g.IP))
		if ip == nil || ip.To4() == nil {
			continue
		}
		for _, tr := range g.Capabilities.Transport {
			if !strings.Contains(strings.ToLower(tr.Type), "openvpn") {
				continue
			}
			for _, port := range tr.Ports {
				if port <= 0 || port > 65535 {
					continue
				}
				keyID := ip.String() + ":" + strconv.Itoa(port)
				if seen[keyID] {
					continue
				}
				seen[keyID] = true
				proto := "tcp"
				if strings.Contains(strings.ToLower(tr.Type), "udp") {
					proto = "udp"
				}
				cfg := fmt.Sprintf("client\ndev tun\nproto %s\nremote %s %d\nnobind\npersist-key\npersist-tun\nremote-cert-tls server\nconnect-retry 1 2\nconnect-timeout 8\nverb 1\n<key>\n%s\n</key>\n<cert>\n%s\n</cert>\n<ca>\n%s\n</ca>\n", proto, ip.String(), port, key, cert, ca)
				cc, cn := countryFromPath(g.Location)
				if cc == "" {
					cc, cn = guessCountryByIP(ip.String())
				}
				out = append(out, Node{HostName: "riseup-" + g.Host, IP: ip.String(), Port: port, Proto: "ovpn", CountryCode: cc, Country: cn, Config: cfg, IPType: "unknown", PurityScore: 75, ISP: "RiseupVPN", Source: "riseupvpn"})
			}
		}
	}
	return out
}

func extractPEM(s, typ string) string {
	begin := "-----BEGIN " + typ + "-----"
	end := "-----END " + typ + "-----"
	i := strings.Index(s, begin)
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i:], end)
	if j < 0 {
		return ""
	}
	j = i + j + len(end)
	return strings.TrimSpace(s[i:j])
}

func sourceHTTPClient(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }

func fetchVPNBookNodes(timeout time.Duration) []Node {
	c := sourceHTTPClient(timeout)
	req, err := http.NewRequest(http.MethodGet, vpnBookURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil
	}
	re := regexp.MustCompile(`(?i)href=["']([^"']+\.ovpn(?:\?[^"']*)?)["']`)
	seen := map[string]bool{}
	var out []Node
	for _, m := range re.FindAllStringSubmatch(string(body), -1) {
		u := m[1]
		if strings.HasPrefix(u, "/") {
			u = "https://www.vpnbook.com" + u
		}
		if !strings.HasPrefix(strings.ToLower(u), "https://") || seen[u] {
			continue
		}
		seen[u] = true
		r, e := c.Get(u)
		if e != nil || r.StatusCode != http.StatusOK {
			if r != nil {
				r.Body.Close()
			}
			continue
		}
		cfg, e := io.ReadAll(r.Body)
		r.Body.Close()
		if e != nil {
			continue
		}
		host, port := firstRemote(string(cfg))
		if host == "" || port == 0 {
			continue
		}
		ip := resolveIPv4(host)
		if ip == "" {
			continue
		}
		cc, cn := guessCountryByIP(ip)
		out = append(out, Node{HostName: "vpnbook-" + host, IP: ip, Port: port, Proto: "ovpn", CountryCode: cc, Country: cn, Config: string(cfg), IPType: "unknown", PurityScore: 55, ISP: "VPNBook", Source: "vpnbook"})
	}
	return out
}

func fetchVpngateScraperNodes(timeout time.Duration) []Node {
	c := sourceHTTPClient(timeout)
	req, err := http.NewRequest(http.MethodGet, vpngateScraperURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil
	}
	// The generated README contains direct config links. Only consume explicit .ovpn links.
	re := regexp.MustCompile(`https://raw\.githubusercontent\.com/fdciabdul/Vpngate-Scraper-API/[^\s)]+/configs/[^\s)]+\.ovpn`)
	seen := map[string]bool{}
	var out []Node
	for _, u := range re.FindAllString(string(body), -1) {
		if seen[u] {
			continue
		}
		seen[u] = true
		r, e := c.Get(u)
		if e != nil || r.StatusCode != http.StatusOK {
			if r != nil {
				r.Body.Close()
			}
			continue
		}
		cfg, e := io.ReadAll(r.Body)
		r.Body.Close()
		if e != nil {
			continue
		}
		host, port := firstRemote(string(cfg))
		if host == "" || port == 0 {
			continue
		}
		ip := resolveIPv4(host)
		if ip == "" {
			continue
		}
		cc, cn := guessCountryByIP(ip)
		out = append(out, Node{HostName: "scraper-" + host, IP: ip, Port: port, Proto: "ovpn", CountryCode: cc, Country: cn, Config: string(cfg), IPType: "unknown", PurityScore: 60, ISP: "Vpngate-Scraper", Source: "vpngate_scraper"})
	}
	return out
}

func fetchPublicVPNListNodes(timeout time.Duration) []Node {
	c := sourceHTTPClient(timeout)
	u := publicVPNListAPIURL + "?protocol=openvpn&status=online&per_page=200&sort=last_checked&order=desc"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil
	}
	var root any
	if json.Unmarshal(body, &root) != nil {
		return nil
	}
	var rows []map[string]any
	if obj, ok := root.(map[string]any); ok {
		for _, key := range []string{"servers", "data", "results"} {
			if arr, ok := obj[key].([]any); ok {
				for _, v := range arr {
					if m, ok := v.(map[string]any); ok {
						rows = append(rows, m)
					}
				}
				break
			}
		}
	} else if arr, ok := root.([]any); ok {
		for _, v := range arr {
			if m, ok := v.(map[string]any); ok {
				rows = append(rows, m)
			}
		}
	}
	var out []Node
	for _, m := range rows {
		host := mapString(m, "host", "hostname", "ip", "server")
		port := mapInt(m, "port", "server_port")
		cfgURL := mapString(m, "config_download_url", "ovpn_url", "config_url")
		if host == "" || port == 0 || !strings.HasPrefix(strings.ToLower(cfgURL), "https://") {
			continue
		}
		r, e := c.Get(cfgURL)
		if e != nil || r.StatusCode != http.StatusOK {
			if r != nil {
				r.Body.Close()
			}
			continue
		}
		cfg, e := io.ReadAll(r.Body)
		r.Body.Close()
		if e != nil || !strings.Contains(strings.ToLower(string(cfg)), "client") {
			continue
		}
		ip := resolveIPv4(host)
		if ip == "" {
			continue
		}
		cc := strings.ToUpper(mapString(m, "country_code", "countryCode"))
		cn := mapString(m, "country_name", "country")
		if cc == "" {
			cc, cn = guessCountryByIP(ip)
		}
		out = append(out, Node{HostName: "publicvpn-" + host, IP: ip, Port: port, Proto: "ovpn", CountryCode: cc, Country: cn, Config: string(cfg), IPType: "unknown", PurityScore: 55, ISP: "PublicVPNList", Source: "publicvpnlist"})
	}
	return out
}

func firstRemote(cfg string) (string, int) {
	for _, line := range strings.Split(cfg, "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 3 && strings.EqualFold(f[0], "remote") {
			p, _ := strconv.Atoi(f[2])
			if p > 0 {
				return f[1], p
			}
		}
	}
	return "", 0
}

func resolveIPv4(host string) string {
	if ip := net.ParseIP(host); ip != nil {
		return ip.To4().String()
	}
	ips, _ := net.LookupIP(host)
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}

func mapString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func mapInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			return int(v)
		case string:
			n, _ := strconv.Atoi(strings.TrimSpace(v))
			if n > 0 {
				return n
			}
		}
	}
	return 0
}
