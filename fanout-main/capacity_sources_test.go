package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResourceThresholdStopsAtEightyPercent(t *testing.T) {
	s := defaultCapacitySettings()
	for _, tt := range []struct {
		cpu, mem float64
		blocked  bool
	}{{79.9, 79.9, false}, {80, 20, true}, {20, 80, true}, {100, 100, true}} {
		r := ResourceSnapshot{Available: true, CPUReady: true, CPUPercent: tt.cpu, MemoryPercent: tt.mem, SampledAt: time.Now()}
		if got := capacityReason(s, r) != ""; got != tt.blocked {
			t.Fatalf("cpu=%v mem=%v blocked=%v", tt.cpu, tt.mem, got)
		}
	}
}
func TestResourceUnknownAndStaleFailClosed(t *testing.T) {
	s := defaultCapacitySettings()
	for _, r := range []ResourceSnapshot{{}, {Available: true, CPUReady: false}, {Available: true, CPUReady: true, SampledAt: time.Now().Add(-10 * time.Second)}} {
		if capacityReason(s, r) == "" {
			t.Fatal("unknown resource readings admitted an exit")
		}
	}
	s.GuardEnabled = false
	if capacityReason(s, ResourceSnapshot{}) != "" {
		t.Fatal("explicit disabled guard ignored")
	}
}
func TestResourceCgroupLimitAndCPUFixture(t *testing.T) {
	files := map[string]string{"/proc/stat": "cpu 10 0 10 80 0 0 0 0 999 999\n", "/proc/meminfo": "MemTotal: 1000 kB\nMemAvailable: 900 kB\nSwapTotal: 500 kB\nSwapFree: 300 kB\n", "/proc/self/cgroup": "0::/system.slice/fanout.service\n"}
	base := filepath.Join("/sys/fs/cgroup", filepath.Clean("/system.slice/fanout.service"))
	files[filepath.Join(base, "memory.max")] = "512000"
	files[filepath.Join(base, "memory.current")] = "460800"
	r := &resourceMonitor{readFile: func(path string) ([]byte, error) {
		v, ok := files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(v), nil
	}}
	r.sample()
	if r.snapshot().CPUReady {
		t.Fatal("one sample claimed CPU delta")
	}
	files["/proc/stat"] = "cpu 20 0 20 160 0 0 0 0 999 999\n"
	r.sample()
	s := r.snapshot()
	if !s.CPUReady || s.CPUPercent != 20 || s.MemoryPercent != 90 || s.Scope != "cgroup" || s.SwapUsed != 204800 {
		t.Fatalf("incorrect readings: %+v", s)
	}
	if capacityReason(defaultCapacitySettings(), s) == "" {
		t.Fatal("stricter service memory limit ignored")
	}
}

