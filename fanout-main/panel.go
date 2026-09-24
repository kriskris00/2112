package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Panel 是 fanout 管理节点链接的后端。
//
// 有两个实现：接管本机 3x-ui 面板的 XUI，以及 fanout 自己跑 Xray 的 Native。
// 界面和编排层只依赖这个接口，两种模式下的操作语义完全一致。
type Panel interface {
	// Kind 返回 "3x-ui" 或 "native"，界面据此提示当前模式。
	Kind() string
	// Describe 给出一行人能读的后端说明。
	Describe() string

	Inbounds(live map[string]bool) ([]Inbound, error)
	InboundDetail(id int, publicHost string) (*InboundDetail, error)
	InboundLinks(ids []int, publicHost string) ([]string, error)

	Bind(inboundTag string, hostname string, tunnels []*Tunnel) error
	Rebind(oldHost string, target *Tunnel, tunnels []*Tunnel) error
	ResyncOutbound(t *Tunnel, tunnels []*Tunnel) error

	CloneToTunnels(templateID int, hosts []string, tunnels []*Tunnel) ([]int, error)
	DeleteInbounds(ids []int, tunnels []*Tunnel) error

	// CreateInbound 新建一个入站。自建模式写自己的库并重建 Xray 配置，
	// 接管 3x-ui 时走面板的 inbounds/add API，让面板照常管这条入站。
	CreateInbound(spec NewInboundSpec, tunnels []*Tunnel) (*CreatedInbound, error)

	// UpdateInbound 改端口、备注与启停。只有非零/非 nil 的字段会被写入。
	UpdateInbound(id int, patch InboundPatch, tunnels []*Tunnel) error

	// AddClient 给入站加一个客户端，email 留空时自动命名。
	AddClient(id int, email string, tunnels []*Tunnel) error
	// DeleteClient 摘掉入站上的一个客户端。
	DeleteClient(id int, email string, tunnels []*Tunnel) error
	// ResetClient 换掉客户端的凭据（UUID / trojan 密码），已分发的旧链接随即失效。
	ResetClient(id int, email string, tunnels []*Tunnel) error

	// OnTunnelsChanged 在隧道集合变化后调用。
	//
	// 自建模式的出站完全由隧道列表推导，新开的出口必须重建配置才有对应出站；
	// 接管 3x-ui 时出站在 Bind/Clone 里顺带同步，这里是空操作，
	// 免得每开一条隧道就白重启一次面板的 Xray。
	OnTunnelsChanged(tunnels []*Tunnel) error

	// Close 释放后端占用的资源。自建模式要停掉自己拉起的 Xray，
	// 否则 fanout 退出后它会变成孤儿进程，下次启动撞端口。
	Close()
}

// InboundPatch 描述对入站的一次局部修改。指针为 nil 表示该字段不动。
type InboundPatch struct {
	Port   *int
	Remark *string
	Enable *bool
}

