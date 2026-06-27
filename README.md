# scafctl-plugin-kubeconfig

Kubeconfig and cluster operations provider for scafctl

## Names

This plugin uses the following names across different surfaces:

| Surface | Value |
|---------|-------|
| Repository | `scafctl-plugin-kubeconfig` |
| Go module | `github.com/oakwood-commons/scafctl-plugin-kubeconfig` |
| Binary | `scafctl-plugin-kubeconfig` |
| Provider name | `kubeconfig` |
| Catalog artifact | `kubeconfig` |

The **provider name** is what users reference in solutions (`provider: kubeconfig`).
It comes from the RPC contract (`GetProviders` / `GetProviderDescriptor`), not from
the binary filename.

## Installation

```bash
# Build from source
task build

# Or download from releases
gh release download --repo oakwood-commons/scafctl-plugin-kubeconfig
```

## Usage

Register this plugin in your scafctl configuration, then reference
the **kubeconfig** provider in your solutions:

```yaml
resolvers:
  cluster-server:
    resolve:
      with:
        - provider: kubeconfig
          inputs:
            operation: current_server
```

## Development

```bash
# Run tests
task test

# Run linter
task lint

# Build
task build

# Full CI pipeline (lint + test + build)
task ci
```

## Local Testing

After building, verify the plugin works end-to-end through the host:

```bash
# 1. Build the binary
task build

# 2. Test the provider directly (quick smoke test)
scafctl run provider kubeconfig operation=current_server --plugin-dir ./dist

# 3. Package as a local catalog artifact
task release:local VERSION=0.1.0

# 4. Install from a sample solution
scafctl plugins install -f ./examples/solution.yaml

# 5. Run the sample solution to verify host registration
scafctl run solution -f ./examples/solution.yaml
```

The full local loop (build, package, install, run) is the most reliable way to
verify that the host registers the provider correctly. Direct `--plugin-dir`
testing may not exercise the same registration path as catalog-installed plugins.

## Release

### Publishing to a catalog

A tagged release should publish both the plugin artifact and refresh the
catalog index:

```bash
# Publish the plugin artifact
scafctl catalog push kubeconfig --version v1.0.0

# Refresh the catalog index so the plugin is discoverable
scafctl catalog index push --catalog oci://ghcr.io/<REGISTRY_OWNER>
```

Both steps are required. Publishing the artifact alone does not make the
plugin appear in catalog listings.

### CI release workflow

The release workflow needs two kinds of authentication:

1. **Container registry auth** for OCI push operations (`docker login` or equivalent).
2. **scafctl auth** for catalog operations (`scafctl auth login github --flow pat --registry ghcr.io --write-registry-auth`).

Standard `docker login` is not sufficient for `scafctl catalog index push`.

### Required secrets

| Secret | Scopes | Purpose |
|--------|--------|---------|
| `GITHUB_TOKEN` | Default | Build, test, create release |
| `CATALOG_PUSH_TOKEN` | `repo`, `read:packages`, `write:packages` | Publish artifact and refresh catalog index |

Create the publishing secret at the org or repo level:

```bash
gh secret set CATALOG_PUSH_TOKEN --org <ORG> --repos scafctl-plugin-kubeconfig --body "$TOKEN"
```

### Token strategy

For official providers, use a machine account or GitHub App for the publishing
token rather than a personal account. This avoids tying release capability to
an individual developer.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache-2.0 -- see [LICENSE](LICENSE) for details.