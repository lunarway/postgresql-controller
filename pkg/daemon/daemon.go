package daemon

import (
	"sync"
	"time"

	"github.com/go-logr/logr"
)

type Daemon struct {
}

func (d *Daemon) Loop(stop chan struct{}, wg *sync.WaitGroup, logger logr.Logger) {
	logger = logger.WithName("daemon")
	defer wg.Done()
	syncTimer := time.NewTimer(5 * time.Second)
	for {
		select {
		case <-stop:
			logger.Info("Stopping loop")
			if !syncTimer.Stop() {
				<-syncTimer.C
			}
			return
		case <-syncTimer.C:
			logger.Info("Syncing...")
		}
	}
}
