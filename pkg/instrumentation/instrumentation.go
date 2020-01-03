package instrumentation

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	labelSuccess = "success"
)

type Instrumentation struct {
	syncDuration *prometheus.HistogramVec
}

// New allocates and returns an Instrumentation struct with metrics registered
// on provided registry.
func New(registry prometheus.Registerer) (*Instrumentation, error) {
	i := Instrumentation{
		syncDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "postgresql_controller",
			Subsystem: "daemon",
			Name:      "sync_duration_seconds",
			Help:      "Duration of resource-to-database synchronisation, in seconds.",
			Buckets:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 1, 2, 4, 6, 8, 10, 15},
		}, []string{labelSuccess}),
	}
	err := registry.Register(i.syncDuration)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (i *Instrumentation) ObserveSyncDuration(d time.Duration, success bool) {
	i.syncDuration.WithLabelValues(strconv.FormatBool(success)).Observe(d.Seconds())
}
