package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// persistedTunnel 是隧道在磁盘上的形态。
// 只存重建所需的信息，运行态（netns、进程、监听）重启后重新建立。
type persistedTunnel struct {
	Slot               int    `json:"slot"`
	Port               int    `json:"port"`
	HostName           string `json:"hostname"`
	CountryCode        string `json:"country_code"`
	Country            string `json:"country"`
	TargetPolicyMode   string `json:"target_policy_mode,omitempty"`
	LegacyTargetJPMode string `json:"target_jp_mode,omitempty"`
	Config             string `json:"config"`
	IP                 string `json:"ip,omitempty"`
	Proto              string `json:"proto,omitempty"`
	UpstreamPort       int    `json:"upstream_port,omitempty"`
	VPNUser            string `json:"vpn_user,omitempty"`
	VPNPass            string `json:"vpn_pass,omitempty"`
	// SOCKS5 凭据要存盘：用户已经把它分发给客户端了，重启后变掉等于全断
	SocksUser string `json:"socks_user,omitempty"`
	SocksPass string `json:"socks_pass,omitempty"`
}

type persistedState struct {
	Tunnels []persistedTunnel `json:"tunnels"`
}

func statePath(dir string) string { return filepath.Join(dir, "state.json") }

// saveState 把当前隧道写入磁盘，供重启后恢复。
func (m *Manager) saveState() error {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	var st persistedState
	m.mu.RLock()
	for _, t := range m.tunnels {
		// 只跳过用户主动停掉的。starting/failed 的隧道也要存：
		// 它们正在重连或等着重试，漏存会让重启后凭空少几个出口。
		if t.Status == "stopped" {
			continue
		}
		st.Tunnels = append(st.Tunnels, persistedTunnel{
			Slot:             t.Slot,
			Port:             t.Port,
			HostName:         t.Node.HostName,
			CountryCode:      t.Node.CountryCode,
			Country:          t.Node.Country,
			TargetPolicyMode: t.TargetPolicyMode,
			Config:           t.Node.Config,
			IP:               t.Node.IP, Proto: t.Node.Proto, UpstreamPort: t.Node.Port, VPNUser: t.Node.VPNUser, VPNPass: t.Node.VPNPass,
			SocksUser: t.Cred.User,
			SocksPass: t.Cred.Pass,
		})
	}

	st.Tunnels = append(st.Tunnels, m.pendingRestore...)
	m.mu.RUnlock()
	blob, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(m.workDir, fmt.Sprintf("state.json.tmp.%d", time.Now().UnixNano()))
	if err := os.WriteFile(tmp, blob, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(m.workDir))
}

// restoreState 读回上次的隧道并逐条拉起。
// 节点配置一并存了盘，所以即使 VPN Gate 列表里该节点已消失也能重建。
func (m *Manager) restoreState() (int, error) {
	blob, err := os.ReadFile(statePath(m.workDir))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	var st persistedState
	if err := json.Unmarshal(blob, &st); err != nil {
		return 0, fmt.Errorf("解析状态文件失败: %w", err)
	}

	// Retain all saved exits, including entries above a newly lowered limit.
	m.mu.Lock()
	m.pendingRestore = append([]persistedTunnel(nil), st.Tunnels...)
	m.mu.Unlock()
	if len(st.Tunnels) > 0 {
		go m.restoreQueued()
	}
	return len(st.Tunnels), nil
}

func (m *Manager) restoreQueued() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
		}
		if m.admissionReason() != "" {
			continue
		}
		m.mu.Lock()
		if len(m.pendingRestore) == 0 {
			m.mu.Unlock()
			return
		}
		if len(m.tunnels) >= m.maxSlots {
			m.mu.Unlock()
			continue
		}
		starting := 0
		for _, t := range m.tunnels {
			if t.Status == "starting" {
				starting++
			}
		}
		if starting >= 2 {
			m.mu.Unlock()
			continue
		}
		p := m.pendingRestore[0]
		slot := p.Slot
		if slot < 1 || slot > absoluteExitLimit || m.tunnels[slot] != nil {
			var err error
			slot, err = m.freeSlot()
			if err != nil {
				m.mu.Unlock()
				continue
			}
		}
		node := Node{HostName: p.HostName, IP: p.IP, Proto: p.Proto, Port: p.UpstreamPort, Country: p.Country, CountryCode: p.CountryCode}
		for _, n := range m.nodes {
			if n.HostName == p.HostName {
				node = n
				break
			}
		}
		node.Config = p.Config
		if p.VPNUser != "" || p.VPNPass != "" {
			node.VPNUser = p.VPNUser
			node.VPNPass = p.VPNPass
		}
		cred := SocksCred{User: p.SocksUser, Pass: p.SocksPass}
		if cred.User == "" || cred.Pass == "" {
			var err error
			cred, err = newSocksCred()
			if err != nil {
				m.mu.Unlock()
				log.Printf("恢复凭据失败: %v", err)
				continue
			}
		}
		policy := p.TargetPolicyMode
		if policy == "" {
			policy = p.LegacyTargetJPMode
		}
		t := &Tunnel{Slot: slot, Port: p.Port, Node: node, Cred: cred, Status: "starting", TargetRegion: strings.ToUpper(strings.TrimSpace(node.CountryCode)), TargetPolicyMode: policy, Since: time.Now()}
		m.tunnels[slot] = t
		m.pendingRestore = m.pendingRestore[1:]
		m.lastAdmission = time.Now()
		m.mu.Unlock()
		go m.bringUpPersist(t, true, true)
	}
}
