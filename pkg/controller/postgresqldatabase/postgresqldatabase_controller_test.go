package postgresqldatabase

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReconcilePostgreSQLDatabase_setStatus(t *testing.T) {
	before := metav1.Time{
		Time: time.Date(2019, time.December, 18, 17, 7, 3, 0, time.UTC),
	}
	now := metav1.Time{
		Time: time.Date(2019, time.December, 18, 18, 7, 3, 0, time.UTC),
	}
	nowFunc := func() metav1.Time {
		return now
	}
	tt := []struct {
		name    string
		before  *lunarwayv1alpha1.PostgreSQLDatabase
		status  lunarwayv1alpha1.PostgreSQLDatabasePhase
		err     error
		changes bool
		after   *lunarwayv1alpha1.PostgreSQLDatabase
	}{
		{
			name: "new status",
			before: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase: "",
				},
			},
			status:  lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
			err:     errors.New("some validation error"),
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
			before: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
					PhaseUpdated: before,
					Error:        "some validation error",
				},
			},
			status:  lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
			err:     errors.New("some validation error"),
			changes: false,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
					PhaseUpdated: before,
					Error:        "some validation error",
				},
			},
		},
		{
			name: "same status and different error",
			before: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
					PhaseUpdated: before,
					Error:        "some validation error",
				},
			},
			status:  lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
			err:     errors.New("some new validation error"),
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
			before: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
					PhaseUpdated: before,
					Error:        "some validation error",
				},
			},
			status:  lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
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
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			changes := updateStatus(nowFunc, tc.before, tc.status, tc.err)
			assert.Equal(t, changes, tc.changes, "change indication not as expected")
			assert.Equal(t, tc.after, tc.before, "database status not as expected")
		})
	}
}
