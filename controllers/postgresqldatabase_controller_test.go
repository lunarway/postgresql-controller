package controllers

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	ctlerrors "go.lunarway.com/postgresql-controller/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStatus_update(t *testing.T) {
	before := metav1.Time{
		Time: time.Date(2019, time.December, 18, 17, 7, 3, 0, time.UTC),
	}
	now := metav1.Time{
		Time: time.Date(2019, time.December, 18, 18, 7, 3, 0, time.UTC),
	}
	tt := []struct {
		name    string
		status  status
		err     error
		changes bool
		after   *lunarwayv1alpha1.PostgreSQLDatabase
	}{
		{
			name: "new status",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase: "",
					},
				},
			},
			err: &ctlerrors.Invalid{
				Err: errors.New("some validation error"),
			},
			changes: true,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
					PhaseUpdated: now,
					Error:        "some validation error",
				},
			},
		},
		{
			name: "same status and error",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseFailed,
						PhaseUpdated: before,
						Error:        "some validation error",
					},
				},
			},
			err:     errors.New("some validation error"),
			changes: false,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseFailed,
					PhaseUpdated: before,
					Error:        "some validation error",
				},
			},
		},
		{
			name: "same status and different error",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
						PhaseUpdated: before,
						Error:        "some validation error",
					},
				},
			},
			err: &ctlerrors.Invalid{
				Err: errors.New("some new validation error"),
			},
			changes: true,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
					PhaseUpdated: now,
					Error:        "some new validation error",
				},
			},
		},
		{
			name: "new status and no error",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
						PhaseUpdated: before,
						Error:        "some validation error",
					},
				},
			},
			err:     nil,
			changes: true,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
					PhaseUpdated: now,
					Error:        "",
				},
			},
		},
		{
			name: "host name changed",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseFailed,
						PhaseUpdated: before,
						Error:        "unknown host",
						Host:         "localhost:1234",
					},
				},
				host: "localhost:5432",
			},
			err:     nil,
			changes: true,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
					PhaseUpdated: now,
					Error:        "",
					Host:         "localhost:5432",
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			tc.status.now = func() metav1.Time {
				return now
			}
			changes := tc.status.update(tc.err)
			assert.Equal(t, changes, tc.changes, "change indication not as expected")
			assert.Equal(t, tc.after, tc.status.database, "database status not as expected")
		})
	}
}
