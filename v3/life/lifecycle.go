package life

import "sync"

type LifeCycleState int

const (
	LCS_STOPPED LifeCycleState = iota
	LCS_STOPPING
	LCS_STARTING
	LCS_STARTED
)

type LifeCycle struct {
	state LifeCycleState
	m     *sync.RWMutex
}

func NewLifeCycleState() *LifeCycle {
	return &LifeCycle{m: new(sync.RWMutex)}
}

func (state *LifeCycle) GetState() LifeCycleState {
	state.m.RLock()
	defer state.m.RUnlock()

	return state.state
}

func (state *LifeCycle) SetState(s LifeCycleState) {
	state.m.Lock()
	defer state.m.Unlock()

	state.state = s
}

func (state *LifeCycle) AlreadyStarted() bool {
	state.m.RLock()
	defer state.m.RUnlock()

	return state.state == LCS_STARTING || state.state == LCS_STARTED
}

func (state *LifeCycle) AlreadyStopped() bool {
	state.m.RLock()
	defer state.m.RUnlock()

	return state.state == LCS_STOPPING || state.state == LCS_STOPPED
}

func (state *LifeCycle) GetLock() *sync.RWMutex {
	return state.m
}
