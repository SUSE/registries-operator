# Development environment for `registries-operator`

- [Architecture](#architecture)
- [Devel](#devel)
- [Testing](#testing)

# Architecture:

## Project structure

This project follows the conventions presented in the [standard Golang
project](https://github.com/golang-standards/project-layout).

This project use `kubebuilder` for scaffolding.  http://kubebuilder.netlify.com/quick_start.html

We also use everywhere `Makefile` targets for simplify the workflow, in dev., testing and ci/deployment.

# Devel

## Dependencies

* `go >= 1.11`

#### Bumping the Kubernetes version used by `registries-operator`

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

For running the operator locally you must ensure that it access the kubernetes
api with privileges for mounting the /etc/docker host volume, like in the psp
fragment below:

```yaml
apiVersion: extensions/v1beta1
kind: PodSecurityPolicy
spec:
  volumes:
    # Kubernetes Host Volume Types
    - hostPath
  allowedHostPaths:
    - pathPrefix: /etc/docker
```

# Testing

What we cover here:

- workflow
- design/goals
- why do we use travis and circleci 

## Workflow:

### Prerequisites:

We assume that you have already have a running cluster with the `KUBECONFIG` env variable set.
You can use `kind` for deploying a cluster or another method of your choice.


* e2e-tests: 
`make e2e-tests` will run e2e tests. This tests are run also in CI with `kind`.


## Key design and goals:

### Local == Remote

This project use everywhere Make targets.
In this way when we run `make test` locally or on CI remotely, there is no difference.
The goal is that for an user/dev should be not differnce to run locally or what is run by CI.

### Makefile is the api

As stated before, the Makefile targets is where other integration tools are dependent for.

In this way we can remove/change the internal logic without modifying the API, and braeking external deps.

## Why do we use Travis and circleCI?

The golden rule is: We use Travis only for `end to end tests` and for the rest we use circleci.
Since `kind` doesn't work in CircleCI because of network isolation, we use travis only for `e2e` tests.
