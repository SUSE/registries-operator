![alpha](https://img.shields.io/badge/stability%3F-beta-yellow.svg)
[![CircleCI](https://circleci.com/gh/kubic-project/registries-operator/tree/master.svg?style=svg)](https://circleci.com/gh/kubic-project/registries-operator/tree/master)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubic-project/registries-operator)](https://goreportcard.com/report/github.com/kubic-project/registries-operator)
Kind-integration-test:![Integration-test](https://travis-ci.org/kubic-project/registries-operator.svg?branch=master)

## Registry operator:

- [Description](#description)
- [Quickstart](#quickstart)
- [Devel](docs/devel.md)
- [Additional Info](#extra)

# Description

A Docker registries operator for Kubernetes, developed inside the [Kubic](https://en.opensuse.org/Portal:Kubic) project.

#### features:

* Automatic installation of registries certificates based on
some [CRD](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)s.

# Quick start

* load the operator with

    ```
    kubectl apply -f https://raw.githubusercontent.com/kubic-project/registries-operator/master/deployments/registries-operator-full.yaml
    ```

* once the operator is running, store the certificate for your registry in a _Secret_ with:

    ```
    kubectl create secret generic suse-ca-crt --from-file=ca.crt=/etc/pki/trust/anchors/SUSE_CaaSP_CA.crt -n kube-system
    ```

  where `/etc/pki/trust/anchors/SUSE_CaaSP_CA.crt` is the certificate and `suse-ca-crt` is the _Secret_.

* create a `Registry` object like this:

    ```yaml
    # registry.yaml
    apiVersion: "kubic.opensuse.org/v1beta1"
    kind: Registry
    metadata:
      name: suse-registry
      namespace: kube-system
    spec:
      hostPort: "registry.suse.de:5000"
      # secret with the ca.crt used for pulling images from this registry
      certificate:
        name: suse-ca-crt
        namespace: kube-system
    ```

    then you can load it with `kubectl apply -f registry.yaml`.

* once this is done, the `suse-ca-crt` should automatically appear in all
  the machines in your cluster, and all the Docker daemons in your cluster
  will be able to `pull` from that registry automatically.

# Devel

* See the [development documentation](docs/devel.md) if you intend to contribute to this project.

# Extra

* the [registries-operator image](https://hub.docker.com/r/opensuse/registries-operator/) in the Docker Hub.
* the [kubic-init](https://github.com/kubic-project/kubic-init) container, a container for
bootstrapping a Kubernetes cluster on top of [MicroOS](https://en.opensuse.org/Kubic:MicroOS)
(an openSUSE-Tumbleweed-based OS focused on running containers).
* the [Kubic Project](https://en.opensuse.org/Portal:Kubic) home page.
