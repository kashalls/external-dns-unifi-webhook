# ExternalDNS Webhook Provider for UniFi

<div align="center">

[![GitHub Release](https://img.shields.io/github/v/release/kashalls/external-dns-unifi-webhook?style=for-the-badge)](https://github.com/kashalls/external-dns-unifi-webhook/releases)&nbsp;&nbsp;
[![Discord](https://img.shields.io/discord/673534664354430999?style=for-the-badge&label&logo=discord&logoColor=white&color=blue)](https://discord.gg/home-operations)

</div>

[ExternalDNS](https://github.com/kubernetes-sigs/external-dns) is a Kubernetes add-on for automatically managing DNS records for Kubernetes ingresses and services by using different DNS providers. This webhook provider allows you to automate DNS records from your Kubernetes clusters into your UniFi Network controller.

## 🎯 Requirements

- ExternalDNS >= v0.14.0
- UniFi OS >= 3.x
- UniFi Network >= 8.2.93

## 🚫 Limitations

- Wildcard CNAME Records are not supported by UniFi.

## ⛵ Deployment

### Gathering your credentials

<details>
<summary>UniFi Api Key - Network v9.0.0+</summary>
<br>

1. Open your UniFi Console's Network Settings and go to `Settings > Control Plane > Admins & Users`.

2. Selecting your user to operate under. Whenever we modify the DNS Records, this user will show up under `System Log > Admin Activity`

3. Go under `Control Plane API Key` and click `Create New`. You can set the name to whatever you want, and the expiration to whatever you feel like.

4. Create a Kubernetes secret called `external-dns-unifi-secret` that will hold your `UNIFI_API_KEY` with their respected values from Step 3.

```yaml
---
apiVersion: v1
kind: Secret
metadata:
    name: external-dns-unifi-secret
stringData:
  api-key: <your-api-key>
```
</details>

<details>
<summary>Username & Password (Deprecated)</summary>
<br>

1. Open your UniFi Console's Network Settings and go to `Settings > Control Plane > Admins & Users`.

2. Select `Create New Admin`.

3. In the menu that appears, enable `Restrict to Local Access Only`. Deselect `Use a Predefined Role`. Set `Network: Site Admin`. All other selections can be set to `None`. Click `Create`.

4. Create a Kubernetes secret called `external-dns-unifi-secret` that holds the `username` and `password` with their respected values from Step 3.

```yaml
---
apiVersion: v1
kind: Secret
metadata:
    name: external-dns-unifi-secret
stringData:
  username: <your-username>
  password: <your-password>
```
</details>

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
          repository: ghcr.io/kashalls/external-dns-unifi-webhook
          tag: main # replace with a versioned release tag
        env:
          - name: UNIFI_HOST
            value: https://192.168.1.1 # replace with the address to your UniFi router/controller
          - name: UNIFI_EXTERNAL_CONTROLLER
            value: "false"
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
    policy: sync
    sources: ["ingress", "service"]
    txtOwnerId: default
    txtPrefix: k8s.
    domainFilters: ["example.com"] # replace with your domain
    ```

4. Install the Helm chart

    ```sh
    helm install external-dns-unifi external-dns/external-dns -f external-dns-unifi-values.yaml --version 1.15.0 -n external-dns
    ```

## Configuration

### Unifi Controller Configuration

| Environment Variable         | Description                                                       | Default Value |
|------------------------------|-------------------------------------------------------------------|---------------|
| `UNIFI_API_KEY`              | The local api key provided for your user                          | N/A           |
| `UNIFI_USER`                 | Username for the Unifi Controller (deprecated use `UNIFI_API_KEY`). | N/A           |
| `UNIFI_PASS`                 | Password for the Unifi Controller (deprecated use `UNIFI_API_KEY`). | N/A           |
| `UNIFI_SKIP_TLS_VERIFY`      | Whether to skip TLS verification (true or false).                 | `true`        |
| `UNIFI_SITE`                 | Unifi Site Identifier (used in multi-site installations)          | `default`     |
| `UNIFI_HOST`                 | Host of the Unifi Controller (must be provided).                  | N/A           |
| `UNIFI_EXTERNAL_CONTROLLER`* | Toggles support for non-UniFi Hardware                            | `false`       |
| `LOG_LEVEL`                  | Change the verbosity of logs (used when making a bug report)      | `info`        |

*`UNIFI_EXTERNAL_CONTROLLER` is used to toggle between two versions of the Network Controller API. If you are running the UniFi software outside of UniFi's official hardware (e.g., Cloud Key or Dream Machine), you'll need to set `UNIFI_EXTERNAL_CONTROLLER` to `true`

### Server Configuration

| Environment Variable             | Description                                                      | Default Value |
|----------------------------------|------------------------------------------------------------------|---------------|
| `SERVER_HOST`                    | The host address where the server listens.                       | `localhost`   |
| `SERVER_PORT`                    | The port where the server listens.                               | `8888`        |
| `SERVER_READ_TIMEOUT`            | Duration the server waits before timing out on read operations.  | N/A           |
| `SERVER_WRITE_TIMEOUT`           | Duration the server waits before timing out on write operations. | N/A           |
| `DOMAIN_FILTER`                  | List of domains to include in the filter.                        | Empty         |
| `EXCLUDE_DOMAIN_FILTER`          | List of domains to exclude from filtering.                       | Empty         |
| `REGEXP_DOMAIN_FILTER`           | Regular expression for filtering domains.                        | Empty         |
| `REGEXP_DOMAIN_FILTER_EXCLUSION` | Regular expression for excluding domains from the filter.        | Empty         |

## ⭐ Stargazers

<div align="center">

[![Star History Chart](https://api.star-history.com/svg?repos=kashalls/external-dns-unifi-webhook&type=Date)](https://star-history.com/#kashalls/external-dns-unifi-webhook&Date)

</div>

---

## 🤝 Gratitude and Thanks

Thanks to all the people who donate their time to the [Home Operations](https://discord.gg/home-operations) Discord community.
