package service

import (
	"strings"
	"sync"

	"github.com/magomedcoder/gen/api/pb/runnerpb"
	"github.com/magomedcoder/gen/pkg/logger"
)

type RunnerState struct {
	Address string
	Name    string
	Enabled bool
	Hints   *RunnerCoreHints
}

type Registry struct {
	mu      sync.RWMutex
	runners map[string]RunnerState
}

func NewRegistry(initial []RunnerState) *Registry {
	runners := make(map[string]RunnerState)
	for _, r := range initial {
		addr := strings.TrimSpace(r.Address)
		if addr != "" {
			runners[addr] = RunnerState{
				Address: addr,
				Name:    strings.TrimSpace(r.Name),
				Enabled: true,
			}
		}
	}
	return &Registry{runners: runners}
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
			Address: state.Address,
			Name:    state.Name,
			Enabled: state.Enabled,
		})
	}
	return out
}

func (r *Registry) SetEnabled(addr string, enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if state, ok := r.runners[addr]; ok {
		state.Enabled = enabled
		r.runners[addr] = state
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
