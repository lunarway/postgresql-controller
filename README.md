# PostgreSQL controller

# Development

This project uses the [Operator SDK framework](https://github.com/operator-framework/operator-sdk) and its associated CLI.  
Follow the [offical installation instructions](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md) to get started.

## Testing

Unit and integration tests are run with the make targets `test/unit` and `test/integration`.

Integration tests require a PostgreSQL instance to run against.
If you set environment variable `POSTGRESQL_CONTROLLER_INTEGRATION_HOST` to the hostname of the instance that will be used and the tests run.
If the flag is not set the tests are skipped.

```
$ make test/unit
Running tests:
go test -v -race -cover ./pkg/...
?   	go.lunarway.com/postgresql-controller/pkg/apis	[no test files]
...
PASS
coverage: 0.0% of statements
ok  	go.lunarway.com/postgresql-controller/pkg/controller/postgresqluser	1.143s	coverage: 0.0% of statements


$ make test/integration
Running integration tests against PostgreSQL instance on localhost:5432:
POSTGRESQL_CONTROLLER_INTEGRATION_HOST=localhost:5432 make test/unit
Running tests:
go test -v -race -cover ./pkg/...
?   	go.lunarway.com/postgresql-controller/pkg/apis	[no test files]
...
PASS
coverage: 34.7% of statements
ok  	go.lunarway.com/postgresql-controller/pkg/controller/postgresqluser	1.232s	coverage: 34.7% of statements
```

To spin up a local cluster with [kind](https://github.com/kubernetes-sigs/kind) ensure to have it installed on your local machine.
You can connect a local running operator by using kind's `KUBECONFIG` and applying the operators resources.
Below example will create a test cluster, apply CRD resources, start a PostgreSQL instance in the cluster and start the operator.

Make sure to forward the postgresql pod for the

```
// Setup kind cluster
$ make test/cluster

// Point kubectl to the cluster (this command is also written to stdout by kind)
$ export KUBECONFIG="$(kind get kubeconfig-path --name="postgresql-controller-test")"

// Apply kubernetes resources for the controller
$ make test/cluster/resources

// Start a PostreSQL instance
$ make test/cluster/postgresql

// Forward PostgreSQL pod before starting the operator
$ kubectl port-forward deploy/postgresql 5432

// Start operator connecting to the cluster
$ make code/run
```

# Releasing

To release a new version of the operator run `make release TAG=vx.x.x`.
This will ensure to update `deploy/operator.yaml` with the version, create a commit and push it.
The `Release` GitHub Action workflow is then triggered and pushes the Docker image to quay.io.
