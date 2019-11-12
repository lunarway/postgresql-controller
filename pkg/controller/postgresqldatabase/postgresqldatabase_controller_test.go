package postgresqldatabase

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func TestReconcilePostgreSQLDatabase_ensurePostgreSQLDatabase_sunshine(t *testing.T) {
	postgresqlHost := os.Getenv("POSTGRESQL_CONTROLLER_INTEGRATION_HOST")
	if postgresqlHost == "" {
		t.Skip("Integration test host not specified")
	}
	connectionString := fmt.Sprintf("postgresql://iam_creator:@%s?sslmode=disable", postgresqlHost)
	db, err := postgresqlConnection(connectionString)
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	logger := testLogger{t: t}
	logf.SetLogger(logf.ZapLoggerTo(&logger, true))

	r := ReconcilePostgreSQLDatabase{
		db: db,
	}

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	password := "test"

	err = r.ensurePostgreSQLDatabase(logf.Log, name, password)
	if err != nil {
		t.Fatalf("ensurePostgreSQLDatabase failed: %v", err)
	}

	serviceConnectionString := fmt.Sprintf("postgresql://%s:%s@%s/%s?sslmode=disable", name, password, postgresqlHost, name)
	db, err = postgresqlConnection(serviceConnectionString)
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	// Validate Schema
	schemas := storedSchema(t, db, name)
	assert.Equal(t, []string{name}, schemas, "schema not as expected")

	// Validate iam_creator not able to see schema
	schemas = storedSchema(t, r.db, name)
	assert.Equal(t, []string(nil), schemas, "schema not as expected")

	// Validate owner of database
	owners := validateOwner(t, r.db, name)
	t.Log(owners)
	assert.Equal(t, []string{name}, owners, "owner not as expected")

}

func TestReconcilePostgreSQLDatabase_ensurePostgreSQLDatabase_idempotency(t *testing.T) {
	postgresqlHost := os.Getenv("POSTGRESQL_CONTROLLER_INTEGRATION_HOST")
	if postgresqlHost == "" {
		t.Skip("Integration test host not specified")
	}
	connectionString := fmt.Sprintf("postgresql://iam_creator:@%s?sslmode=disable", postgresqlHost)
	db, err := postgresqlConnection(connectionString)
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	logger := testLogger{t: t}
	logf.SetLogger(logf.ZapLoggerTo(&logger, true))

	r := ReconcilePostgreSQLDatabase{
		db: db,
	}

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	password := "test"

	err = r.ensurePostgreSQLDatabase(logf.Log, name, password)
	if err != nil {
		t.Fatalf("ensurePostgreSQLDatabase failed: %v", err)
	}

	// Invoke again with same name
	err = r.ensurePostgreSQLDatabase(logf.Log, name, password)
	if err != nil {
		t.Logf("%#v", err)
		t.Fatalf("ensurePostgreSQLDatabase failed: %v", err)
	}
}

func TestReconcilePostgreSQLDatabase_getSecretValue(t *testing.T) {
	logger := testLogger{t: t}
	logf.SetLogger(logf.ZapLoggerTo(&logger, true))

	tt := []struct {
		name       string
		secretName string
		namespace  string
		key        string
		value      string
		output     string
		err        error
	}{
		{
			name:       "sunshine",
			secretName: "test",
			namespace:  "test",
			key:        "test",
			value:      "dGVzdA==",
			output:     "test",
			err:        nil,
		},
		{
			name:       "illegal base64",
			secretName: "test",
			namespace:  "test",
			key:        "test",
			value:      "dGVzdA",
			output:     "",
			err:        errors.New("illegal base64 data at input byte 4"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.secretName,
					Namespace: tc.namespace,
				},
				Data: map[string][]byte{
					tc.key: []byte(tc.value),
				},
			}
			// Objects to track in the fake client.
			objs := []runtime.Object{
				secret,
			}

			// Create a fake client to mock API calls.
			cl := fake.NewFakeClient(objs...)

			r := &ReconcilePostgreSQLDatabase{
				client: cl,
			}
			password, err := r.getSecretValue(tc.secretName, tc.namespace, tc.key)
			if tc.err != nil {
				assert.EqualErrorf(t, err, tc.err.Error(), "wrong output error: %v", err.Error())
			} else {
				assert.NoError(t, err, "unexpected output error")
			}
			assert.Equal(t, tc.output, password, "password not as expected")
		})
	}
}

func TestReconcilePostgreSQLDatabase_getConfigMapValue(t *testing.T) {
	logger := testLogger{t: t}
	logf.SetLogger(logf.ZapLoggerTo(&logger, true))

	tt := []struct {
		name          string
		configMapName string
		namespace     string
		key           string
		value         string
		output        string
		err           error
	}{
		{
			name:          "sunshine",
			configMapName: "test",
			namespace:     "test",
			key:           "test",
			value:         "test",
			output:        "test",
			err:           nil,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.configMapName,
					Namespace: tc.namespace,
				},
				Data: map[string]string{
					tc.key: tc.value,
				},
			}
			// Objects to track in the fake client.
			objs := []runtime.Object{
				configMap,
			}

			// Create a fake client to mock API calls.
			cl := fake.NewFakeClient(objs...)

			r := &ReconcilePostgreSQLDatabase{
				client: cl,
			}
			password, err := r.getConfigMapValue(tc.configMapName, tc.namespace, tc.key)
			if tc.err != nil {
				assert.EqualErrorf(t, err, tc.err.Error(), "wrong output error: %v", err.Error())
			} else {
				assert.NoError(t, err, "unexpected output error")
			}
			assert.Equal(t, tc.output, password, "password not as expected")
		})
	}
}

var _ io.Writer = &testLogger{}

// testLogger is an io.Writer used for reporting logs to the test runner.
type testLogger struct {
	t *testing.T
}

func validateOwner(t *testing.T, db *sql.DB, owner string) []string {
	t.Helper()
	rows, err := db.Query("SELECT pg_catalog.pg_get_userbyid(d.datdba) FROM pg_catalog.pg_database d WHERE d.datname = $1", owner)
	if err != nil {
		t.Fatalf("get owner failed: %v", err)
	}
	defer rows.Close()
	return stringsResult(t, rows)
}

// storedRoles returns roles for a specific user name sorted by name.
func storedSchema(t *testing.T, db *sql.DB, schemaName string) []string {
	t.Helper()
	rows, err := db.Query("select schema_name from information_schema.schemata where schema_name = $1", schemaName)
	if err != nil {
		t.Fatalf("get schema for schema query failed: %v", err)
	}
	defer rows.Close()
	return stringsResult(t, rows)
}

func stringsResult(t *testing.T, rows *sql.Rows) []string {
	var results []string
	for rows.Next() {
		var result string
		err := rows.Scan(&result)
		if err != nil {
			t.Fatalf("scan row failed: %v", err)
		}
		results = append(results, result)
	}
	err := rows.Err()
	if err != nil {
		t.Fatalf("scanning rows failed: %v", err)
	}
	sort.Strings(results)
	return results
}

func (t *testLogger) Write(p []byte) (int, error) {
	t.t.Logf("%s", p)
	return len(p), nil
}
