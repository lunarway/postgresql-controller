package daemon_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/pkg/daemon"
	"go.lunarway.com/postgresql-controller/test"
)

func TestLoop(t *testing.T) {
	logger := test.NewLogger(t)
	syncInterval := 10 * time.Millisecond
	testDuration := 100 * time.Millisecond
	expectedSyncCount := int32(10)

	var actualSyncCount int32
	d := daemon.New(daemon.Configuration{
		SyncInterval: syncInterval,
		Logger:       logger,
		Sync: func() {
			atomic.AddInt32(&actualSyncCount, 1)
		},
	})

	shutdown := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go d.Loop(shutdown, &wg)
	time.Sleep(testDuration)

	close(shutdown)

	// wait for wait group to be done but limit the time with a timeout in case
	// the loop never stops.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-time.After(5 * time.Second):
		t.Fatal("Loop did not stop on shutdown signal")
	case <-done:
	}

	// assert that we sync multiple times with at most 20% off of the expected
	// count. The exact number is not important as we only want to verify that the
	// loop actually loops.
	assert.InEpsilon(t, expectedSyncCount, actualSyncCount, 0.2, "sync count %d not as the expected %d", actualSyncCount, expectedSyncCount)
}
