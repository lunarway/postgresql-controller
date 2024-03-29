# PostgreSQL controller

**BE AWARE** This is under active development and not ready for production in any way.
More features described in this readme are goals more than implemented functionality of the controller at the moment.

This is a Kubernetes controller for managing users and their access rights to a PostgreSQL database instance.
Its purpose is to make a codified description of what users have access to what databases and for what reason along with providing an auditable log of changes.

# Design

The controller will handle user and database management on PostgreSQL instances with two Kubernetes [Custom Resource Definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

The controller needs access to a role in PostreSQL privileged to create databases and roles.

```sql
CREATE USER iam_creator CREATEDB CREATEROLE PASSWORD 'strongpassword';
```

## Databases

The CRD `PostgreSQLDatabase` specified details about a database on a specific instance.
It is scoped as cluster wide ie. has no namespace.

The main purpose of this resource is to create databases on a specific host with a specific name used by application services.
Instances of `PostgreSQLUser` can then give a specific developer access to the database by referencing the name.

```yaml
apiVersion: lunar.bank/v1beta1
kind: PostgreSQLDatabase
metadata:
  name: user
spec:
  name: user
  password:
    value: <strong-password>
  host:
    value: some.host.com
```

The `password` and `host` fields are modelled like Kubernetes core's [`EnvVarSource`](https://github.com/kubernetes/api/blob/665c8a257c1af277521b08dd43d5c73570405ef0/core/v1/types.go#L1847-L1862), so it can specify raw values like above or reference ConfigMaps and Secrets.

<details>
<summary>Example of a database referencing ConfigMap and Secret resource</summary>

```yaml
apiVersion: lunar.bank/v1beta1
kind: PostgreSQLDatabase
metadata:
  name: user
spec:
  name: user
  password:
    valueFrom:
      secretKeyRef:
        name: user-db
        key: db.password
  host:
    valueFrom:
      configMapKeyRef:
        name: database
        key: db.host
```

</details>

The controller will ensure that a database exists on the host based on its configuration.  
If a resources is deleted we _might_ delete the database in the future, preferrable behind a flag to avoid loosing data.

There are created four roles for all databases.
One with login priviledges according to the custom resource name and password.
The other three are `read`, `readwrite` and `readowningwrite` roles used when granting users access to the database.
They are named as the database with a `_read`, `_readwrite` and `_readowningwrite` suffix and have the priviledge to `SELECT` and `SELECT, INSERT, UPDATE, DELETE` respectively.
The owning write role is also granted the owning role to allow using `DROP` and `ALTER`.
Default priviledges on the database ensures that each role have access to objects created by the service role.

## Users

The CRD `PostgreSQLUser` contains metadata about the user along with its access rights to databases.

The access rights are devided into `read`, `write` and `owningWrite` and specifies a `host` and `reason` as a minimum.
Either the `database` field or `allDatabases` must be set as well.
It is possible to set `start` and `stop` timestamps to limit the lifetime of capabilities e.g. automatic revocation after completing a support ticket.

We generally do not limit access to data but instead rely on strong audits.

This is an example of a user `bso` that has read access to all databases and write access to the `user` database in schema `user` between 10 AM to 2 PM on september 9th.
The read capability uses a static host name `some.host.com` and the write capability references a `database` ConfigMap on key `db.host`.

```yaml
apiVersion: lunar.bank/v1
kind: PostgreSQLUser
metadata:
  name: bso
spec:
  name: bso
  read:
    - host:
        value: some.host.com
      reason: "I am a developer"
  write:
    - host:
        valueFrom:
          configMapKeyRef:
            name: database
            key: db.host
      database:
        value: user
      schema:
        value: user
      reason: "Related to support ticket LW-1234"
      start: 2019-09-16T10:00:00Z
      stop: 2019-09-16T14:00:00Z
```

From the configuration the user will be created with a `<name>` user on the host and granted rights to access the required databases.
The flag `--user-role-prefix` can be used to prefix all created roles.
This can make it easier to see what roles are for human users and what are for services.

```
--user-role-prefix=iam_developer_
```

It is also possible to add custom roles to all created users. This is useful for IAM authentication as roles with the `rds_iam` role can connect to the database using AWS IAM.
Further these roles can be used to group users created by specific controllers.
Be aware that the controller will not create these roles.
They need to be available by other means.

```
--user-roles=rds_iam,iam_developer
```

The `rds_iam` is the default value of the flag.

A policy will also be added to AWS IAM for the specific user allowing it to connect to the host.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["rds-db:connect"],
      "Resource": [
        "arn:aws:rds-db:region:account:dbuser:*/iam_<name>"
      ],
      "Condition": {
        "StringLike": {
          "aws:userid": "*:<name>@lunar.app"
        }
      }
    }
  ]
}
```

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
// Setup kind cluster (this will use KUBECONFIG=~/.kube/config to store the kubeconfig)
$ make test/cluster

// Apply kubernetes resources for the controller
$ make test/cluster/resources

// Forward PostgreSQL pod before starting the operator
$ kubectl port-forward deploy/postgresql 5432

// Start operator connecting to the cluster
$ make run
```

# Releasing

To release a new version of the operator run `make release TAG=vx.x.x`.
This will ensure to update `deploy/operator.yaml` with the version, create a commit and push it.
The `Release` GitHub Action workflow is then triggered and pushes the Docker image to quay.io.