// CreatedInbound 是新建入站后回给界面的摘要。
type CreatedInbound struct {
	ID       int    `json:"id"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Remark   string `json:"remark"`
	Network  string `json:"network"`
	Security string `json:"security"`
}

// closePanel 在进程退出时释放后端资源。
func closePanel() {
	panelState.mu.Lock()
	p := panelState.current
	panelState.mu.Unlock()
	if p != nil {
		p.Close()
	}
}

// panelState 缓存已选定的后端。探测涉及执行 x-ui 命令，没必要每个请求都做一次。
var panelState struct {
	mu      sync.Mutex
	current Panel
	workDir string
	forced  string
}

// panelModeFile 存界面里选过的后端。命令行 -panel 优先级更高。
func panelModeFile(dir string) string { return filepath.Join(dir, "panel_mode") }

// configurePanel 记录自建模式需要的工作目录与用户指定的模式。
// mode 为空表示读盘上界面选过的模式，都没有才自动探测。
func configurePanel(workDir, mode string) {
	panelState.mu.Lock()
	defer panelState.mu.Unlock()
	panelState.workDir = workDir
	if mode == "" {
		blob, err := os.ReadFile(panelModeFile(workDir))
		if err == nil {
			mode = strings.TrimSpace(string(blob))
		}
	}
	panelState.forced = mode
	panelState.current = nil
}

// savePanelMode 把界面选的后端记到工作目录，重启后仍然生效。空值等于删档回到自动探测。
func savePanelMode(dir, mode string) error {
	if dir == "" {
		return nil
	}
	path := panelModeFile(dir)
	if mode == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, []byte(mode), 0600)
}

// openPanel 专一接管本机 3x-ui 面板
func openPanel() (Panel, error) {
	panelState.mu.Lock()
	defer panelState.mu.Unlock()

	if panelState.current != nil {
		return panelState.current, nil
	}

	// 专一联动 3x-ui 面板
	x, err := DetectXUI(panelState.workDir)
	if err == nil && x != nil {
		panelState.current = x
		return x, nil
	}

	// 若 3x-ui 暂未启动，尝试唤醒系统服务
	if hasCmd("systemctl") {
		_ = exec.Command("systemctl", "start", "x-ui").Run()
		time.Sleep(1 * time.Second)
		if x2, err2 := DetectXUI(panelState.workDir); err2 == nil && x2 != nil {
			panelState.current = x2
			return x2, nil
		}
	} else if hasCmd("rc-service") {
		_ = exec.Command("rc-service", "x-ui", "start").Run()
		time.Sleep(1 * time.Second)
		if x2, err2 := DetectXUI(panelState.workDir); err2 == nil && x2 != nil {
			panelState.current = x2
			return x2, nil
		}
	}

	if err != nil {
		return nil, fmt.Errorf("联动 3x-ui 面板失败: %w (请确保 3x-ui 服务已启动)", err)
	}
	return nil, fmt.Errorf("未检测到 3x-ui 面板，请确认已安装并运行")
}

// currentPanelMode 返回当前生效的后端类型（统一为 3x-ui）。
func currentPanelMode() string {
	return "3x-ui"
}

// availablePanelModes 探测 3x-ui 在本机是否可用。
func availablePanelModes(workDir string) []map[string]any {
	modes := []map[string]any{}
	xuiOK, xuiReason := true, ""
	if _, err := DetectXUI(workDir); err != nil {
		xuiOK, xuiReason = false, err.Error()
	}
	modes = append(modes, map[string]any{"mode": "3x-ui", "label": "3x-ui 面板 (专一联动)", "available": xuiOK, "reason": xuiReason})
	return modes
}

// switchPanelMode 运行时切换后端。mode 传空表示恢复自动探测。
//
// 先关掉旧后端释放资源，再按新模式探测；探测失败时回滚到自动模式，
// 避免把 fanout 卡在一个连不上的后端上。
func switchPanelMode(mode string) (Panel, error) {
	switch mode {
	case "", "3x-ui", "native", "xray-cf-lite":
	default:
		return nil, fmt.Errorf("未知后端模式 %q", mode)
	}

	panelState.mu.Lock()
	old := panelState.current
	workDir := panelState.workDir
	panelState.mu.Unlock()
	if old != nil {
		old.Close()
	}

	panelState.mu.Lock()
	panelState.forced = mode
	panelState.current = nil
	panelState.mu.Unlock()

	p, err := openPanel()
	if err != nil {
		// 回滚到自动探测，别把用户卡在坏模式里
		panelState.mu.Lock()
		panelState.forced = ""
		panelState.current = nil
		panelState.mu.Unlock()
		return nil, err
	}
	if err := savePanelMode(workDir, mode); err != nil {
		log.Printf("记录后端模式失败（本次切换仍然生效）: %v", err)
	}
	return p, nil
}

// xuiAbsent 判断本机是否根本没装 3x-ui。
func xuiAbsent() bool {
	if _, err := os.Stat(xuiBinary); err == nil {
		return false
	}
	if _, err := os.Stat(xuiMenu); err == nil {
		return false
	}
	return true
}
