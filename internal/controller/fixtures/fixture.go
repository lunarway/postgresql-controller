package fixtures

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type Fixture struct {
	t    *testing.T
	log  logr.Logger
	ctx  context.Context
	host string

	data fixtureData

	kubeClient client.Client
}

func (f *Fixture) GivenASeededDatabase() *Fixture {
	f.t.Helper()

	f.seedDatabase(f.host, f.data.databaseName, f.data.userName, f.data.managerRole)

	return f
}

func (f *Fixture) toResourceName(raw string) string {
	return strings.ReplaceAll(raw, "_", "-")
}

func (f *Fixture) toNamespacedName(raw string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: f.data.namespace,
		Name:      f.toResourceName(raw),
	}
}

func (f *Fixture) incrementResource(index int, raw string) string {
	return fmt.Sprintf("%s_%d", raw, index)
}

func (f *Fixture) addK8sResources(resources ...client.Object) {
	f.t.Helper()

	var (
		ctx      = context.Background()
		timeout  = time.Second * 5
		interval = time.Millisecond * 250
	)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	egrp, ctx := errgroup.WithContext(ctx)

	for _, resource := range resources {
		egrp.Go(func() error {
			assert.EventuallyWithT(
				f.t,
				func(collect *assert.CollectT) {
					err := f.kubeClient.Create(ctx, resource)
					assert.NoError(f.t, err, "failed to create kubernetes resource")
				},
				timeout,
				interval,
			)

			return nil
		})
	}

	err := egrp.Wait()
	assert.NoError(f.t, err)
}

func checkResource[T client.Object](fixture *Fixture, lookup types.NamespacedName, assertFunc func(f *assert.CollectT, obj T)) {
	fixture.t.Helper()

	var (
		ctx      = context.Background()
		timeout  = time.Second * 5
		interval = time.Millisecond * 50
	)

	assert.EventuallyWithT(fixture.t, func(collect *assert.CollectT) {
		// A little bit of a cursed hack, it isn't possible to new up the type behind the client.Object without reaching out to reflection to remove the pointer
		var typ T
		obj := reflect.New(reflect.TypeOf(typ).Elem()).Interface().(T)

		err := fixture.kubeClient.Get(ctx, lookup, obj)

		assert.NoError(collect, err, "failed to get kubernetes resource")
		if err == nil {
			assertFunc(collect, obj)
		}
	}, timeout, interval)

}

func (f *Fixture) seedDatabase(host, databaseName, userName string, managerRole string) {
	t := f.t

	t.Helper()

	dbConn, err := postgres.Connect(logf.Log, postgres.ConnectionString{
		Database: "postgres",
		Host:     host,
		Password: "iam_creator",
		User:     "iam_creator",
	})
	require.NoErrorf(t, err, "failed to connect to database host to seed database '%s'", databaseName)

	// Create the ManagmentRole
	err = f.createManagerRole(logf.Log, dbConn, managerRole)
	require.NoErrorf(t, err, "failed to create managerRole for dbConn during seedDatabase")

	err = postgres.Database(logf.Log, host, postgres.Credentials{
		User:     "iam_creator",
		Password: "iam_creator",
	}, postgres.Credentials{
		Name:     databaseName,
		Password: databaseName,
		User:     userName,
	}, managerRole)
	require.NoErrorf(t, err, "failed to created seeded database '%s'", databaseName)

	db1Conn, err := postgres.Connect(logf.Log, postgres.ConnectionString{
		Database: databaseName,
		Host:     host,
		Password: databaseName,
		User:     userName,
	})
	require.NoErrorf(t, err, "failed to connect to database '%s' to create a table", databaseName)

	_, err = db1Conn.Exec(`CREATE TABLE movies(title varchar(50));`)
	require.NoErrorf(t, err, "failed to create table in database '%s'", databaseName)
}

func (f *Fixture) createManagerRole(log logr.Logger, db *sql.DB, roleName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE ROLE %s LOGIN;", roleName))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_object" {
			return err
		}
		log.Info("role already exists", "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info("role created")
	}
	return nil
}

type FixtureOption = func(f *Fixture)

func WithKubeClient(k client.Client) FixtureOption {
	return func(f *Fixture) {
		f.kubeClient = k
	}
}

// Test sets up a common fixture testing routine
func Test(testFunc func(f *Fixture), fixtureOptions ...FixtureOption) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		logf.SetLogger(zap.New(zap.UseDevMode(true)))

		host := test.Integration(t)

		fixture := &Fixture{
			t:    t,
			log:  logf.Log,
			ctx:  context.Background(),
			host: host,

			data: newFixtureData(),
		}

		for _, opt := range fixtureOptions {
			opt(fixture)
		}

		testFunc(fixture)
	}
}
