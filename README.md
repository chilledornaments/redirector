# Redirector

Redirector is a simple application that handles redirecting requests based on a ruleset. Redirector allows you to manage your redirects declaratively and, additionally, with regular expressions. See the features section for more information.

A few important notes worth mentioning:
- Redirector does no proxying. If it receives a request for which it has no matching rule, the client receives a configurable response.
- Redirector is intended to be run behind the ingress nginx controller. This doesn't mean you can't use it with, say, the Kong Ingress Controller, but it'll require work on your end. PRs are welcome!

## Why use Redirector?

- YAML-defined redirection rules with support for regular expressions and YAML anchors.
- Parameter manipulation in `Location` header. Useful for injecting information into the destination URL.
- Generation of Kubernetes Ingress from a Redirector configuration.
- Request metrics to see which URLs are hit most frequently.
- Lightweight footprint. The final container image is <30 MB. The application needs ~100 mCPU and <50 MB memory.
- Per-rule status code and Cache-Control settings.
- Designed to be run in Kubernetes.
- Live configuration reloading.

The key difference between Redirector and existing solutions is the ability to set query parameters in the `Location` header. Parameters can be combined between the incoming request and those configured in the role. Parameters from a rule can also completely overwrite those in the request. 

If the ingress nginx controller or [Gateway API](https://gateway-api.sigs.k8s.io/guides/http-redirect-rewrite/) work for you, use those! Don't add extra tech to your stack that you don't need.

## Running

### Server

To start the Redirector server, run `./redirector server`. By default, the container will start the server.

#### Configuration

`cache_control_max_age` sets the value for the `Cache-Control` header `max-age` directive. To disable sending this header at all, set `cache_control_max_age: -1`. By default, the value is one week. 

Default values for server configuration:

```yaml
listen_address: '0.0.0.0:8484' # address for redirector service to listen on
metrics_server_listen_address: '0.0.0.0:8485' # address for metrics server

location_on_miss: '' # value for Location header if no matching rule found for request 
status_on_miss: 404 # status code to send to client if no matching rule found for request
cache_control_max_age: 604800 # value for max-age directive of Cache-Control header

cache:
  cleanup_interval: 3600 # how frequently the in-memory cache cleanup job runs
  ttl: 86400 # how long matched rules are kept in the in-memory cache
```

##### Handling misses

By default, if Redirector receives a request for which it finds no matching rule, it returns a 404 and does not send the client a `Location` header.

You can change this behavior with two settings:

- `location_on_miss`: will populate the `Location` header.
- `status_on_miss`: will set the status code for the response.

##### Caching

In order to avoid finding a match for every request, Redirector stores matches in an in-memory cache. 

#### In Kubernetes

Redirector is intended to be used with and tested against the [ingress nginx controller](https://github.com/kubernetes/ingress-nginx). 

Redirector needs a deployment, a service, and an ingress. See the [README in the charts directory](charts/redirector/README.md) for more information.

Please note that when deploying with Helm, the chart expects the contents of the configuration file to be provided as a base64 encoded string.
The chart will decode these into a ConfigMap. Alternatively, you can disable the ConfigMap creation entirely and create the ConfigMap yourself.


### Ingress generation

Redirector can generate a Kubernetes Ingress manifest for a given ruleset. This prevents you from having to either 1) route `/` traffic to Redirector by default or 2) redefine an Ingress after you've already _basically_ done so in the Redirector settings file.

To generate a manifest:
```shell
CONFIG_PATH=./fixtures/rules.yml ./redirector generate
```

The manifest can be configured with flags:
- `-out`: The file to output the manifest to. Defaults to `./redirector-ingress.yml`.
- `-namespace`: Kubernetes namespace for Ingress. Defaults to `redirector`.
- `-service-name`: Name of Redirector Kubernetes service to send requests to. Defaults to `redirector`.
- `-ingress-name`: `metadata.name` for Ingress. Defaults to `redirector`.
- `-ingress-class`: Ingress class. Defaults to `nginx`.


## Rules

Rules, in their simplest form, contain a `from` and a `to` directive. These two directives are string values that can optionally contain RE2-accepted regular expressions.

Rules can also contain: a specific status code to return; query parameters to add in the `Location` header and a strategy for handling existing query parameters.

By default, a rule returns a 301 status code and a `Cache-Control: max-age=604800` header. The code can be set in `rules[].rule.code`. Changing the Cache-Control header is done in the server settings.

See the [regex docs](https://github.com/google/re2/wiki/Syntax) for much more information on writing regular expressions.


### Rules for rules

- Don't use regular expressions for hostnames. The rule parser will ignore it and requests will never match a rule.

- Hostnames can only contain a-z, A-Z, 0-9, `.`, `_`, `-` and characters. 

- In the case of rule conflicts, the last-declared rule wins.

- The `to` directive **must** contain a protocol. If it does not, it will be discarded.

- `from` directives don't allow matching based on query parameters. 

- Ports are dropped from the `from` directive.

- The hostname in a request is normalized to drop the port, if present.

- If you're going to run in Kubernetes and store the configuration as a ConfigMap, it must be less than 1048576 bytes in size due to [Kubernetes limitations](https://kubernetes.io/docs/concepts/configuration/configmap/).

- The path in each rule's `from` directive will have a `^` prepended to it.

- If you don't want a `from` directive to act as a prefix, anchor it with `$`.

- Do not include parameters in the `to` directive, they will be dropped. To add parameters to a rule, use the `parameters` object.

- Include a `strategy` for all `parameters` objects. 


### Query Parameters

A rule can specify a `parameters` object, which dictates how parameters are added to the `Location` header. By default, parameters in the request are omitted from the `Location` header sent by Redirector.

Parameters can be handled in one of three ways:
- dropped, which is the default
- rule parameters can be added without regard for request parameters using the `replace` strategy.
- request parameters can be combined with rule parameters using the `combine` strategy. Rule parameters overwrite any request parameters. This is useful if we want to maintain parameters from the original request.

A rule's parameters object looks like so:
```yaml
parameters:
  strategy: "replace" # replace is default, can also be "combine"
  values:
    foo: ['bar']
    whiz: ['bang', 'bang']
```

**An important note:** you _must_ specify parameters for the `Location` header in a `parameters` object. Parameters in a `to:` directive are dropped.

## Building 

Install Go >= 1.24, then `go build -v ./... -o redirector`

## Tests

### Unit Tests

Unit tests can be run with `go test -v ./... --tags unit_test`

### Integration Tests

Integration tests require Python. Dependencies are defined
in `tests/integration/requirements.txt`.

By default, the integration test suite assumes it's testing against resources running in Kubernetes. To disable the tests that expect Kubernetes behavior, set `SKIP_KUBERNETES_INTEGRATION_TESTS` in the environment before running the test suite.

## Motivation

I had a need for this solution at my day job.
We needed to move years of redirect rules from a virtual appliance to Kubernetes.
The virtual appliance allows us to run TCL code on any request, which we use to manipulate headers that end up in the `Location` header. The TCL code is also _not_ version controlled and everyone is afraid to touch it.

The ingress nginx controller can handle the path rewriting we needed, but not the query logic.