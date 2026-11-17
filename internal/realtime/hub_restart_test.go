package realtime

import (
	"sync"
	"testing"
	"time"
)

// These tests guard #79: each background loop runs once, whatever Restart is
// called on.

// hubLoopCount is the number of background loops Start and Restart run.
const hubLoopCount = 4

// waitLoops waits until want loops are running, then checks the count stays
// there, so a loop that starts or stops late still fails the test.
func waitLoops(t *testing.T, eh *Hub, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for eh.loopsRunning.Load() != want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := eh.loopsRunning.Load(); got != want {
		t.Fatalf("loops running = %d, want %d", got, want)
	}
}

func TestRestartOnLiveHubKeepsOneCopyOfEachLoop(t *testing.T) {
	eh := newTestHub(t)
	eh.startLoops()
	waitLoops(t, eh, hubLoopCount)

	if err := eh.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	waitLoops(t, eh, hubLoopCount)
}

func TestRestartAfterCancelStartsOneCopyOfEachLoop(t *testing.T) {
	eh := newTestHub(t)
	eh.startLoops()
	waitLoops(t, eh, hubLoopCount)

	eh.Cancel()
	// Several connections can see the cancelled context at once and all call
	// Restart (websocket.controller.go).
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := eh.Restart(); err != nil {
				t.Errorf("Restart: %v", err)
			}
		}()
	}
	wg.Wait()
	t.Cleanup(func() { eh.Cancel() })

	waitLoops(t, eh, hubLoopCount)
}
