package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const absoluteExitLimit = 254

type admissionError struct{ reason string }

func (e *admissionError) Error() string { return e.reason }

type CapacitySettings struct {
	MaxExits             int  `json:"max_exits"` // zero: automatic, bounded by IPv4 namespace layout
	ResourceLimitPercent int  `json:"resource_limit_percent"`
	GuardEnabled         bool `json:"guard_enabled"`
}

func defaultCapacitySettings() CapacitySettings {
	return CapacitySettings{ResourceLimitPercent: 80, GuardEnabled: true}
}
func (s CapacitySettings) validate() error {
	if s.MaxExits < 0 || s.MaxExits > absoluteExitLimit {
		return fmt.Errorf("总出口数量必须在 1–254 之间，0 表示自动")
	}
	if s.ResourceLimitPercent < 50 || s.ResourceLimitPercent > 95 {
		return fmt.Errorf("资源阈值必须在 50–95%% 之间")
	}
	return nil
}

type ResourceSnapshot struct {
	Available     bool      `json:"available"`
	CPUReady      bool      `json:"cpu_ready"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryPercent float64   `json:"memory_percent"`
	MemoryUsed    uint64    `json:"memory_used"`
	MemoryTotal   uint64    `json:"memory_total"`
	SwapUsed      uint64    `json:"swap_used"`
	SwapTotal     uint64    `json:"swap_total"`
	Scope         string    `json:"scope"`
	SampledAt     time.Time `json:"sampled_at"`
	Error         string    `json:"error,omitempty"`
}

type resourceMonitor struct {
	mu                  sync.RWMutex
	current             ResourceSnapshot
	readFile            func(string) ([]byte, error)
	prevTotal, prevIdle uint64
	prevCGUsage         uint64
	prevCGAt            time.Time
}

func newResourceMonitor() *resourceMonitor { return &resourceMonitor{readFile: os.ReadFile} }
func parseCPUStat(data string) (total, idle uint64, err error) {
	lines := strings.Split(data, "\n")
	if len(lines) == 0 {
		return 0, 0, fmt.Errorf("CPU 数据为空")
	}
	f := strings.Fields(lines[0])
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, fmt.Errorf("CPU 数据无效")
	}
	for i := 1; i < len(f) && i <= 8; i++ {
		n, e := strconv.ParseUint(f[i], 10, 64)
		if e != nil {
			return 0, 0, e
		}
		total += n
		if i == 4 || i == 5 {
			idle += n
		}
	}
	return total, idle, nil
}
func parseMemInfo(data string) (map[string]uint64, error) {
	values := map[string]uint64{}
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			continue
		}
		if len(f) > 2 && f[2] == "kB" {
			v *= 1024
		}
		values[strings.TrimSuffix(f[0], ":")] = v
	}
	if values["MemTotal"] == 0 {
		return nil, fmt.Errorf("内存数据无效")
	}
	return values, nil
}
func percentage(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return math.Min(100, math.Max(0, float64(used)/float64(total)*100))
}
func (r *resourceMonitor) sample() {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := ResourceSnapshot{SampledAt: time.Now(), Scope: "host"}
	cpu, e1 := r.readFile("/proc/stat")
	mem, e2 := r.readFile("/proc/meminfo")
	if e1 != nil || e2 != nil {
		s.Error = "无法读取 Linux 资源数据"
		r.current = s
		return
	}
	total, idle, err := parseCPUStat(string(cpu))
	if err != nil {
		s.Error = err.Error()
		r.current = s
		return
	}
	values, err := parseMemInfo(string(mem))
	if err != nil {
		s.Error = err.Error()
		r.current = s
		return
	}
	s.Available = true
	if r.prevTotal > 0 && total > r.prevTotal && idle >= r.prevIdle {
		delta := total - r.prevTotal
		idleDelta := idle - r.prevIdle
		if idleDelta <= delta {
			s.CPUReady = true
			s.CPUPercent = percentage(delta-idleDelta, delta)
		}
	}
	r.prevTotal, r.prevIdle = total, idle
	s.MemoryTotal = values["MemTotal"]
	available, ok := values["MemAvailable"]
	if !ok {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	if available > s.MemoryTotal {
		available = s.MemoryTotal
	}
	s.MemoryUsed = s.MemoryTotal - available
	s.MemoryPercent = percentage(s.MemoryUsed, s.MemoryTotal)
	s.SwapTotal = values["SwapTotal"]
	if values["SwapFree"] <= s.SwapTotal {
		s.SwapUsed = s.SwapTotal - values["SwapFree"]
	}
	// A systemd/container memory limit may be stricter than the guest's RAM.
	cg, err := r.readFile("/proc/self/cgroup")
	if err == nil {
		for _, line := range strings.Split(string(cg), "\n") {
			parts := strings.SplitN(line, ":", 3)
			if len(parts) != 3 {
				continue
			}
			var base, limitFile, usedFile string
			if parts[1] == "" {
				base = filepath.Join("/sys/fs/cgroup", filepath.Clean("/"+parts[2]))
				r.applyCgroupCPU(&s, base)
				limitFile = "memory.max"
				usedFile = "memory.current"
			} else if strings.Contains(","+parts[1]+",", ",memory,") {
				base = filepath.Join("/sys/fs/cgroup/memory", filepath.Clean("/"+parts[2]))
				limitFile = "memory.limit_in_bytes"
				usedFile = "memory.usage_in_bytes"
			} else {
				continue
			}
			lb, e := r.readFile(filepath.Join(base, limitFile))
			if e != nil {
				continue
			}
			ub, e := r.readFile(filepath.Join(base, usedFile))
			if e != nil {
				continue
			}
			limit, e := strconv.ParseUint(strings.TrimSpace(string(lb)), 10, 64)
			if e != nil || limit == 0 || limit > s.MemoryTotal {
				continue
			}
			used, e := strconv.ParseUint(strings.TrimSpace(string(ub)), 10, 64)
			if e != nil {
				continue
			}
			p := percentage(used, limit)
			if p > s.MemoryPercent {
				s.MemoryPercent = p
				s.MemoryUsed = used
				s.MemoryTotal = limit
				s.Scope = "cgroup"
			}
		}
	}
	r.current = s
}

func (r *resourceMonitor) applyCgroupCPU(s *ResourceSnapshot, base string) {
	b, e := r.readFile(filepath.Join(base, "cpu.max"))
	if e != nil {
		return
	}
	f := strings.Fields(string(b))
	if len(f) != 2 || f[0] == "max" {
		return
	}
	quota, e1 := strconv.ParseFloat(f[0], 64)
	period, e2 := strconv.ParseFloat(f[1], 64)
	if e1 != nil || e2 != nil || quota <= 0 || period <= 0 {
		return
	}
	b, e = r.readFile(filepath.Join(base, "cpu.stat"))
	if e != nil {
		return
	}
	var usage uint64
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "usage_usec" {
			usage, e = strconv.ParseUint(f[1], 10, 64)
			found = e == nil
			break
		}
	}
	if !found {
		return
	}
	if !r.prevCGAt.IsZero() && usage >= r.prevCGUsage {
		elapsed := s.SampledAt.Sub(r.prevCGAt).Seconds()
		if elapsed > 0 {
			load := math.Min(100, float64(usage-r.prevCGUsage)/1e6/elapsed/(quota/period)*100)
			if load > s.CPUPercent {
				s.CPUPercent = load
			}
			s.CPUReady = true
		}
	}
	r.prevCGUsage = usage
	r.prevCGAt = s.SampledAt
}
func (r *resourceMonitor) snapshot() ResourceSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}
func (r *resourceMonitor) run(stop <-chan struct{}) {
	r.sample()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			r.sample()
		}
	}
}

func capacityReason(s CapacitySettings, r ResourceSnapshot) string {
	if !s.GuardEnabled {
		return ""
	}
	if !r.Available || !r.CPUReady || time.Since(r.SampledAt) > 5*time.Second {
		return "资源监测正在初始化或不可用，暂缓新增出口"
	}
	limit := float64(s.ResourceLimitPercent)
	if r.MemoryPercent >= limit {
		return fmt.Sprintf("内存占用 %.1f%% 已达到 %d%% 阈值，暂停新增出口", r.MemoryPercent, s.ResourceLimitPercent)
	}
	if r.CPUPercent >= limit {
		return fmt.Sprintf("CPU 占用 %.1f%% 已达到 %d%% 阈值，暂停新增出口", r.CPUPercent, s.ResourceLimitPercent)
	}
	return ""
}

func (m *Manager) configureCapacity(explicitMax *int) error {
	settings := defaultCapacitySettings()
	path := filepath.Join(m.workDir, "capacity.json")
	b, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(b, &settings); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if explicitMax != nil {
		settings.MaxExits = *explicitMax
	}
	if err = settings.validate(); err != nil {
		return err
	}
	if err = m.setCapacity(settings); err != nil {
		return err
	}
	m.resources = newResourceMonitor()
	go m.resources.run(m.stopCh)
	return nil
}
func (m *Manager) setCapacity(s CapacitySettings) error {
	if err := s.validate(); err != nil {
		return err
	}
	m.capacityMu.Lock()
	defer m.capacityMu.Unlock()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(m.workDir, "capacity.json")
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	m.mu.Lock()
	m.capacity = s
	m.maxSlots = s.MaxExits
	if m.maxSlots == 0 {
		m.maxSlots = absoluteExitLimit
	}
	m.capacityConfigured = true
	m.mu.Unlock()
	return nil
}
func (m *Manager) admissionReason() string {
	m.mu.RLock()
	s, configured := m.capacity, m.capacityConfigured
	m.mu.RUnlock()
	if !configured {
		return ""
	}
	if m.resources == nil {
		if s.GuardEnabled {
			return "资源监测尚未启动"
		}
		return ""
	}
	return capacityReason(s, m.resources.snapshot())
}

func (m *Manager) startWithResourceWait(node Node, mode string, exact bool) (*Tunnel, error) {
	deadline := time.Now().Add(90 * time.Second)
	for {
		var t *Tunnel
		var err error
		if exact {
			t, err = m.StartExact(node)
		} else {
			t, err = m.StartWithPolicy(node, mode)
		}
		if err == nil {
			return t, nil
		}
		if _, limited := err.(*admissionError); !limited {
			return nil, err
		}
		if !strings.HasPrefix(err.Error(), "正在建立其他出口") && !strings.HasPrefix(err.Error(), "资源监测正在初始化") {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("等待启动额度超时：%w", err)
		}
		select {
		case <-m.stopCh:
			return nil, fmt.Errorf("服务正在停止")
		case <-time.After(time.Second):
		}
	}
}
func (m *Manager) apiCapacity() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var s CapacitySettings
			if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
				writeJSON(w, 400, map[string]string{"error": "请求格式错误"})
				return
			}
			if err := m.setCapacity(s); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
		} else if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET, POST")
			writeJSON(w, 405, map[string]string{"error": "仅支持 GET 或 POST"})
			return
		}
		m.mu.RLock()
		settings, active, maxSlots, pending := m.capacity, len(m.tunnels), m.maxSlots, len(m.pendingRestore)
		m.mu.RUnlock()
		resources := ResourceSnapshot{}
		if m.resources != nil {
			resources = m.resources.snapshot()
		}
		reason := m.admissionReason()
		if active >= maxSlots {
			reason = fmt.Sprintf("总出口已达到数量上限 %d", maxSlots)
		}
		writeJSON(w, 200, map[string]any{"settings": settings, "resources": resources, "active": active, "max_exits": maxSlots, "pending_restore": pending, "blocked": reason != "", "reason": reason})
	}
}
