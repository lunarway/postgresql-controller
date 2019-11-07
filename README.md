# PostgreSQL controller

This is a Kubernetes controller for managing users and their access rights to a PostgreSQL database instance.
Its purpose is to make a codified description of what users have access to what databases and for what reason along with providing an auditable log of changes.

# Design

The controller defines a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) called `PostgreSQLUser`.  
It contains metadata about the user along with its access rights to databases.

This is an example of a user `bso` that has the role `iam_developer` and write access to the `user` database between 10 AM to 2 PM on september 9th.

```yaml
apiVersion: lunar.bank/v1beta1
kind: PostgreSQLUser
metadata:
  name: bso
  namespace: dev
spec:
  role: iam_developer
  reason: "I am a developer"
  write:
  - database: user
    reason: "Related to support ticket LW-1234"
    start: 2019-09-16T10:00:00Z
    end: 2019-09-16T14:00:00Z
```

Users are created with the `rds_iam` role allowing them to sign in with a short lived password issued from AWS.

# Development

This project uses the [Operator SDK framework](https://github.com/operator-framework/operator-sdk) and its associated CLI.  
Follow the [offical installation instructions](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md) to get started.

# Releasing

To release a new version of the operator run `make release TAG=vx.x.x`.
This will ensure to update `deploy/operator.yaml` with the version, create a commit and push it.
The `Release` GitHub Action workflow is then triggered and pushes the Docker image to quay.io.
