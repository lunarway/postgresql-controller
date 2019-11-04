# PostgreSQL controller

# Development

This project uses the [Operator SDK framework](https://github.com/operator-framework/operator-sdk) and its associated CLI.  
Follow the [offical installation instructions](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md) to get started.

# Releasing

To release a new version of the operator run `make release TAG=vx.x.x`.
This will ensure to update `deploy/operator.yaml` with the version, create a commit and push it.
The `Release` GitHub Action workflow is then triggered and pushes the Docker image to quay.io.
