package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGovernmentClassificationKeepsSuspectsSeparate(t *testing.T) {
	for _, c := range []struct {
		node   Node
		status string
	}{
		{Node{Source: "gov"}, "suspected"},
		{Node{IPType: "gov"}, "suspected"},
		{Node{ISP: "City of Example Municipal Network"}, "suspected"},
		{Node{HostName: "relay.agency.gov"}, "suspected"},
		{Node{FeedCategory: "gov", AuthorizationURL: "https://agency.gov/policy"}, "declared"},
		{Node{FeedCategory: "gov", AuthorizationURL: "https://agency.gov.evil.example/policy"}, ""},
		{Node{GovernmentStatus: "declared"}, ""},
		{Node{HostName: "regular-relay", ISP: "Example Broadband"}, ""},
	} {
		got, reason := governmentClassification(c.node)
		if got != c.status || (got != "" && reason == "") {
			t.Fatalf("classification %q expected %q for %+v", got, c.status, c.node)
		}
		if nodeMatchesSource(c.node, "gov") != (got != "") || nodeMatchesSource(c.node, "gov_suspected") != (got == "suspected") || nodeMatchesSource(c.node, "gov_authorized") != (got == "declared") {
			t.Fatal("government filters disagree")
		}
	}
}

func TestFourSuspectedGovernmentCandidatesAcrossViews(t *testing.T) {
	// Synthetic fixtures verify four candidates survive filtering, not a claim
	// that four real government VPNs are currently available.
	nodes := []Node{
		{HostName: "legacy", IP: "192.0.2.1", Port: 443, Proto: "ovpn", Config: "client\n", CountryCode: "JP", Source: "gov"},
		{HostName: "typed", IP: "192.0.2.2", Port: 443, Proto: "ovpn", Config: "client\n", CountryCode: "KR", IPType: "gov"},
		{HostName: "metadata", IP: "192.0.2.3", Port: 443, Proto: "ovpn", Config: "client\n", CountryCode: "GB", ISP: "Department of Example"},
		{HostName: "relay.example.gov", IP: "192.0.2.4", Port: 443, Proto: "ovpn", Config: "client\n", CountryCode: "US"},
		{HostName: "regular", IP: "192.0.2.5", Port: 443, Proto: "ovpn", Config: "client\n", CountryCode: "CA"},
	}
	m := NewManager(20, t.TempDir())
	m.nodes = nodes
	for _, source := range []string{"gov", "gov_suspected"} {
		ls := &LiveScanner{verified: nodes}
		if len(ls.GetVerifiedNodes(source, "all", "")) != 4 {
			t.Fatal("live list dropped suspects")
		}
		if len(m.Regions(source)) != 4 {
			t.Fatal("region selector dropped suspects")
		}
		for _, n := range nodes[:4] {
			if !orchestrateSourceAllowed(n, []string{source}) {
				t.Fatal("orchestration dropped suspects")
			}
		}
		w := httptest.NewRecorder()
		apiNodes(m)(w, httptest.NewRequest("GET", "/api/nodes?source="+source, nil))
		var d struct {
			Nodes []Node `json:"nodes"`
		}
		if json.Unmarshal(w.Body.Bytes(), &d) != nil || len(d.Nodes) != 4 {
			t.Fatalf("API lost candidates: %s", w.Body.String())
		}
		for _, n := range d.Nodes {
			if n.GovernmentStatus != "suspected" || !strings.Contains(n.GovernmentEvidence, "未核验") {
				t.Fatal("API omitted uncertainty")
			}
		}
	}
	if len(m.Regions("gov_authorized")) != 0 {
		t.Fatal("suspects promoted to authorized")
	}
}

func TestGovernmentEvidenceSurvivesRestartAndDistinctPorts(t *testing.T) {
	dir := t.TempDir()
	n := Node{HostName: "public-relay", IP: "192.0.2.8", Port: 443, Proto: "ovpn", Config: "client\nremote 192.0.2.8 443\n", CountryCode: "JP", Source: "vpngate", ISP: "Example Municipality", GovernmentStatus: "suspected", GovernmentEvidence: "公开源 Message 自报 municipal；疑似，未核验"}
	other := n
	other.Port = 1194
	other.HostName = "second-port"
	saveNodesToCache(dir, []Node{n, other})
	got := loadInitialNodes(dir)
	if len(got) != 2 {
		t.Fatalf("restart merged distinct ports: %d", len(got))
	}
	for _, node := range got {
		if !nodeMatchesSource(node, "gov_suspected") || node.ISP != n.ISP || node.GovernmentEvidence != n.GovernmentEvidence {
			t.Fatalf("lost classification: %+v", node)
		}
	}
}

func TestGovernmentParserDoesNotInventVerifiedProvider(t *testing.T) {
	raw := strings.Replace(sampleNodeCSV("public-relay"), ",op,,", ",Example Public Operator,municipal network,", 1)
	nodes, err := parseNodeCSV(raw)
	if err != nil || len(nodes) != 1 {
		t.Fatal(err)
	}
	n := nodes[0]
	if n.GovernmentStatus != "suspected" || !nodeMatchesSource(n, "gov") || nodeMatchesSource(n, "gov_authorized") {
		t.Fatal("operator/message signal missing or promoted")
	}
	if n.ISP != "Example Public Operator" || n.PurityScore == 99 {
		t.Fatalf("invented provider/quality: %+v", n)
	}
}

func TestGovernmentEvidenceRetainedWhenDuplicateMetadataChanges(t *testing.T) {
	old := Node{IP: "192.0.2.9", Port: 443, Proto: "ovpn", Config: "old", Source: "gov"}
	fresh := Node{IP: old.IP, Port: old.Port, Proto: old.Proto, Config: "fresh", VPNPass: "fresh-secret"}
	for _, n := range []Node{retainGovernmentEvidence(fresh, old), dedupNodesByIP([]Node{fresh, old})[0]} {
		if !nodeMatchesSource(n, "gov_suspected") || n.Config != "fresh" || n.VPNPass != "fresh-secret" || !strings.Contains(n.GovernmentEvidence, "可能过期") {
			t.Fatalf("lost historical evidence or changed profile: %+v", n)
		}
	}
	other := fresh
	other.Port = 1194
	if nodeMatchesSource(retainGovernmentEvidence(other, old), "gov") {
		t.Fatal("evidence crossed different service endpoint")
	}
	declared := old
	declared.Source = "feed:agency"
	declared.FeedCategory = "gov"
	declared.AuthorizationURL = "https://agency.gov/policy"
	if nodeMatchesSource(retainGovernmentEvidence(fresh, declared), "gov_authorized") {
		t.Fatal("authorization transferred between providers")
	}
}
