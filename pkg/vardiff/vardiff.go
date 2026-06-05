package vardiff

import (
	"sync"
	"time"
)

type VarDiff struct {
	targetShareRate  float64 // shares per minute
	retargetInterval time.Duration
	minDiff          float64
	maxDiff          float64
	variancePercent  float64
}

type MinerState struct {
	mu               sync.Mutex
	currentDiff      float64
	sharesInWindow   int
	windowStart      time.Time
	lastRetarget     time.Time
}

func New(targetRate, minDiff, maxDiff, variance float64, interval time.Duration) *VarDiff {
	return &VarDiff{
		targetShareRate:  targetRate,
		retargetInterval: interval,
		minDiff:          minDiff,
		maxDiff:          maxDiff,
		variancePercent:  variance,
	}
}

func (v *VarDiff) NewMinerState(initialDiff float64) *MinerState {
	return &MinerState{
		currentDiff:  initialDiff,
		windowStart:  time.Now(),
		lastRetarget: time.Now(),
	}
}

func (v *VarDiff) RecordShare(state *MinerState) float64 {
	state.mu.Lock()
	defer state.mu.Unlock()
	
	state.sharesInWindow++
	
	if time.Since(state.lastRetarget) < v.retargetInterval {
		return state.currentDiff
	}
	
	elapsed := time.Since(state.windowStart).Minutes()
	if elapsed < 1 {
		return state.currentDiff
	}
	
	actualRate := float64(state.sharesInWindow) / elapsed
	
	if actualRate < 1 {
		return state.currentDiff
	}
	
	ratio := actualRate / v.targetShareRate
	
	allowedVariance := v.variancePercent / 100.0
	if ratio > 1-allowedVariance && ratio < 1+allowedVariance {
		return state.currentDiff
	}
	
	newDiff := state.currentDiff * ratio
	
	if newDiff < v.minDiff {
		newDiff = v.minDiff
	}
	if newDiff > v.maxDiff {
		newDiff = v.maxDiff
	}
	
	state.currentDiff = newDiff
	state.sharesInWindow = 0
	state.windowStart = time.Now()
	state.lastRetarget = time.Now()
	
	return newDiff
}
