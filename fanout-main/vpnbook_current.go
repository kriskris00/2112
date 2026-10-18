package main

import (
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Adapt to the provider's official 2026 profile download endpoint; credentials
// come from its public page and are kept out of API node responses.
func fetchVPNBookCurrent(timeout time.Duration) []Node {
	if timeout <= 0 || timeout > 12*time.Second {
		timeout = 12 * time.Second
	}
	c := sourceHTTPClient(timeout)
	resp, e := c.Get("https://www.vpnbook.com/freevpn/openvpn")
	if e != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		return nil
	}
	codes := regexp.MustCompile(`<code[^>]*>([^<]{1,80})</code>`).FindAllStringSubmatch(string(b), 3)
	if len(codes) < 2 || html.UnescapeString(codes[0][1]) != "vpnbook" {
		return nil
	}
	pass := html.UnescapeString(codes[1][1])
	if strings.ContainsAny(pass, "\r\n\x00") || pass == "" {
		return nil
	}
	hosts := regexp.MustCompile(`\b(?:us|ca|uk|de|fr|pl)[0-9]+\.vpnbook\.com\b`).FindAllString(string(b), -1)
	seen := map[string]bool{}
	out := []Node{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 3)
	for _, host := range hosts {
		if seen[host] || len(seen) >= 12 {
			continue
		}
		seen[host] = true
		host := host
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r, e := c.Get("https://www.vpnbook.com/api/openvpn?hostname=" + url.QueryEscape(host) + "&protocol=tcp443")
			if e != nil {
				return
			}
			defer r.Body.Close()
			if r.StatusCode != 200 {
				return
			}
			cfg, e := io.ReadAll(io.LimitReader(r.Body, (512<<10)+1))
			if e != nil || len(cfg) > 512<<10 {
				return
			}
			ns, e := parseFeedNodes(cfg, "ovpn")
			if e != nil {
				return
			}
			ip := resolveIPv4(host)
			if ip == "" {
				return
			}
			cc := strings.ToUpper(host[:2])
			if cc == "UK" {
				cc = "GB"
			}
			n := ns[0]
			n.IP = ip
			n.HostName = "vpnbook-" + host
			n.CountryCode = cc
			n.Country = countryNameZH[cc]
			n.Source = "vpnbook"
			n.ISP = "VPNBook（官方公开配置）"
			n.VPNUser = "vpnbook"
			n.VPNPass = pass
			n.IPType = "unknown"
			n.PurityScore = 0
			mu.Lock()
			out = append(out, n)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}
