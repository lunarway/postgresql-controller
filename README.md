# PostgreSQL controller

This is a Kubernetes controller for managing users and their access rights to a PostgreSQL database instance.
Its purpose is to make a codified description of what users have access to what databases and for what reason along with providing an auditable log of changes.

# Design

The controller will handle user and database management on PostgreSQL instances with two Kubernetes [Custom Resource Definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

## Databases

The CRD `PostgreSQLDatabase` specified details about a database on a specific instance.
It is scoped as cluster wide ie. has no namespace.

The main purpose of this resource is to create databases on a specific host with a specific name used by application services.
Instances of `PostgreSQLUser` can then give a specific developer access to the database by referenceing the name.

```yaml
apiVersion: lunar.bank/v1beta1
kind: PostgreSQLDatabase
metadata:
  name: user
spec:
  name: user
  host:
    value: some.host.com
```

The `host` field is modelled like Kubernetes core's [`EnvVarSource`](https://github.com/kubernetes/api/blob/665c8a257c1af277521b08dd43d5c73570405ef0/core/v1/types.go#L1847-L1862), so it can specify raw values like above or reference ConfigMaps etc.

<details>
<summary>Example of a database referencing a ConfigMap resource</summary>

```yaml
apiVersion: lunar.bank/v1beta1
kind: PostgreSQLDatabase
metadata:
  name: user
spec:
  name: user
  host:
    valueFrom:
      configMapKeyRef:
        name: database
        key: db.host
```

</details>

The controller will ensure that a database exists on the host based on its configuration.  
If a resources is deleted we _might_ delete the database in the future, preferrable behind a flag to avoid loosing data.

## Users

The CRD `PostgreSQLUser` contains metadata about the user along with its access rights to databases.

The access rights are devided into read and write and specifies a `host` and `reason` as a minimum.
This `database` field is required for writes along with `start` and `end`.
This ensures no unnecessary capabilities are left on a user ie. after completing a support ticket.

For reads, it will default to all databases on the instance and requires no time limit.
We generally do not limit access to data but instead rely on strong audits.

This is an example of a user `bso` that has read access to all databases and write access to the `user` database between 10 AM to 2 PM on september 9th.
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
      database: user
      reason: "Related to support ticket LW-1234"
      start: 2019-09-16T10:00:00Z
      end: 2019-09-16T14:00:00Z
```

From the configuration the user will be created with an `iam_<name>` user on the host and granted rights to access the required databases.
Further the role `rds_iam` will be granted allowing the user to sign in with IAM credentials.

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
          "aws:userid": "*:<name>@lunarway.com"
        }
      }
    }
  ]
}
```

# Development

This project uses the [Operator SDK framework](https://github.com/operator-framework/operator-sdk) and its associated CLI.  
Follow the [offical installation instructions](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md) to get started.

# Releasing

To release a new version of the operator run `make release TAG=vx.x.x`.
This will ensure to update `deploy/operator.yaml` with the version, create a commit and push it.
The `Release` GitHub Action workflow is then triggered and pushes the Docker image to quay.io.
