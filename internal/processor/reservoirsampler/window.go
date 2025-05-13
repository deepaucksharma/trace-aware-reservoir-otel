package reservoirsampler

import (
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// WindowManager handles time-based sampling windows
type WindowManager struct {
	// Configuration
	windowDuration time.Duration
	
	// State
	currentWindow   int64
	windowStartTime time.Time
	windowEndTime   time.Time
	windowCount     *atomic.Int64
	
	// Synchronization
	lock sync.RWMutex
	
	// Callbacks
	onWindowRollover func()
	
	// Logging
	logger *zap.Logger
}

// NewWindowManager creates a new window manager
func NewWindowManager(windowDuration time.Duration, onWindowRollover func(), logger *zap.Logger) *WindowManager {
	windowCount := atomic.NewInt64(0)
	
	wm := &WindowManager{
		windowDuration:   windowDuration,
		windowCount:      windowCount,
		onWindowRollover: onWindowRollover,
		logger:           logger,
	}
	
	// Initialize the first window
	wm.initializeWindow()
	
	return wm
}

// GetCurrentState returns the current window state
func (w *WindowManager) GetCurrentState() (windowID int64, startTime time.Time, endTime time.Time, count int64) {
	w.lock.RLock()
	defer w.lock.RUnlock()
	
	return w.currentWindow, w.windowStartTime, w.windowEndTime, w.windowCount.Load()
}

// SetState sets the window state (used when loading from checkpoint)
func (w *WindowManager) SetState(windowID int64, startTime time.Time, endTime time.Time, count int64) {
	w.lock.Lock()
	defer w.lock.Unlock()
	
	w.currentWindow = windowID
	w.windowStartTime = startTime
	w.windowEndTime = endTime
	w.windowCount.Store(count)
}

// IncrementCount increments the window span count and returns the new count
func (w *WindowManager) IncrementCount() int64 {
	return w.windowCount.Inc()
}

// CheckRollover checks if it's time to roll over to a new window
// Returns true if a rollover occurred
func (w *WindowManager) CheckRollover() bool {
	now := time.Now()
	
	// Quick check without locking
	if now.Before(w.windowEndTime) {
		return false
	}
	
	w.lock.Lock()
	defer w.lock.Unlock()
	
	// Double check after acquiring the lock
	if now.After(w.windowEndTime) {
		// Notify callback before rolling over
		if w.onWindowRollover != nil {
			w.onWindowRollover()
		}
		
		// Start a new window
		w.initializeWindowLocked()
		
		w.logger.Info("Started new sampling window",
			zap.Int64("window", w.currentWindow),
			zap.Time("start", w.windowStartTime),
			zap.Time("end", w.windowEndTime))
		
		return true
	}
	
	return false
}

// initializeWindow initializes a new sampling window
func (w *WindowManager) initializeWindow() {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.initializeWindowLocked()
}

// initializeWindowLocked initializes a new sampling window (must be called with lock held)
func (w *WindowManager) initializeWindowLocked() {
	now := time.Now()
	
	w.windowStartTime = now
	w.windowEndTime = now.Add(w.windowDuration)
	w.currentWindow++
	w.windowCount.Store(0)
}