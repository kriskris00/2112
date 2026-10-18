package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type SourceFeed struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	URL              string `json:"url"`
	Format           string `json:"format"`
	Category         string `json:"category"`
	CountryCode      string `json:"country_code,omitempty"`
	AuthorizationURL string `json:"authorization_url,omitempty"`
	Enabled          bool   `json:"enabled"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	HasPassword      bool   `json:"has_password,omitempty"`
	ClearPassword    bool   `json:"clear_password,omitempty"`
}
type FeedStatus struct {
	Count     int       `json:"count"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

func vpnCredentials(n Node) (string, string) {
	if n.VPNUser != "" || n.VPNPass != "" {
		return n.VPNUser, n.VPNPass
	}
	return "vpn", "vpn"
}
func validFeedURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func governmentDomain(host string) bool {
	for _, s := range []string{".gov", ".gov.uk", ".go.jp", ".go.kr", ".gov.tw", ".gov.hk", ".gov.sg", ".gov.au", ".govt.nz", ".gc.ca"} {
		if strings.HasSuffix(strings.ToLower(host), s) {
			return true
		}
	}
	return false
}
func validateFeeds(feeds []SourceFeed) error {
	if len(feeds) > 20 {
		return fmt.Errorf("最多配置 20 个授权来源")
	}
	ids, urls := map[string]bool{}, map[string]bool{}
	for _, f := range feeds {
		if !regexp.MustCompile(`^[a-z0-9_-]{1,40}$`).MatchString(f.ID) || ids[f.ID] {
			return fmt.Errorf("来源标识无效或重复")
		}
		ids[f.ID] = true
		if strings.TrimSpace(f.Name) == "" || len(f.Name) > 100 || !validFeedURL(f.URL) || urls[f.URL] {
			return fmt.Errorf("来源名称无效或 HTTPS 地址重复/无效")
		}
		urls[f.URL] = true
		if f.Format != "ovpn" && f.Format != "zip" && f.Format != "csv" && f.Format != "json" {
			return fmt.Errorf("不支持的来源格式")
		}
		if f.Category != "custom" && f.Category != "edu" && f.Category != "gov" {
			return fmt.Errorf("来源分类无效")
		}
		if f.CountryCode != "" && !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(f.CountryCode) {
			return fmt.Errorf("国家代码须为两个大写英文字母")
		}
		if f.Category == "gov" {
			u, e := url.Parse(f.AuthorizationURL)
			if e != nil || !validFeedURL(f.AuthorizationURL) || !governmentDomain(u.Hostname()) {
				return fmt.Errorf("政府来源需填写政府官方域名上的公开授权说明链接")
			}
		}
		if strings.ContainsAny(f.Username+f.Password, "\r\n\x00") || len(f.Username) > 255 || len(f.Password) > 255 {
			return fmt.Errorf("来源凭据无效")
		}
	}
	return nil
}
func (m *Manager) loadFeeds() error {
	b, e := os.ReadFile(filepath.Join(m.workDir, "source_feeds.json"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var feeds []SourceFeed
	if e = json.Unmarshal(b, &feeds); e != nil {
		return e
	}
	if e = validateFeeds(feeds); e != nil {
		return e
	}
	m.feedsMu.Lock()
	m.feeds = feeds
	m.feedsMu.Unlock()
	return nil
}
func (m *Manager) saveFeeds(next []SourceFeed) error {
	m.feedsMu.Lock()
	defer m.feedsMu.Unlock()
	for i := range next {
		if next[i].ClearPassword {
			next[i].Password = ""
		}
		for _, old := range m.feeds {
			if next[i].ID == old.ID && next[i].Password == "" && !next[i].ClearPassword {
				next[i].Password = old.Password
			}
		}
		next[i].HasPassword = false
		next[i].ClearPassword = false
	}
	if e := validateFeeds(next); e != nil {
		return e
	}
	b, e := json.MarshalIndent(next, "", "  ")
	if e != nil {
		return e
	}
	path := filepath.Join(m.workDir, "source_feeds.json")
	if e = os.WriteFile(path+".tmp", b, 0600); e != nil {
		return e
	}
	if e = os.Rename(path+".tmp", path); e != nil {
		return e
	}
	m.feeds = next
	return nil
}
func (m *Manager) apiFeeds() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var feeds []SourceFeed
			if e := json.NewDecoder(r.Body).Decode(&feeds); e != nil {
				writeJSON(w, 400, map[string]string{"error": "来源格式错误"})
				return
			}
			if e := m.saveFeeds(feeds); e != nil {
				writeJSON(w, 400, map[string]string{"error": e.Error()})
				return
			}
		} else if r.Method != "GET" {
			writeJSON(w, 405, map[string]string{"error": "仅支持 GET 或 POST"})
			return
		}
		m.feedsMu.RLock()
		safe := append([]SourceFeed{}, m.feeds...)
		status := map[string]FeedStatus{}
		for k, v := range m.feedStatus {
			status[k] = v
		}
		m.feedsMu.RUnlock()
		for i := range safe {
			safe[i].HasPassword = safe[i].Password != ""
			safe[i].Password = ""
		}
		writeJSON(w, 200, map[string]any{"feeds": safe, "status": status, "notice": "分类由管理员声明，不代表机构身份或质量已核验；节点仍需真实出口验证。"})
	}
}
func parseFeedNodes(body []byte, format string) ([]Node, error) {
	switch format {
	case "csv":
		return parseNodeCSV(string(body))
	case "json":
		var items []struct {
			HostName    string `json:"hostname"`
			IP          string `json:"ip"`
			Port        int    `json:"port"`
			Config      string `json:"config"`
			CountryCode string `json:"country_code"`
			Country     string `json:"country"`
		}
		if e := json.Unmarshal(body, &items); e != nil {
			return nil, e
		}
		if len(items) > 500 {
			return nil, fmt.Errorf("来源节点超过 500")
		}
		out := []Node{}
		for _, v := range items {
			if e := validateOpenVPNConfig(v.Config); e != nil {
				return nil, e
			}
			out = append(out, Node{HostName: v.HostName, IP: v.IP, Port: v.Port, Proto: "ovpn", Config: v.Config, CountryCode: v.CountryCode, Country: v.Country})
		}
		return out, nil
	case "ovpn":
		if e := validateOpenVPNConfig(string(body)); e != nil {
			return nil, e
		}
		host, port := firstRemote(string(body))
		if host == "" || port < 1 || port > 65535 {
			return nil, fmt.Errorf("配置缺少有效 remote")
		}
		return []Node{{HostName: host, IP: host, Port: port, Proto: "ovpn", Config: string(body)}}, nil
	case "zip":
		z, e := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if e != nil {
			return nil, e
		}
		out := []Node{}
		total := 0
		for _, file := range z.File {
			if !strings.HasSuffix(strings.ToLower(file.Name), ".ovpn") {
				continue
			}
			if len(out) >= 64 {
				return nil, fmt.Errorf("压缩包最多 64 个配置")
			}
			r, e := file.Open()
			if e != nil {
				return nil, e
			}
			b, e := io.ReadAll(io.LimitReader(r, (512<<10)+1))
			r.Close()
			total += len(b)
			if e != nil || len(b) > 512<<10 || total > 8<<20 {
				return nil, fmt.Errorf("压缩包解压内容过大")
			}
			nodes, e := parseFeedNodes(b, "ovpn")
			if e != nil {
				return nil, e
			}
			out = append(out, nodes...)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("压缩包没有有效配置")
		}
		return out, nil
	}
	return nil, fmt.Errorf("不支持的来源格式")
}
func (m *Manager) fetchFeeds(source string) []Node {
	m.feedsMu.RLock()
	feeds := append([]SourceFeed(nil), m.feeds...)
	m.feedsMu.RUnlock()
	out := []Node{}
	for _, f := range feeds {
		if !f.Enabled || (source != "all" && source != "" && source != "feed:"+f.ID && source != f.Category) {
			continue
		}
		status := FeedStatus{CheckedAt: time.Now()}
		client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > 4 || r.URL.Scheme != "https" {
				return fmt.Errorf("来源重定向被拒绝")
			}
			return nil
		}}
		resp, e := client.Get(f.URL)
		var nodes []Node
		if e == nil {
			if resp.StatusCode != 200 {
				e = fmt.Errorf("HTTP %d", resp.StatusCode)
			} else {
				var b []byte
				b, e = io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
				if e == nil && len(b) > 4<<20 {
					e = fmt.Errorf("来源响应超过 4 MiB")
				}
				if e == nil {
					nodes, e = parseFeedNodes(b, f.Format)
				}
			}
			resp.Body.Close()
		}
		if e != nil {
			status.Error = "读取或校验失败：" + firstLine(e.Error())
		} else {
			for _, n := range nodes {
				if n.Config == "" || validateOpenVPNConfig(n.Config) != nil {
					continue
				}
				if net.ParseIP(n.IP) == nil {
					n.IP = resolveIPv4(n.IP)
				}
				if net.ParseIP(n.IP) == nil || n.Port < 1 || n.Port > 65535 {
					continue
				}
				if n.HostName == "" {
					n.HostName = n.IP
				}
				n.HostName = f.ID + "-" + n.HostName + fmt.Sprint("-", n.Port)
				n.Source = "feed:" + f.ID
				n.FeedCategory = f.Category
				if f.CountryCode != "" {
					n.CountryCode = f.CountryCode
					n.Country = countryNameZH[f.CountryCode]
				} else if n.CountryCode == "" {
					n.CountryCode = "GLOBAL"
					n.Country = "地区未声明"
				}
				n.AuthorizationURL = f.AuthorizationURL
				n.IPType = "unknown"
				n.PurityScore = 0
				n.ISP = f.Name
				n.VPNUser = f.Username
				n.VPNPass = f.Password
				out = append(out, n)
				status.Count++
			}
		}
		m.feedsMu.Lock()
		if m.feedStatus == nil {
			m.feedStatus = map[string]FeedStatus{}
		}
		m.feedStatus[f.ID] = status
		m.feedsMu.Unlock()
	}
	return dedupNodesByIP(out)
}

