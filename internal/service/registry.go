package service

import (
	"sort"
	"strings"
	"sync"

	"github.com/magomedcoder/gen/api/pb/app/runnerpb"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
)

type RunnerState struct {
	ID            int64
	Address       string
	Host          string
	Port          int32
	Name          string
	Enabled       bool
	SelectedModel string
	Hints         *RunnerCoreHints
}

type Registry struct {
	mu      sync.RWMutex
	runners map[string]RunnerState
}

func RunnerStatesFromDomain(rows []domain.Runner) []RunnerState {
	out := make([]RunnerState, 0, len(rows))
	for _, r := range rows {
		addr := domain.RunnerListenAddress(r.Host, r.Port)
		if addr == "" {
			continue
		}
		out = append(out, RunnerState{
			ID:            r.ID,
			Address:       addr,
			Host:          strings.TrimSpace(r.Host),
			Port:          r.Port,
			Name:          strings.TrimSpace(r.Name),
			Enabled:       r.Enabled,
			SelectedModel: strings.TrimSpace(r.SelectedModel),
		})
	}
	return out
}

func NewRegistry(initial []RunnerState) *Registry {
	r := &Registry{runners: make(map[string]RunnerState)}
	r.ReplaceAll(initial)
	return r
}

func (r *Registry) ReplaceAll(states []RunnerState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := make(map[string]RunnerState, len(states))
	for _, st := range states {
		addr := strings.TrimSpace(st.Address)
		if addr == "" {
			continue
		}
		st.Address = addr
		next[addr] = st
	}
	r.runners = next
}

func (r *Registry) Register(addr string) {
	r.RegisterWithNameAndHints(addr, "", nil)
}

func (r *Registry) RegisterWithName(addr, name string) {
	r.RegisterWithNameAndHints(addr, name, nil)
}

func (r *Registry) RegisterWithNameAndHints(addr, name string, hints *RunnerCoreHints) {
	if addr == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.runners[addr]
	if !ok {
		r.runners[addr] = RunnerState{
			Address: addr,
			Name:    strings.TrimSpace(name),
			Enabled: true,
			Hints:   hints,
		}
		logger.I("Registry: раннер зарегистрирован: %s", addr)
		return
	}
	if strings.TrimSpace(name) != "" {
		existing.Name = strings.TrimSpace(name)
	}

	if hints != nil {
		existing.Hints = hints
	}

	r.runners[addr] = existing
}

func (r *Registry) Unregister(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runners[addr]; ok {
		delete(r.runners, addr)
		logger.I("Registry: раннер отключён: %s", addr)
	}
}

func (r *Registry) GetRunners() []*runnerpb.RunnerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*runnerpb.RunnerInfo, 0, len(r.runners))
	for _, state := range r.runners {
		out = append(out, &runnerpb.RunnerInfo{
			Address:       state.Address,
			Enabled:       state.Enabled,
			Name:          state.Name,
			Id:            state.ID,
			Host:          state.Host,
			Port:          state.Port,
			SelectedModel: state.SelectedModel,
		})
	}
	return out
}

func (r *Registry) DefaultRunnerListenAddress() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	type pair struct {
		id   int64
		addr string
	}
	var list []pair
	for _, st := range r.runners {
		if !st.Enabled {
			continue
		}
		a := strings.TrimSpace(st.Address)
		if a == "" {
			continue
		}
		list = append(list, pair{st.ID, a})
	}
	if len(list) == 0 {
		return ""
	}
	sort.Slice(list, func(i, j int) bool { return list[i].id < list[j].id })
	return list[0].addr
}

func (r *Registry) SetEnabled(addr string, enabled bool) {
	a := strings.TrimSpace(addr)
	r.mu.Lock()
	defer r.mu.Unlock()
	if state, ok := r.runners[a]; ok {
		state.Enabled = enabled
		r.runners[a] = state
	}
}

func (r *Registry) HasActiveRunners() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, state := range r.runners {
		if state.Enabled {
			return true
		}
	}
	return false
}

func (r *Registry) GetEnabledAddresses() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for _, state := range r.runners {
		if state.Enabled {
			out = append(out, state.Address)
		}
	}
	return out
}

func (r *Registry) IsEnabledRunner(addr string) bool {
	a := strings.TrimSpace(addr)
	if a == "" {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	st, ok := r.runners[a]

	return ok && st.Enabled
}

func (r *Registry) GetByID(id int64) (RunnerState, bool) {
	if id <= 0 {
		return RunnerState{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, st := range r.runners {
		if st.ID == id {
			return st, true
		}
	}
	return RunnerState{}, false
}
