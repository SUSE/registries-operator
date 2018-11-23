# Development environment for `registries-operator`

## Project structure

This project follows the conventions presented in the [standard Golang
project](https://github.com/golang-standards/project-layout).

## Dependencies

* `go >= 1.11`

### Bumping the Kubernetes version used by `registries-operator`

Update the constraints in [`go.mod`](../go.mod).

## Building

A simple `make` should be enough. This should compile [the main
function](../cmd/registries-operator/main.go) and generate a `registries-operator` binary as
well as a _Docker_ image.

## Running `registries-operator` in your Development Environment

There are multiple ways you can run the `registries-operator` for bootstrapping
and managing your Kubernetes cluster:

### ... in your local machine

You can run the `registries-operator` container locally with a
`make KUBECONFIG=my-kubeconfig local-run`. This will:

  * build the `registries-operator` image
  * run it locally  using  `my-kubeconfig` to connect to the kubernetes cluster
