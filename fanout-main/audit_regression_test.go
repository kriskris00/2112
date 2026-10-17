package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenVPNRejectsPrivilegedOptions(t *testing.T) {
	for _, option := range []string{"plugin /tmp/evil.so", "config /etc/shadow", "up /bin/sh", "script-security 3", "log /etc/passwd", "management 0.0.0.0 1234", "ca /etc/shadow", "auth-user-pass /etc/shadow", "setenv PATH /tmp", "<connection>"} {
		if err := validateOpenVPNConfig("client\nremote example.com 443\n" + option + "\n"); err == nil {
			t.Errorf("accepted %q", option)
		}
	}
	safe := "client\ndev tun\nproto tcp\nremote example.com 443\nauth-user-pass\n<ca>\nCERT\n</ca>\n"
	if err := validateOpenVPNConfig(safe); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenVPNConfig(safe + "<key>\n"); err == nil {
		t.Fatal("unclosed key accepted")
	}
}

func TestUDPDomainResponseRoundTrip(t *testing.T) {
	packet := buildUDPRespHeader(atypDomain, []byte("dns.example"), 53, []byte("reply"))
	_, _, host, port, payload, ok := parseUDPReqHeader(packet)
	if !ok || host != "dns.example" || port != 53 || string(payload) != "reply" {
		t.Fatalf("invalid domain reply: %v", packet)
	}
}

func TestAuthRejectsCrossSiteLegacyGET(t *testing.T) {
	a := newTestAuth()
	token, _ := a.issue()
	handler := a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, origin := range []string{"https://evil.example", "null"} {
		req := httptest.NewRequest("GET", "http://localhost/api/stop", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatalf("cross-origin request accepted: %d", w.Code)
		}
	}
	req := httptest.NewRequest("GET", "http://localhost/api/tunnels", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 204 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("same-origin request failed")
	}
}

func TestEmptyCredentialsFailClosed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "password"), []byte("\n"), 0600)
	if _, _, err := NewAuth(dir); err == nil {
		t.Fatal("empty password accepted")
	}
}

func TestEmptyBasePathPersists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "basepath"), []byte("\n"), 0600)
	bp, created, err := LoadBasePath(dir)
	if err != nil || created || bp != "" {
		t.Fatalf("empty path reset: %q %v %v", bp, created, err)
	}
}

func TestNodeDedupRetainsEndpoints(t *testing.T) {
	nodes := []Node{{IP: "1.2.3.4", Port: 80, Proto: "http"}, {IP: "1.2.3.4", Port: 80, Proto: "socks5"}, {IP: "1.2.3.4", Port: 81, Proto: "http"}, {IP: "1.2.3.4", Port: 80, Proto: "http"}}
	if got := len(dedupNodesByIP(nodes)); got != 3 {
		t.Fatalf("got %d unique endpoints", got)
	}
}

func TestUpstreamUDPIsNotTCP(t *testing.T) {
	for _, proto := range []string{"http", "https", "socks5"} {
		if conn, err := makeUpstreamDialer(proto, "127.0.0.1:1", time.Second)("udp", "1.1.1.1:53"); err == nil {
			conn.Close()
			t.Fatal("UDP silently became TCP")
		}
	}
}

func TestExitProbeRejectsErrorPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "denied", 403) }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	tunnel := &Tunnel{Node: Node{IP: "1.2.3.4"}, dialer: func(network, addr string) (net.Conn, error) { return net.Dial("tcp", u.Host) }}
	if ip, err := tunnel.probeExitIP(); err == nil || ip != "" {
		t.Fatalf("error response became success: %s %v", ip, err)
	}
}

func TestBoundOfflineInboundBlocks(t *testing.T) {
	cfg := buildXrayConfig([]*nativeInbound{{Enable: true, Port: 10000, BoundTo: "offline"}}, nil)
	rules := cfg["routing"].(map[string]any)["rules"].([]any)
	if len(rules) != 1 || rules[0].(map[string]any)["outboundTag"] != "block" {
		t.Fatal("offline binding falls through to direct")
	}
}

func TestSOCKSRequestRejectsInvalidVersion(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	go func() { _, _ = client.Write([]byte{4, 1, 0, 1}); client.Close() }()
	if _, _, err := socksReadRequestWithCmd(server); err == nil {
		t.Fatal("invalid SOCKS version accepted")
	}
}

func TestHTTPProxyRejectsMisleadingStatus(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, e := ln.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(time.Second))
		b := make([]byte, 1024)
		c.Read(b)
		io.WriteString(c, "HTTP/1.1 500 error 200\r\n\r\n")
	}()
	c, err := makeUpstreamDialer("http", ln.Addr().String(), time.Second)("tcp", "1.1.1.1:80")
	if err == nil {
		c.Close()
		t.Fatal("500 containing 200 accepted")
	}
	if !strings.Contains(err.Error(), "connect failed") {
		t.Fatal(err)
	}
}

func TestInboundRejectsInvalidPort(t *testing.T) {
	for _, port := range []int{-1, 65536} {
		if _, err := normalizeInboundSpec(NewInboundSpec{Port: port}, nil); err == nil {
			t.Fatalf("accepted port %d", port)
		}
	}
}

func TestWebSettingsWriteFailurePreservesListener(t *testing.T) {
	dir := t.TempDir()
	cur, err := loadWebSettings(dir, freePort(t), false)
	if err != nil {
		t.Fatal(err)
	}
	srv := newWebServer(http.NotFoundHandler())
	if err = srv.reload(cur); err != nil {
		t.Fatal(err)
	}
	defer srv.srv.Close()
	oldPath := webSettingsPath
	webSettingsPath = filepath.Join(dir, "missing", "settings.json")
	defer func() { webSettingsPath = oldPath }()
	if err := srv.applyWebSettings(WebSettings{Port: freePort(t), ListenAddr: "127.0.0.1"}); err == nil {
		t.Fatal("write failure ignored")
	}
	if srv.addr != cur.listenAddrString() || getWebSettings() != cur {
		t.Fatal("failed persistence changed active listener")
	}
}
