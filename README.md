# ExternalDNS Webhook Provider for UniFi

[![Tests](https://github.com/home-operations/external-dns-unifi-webhook/actions/workflows/tests.yaml/badge.svg)](https://github.com/home-operations/external-dns-unifi-webhook/actions/workflows/tests.yaml)
[![Lint](https://github.com/home-operations/external-dns-unifi-webhook/actions/workflows/lint.yaml/badge.svg)](https://github.com/home-operations/external-dns-unifi-webhook/actions/workflows/lint.yaml)
[![Release](https://img.shields.io/github/v/release/home-operations/external-dns-unifi-webhook)](https://github.com/home-operations/external-dns-unifi-webhook/releases)
[![License](https://img.shields.io/github/license/home-operations/external-dns-unifi-webhook)](LICENSE)
[![Discord](https://img.shields.io/discord/673534664354430999?label=discord&logo=discord&logoColor=white&color=blue)](https://discord.gg/home-operations)

[ExternalDNS](https://github.com/kubernetes-sigs/external-dns) is a Kubernetes add-on for automatically managing DNS records for Kubernetes ingresses and services by using different DNS providers. This webhook provider allows you to automate DNS records from your Kubernetes clusters into your UniFi Network controller.

## 🎯 Requirements

- ExternalDNS >= v0.14.0
- UniFi OS >= 3.x
- UniFi Network >= 10.3.58

## 🚫 Limitations

_UniFi uses [dnsmasq](https://dnsmasq.org) as the backend of its DNS resolver and DHCP server._
_This project is subject to the limitations of dnsmasq. Please report any issues you encounter utilizing this provider._

- **Wildcards** (`*.example.com`) are not supported by dnsmasq.
- **Only one CNAME per name** is allowed by dnsmasq. The webhook handles this transparently:
    - If a CNAME already exists at the target name when a new one is created, the existing record is evicted first.
    - If external-dns sends multiple targets for a CNAME (e.g. `deployment.example.com 0 IN CNAME external.example.com internal.example.com`), only the first target is used and the rest are dropped with a warning.

## 🔁 Upgrading from earlier versions

The migration to UniFi Network 10.3.58's [Integration API](https://developer.ui.com/network/) introduced several breaking changes:

| Setting                                                                                            | Old                                                             | New                                                                                                                                                      |
| -------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `UNIFI_EXTERNAL_CONTROLLER`                                                                        | Toggle for non-Ubiquiti hardware                                | **Removed.** Point `UNIFI_HOST` at the controller directly; only internal connections are supported.                                                     |
| `DOMAIN_FILTER`, `EXCLUDE_DOMAIN_FILTER`, `REGEXP_DOMAIN_FILTER`, `REGEXP_DOMAIN_FILTER_EXCLUSION` | Provider-side filter                                            | **Removed.** Configure `--domain-filter` (and friends) on the external-dns controller instead. See the [Domain filtering](#domain-filtering) note below. |
| `LOG_FORMAT=test`                                                                                  | Switched to text output                                         | **Renamed** to `LOG_FORMAT=text`.                                                                                                                        |
| API endpoint                                                                                       | `/proxy/network/v2/api/site/{site}/static-dns/*` (undocumented) | `/proxy/network/integration/v1/sites/{siteId}/dns/policies/*` (official, requires Network 10.3.58+).                                                     |

## ⛵ Deployment

### Creating UniFi Credentials

Authentication uses a UniFi API Key (Network 9.0.0+). Username & password authentication is no longer supported.

1. Open your UniFi controller/Console's admin page either via [unifi.ui.com](https://unifi.ui.com) or via the IP address of your controller

2. On the left navigation bar (that runs the length of the page) click the _people_ icon (`Admin & Users`)

3. Click `+ Create New` at the top of the page and fill it out using the below details

| Field Name                    | Value                                   |
| ----------------------------- | --------------------------------------- |
| First name                    | `External`                              |
| Last name                     | `DNS`                                   |
| Admin                         | :white_check_mark:                      |
| Restrict to local access only | :white_check_mark:                      |
| Username                      | `externaldns`                           |
| Password                      | Make up a password, but make note of it |
| Use a pre defined role        | :white_check_mark:                      |
| Role                          | `Super Admin`                           |

Your user should now look like the below

![UniFi Creating super admin](md-assets/unifi-user-api-superadmin.png)

4. Login to your console as the user you have just created. This will need to be done via the controller's IP address

5. **Gear Icon** > **Control Plane** > **Integrations**

Give the API key a name, something like `external-dns`

Copy this Key, we will need it later. Your page should now look like the below

![UniFi Creating API Key](md-assets/unifi-subuser-create-api-key.png)

6. Remove elevated permissions from the user

Log back in as your normal account, head over to where we created the External DNS account
(On the left navigation bar (that runs the length of the page) click the _people_ icon (`Admin & Users`))

Open that account, click the **Gear Icon** then match the below

We have unselected **Use a Predefined Role** and changed the _ufo_ icon to `Site admin` and the _person_ to `None`

![UniFi remove excess permissions](md-assets/change-superadmin-account-to-site-admin.png)

You're probably thinking _wow, that was long_, and it's because only super admins can create API Keys, but they do not need
those permissions the entire time to be able to _have_ API Key attached to that user. It's a ~bug~ feature in UniFi

The `Site Admin` permissions are more than enough to allow that user to create and manage DNS records in our controller

7. Create a Kubernetes secret called `external-dns-unifi-secret` that will hold your `UNIFI_API_KEY` with their respected values from Step 3.

```yaml
---
apiVersion: v1
kind: Secret
metadata:
    name: external-dns-unifi-secret
stringData:
    api-key: <your-api-key>
```

You should now follow the [Installing the provider](#installing-the-provider) instructions

### Installing the provider

1. Add the ExternalDNS Helm repository to your cluster.

    ```sh
    helm repo add external-dns https://kubernetes-sigs.github.io/external-dns/
    ```

2. Deploy your `external-dns-unifi-secret` secret that holds your authentication credentials from either of the credential types above.

3. Create the helm values file, for example `external-dns-unifi-values.yaml`:

    ```yaml
    fullnameOverride: external-dns-unifi
    logLevel: &logLevel debug
    provider:
        name: webhook
        webhook:
            image:
                repository: ghcr.io/home-operations/external-dns-unifi-webhook
                tag: main # replace with a versioned release tag
            env:
                - name: UNIFI_HOST
                  value: https://192.168.1.1 # replace with the address to your UniFi router/controller
                - name: UNIFI_API_KEY
                  valueFrom:
                      secretKeyRef:
                          name: external-dns-unifi-secret
                          key: api-key
                - name: LOG_LEVEL
                  value: *logLevel
            livenessProbe:
                httpGet:
                    path: /healthz
                    port: http-webhook
                initialDelaySeconds: 10
                timeoutSeconds: 5
            readinessProbe:
                httpGet:
                    path: /readyz
                    port: http-webhook
                initialDelaySeconds: 10
                timeoutSeconds: 5
    extraArgs:
        - --ignore-ingress-tls-spec
    policy: create-only
    sources: ["ingress", "service"]
    txtOwnerId: default
    txtPrefix: k8s.
    domainFilters: ["example.com"] # replace with your domain
    ```

    For additional customization, refer to the [helm values](https://github.com/kubernetes-sigs/external-dns/blob/master/charts/external-dns/values.yaml).

4. Install the Helm chart

    ```sh
    helm install external-dns-unifi external-dns/external-dns -f external-dns-unifi-values.yaml --version 1.15.0 -n external-dns
    ```

## Configuration

### UniFi controller

| Environment Variable        | Description                                                                           | Default Value |
| --------------------------- | ------------------------------------------------------------------------------------- | ------------- |
| `UNIFI_HOST`                | Host of the UniFi controller (required).                                              | N/A           |
| `UNIFI_API_KEY`             | The local API key provided for your user (required).                                  | N/A           |
| `UNIFI_SITE`                | UniFi site name (e.g. `default`) or site UUID. Resolved to the API's UUID at startup. | `default`     |
| `UNIFI_SKIP_TLS_VERIFY`     | Whether to skip TLS verification.                                                     | `true`        |
| `UNIFI_CA_CERT`             | Path to a PEM file with extra trusted CAs (alternative to skipping verification).     | N/A           |
| `UNIFI_APPLY_WORKERS`       | Maximum concurrent record operations during `ApplyChanges`.                           | `5`           |
| `UNIFI_RETRY_ATTEMPTS`      | Total attempts per request (including the first).                                     | `3`           |
| `UNIFI_RETRY_INITIAL_DELAY` | Initial backoff before the first retry.                                               | `500ms`       |
| `UNIFI_RETRY_MAX_DELAY`     | Maximum backoff between retries (also caps `Retry-After`).                            | `10s`         |

The webhook talks to the UniFi Network [Integration API](https://developer.ui.com/network/) over the controller's local `/proxy/network/integration/v1/...` paths. External / cloud-proxied controllers are not supported — point `UNIFI_HOST` at the controller directly.

#### Domain filtering

Configure `--domain-filter` (and friends) on the external-dns controller itself, not on this webhook. UniFi has no zone concept the webhook could narrow against, so we follow the [external-dns `GetDomainFilter` contract](https://github.com/kubernetes-sigs/external-dns/blob/v0.21.0/docs/contributing/sources-and-providers.md#implementing-getdomainfilter) and leave the filter to the controller.

### Webhook server

| Environment Variable         | Description                                                         | Default Value     |
| ---------------------------- | ------------------------------------------------------------------- | ----------------- |
| `SERVER_HOST`                | Host address for the webhook server.                                | `localhost`       |
| `SERVER_PORT`                | Port for the webhook server.                                        | `8888`            |
| `SERVER_READ_TIMEOUT`        | Read timeout for the webhook server.                                | `60s`             |
| `SERVER_READ_HEADER_TIMEOUT` | Read-header timeout (Slowloris mitigation).                         | `5s`              |
| `SERVER_WRITE_TIMEOUT`       | Write timeout for the webhook server.                               | `60s`             |
| `SERVER_IDLE_TIMEOUT`        | Keep-alive idle timeout.                                            | `120s`            |
| `SERVER_MAX_HEADER_BYTES`    | Maximum request header size.                                        | `65536`           |
| `SERVER_MAX_BODY_BYTES`      | Maximum POST body size before returning 413.                        | `5242880` (5 MiB) |
| `HEALTH_SERVER_ADDR`         | Address for the secondary `/metrics`, `/healthz`, `/readyz` server. | `0.0.0.0:8080`    |
| `READINESS_CACHE_TTL`        | How long `/readyz` caches the upstream probe result.                | `30s`             |
| `PPROF_ENABLED`              | Mount `/debug/pprof/*` on the health server (don't enable in prod). | `false`           |
| `LOG_LEVEL`                  | Log verbosity (`debug` / `info` / `warn` / `error`).                | `info`            |
| `LOG_FORMAT`                 | Set to `text` for human-readable output instead of JSON.            | JSON              |

### Observability endpoints

The webhook exposes operational endpoints on two listeners:

| Endpoint        | Port (default) | Purpose                                                                                    |
| --------------- | -------------- | ------------------------------------------------------------------------------------------ |
| `/`             | `8888`         | External-DNS Negotiate (returns the provider's media type).                                |
| `/records`      | `8888`         | External-DNS `GET` (list) and `POST` (apply changes).                                      |
| `/healthz`      | `8888`, `8080` | Liveness — always `200 OK` while the process is running.                                   |
| `/readyz`       | `8888`, `8080` | Readiness — probes the UniFi controller via `Records()`. Cached for `READINESS_CACHE_TTL`. |
| `/metrics`      | `8080`         | Prometheus metrics.                                                                        |
| `/debug/pprof/` | `8080`         | Go pprof endpoints. Only mounted when `PPROF_ENABLED=true`.                                |

`/healthz` and `/readyz` are mounted on both ports so Kubernetes probes can target the webhook port directly without exposing a second container port through the chart.

## ⭐ Stargazers

<div align="center">

[![Star History Chart](https://api.star-history.com/svg?repos=home-operations/external-dns-unifi-webhook&type=Date)](https://star-history.com/#home-operations/external-dns-unifi-webhook&Date)

</div>

---

## 🤝 Gratitude and Thanks

Thanks to all the people who donate their time to the [Home Operations](https://discord.gg/home-operations) Discord community.
