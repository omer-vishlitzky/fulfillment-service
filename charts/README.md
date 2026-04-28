# Kubernetes deployment

This directory contains the Helm charts used to deploy the service to a Kubernetes cluster.

The main chart is `service`, which can be configured for either OpenShift (intended for production environments) or
Kind (intended for development and testing environments) using the `variant` value.

## Prerequisites

The fulfillment service chart does not include a database or an identity provider. These must be
installed separately before deploying the service:

- _PostgreSQL_ - The service requires an external PostgreSQL database. The database connection
  details are passed to the chart via the `database.connection` value. See the
  `charts/service/values.yaml` file for details.

- _Keycloak_ - The service requires Keycloak issuer for authentication. The issuer URL is passed
  via `auth.issuerUrl`. You must create at least the `osac-admin` and `osac-controller` service
  account clients and pass the credentials via the `auth.controllerCredentials` value. See the
  `charts/service/values.yaml` file for the expected format.

Note that the PostgreSQL and Keycloak Helm charts that are included in this project are intended
only for development environments, and shouldn't be used in production.

## OpenShift

The gRPC protocol is based on HTTP2, which isn't enabled by default in OpenShift. To enable it run
this command:

```shell
$ oc annotate ingresses.config/cluster ingress.operator.openshift.io/default-enable-http2=true
```

Install the _cert-manager_ operator:

```shell
$ oc new-project cert-manager-operator

$ oc create -f - <<.
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  namespace: cert-manager-operator
  name: cert-manager-operator
spec:
  upgradeStrategy: Default
.

$ oc create -f - <<.
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  namespace: openshift-operators
  name: cert-manager
spec:
  channel: stable
  installPlanApproval: Automatic
  name: cert-manager
  source: community-operators
  sourceNamespace: openshift-marketplace
.
```

Install the _trust-manager_ operator:

```shell
$ helm install trust-manager oci://quay.io/jetstack/charts/trust-manager \
--version v0.20.0 \
--namespace cert-manager-operator \
--set app.trust.namespace=cert-manager \
--set defaultPackage.enabled=false \
--wait
```

Create the default CA:

```shell
$ helm install default-ca charts/ca \
--namespace cert-manager
```

Install the _Authorino_ operator:

```shell
$ oc create -f - <<.
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  namespace: openshift-operators
  name: authorino-operator
spec:
  name: authorino-operator
  sourceNamespace: openshift-marketplace
  source: redhat-operators
  channel: stable
  installPlanApproval: Automatic
.
```

Create the Kubernetes Secret containing the database connection details. The keys must match the
parameters expected by the service chart (for example, `url` for the connection URL, `user` and
`password` for the credentials):

```shell
$ kubectl create secret generic fulfillment-database \
--namespace osac \
--from-literal=url='postgres://db.example.com:5432/fulfillment?sslmode=verify-full' \
--from-literal=user=fulfillment \
--from-literal=password=...
```

Create the Kubernetes Secret containing the controller OAuth client credentials. The client
identifier and secret must match the `osac-controller` service account created in Keycloak:

```shell
$ kubectl create secret generic fulfillment-controller-credentials \
--namespace osac \
--from-literal=client-id=osac-controller \
--from-literal=client-secret=...
```

Deploy the application:

```shell
$ helm install fulfillment-service charts/service \
--namespace osac \
--create-namespace \
--values service-values.yaml
```

Where `service-values.yaml` contains at least:

```yaml
variant: openshift

certs:
  issuerRef:
    name: default-ca
  caBundle:
    configMap: ca-bundle

auth:
  issuerUrl: https://your-oauth-issuer-url
  controllerCredentials:
  - secret:
      name: fulfillment-controller-credentials
      items:
      - key: client-id
        param: client-id
      - key: client-secret
        param: client-secret

database:
  connection:
  - secret:
      name: fulfillment-database
      items:
      - key: url
        param: url
      - key: user
        param: user
      - key: password
        param: password
```

## Kind

To create the Kind cluster use a command like this:

