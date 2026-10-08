# gcp-wli-token-fetcher

A tiny sidecar that fetches a **Google-signed OIDC identity token** for a Google
Service Account (GSA) from the GCE/GKE metadata server and keeps it fresh in a
file on a shared volume. The main container of the Pod exchanges that token for
credentials with any service that trusts Google as an OIDC issuer, such as
**Microsoft Entra ID**, **AWS STS** or **HashiCorp Vault** (see [Use cases](#use-cases)).

It is meant to run as a sidecar next to a workload that needs a short-lived Google
identity token but cannot mint one itself.

## What it does

On startup, and then every `RENEW_INTERVAL`, the fetcher:

1. Reads the existing token from `TOKEN_FILE` (if present) and inspects its JWT
   `exp` claim. The signature is not verified.
2. If the token is missing, unparseable, has no `exp` claim, or expires within
   `TOKEN_RENEW_THRESHOLD`, it requests a new identity token from the metadata
   server (10s timeout):

   ```http
   GET {METADATA_SERVER_URL}/computeMetadata/v1/instance/service-accounts/{GSA_NAME}/identity?audience={TOKEN_AUDIENCE}&scope={TOKEN_SCOPE}
   Metadata-Flavor: Google
   ```

   The `scope` parameter is only sent when `TOKEN_SCOPE` is set.
3. Writes the new token to `TOKEN_FILE` atomically (temp file + rename), so
   readers never see a partial token. Parent directories are created as needed
   and the file is world-readable (`0644`), so a container running as a
   different user can read it.

A failed fetch is logged and retried on the next tick. The existing token is
left untouched and the process keeps running. It stops on `SIGTERM`/`SIGINT`.

## Prerequisites

The metadata server must be able to issue identity tokens for `GSA_NAME`. That
means one of the following:

- On **GKE with Workload Identity**, the Pod's Kubernetes service account is
  annotated with `iam.gke.io/gcp-service-account: <GSA_NAME>` and has
  `roles/iam.workloadIdentityUser` on that GSA.
- On **GCE**, `GSA_NAME` is the service account attached to the VM.

## Use cases

The token has issuer `https://accounts.google.com`, audience `TOKEN_AUDIENCE`
and subject set to the GSA's numeric unique ID. The trust you configure on the
consuming side must match these claims. Many clients read the token from a file
path given in an environment variable; point that variable at `TOKEN_FILE`:

| Consumer | Token file setting | Trust setup |
| --- | --- | --- |
| Azure SDKs (`WorkloadIdentityCredential`) | `AZURE_FEDERATED_TOKEN_FILE`, plus `AZURE_CLIENT_ID`, `AZURE_TENANT_ID` | [Federated identity credential](https://learn.microsoft.com/en-us/entra/workload-id/workload-identity-federation-create-trust) on an Entra app or user-assigned managed identity. Its audience must equal `TOKEN_AUDIENCE` (Entra's default is `api://AzureADTokenExchange`). |
| Terraform `azurerm` provider / backend | `ARM_USE_OIDC=true`, `ARM_OIDC_TOKEN_FILE_PATH` | Same as above. |
| AWS SDKs, AWS CLI, Terraform `aws` provider | `AWS_WEB_IDENTITY_TOKEN_FILE`, plus `AWS_ROLE_ARN` | IAM role trusting `"Federated": "accounts.google.com"` for `sts:AssumeRoleWithWebIdentity`. For service-account tokens, `accounts.google.com:aud` matches the GSA's unique ID and `accounts.google.com:oaud` matches `TOKEN_AUDIENCE`. |
| Vault Agent | `auto_auth` method `jwt` with `path = TOKEN_FILE` and `remove_jwt_after_reading = false` | [JWT auth method](https://developer.hashicorp.com/vault/docs/auth/jwt) with `oidc_discovery_url="https://accounts.google.com"`, and a role bound to the audience and subject. |

The `remove_jwt_after_reading = false` setting matters because Vault Agent
deletes the file after reading it by default.

## Configuration

Options can be set via an environment variable or a command-line flag. The flag
wins if both are provided.

| Env var                 | Flag                 | Required | Default                           | Description                                                                  |
| ----------------------- | -------------------- | -------- | --------------------------------- | ---------------------------------------------------------------------------- |
| `TOKEN_FILE`            | `-file`              | yes      | —                                 | Path where the token is written, e.g. `/data/oidc/token`.                    |
| `GSA_NAME`              | `-gsaname`           | yes      | —                                 | Google Service Account email, e.g. `name@myproject.iam.gserviceaccount.com`. |
| `TOKEN_AUDIENCE`        | `-audience`          | yes      | —                                 | Identity token audience, e.g. `AzureADTokenExchange`.                        |
| `TOKEN_SCOPE`           | `-scope`             | no       | —                                 | Identity token scope, e.g. `user_impersonation` (Azure-specific).            |
| `RENEW_INTERVAL`        | `-interval`          | no       | `1m`                              | How often to check the token (positive Go duration).                         |
| `TOKEN_RENEW_THRESHOLD` | `-renewthreshold`    | no       | `30m`                             | Renew when the token's remaining lifetime drops to this Go duration or less. |
| `METADATA_SERVER_URL`   | `-metadataserverurl` | no       | `http://metadata.google.internal` | Base URL of the metadata server.                                             |
| `LOG_LEVEL`             | —                    | no       | `INFO`                            | `DEBUG`, `INFO`, `WARN` or `ERROR` (case-insensitive). JSON logs on stdout.  |

Google identity tokens are valid for 1 hour, so keep `TOKEN_RENEW_THRESHOLD`
below that. Otherwise a new token is fetched on every tick.

Run `gcp-wli-token-fetcher -version` to print version, commit, and build date.

### Checking the token

`gcp-wli-token-fetcher -check` exits `0` if `TOKEN_FILE` holds a token that has
not expired yet. It exits `1` otherwise (missing file, unparseable token, or
expired) and prints the reason to stderr. Only `-file` / `TOKEN_FILE` is
required. It is meant to be used as an exec probe; see the Kubernetes example
below.

## Running

### Container image

Multi-arch (`linux/amd64`, `linux/arm64`) images are published to GitHub
Container Registry on every release:

```text
ghcr.io/zepellin/gcp-wli-token-fetcher:<version>   # e.g. 1.2.3, 1.2, 1
ghcr.io/zepellin/gcp-wli-token-fetcher:latest      # latest non-prerelease
```

The image is built `FROM scratch` and contains only the binary
(`/gcp-wli-token-fetcher`, the entrypoint) and CA certificates.

### Kubernetes sidecar

Run the fetcher as a [native sidecar](https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/)
(an init container with `restartPolicy: Always`, Kubernetes 1.29+) with a
`-check` startup probe. Kubernetes then holds back the main containers until the
first token has been written, so they need no wait-for-token loop of their own.
A shared `emptyDir` volume makes the token file visible to both containers:

```yaml
spec:
  serviceAccountName: my-wli-sa-name # bound to GSA_NAME via Workload Identity
  initContainers:
    - name: gcp-wli-token-fetcher
      image: ghcr.io/zepellin/gcp-wli-token-fetcher:latest
      restartPolicy: Always
      startupProbe:
        exec:
          command: ["/gcp-wli-token-fetcher", "-check"]
        periodSeconds: 2
        failureThreshold: 45 # ~90s, longer than RENEW_INTERVAL
      env:
        - name: TOKEN_FILE
          value: /var/run/tokensource/token
        - name: GSA_NAME
          value: my-wli-sa-name@gcp-project-name.iam.gserviceaccount.com
        - name: TOKEN_AUDIENCE
          value: AzureADTokenExchange
        - name: TOKEN_SCOPE
          value: user_impersonation
      resources:
        requests:
          cpu: 20m
          memory: 32Mi
      volumeMounts:
        - name: tokenstore
          mountPath: /var/run/tokensource
  containers:
    - name: workload-container
      # ...
      volumeMounts:
        - name: tokenstore
          mountPath: /var/run/tokensource
          readOnly: true
  volumes:
    - name: tokenstore
      emptyDir: {}
```

#### Using `-check` in probes

- The startupProbe only holds back the main container when the fetcher is a
  native sidecar. As a regular container (e.g. `extraContainers`), keep the wait loop.
- The probe uses the container's env (`TOKEN_FILE`). If you pass `-file` in
  `args` instead, add it to the probe command too.
- If the first fetch fails, the next try waits for `RENEW_INTERVAL`. Make the
  probe window (`periodSeconds × failureThreshold`) longer than that to avoid a restart.
- `-check` also works as a liveness probe: it restarts the fetcher once the
  token has expired. Avoid it as a readiness probe, because an expired token
  would make the whole Pod unready.

## Development

Requires Go 1.24+ (the `toolchain` directive in `go.mod` pins the version used).

```sh
go build -v .                               # build
go test -v ./...                            # test
docker build -t gcp-wli-token-fetcher .     # container image
```

CI runs build and tests on pushes and pull requests to `main`, and a
[gosec](https://github.com/securego/gosec) scan on every push and weekly.

## Releases

Creating a **GitHub Release** triggers two workflows:

- [release.yaml](.github/workflows/release.yaml) attaches standalone binaries
  for `linux` and `darwin` (`amd64`, `arm64`) to the release.
- [container-release.yaml](.github/workflows/container-release.yaml) builds and
  pushes the multi-arch image to GHCR, tagged with the release's semver
  (`X.Y.Z`, `X.Y`, `X`) and `latest` unless it is a prerelease.
