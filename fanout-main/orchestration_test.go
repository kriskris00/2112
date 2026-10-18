package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestOrchestrationReservationIsAtomic(t *testing.T) {
	m := &Manager{}
	var winners int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if m.tryBeginOrchestration() {
				atomic.AddInt32(&winners, 1)
			}
		}()
	}
	wg.Wait()
	if winners != 1 {
		t.Fatalf("got %d task owners", winners)
	}
	defer m.orchestrateMu.Unlock()
	if !m.OrchestrateProgress().Running {
		t.Fatal("task not visible before worker dispatch")
	}
}

func TestBusyOrchestrationPreservesActiveProgress(t *testing.T) {
	m := &Manager{}
	if !m.tryBeginOrchestration() {
		t.Fatal("reservation failed")
	}
	defer m.orchestrateMu.Unlock()
	m.setOrchestrateProgress(func(p *AutoOrchestrateProgress) {
		p.Stage = "probe"
		p.CandidatesTested = 7
		p.Message = "active progress"
	})
	before := m.OrchestrateProgress()
	m.AutoOrchestrateWithOptions(DefaultAutoOrchestrateOptions())
	if got := m.OrchestrateProgress(); got != before {
		t.Fatalf("busy caller overwrote active progress: %+v", got)
	}
}

func TestOrchestrationBusyAttachesInsteadOf409(t *testing.T) {
	m := &Manager{}
	if !m.tryBeginOrchestration() {
		t.Fatal("reservation failed")
	}
	defer m.orchestrateMu.Unlock()
	before := m.OrchestrateProgress()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/auto/orchestrate", strings.NewReader(`{"max_starts":8}`))
			w := httptest.NewRecorder()
			apiAutoOrchestrate(m)(w, req)
			if w.Code != http.StatusAccepted {
				t.Errorf("busy status: %d", w.Code)
				return
			}
			var body struct {
				Status   string                  `json:"status"`
				Progress AutoOrchestrateProgress `json:"progress"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Error(err)
				return
			}
			if body.Status != "busy" || !body.Progress.Running {
				t.Error("missing active task state")
			}
		}()
	}
	wg.Wait()
	if m.OrchestrateProgress() != before {
		t.Fatal("duplicate HTTP requests changed progress")
	}
}

func TestOrchestrationFullCapacityExitsWithoutProbing(t *testing.T) {
	tunnel := &Tunnel{Slot: 1, Status: "up"}
	m := &Manager{maxSlots: 1, tunnels: map[int]*Tunnel{1: tunnel}}
	m.AutoOrchestrateWithOptions(DefaultAutoOrchestrateOptions())
	p := m.OrchestrateProgress()
	if p.Running || p.CandidatesTested != 0 || !strings.Contains(p.StopReason, "槽位已满") {
		t.Fatalf("full capacity did not exit: %+v", p)
	}
	if !m.orchestrateMu.TryLock() {
		t.Fatal("task lock leaked")
	}
	m.orchestrateMu.Unlock()
	if m.tunnels[1] != tunnel {
		t.Fatal("existing tunnel was changed")
	}
}

func TestAvailableOrchestrationSlotsHonorsLegacyOccupancy(t *testing.T) {
	m := &Manager{maxSlots: 2, tunnels: map[int]*Tunnel{72: {Slot: 72}, 139: {Slot: 139}}}
	if got := m.availableOrchestrationSlots(); got != 0 {
		t.Fatalf("legacy occupied slots ignored: %d", got)
	}
}

func TestFreeSlotHonorsLegacyOccupancy(t *testing.T) {
	m := &Manager{maxSlots: 2, tunnels: map[int]*Tunnel{72: {Slot: 72}, 139: {Slot: 139}}}
	if _, err := m.freeSlot(); err == nil {
		t.Fatal("legacy slots allowed capacity overflow")
	}
}