```yaml
$ kind create cluster --config - <<.
apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
name: osac
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30000
    hostPort: 8000
    listenAddress: "0.0.0.0"
.
```

The cluster uses a single port mapping: external port 8000 on the host is forwarded to internal port
30000 in the cluster. This port is used by the Envoy Gateway for ingress traffic.

Install the _cert-manager_ operator:

```shell
$ helm install cert-manager oci://quay.io/jetstack/charts/cert-manager \
--version v1.19.1 \
--namespace cert-manager \
--create-namespace \
--set crds.enabled=true \
--wait
```

Install the _trust-manager_ operator:

```shell
$ helm install trust-manager oci://quay.io/jetstack/charts/trust-manager \
--version v0.20.0 \
--namespace cert-manager \
--set defaultPackage.enabled=false \
--wait
```

Create the default CA:

```shell
$ helm install default-ca charts/ca \
--namespace cert-manager
```

Install the _Envoy Gateway_ that provides the Gateway API implementation used for routing traffic to
the services:

```shell
$ helm install envoy-gateway oci://docker.io/envoyproxy/gateway-helm \
--version v1.6.1 \
--namespace envoy-gateway \
--create-namespace \
--wait
```

Create the default gateway configuration. First, create an `EnvoyProxy` resource that configures the
gateway service to use a `NodePort` with port 30000 (the internal ingress port mapped from the
host):

```shell
$ kubectl apply -f - <<.
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: EnvoyProxy
metadata:
  namespace: envoy-gateway
  name: default
spec:
  provider:
    type: Kubernetes
    kubernetes:
      envoyService:
        type: NodePort
        patch:
          type: StrategicMerge
          value:
            spec:
              ports:
              - name: https
                port: 443
                nodePort: 30000
.
```

Create the default `GatewayClass` that references the `EnvoyProxy` configuration:

```shell
$ kubectl apply -f - <<.
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: default
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
  parametersRef:
    group: gateway.envoyproxy.io
    kind: EnvoyProxy
    namespace: envoy-gateway
    name: default
.
```

Create the default `Gateway` with a TLS passthrough listener:

```shell
$ kubectl apply -f - <<.
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  namespace: envoy-gateway
  name: default
spec:
  gatewayClassName: default
  listeners:
  - name: tls
    protocol: TLS
    port: 443
    tls:
      mode: Passthrough
    allowedRoutes:
      namespaces:
        from: All
.
```

Install the _Authorino_ operator:

```shell
$ kubectl apply -f https://raw.githubusercontent.com/Kuadrant/authorino-operator/refs/heads/release-v0.23.1/config/deploy/manifests.yaml
```

Create the Kubernetes secret containing the database connection details:

```shell
$ kubectl create secret generic fulfillment-database \
--namespace osac \
--from-literal=url='postgres://db.example.com:5432/fulfillment?sslmode=verify-full' \
--from-literal=user=fulfillment \
--from-literal=password=...
```

Create the Kubernetes secret containing the controller OAuth client credentials. The client
identifier and secret must match the `osac-controller` service account created in Keycloak:

```shell
$ kubectl create secret generic fulfillment-controller-credentials \
--namespace osac \
--from-literal=client-id=osac-controller \
--from-literal=client-secret=...
```

Deploy the application:

```shell
$ helm install fulfillment-service charts/service \
--namespace osac \
--create-namespace \
--values service-values.yaml
```

Where `service-values.yaml` contains at least:

```yaml
variant: kind

certs:
  issuerRef:
    name: default-ca
  caBundle:
    configMap: ca-bundle

auth:
  issuerUrl: https://your-oauth-issuer-url
  controllerCredentials:
  - secret:
      name: fulfillment-controller-credentials
      items:
      - key: client-id
        param: client-id
      - key: client-secret
        param: client-secret

database:
  connection:
  - secret:
      name: fulfillment-database
      items:
      - key: url
        param: url
      - key: user
        param: user
      - key: password
        param: password
```
