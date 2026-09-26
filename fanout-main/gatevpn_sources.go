package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
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
