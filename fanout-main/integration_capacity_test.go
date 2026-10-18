package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStopAllClearsPendingRestoreAndRejectsGet(t *testing.T) {
	m := NewManager(10, t.TempDir())
	m.pendingRestore = []persistedTunnel{{Slot: 7, HostName: "queued"}}
	h := apiStopAll(m)
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, "/api/stop-all", nil))
	if w.Code != http.StatusMethodNotAllowed || len(m.pendingRestore) != 1 {
		t.Fatal("GET mutated queued exits")
	}
	w = httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/api/stop-all", nil))
	b, err := os.ReadFile(statePath(m.workDir))
	var state persistedState
	if err != nil || json.Unmarshal(b, &state) != nil || len(state.Tunnels) != 0 || len(m.pendingRestore) != 0 || w.Code != http.StatusOK {
		t.Fatalf("pending restore survived stop all: %s %v", b, err)
	}
}

func TestCacheRoundTripPreservesProviderAndProxy(t *testing.T) {
	dir := t.TempDir()
	original := []Node{{HostName: "owned", IP: "192.0.2.1", Port: 443, Proto: "ovpn", Country: "Example, Region", CountryCode: "ZZ", Config: "client\nremote 192.0.2.1 443\n", Source: "feed:owned", VPNUser: "user", VPNPass: "secret", FeedCategory: "gov", AuthorizationURL: "https://agency.gov/authorization"}, {HostName: "proxy", IP: "192.0.2.2", Port: 1080, Proto: "socks5", CountryCode: "ZZ", Source: "proxy"}}
	saveNodesToCache(dir, original)
	b, e := os.ReadFile(filepath.Join(dir, "cached_nodes.csv"))
	if e != nil {
		t.Fatal(e)
	}
	got, e := parseNodeCSV(string(b))
	if e != nil || len(got) != 2 {
		t.Fatalf("cache parse: %v %d", e, len(got))
	}
	byHost := map[string]Node{}
	for _, n := range got {
		byHost[n.HostName] = n
	}
	if n := byHost["owned"]; n.VPNPass != "secret" || n.Source != "feed:owned" || n.Country != "Example, Region" || n.Port != 443 {
		t.Fatalf("provider cache metadata changed: %+v", n)
	}
	if n := byHost["proxy"]; n.Config != "" || n.Proto != "socks5" || n.Port != 1080 {
		t.Fatal("cached SOCKS proxy turned into OpenVPN")
	}
}
func TestDisabledSourceIsExcludedWithoutDeletingLiveTunnel(t *testing.T) {
	live := &Tunnel{Slot: 1}
	m := &Manager{tunnels: map[int]*Tunnel{1: live}, feeds: []SourceFeed{{ID: "owned", Enabled: false}}}
	nodes := m.filterManagedCandidates([]Node{{Source: "feed:owned"}, {Source: "vpngate"}})
	if len(nodes) != 1 || nodes[0].Source != "vpngate" || m.tunnels[1] != live {
		t.Fatal("source disable either retained candidates or deleted live exit")
	}
}
func TestInvalidCombinedSettingsDoNotChangePassword(t *testing.T) {
	a, _, e := NewAuth(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	old := a.Password()
	req := httptest.NewRequest("POST", "/api/settings", strings.NewReader(`{"password":"new-password","port":70000}`))
	w := httptest.NewRecorder()
	apiSettings(a, newWebServer(http.NotFoundHandler()))(w, req)
	if w.Code != 400 || a.Password() != old {
		t.Fatal("invalid port request partially changed password")
	}
}
func TestISPModeOnlySettingPersists(t *testing.T) {
	if _, e := loadExitNodeLimitSettings(t.TempDir()); e != nil {
		t.Fatal(e)
	}
	a := newTestAuth()
	req := httptest.NewRequest("POST", "/api/settings", strings.NewReader(`{"exit_limit_mode":"random"}`))
	w := httptest.NewRecorder()
	apiSettings(a, newWebServer(http.NotFoundHandler()))(w, req)
	if w.Code != 200 || getExitNodeLimitSettings().Mode != "random" {
		t.Fatal("mode-only settings were ignored")
	}
}
func TestQueuedBatchStopsOnResourcePressure(t *testing.T) {
	m := &Manager{maxSlots: 254, workDir: t.TempDir(), stopCh: make(chan struct{}), tunnels: map[int]*Tunnel{}, nodes: []Node{{HostName: "selected", IP: "192.0.2.2", Port: 443, CountryCode: "ZZ"}}, capacity: defaultCapacitySettings(), capacityConfigured: true, resources: &resourceMonitor{current: ResourceSnapshot{Available: true, CPUReady: true, CPUPercent: 90, SampledAt: time.Now()}}}
	req := httptest.NewRequest("POST", "/api/nodes/batch_start", strings.NewReader(`{"hosts":["selected"],"queue":true}`))
	w := httptest.NewRecorder()
	apiBatchStart(m, &LiveScanner{})(w, req)
	if w.Code != 202 {
		t.Fatalf("batch not queued: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Job JobView `json:"job"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		views := m.jobs.Views()
		if len(views) > 0 && views[0].Status != "running" {
			if !strings.Contains(views[0].Steps[0].Detail, "CPU") {
				t.Fatal("resource reason overwritten")
			}
			if len(m.tunnels) != 0 {
				t.Fatal("queued batch bypassed resource guard")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("blocked batch did not finish")
}
func TestMutationEndpointsRejectGET(t *testing.T) {
	m := &Manager{}
	for _, h := range []http.HandlerFunc{apiAutoOrchestrate(m), apiLiveScan(&LiveScanner{}, m), apiBatchStart(m, &LiveScanner{})} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest("GET", "/api/test", nil))
		if w.Code != 405 {
			t.Fatal("GET initiated background work")
		}
	}
}

func TestSourceSelectionConsistentAcrossOrchestrationAndLiveList(t *testing.T) {
	gov := Node{HostName: "authorized", Source: "feed:agency", FeedCategory: "gov", AuthorizationURL: "https://agency.gov/authorization"}
	fake := Node{HostName: "heuristic", Source: "gov", IPType: "gov"}
	if !orchestrateSourceAllowed(gov, []string{"gov_authorized"}) || orchestrateSourceAllowed(fake, []string{"gov_authorized"}) {
		t.Fatal("orchestration used a different government source policy")
	}
	s := &LiveScanner{verified: []Node{gov, fake}}
	nodes := s.GetVerifiedNodes("gov_authorized", "all", "")
	if len(nodes) != 1 || nodes[0].HostName != "authorized" {
		t.Fatal("live source list disagreed with orchestration")
	}
	custom := Node{Source: "feed:owned", FeedCategory: "custom"}
	if !orchestrateSourceAllowed(custom, []string{"custom"}) {
		t.Fatal("authorized custom source excluded from manual orchestration")
	}
}
