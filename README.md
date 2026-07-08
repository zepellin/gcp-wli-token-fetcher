# gcp-wli-token-fetcher

A tiny sidecar that fetches a **GCP Workload Identity federation token** from the
GCE/GKE metadata server and keeps it fresh in a file on a shared volume, so that
the main container of a Pod can read it and exchange it with an external identity
provider (e.g. **Azure AD token exchange**).

It is meant to run as an `extraContainer` / sidecar next to a workload (for
example [Atlantis](https://www.runatlantis.io/)) that needs a short-lived Google
identity token but cannot mint one itself.

## What it does

On startup, and then on a cron schedule, the fetcher:

1. Reads the existing token from `TOKEN_FILE` (if present) and inspects its JWT
   `exp` claim.
2. If the token is missing, unparseable, or expires within
   `TOKEN_RENEW_THRESHOLD`, it requests a new identity token from the metadata
   server:
   ```
   GET {METADATA_SERVER_URL}/computeMetadata/v1/instance/service-accounts/{GSA_NAME}/identity?audience={TOKEN_AUDIENCE}&scope={TOKEN_SCOPE}
   Metadata-Flavor: Google
   ```
3. Writes the returned token back to `TOKEN_FILE` (creating parent directories
   as needed).

The process runs forever, re-evaluating the token on every cron tick.

## Configuration

Every option can be set via an environment variable or a command-line flag. The
flag wins if both are provided.

| Env var                 | Flag                | Required | Default                          | Description                                                                 |
| ----------------------- | ------------------- | -------- | -------------------------------- | --------------------------------------------------------------------------- |
| `TOKEN_FILE`            | `-file`             | yes      | —                                | Path where the token is written, e.g. `/data/oidc/token`.                   |
| `GSA_NAME`              | `-gsaname`          | yes      | —                                | Google Service Account email, e.g. `name@myproject.iam.gserviceaccount.com`.|
| `TOKEN_AUDIENCE`        | `-audience`         | yes      | —                                | Identity token audience, e.g. `AzureADTokenExchange`.                       |
| `TOKEN_SCOPE`           | `-scope`            | yes      | —                                | Identity token scope, e.g. `user_impersonation`.                            |
| `CRON_SPEC`             | `-cronspec`         | no       | `* * * * *`                      | Cron schedule for the renewal routine.                                      |
| `TOKEN_RENEW_THRESHOLD` | `-renewthreshold`   | no       | `30m0s`                          | Renew when the token's remaining TTL drops below this Go duration.          |
| `METADATA_SERVER_URL`   | `-metadataserverurl`| no       | `http://metadata.google.internal`| Base URL of the metadata server.                                            |
| `LOG_LEVEL`             | —                   | no       | `INFO`                           | `DEBUG`, `INFO`, `WARN`, or `ERROR`. Logs are JSON on stdout.               |

Run `gcp-wli-token-fetcher -version` to print version, commit, and build date.

## Running

### Docker

Prebuilt images are published to GitHub Container Registry:

```bash
ghcr.io/zepellin/gcp-wli-token-fetcher:latest
```

### Kubernetes sidecar

Mount a shared volume so the main container and the fetcher both see the token file:

```yaml
extraContainers:
  - name: gcp-wli-token-fetcher
    image: ghcr.io/zepellin/gcp-wli-token-fetcher:latest
    imagePullPolicy: IfNotPresent
    env:
      - name: TOKEN_AUDIENCE
        value: AzureADTokenExchange
      - name: TOKEN_FILE
        value: /home/atlantis/tokensource/samename@gcp-project-name.iam.gserviceaccount.com/token
      - name: GSA_NAME
        value: samename@gcp-project-name.iam.gserviceaccount.com
      - name: TOKEN_SCOPE
        value: user_impersonation
      - name: LOG_LEVEL
        value: DEBUG
    resources:
      requests:
        cpu: 20m
        memory: 32Mi
    volumeMounts:
      - name: tokenstore
        mountPath: /home/atlantis/tokensource
```

## Development

```sh
go build -v .      # build
go test -v ./...   # test
```

## Releases

- Pushing a **GitHub Release** builds and pushes a multi-arch container image to
  GHCR (see [.github/workflows/container-release.yaml](.github/workflows/container-release.yaml)).
- The same release event also publishes standalone Go binaries via
  [.github/workflows/release.yaml](.github/workflows/release.yaml).
