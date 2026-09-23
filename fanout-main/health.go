package main

import (
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	healthInterval = 30 * time.Second
	healthFailures = 4 // 连续失败 4 次才判定掉线，容忍偶发抖动
	healthTimeout  = 8 * time.Second
)

// WatchHealth 周期检查每条隧道是否还能出网，掉线的自动换节点重连。
// VPN Gate 是志愿者节点，运行中掉线很常见。
func (m *Manager) WatchHealth() {
	fails := map[int]int{}

	for range time.Tick(healthInterval) {
		for _, t := range m.Tunnels() {
			if t.Status != "up" {
				continue
			}
			if m.tunnelHealthy(t) {
				fails[t.Slot] = 0
				continue
			}

			fails[t.Slot]++
			if fails[t.Slot] < healthFailures {
				log.Printf("隧道 %d (%s) 探测失败 %d 次", t.Slot, t.Node.HostName, fails[t.Slot])
				continue
			}

			log.Printf("隧道 %d (%s) 已掉线，正在换节点重连", t.Slot, t.Node.HostName)
			fails[t.Slot] = 0
			m.reconnect(t, t.Node.HostName)
		}
	}
}

// tunnelHealthy 判断隧道是否还真的走在 VPN 上。
//
// 只看"能不能出网"是不够的：netns 通过 veth 走母机 NAT，
// openvpn 死掉后照样能出网，只是出口变回了母机 IP。
// 所以要比对出口 IP 是否仍是建立隧道时拿到的那个。
func (m *Manager) tunnelHealthy(t *Tunnel) bool {
	// 1. 上游公开代理（无 netns，通过 dialer 探测连通性）
	if t.Node.Proto == "socks5" || t.Node.Proto == "http" || t.Node.Config == "" {
		if t.dialer == nil {
			return false
		}
		client := &http.Client{
			Transport: &http.Transport{Dial: t.dialer},
			Timeout:   healthTimeout,
		}
		resp, err := client.Get("http://1.1.1.1/cdn-cgi/trace")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return true
		}
		if resp != nil {
			resp.Body.Close()
		}
		resp2, err2 := client.Get("http://api.ipify.org")
		if err2 == nil && resp2.StatusCode == http.StatusOK {
			resp2.Body.Close()
			return true
		}
		if resp2 != nil {
			resp2.Body.Close()
		}
		return false
	}

	// 2. OpenVPN netns 隧道探测
	// 直连 IP 探测（无需域名解析，抗限流）
	out, err := exec.Command("ip", "netns", "exec", t.nsName(),
		"curl", "-s", "--max-time", strconv.Itoa(int(healthTimeout.Seconds())),
		"http://1.1.1.1/cdn-cgi/trace").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "ip=") {
				got := strings.TrimSpace(strings.TrimPrefix(line, "ip="))
				if got != "" {
					return got == t.ExitIP
				}
			}
		}
	}

	// 备用通过 ipify 探测
	out2, err2 := exec.Command("ip", "netns", "exec", t.nsName(),
		"curl", "-s", "--max-time", strconv.Itoa(int(healthTimeout.Seconds())),
		"http://api.ipify.org").Output()
	if err2 == nil {
		got := strings.TrimSpace(string(out2))
		if got != "" {
			return got == t.ExitIP
		}
	}
	return false
}

// reconnect 就地把一条隧道换到别的节点上，保持槽位与端口不变，
// 这样已经分发出去的客户端配置仍然可用。
//
// oldHost 必须是本次重连前那条隧道真正绑着的节点名。调用方若已经
// 改过 t.Node（比如手动换节点），就要把改之前的名字传进来，
// 否则 rebind 找不到旧绑定，入站会掉成孤儿。
func (m *Manager) reconnect(t *Tunnel, oldHost string) {
	t.Status = "starting"
	t.Err = "正在换节点重连"
	t.ExitIP = ""

	if t.ovpn != nil && t.ovpn.Process != nil {
		_ = t.ovpn.Process.Kill()
		t.ovpn = nil
	}
	t.teardownNetns()

	go func() {
		// 通知延后到 rebind/resync 之后：那两步会把入站改绑到新节点，
		// 提前重建配置会因为入站还指着旧节点名而丢掉路由规则
		m.bringUpPersist(t, false, true)
		if t.Status != "up" {
			return
		}
		// 出站 tag 跟着节点名走，换了节点就要把原来指向它的入站重新绑过去，
		// 否则面板里的路由会指向一个已经不存在的出站。
		if t.Node.HostName != oldHost {
			if err := m.rebind(oldHost, t); err != nil {
				log.Printf("重连后同步 3x-ui 绑定失败: %v", err)
			}
			return
		}
		// 节点名没变也要重写一次出站：出口 IP 可能变了，
		// 而且上一轮换节点时留下的绑定需要重新指回来。
		if err := m.resync(t); err != nil {
			log.Printf("重连后重写 3x-ui 出站失败: %v", err)
		}
	}()
}