func TestCgroupCPUQuotaOverridesLowHostAverage(t *testing.T) {
	base := filepath.Join("/sys/fs/cgroup", "test")
	r := &resourceMonitor{prevCGUsage: 100000, prevCGAt: time.Now().Add(-time.Second), readFile: func(path string) ([]byte, error) {
		if path == filepath.Join(base, "cpu.max") {
			return []byte("100000 100000"), nil
		}
		if path == filepath.Join(base, "cpu.stat") {
			return []byte("usage_usec 1050000\n"), nil
		}
		return nil, os.ErrNotExist
	}}
	s := ResourceSnapshot{CPUPercent: 2, SampledAt: time.Now()}
	r.applyCgroupCPU(&s, base)
	if !s.CPUReady || s.CPUPercent < 90 {
		t.Fatalf("container quota CPU ignored: %+v", s)
	}
}
func TestCapacitySavePersistsAndDoesNotDeleteExits(t *testing.T) {
	m := &Manager{workDir: t.TempDir(), tunnels: map[int]*Tunnel{72: {Slot: 72}, 139: {Slot: 139}}}
	s := defaultCapacitySettings()
	s.MaxExits = 1
	if err := m.setCapacity(s); err != nil {
		t.Fatal(err)
	}
	if len(m.tunnels) != 2 || m.maxSlots != 1 {
		t.Fatal("lowering limit removed existing exits")
	}
	blob, err := os.ReadFile(filepath.Join(m.workDir, "capacity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved CapacitySettings
	json.Unmarshal(blob, &saved)
	if saved != s {
		t.Fatal("capacity did not persist")
	}
	s.MaxExits = 0
	if err = m.setCapacity(s); err != nil || m.maxSlots != 254 {
		t.Fatal("automatic capacity remains fixed at forty")
	}
}
func TestCapacityWriteFailureDoesNotChangeRunningLimit(t *testing.T) {
	m := &Manager{workDir: filepath.Join(t.TempDir(), "missing"), maxSlots: 40}
	if err := m.setCapacity(defaultCapacitySettings()); err == nil {
		t.Fatal("write failure ignored")
	}
	if m.maxSlots != 40 {
		t.Fatal("failed write changed live limit")
	}
}
func TestCapacityLimitsValidate(t *testing.T) {
	for _, s := range []CapacitySettings{{MaxExits: 255, ResourceLimitPercent: 80}, {MaxExits: -1, ResourceLimitPercent: 80}, {MaxExits: 80, ResourceLimitPercent: 0}, {MaxExits: 80, ResourceLimitPercent: 96}} {
		if s.validate() == nil {
			t.Fatalf("accepted invalid capacity %+v", s)
		}
	}
}
func TestEveryCreationPathRespectsResourceGuard(t *testing.T) {
	r := &resourceMonitor{current: ResourceSnapshot{Available: true, CPUReady: true, CPUPercent: 81, MemoryPercent: 10, SampledAt: time.Now()}}
	m := &Manager{maxSlots: 254, capacity: defaultCapacitySettings(), capacityConfigured: true, resources: r, tunnels: map[int]*Tunnel{}}
	n := Node{HostName: "guard-test", IP: "192.0.2.1", Port: 443, CountryCode: "ZZ"}
	for _, start := range []func(Node) (*Tunnel, error){m.Start, m.StartExact, func(n Node) (*Tunnel, error) { return m.StartWithPolicy(n, "random") }} {
		if _, e := start(n); e == nil {
			t.Fatal("creation bypassed resource protection")
		}
	}
	if len(m.tunnels) != 0 {
		t.Fatal("blocked creation occupied slots")
	}
}
func TestQueuedRestoreIsPreservedInState(t *testing.T) {
	m := &Manager{workDir: t.TempDir(), tunnels: map[int]*Tunnel{}, pendingRestore: []persistedTunnel{{Slot: 72, Port: 20072, HostName: "saved", SocksUser: "u", SocksPass: "p", VPNUser: "provider", VPNPass: "provider-secret"}}}
	if e := m.saveState(); e != nil {
		t.Fatal(e)
	}
	blob, _ := os.ReadFile(statePath(m.workDir))
	var st persistedState
	json.Unmarshal(blob, &st)
	if len(st.Tunnels) != 1 || st.Tunnels[0].VPNPass != "provider-secret" {
		t.Fatal("paused restore lost its data")
	}
}
func TestSourcePasswordIsNotReturnedAndEmptyEditRetainsIt(t *testing.T) {
	m := &Manager{workDir: t.TempDir()}
	f := SourceFeed{ID: "official", Name: "Official", URL: "https://example.com/profile.ovpn", Format: "ovpn", Category: "custom", Enabled: true, Username: "user", Password: "private-secret"}
	if e := m.saveFeeds([]SourceFeed{f}); e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	m.apiFeeds()(w, httptest.NewRequest("GET", "/api/source-feeds", nil))
	if strings.Contains(w.Body.String(), "private-secret") || !strings.Contains(w.Body.String(), "has_password") {
		t.Fatal("source password exposed")
	}
	f.Password = ""
	f.Name = "Renamed"
	if e := m.saveFeeds([]SourceFeed{f}); e != nil {
		t.Fatal(e)
	}
	if m.feeds[0].Password != "private-secret" {
		t.Fatal("blank edit erased password")
	}
	f.ClearPassword = true
	if e := m.saveFeeds([]SourceFeed{f}); e != nil {
		t.Fatal(e)
	}
	if m.feeds[0].Password != "" {
		t.Fatal("explicit clear ignored")
	}
}
func TestGovernmentLabelRequiresOfficialAuthorizationLink(t *testing.T) {
	f := SourceFeed{ID: "gov", Name: "Authorized source", URL: "https://example.com/authorized.ovpn", Format: "ovpn", Category: "gov", Enabled: true}
	if validateFeeds([]SourceFeed{f}) == nil {
		t.Fatal("government label without evidence accepted")
	}
	f.AuthorizationURL = "https://agency.gov.example.com/policy"
	if validateFeeds([]SourceFeed{f}) == nil {
		t.Fatal("government lookalike domain accepted")
	}
	f.AuthorizationURL = "https://agency.gov/authorized-vpn"
	if e := validateFeeds([]SourceFeed{f}); e != nil {
		t.Fatal(e)
	}
	if nodeMatchesSource(Node{IPType: "gov", Source: "gov"}, "gov_authorized") {
		t.Fatal("heuristic label passed as authorized government source")
	}
}
func TestSourceFormatsRejectUnsafeConfigAndZIP(t *testing.T) {
	if _, e := parseFeedNodes([]byte("client\nplugin /tmp/evil.so\n"), "ovpn"); e == nil {
		t.Fatal("unsafe source accepted")
	}
	config := "client\ndev tun\nremote 192.0.2.2 443\n"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("../../profile.ovpn")
	f.Write([]byte(config))
	zw.Close()
	nodes, e := parseFeedNodes(buf.Bytes(), "zip")
	if e != nil || len(nodes) != 1 || nodes[0].Port != 443 {
		t.Fatalf("bounded in-memory ZIP parse failed: %v", e)
	}
	// Archive member names are never extracted into the filesystem.
	jsonBody := fmt.Sprintf(`[{"ip":"192.0.2.2","port":443,"config":%q}]`, config)
	if n, e := parseFeedNodes([]byte(jsonBody), "json"); e != nil || len(n) != 1 {
		t.Fatal("JSON source parsing failed")
	}
}
func TestCapacityAndFeedsRequireLogin(t *testing.T) {
	m := &Manager{}
	a := newTestAuth()
	for _, handler := range []http.Handler{m.apiCapacity(), m.apiFeeds()} {
		w := httptest.NewRecorder()
		a.Wrap(handler).ServeHTTP(w, httptest.NewRequest("GET", "/api/capacity", nil))
		if w.Code != 401 {
			t.Fatal("administrative configuration is public")
		}
	}
}
func TestLegacySourceURLsPersist(t *testing.T) {
	m := &Manager{workDir: t.TempDir()}
	if e := m.setLegacySources("https://example.com/list.csv"); e != nil {
		t.Fatal(e)
	}
	SetCustomSourceURL("")
	defer SetCustomSourceURL("")
	if e := m.loadLegacySources(); e != nil {
		t.Fatal(e)
	}
	if GetSourceInfo().CustomURL != "https://example.com/list.csv" {
		t.Fatal("custom URL disappeared after restart")
	}
	if m.setLegacySources("file:///etc/shadow") == nil {
		t.Fatal("non-web URL accepted")
	}
}
func TestNodeAPIDoesNotExposeProviderPassword(t *testing.T) {
	b, _ := json.Marshal(Node{VPNUser: "private-user", VPNPass: "private-pass"})
	if strings.Contains(string(b), "private-") {
		t.Fatal("provider credentials exposed in Node response")
	}
}
