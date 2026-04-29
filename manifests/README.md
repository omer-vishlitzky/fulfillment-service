# Kubernetes deployment

This directory contains the manifests used to deploy the service to a Kubernetes cluster.

There are currently two variants of the manifests: one for OpenShift, intended for production
environments, and another for Kind, intended for development and testing environments.

Unlike the Helm chart in `charts/service`, these kustomize manifests include a PostgreSQL database
as part of the deployment. However, an OAuth identity provider (such as Keycloak) must be installed
separately. The base manifests expect the issuer URL
`https://keycloak.keycloak.svc.cluster.local:8000/realms/osac`; this can be changed with a kustomize
patch in a custom overlay.

## Prerequisites

Before deploying the service you need to create a Kubernetes secret containing the OAuth client
credentials that the controller uses to authenticate to the API. The client identifier and secret
must match the `osac-controller` service account configured in the identity provider. That service
account must also have the following roles from the `realm-management` client:

- `manage-realm` - Manage the realm configuration, including organizations.
- `manage-users` - Create, update and delete users.
- `view-realm` - View the realm configuration.
- `view-users` - View users.

The secret must be named `fulfillment-controller-credentials` and contain the keys `client-id` and
`client-secret`:

```shell
$ kubectl create secret generic fulfillment-controller-credentials \
--namespace osac \
--from-literal=client-id=osac-controller \
--from-literal=client-secret=...
```

If you need to use a different secret name or different keys, you can override the volume definition
in the controller deployment using a kustomize patch in your overlay.

## Enabling IDP Integration (Optional)

The kustomize manifests include the IDP configuration flags by default, pointing to
`https://keycloak.keycloak.svc.cluster.local:8000`. The controller uses OAuth client credentials
flow to authenticate with the identity provider's admin API.

By default, the IDP integration reuses the same `fulfillment-controller-credentials` secret (with
keys `client-id` and `client-secret`) that is already configured for API authentication. The
`osac-controller` service account has both API access permissions and the required IDP
realm-management roles (manage-realm, manage-users, view-realm, view-users).

**No additional setup is needed** - the organization controller will automatically start if the
`fulfillment-controller-credentials` secret exists.

If you want to use separate credentials for IDP (for better separation of concerns), you can create
a kustomize patch to override the `idp-credentials` volume:

```yaml
# kustomization.yaml
patches:
- patch: |-
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: fulfillment-controller
    spec:
      template:
        spec:
          volumes:
          - name: idp-credentials
            secret:
              secretName: fulfillment-idp-credentials
              optional: true
```

Then create the separate secret:

```shell
$ kubectl create secret generic fulfillment-idp-credentials \
--namespace osac \
--from-literal=client-id=osac-idp-manager \
--from-literal=client-secret=...
```

If neither the reused credentials nor separate IDP credentials are available, the controller will
log that IDP is not configured and will not start the organization controller. The volume is marked
as optional, so the pod will start successfully.

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

To deploy the application run this:

```shell
$ oc apply -k manifests/overlays/openshift
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

Deploy the application:

```shell
$ kubectl apply -k manifests/overlays/kind
```