func (m *Manager) loadLegacySources() error {
	b, e := os.ReadFile(filepath.Join(m.workDir, "custom_sources.txt"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	SetCustomSourceURL(string(b))
	return nil
}

func (m *Manager) filterManagedCandidates(nodes []Node) []Node {
	m.feedsMu.RLock()
	defer m.feedsMu.RUnlock()
	allowed := map[string]SourceFeed{}
	for _, f := range m.feeds {
		if f.Enabled {
			allowed["feed:"+f.ID] = f
		}
	}
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if strings.HasPrefix(n.Source, "feed:") {
			f, ok := allowed[n.Source]
			if !ok {
				continue
			}
			n.VPNUser = f.Username
			n.VPNPass = f.Password
			n.FeedCategory = f.Category
			n.AuthorizationURL = f.AuthorizationURL
			if f.CountryCode != "" {
				n.CountryCode = f.CountryCode
			}
		}
		out = append(out, n)
	}
	return out
}
func (m *Manager) setLegacySources(raw string) error {
	if len(raw) > 8192 {
		return fmt.Errorf("来源地址过长")
	}
	count := 0
	for _, line := range strings.FieldsFunc(raw, func(c rune) bool { return c == '\n' || c == ',' }) {
		u, e := url.Parse(strings.TrimSpace(line))
		if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
			return fmt.Errorf("来源地址必须是有效 HTTP(S) 地址，不可内嵌凭据")
		}
		count++
	}
	if count > 20 {
		return fmt.Errorf("最多 20 个来源地址")
	}
	path := filepath.Join(m.workDir, "custom_sources.txt")
	if e := os.WriteFile(path+".tmp", []byte(raw), 0600); e != nil {
		return e
	}
	if e := os.Rename(path+".tmp", path); e != nil {
		return e
	}
	SetCustomSourceURL(raw)
	return nil
}
