// Package daemon provides a Daemon struct that runs a controller loop at a
// specified interval.
//
// It handles loop life cycle events and only requires an interval duration and
// a function that will be called within each interval.
package daemon

import (
	"time"

	"github.com/go-logr/logr"
)

// Configuration is a configuration struct specifying what and how a Daemon
// instance must run.
//
// Default values are set for all fields so they can be omitted. Be sure to set
// the Sync function though as without it nothing will every be triggered by the
// daemon.
type Configuration struct {
	Logger       logr.Logger
	SyncInterval time.Duration

	// Sync is the function called in every sync interval by the daemon.
	Sync func()
}

func (c *Configuration) setDefaults() {
	if c.SyncInterval == 0 {
		c.SyncInterval = 5 * time.Minute
	}
	if c.Sync == nil {
		c.Sync = func() {}
	}
}

// Daemon provides a scheduled invokation of the configured Sync function. Start
// the daemon by call the blocking method Loop and stop it again by closing the
// provided stop channel.
type Daemon struct {
	config Configuration
	// syncSoon is a limited buffer of sync requests. Use method askForSync to
	// schedule new syncs through the buffer.
	syncSoon chan struct{}
}

// New allocates and returns an unstarted Daemon struct.
func New(c Configuration) *Daemon {
	c.setDefaults()
	d := Daemon{
		config:   c,
		syncSoon: make(chan struct{}, 1),
	}
	return &d
}

// askForSync requests a new sync. It ensures that only one sync can be running
// at any given time and drops sync requests if one is already running.
func (d *Daemon) askForSync() {
	select {
	case d.syncSoon <- struct{}{}:
	default:
		d.config.Logger.Info("Skipping sync as one is already in progress")
	}
}

// Loop starts the daemon syncronization loop. It will run until provided stop
// channel is closed.
func (d *Daemon) Loop(stop chan struct{}) {
	d.config.Logger.Info("Starting loop")
	syncTimer := time.NewTimer(d.config.SyncInterval)
	d.askForSync()

	for {
		select {
		case <-stop:
			d.config.Logger.Info("Stopping loop")
			// ensure to drain the timer channel before exiting as we don't know if
			// the shutdown is started before or after the timer have triggered.
			if !syncTimer.Stop() {
				select {
				case <-syncTimer.C:
				default:
				}
			}
			return
		case <-d.syncSoon:
			// ensure to drain the timer channel before syncing as we don't know if
			// the sync was scheduled by the timer or another event.
			if !syncTimer.Stop() {
				select {
				case <-syncTimer.C:
				default:
				}
			}
			d.config.Sync()
			syncTimer.Reset(d.config.SyncInterval)
		case <-syncTimer.C:
			d.config.Logger.Info("Sync timer asking for sync")
			// request a new sync in the sync buffer. This might be a noop if a sync
			// is already running.
			d.askForSync()
		}
	}
}
