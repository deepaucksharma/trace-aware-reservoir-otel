package reservoir

import (
	"sync"
	"time"
)

// TimeWindow manages time-based windows for the reservoir
type TimeWindow struct {
	duration        time.Duration
	currentID       int64
	currentStart    time.Time
	currentEnd      time.Time
	mu              sync.RWMutex
	rolloverCounter int64
	callback        func()
}

// NewTimeWindow creates a new time window manager
func NewTimeWindow(duration time.Duration) *TimeWindow {
	now := time.Now()
	return &TimeWindow{
		duration:     duration,
		currentID:    now.Unix(),
		currentStart: now,
		currentEnd:   now.Add(duration),
	}
}

// SetRolloverCallback sets the function to call when a window rolls over
func (w *TimeWindow) SetRolloverCallback(callback func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	w.callback = callback
}

// CheckRollover checks if the current time window has expired
func (w *TimeWindow) CheckRollover() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	now := time.Now()
	if now.After(w.currentEnd) {
		// Create a new window
		w.currentID = now.Unix()
		w.currentStart = now
		w.currentEnd = now.Add(w.duration)
		w.rolloverCounter++
		
		// Call the rollover callback if set
		if w.callback != nil {
			go w.callback()
		}
		
		return true
	}
	
	return false
}

// Current returns the current window information
func (w *TimeWindow) Current() (id int64, start, end time.Time) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	return w.currentID, w.currentStart, w.currentEnd
}

// SetState allows manual setting of window state (used for restoring from checkpoint)
func (w *TimeWindow) SetState(id int64, start, end time.Time, count int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	w.currentID = id
	w.currentStart = start
	w.currentEnd = end
	w.rolloverCounter = count
}

// GetDuration returns the configured window duration
func (w *TimeWindow) GetDuration() time.Duration {
	return w.duration
}
