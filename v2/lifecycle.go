package server

import "sync"

var lifeCycleStateIMap map[lifeCycleI]*sync.Mutex

type lifeCycleState int

const (
	lcs_stopped  lifeCycleState = iota
	lcs_stopping
	lcs_starting
	lcs_started
)

func (state lifeCycleState) AlreadyStarted() bool {
	return state == lcs_starting || state == lcs_started
}

func (state lifeCycleState) AlreadyStopped() bool {
	return state == lcs_stopping || state == lcs_stopped
}

type lifeCycleI interface {
	getState() lifeCycleState
	setState(state lifeCycleState)
}

func getLifegetLifeCycleILock(i lifeCycleI) *sync.Mutex {
	lock := lifeCycleStateIMap[i]
	if lock != nil {
		return lock
	}

	lock = new(sync.Mutex)
	lifeCycleStateIMap[i] = lock
	return lock
}

func getLifeCycleState(i lifeCycleI) lifeCycleState {
	lock := getLifegetLifeCycleILock(i)
	lock.Lock()
	defer lock.Unlock()

	return i.getState()
}

func setLifeCycleState(i lifeCycleI, state lifeCycleState) {
	lock := getLifegetLifeCycleILock(i)
	lock.Lock()
	defer lock.Unlock()

	i.setState(state)
}
