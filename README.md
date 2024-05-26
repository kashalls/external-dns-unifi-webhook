# ExternalDNS Webhook Provider for UniFi

> ⚠️ This software is experimental and **NOT FIT FOR PRODUCTION USE!**
>
> This webhook relies on Early Access features from Unifi, and may stop working at any time.

[ExternalDNS](https://github.com/kubernetes-sigs/external-dns) is a Kubernetes add-on for automatically managing DNS records for Kubernetes ingresses and services by using different DNS providers. This webhook allows to manage your UniFi domains inside your Kubernetes cluster.

## Requirements

- ExternalDNS >= v0.14.0
- UniFi OS >= 4.0.3 (currently Early Access only)
- UniFi Network >= 8.2.87 (currently Early Access only)

## Deployment

1. Create a local user with a password in your UniFi OS, this user only needs read/write access to the UniFi Network appliance.

2. Add the ExternalDNS Helm repository to your cluster.

    ```sh
    helm repo add external-dns https://kubernetes-sigs.github.io/external-dns/
    ```

3. Create a secret that holds `UNIFI_USER` and `UNIFI_PASS`

4. Create the helm values file, for example `external-dns-unifi-values.yaml`:

    ```yaml
    fullnameOverride: external-dns-unifi
    logLevel: debug
    provider:
      name: webhook
      webhook:
        image:
          repository: ghcr.io/kashalls/external-dns-unifi-webhook
          tag: main # replace with a versioned release tag
        env:
          - name: UNIFI_HOST
            value: https://192.168.1.1 # replace with the address to your UniFi router
          - name: UNIFI_USER
            valueFrom:
              secretKeyRef:
                name: external-dns-unifi-secret
                key: username
          - name: UNIFI_PASS
            valueFrom:
              secretKeyRef:
                name: external-dns-unifi-secret
                key: password
          - name: LOG_LEVEL
            value: debug
        livenessProbe:
          httpGet:
            path: /healthz
            port: http-wh-metrics
          initialDelaySeconds: 10
          timeoutSeconds: 5
        readinessProbe:
          httpGet:
            path: /readyz
            port: http-wh-metrics
          initialDelaySeconds: 10
          timeoutSeconds: 5
    policy: sync
    sources: ["ingress", "service"]
    txtOwnerId: default
    txtPrefix: k8s.
    domainFilters: ["example.com"] # replace with your domain
    ```

5. Install the Helm chart

    ```sh
    helm install external-dns-unifi external-dns/external-dns -f external-dns-unifi-values yaml --version 1.14.3 -n external-dns
    ```
