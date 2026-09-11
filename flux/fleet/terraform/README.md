# Flux bootstrap (Terraform)

Bootstraps the Flux Operator on a cluster with OpenTofu/Terraform, then hands
steady-state reconciliation to Flux. No workload content lives here — the
desired state is `../clusters/<cluster_name>`.

## Why barebone Talos needs prerequisites

The Talos base machine config ships with no pod networking, no ClusterIP
DNAT, and no cluster DNS: `talos/clusters/_base/patches.yml` deletes the
`KubeFlannelCNIConfig` (no CNI) and sets `KubeCoreDNSConfig{enabled:false}`,
while per-cluster patches set `KubeProxyConfig{enabled:false}` (Cilium
replaces kube-proxy). At `tofu apply` time a normally-networked bootstrap
Pod can therefore never become Ready, so the module is wired for it:

- `job.host_network = true` runs the bootstrap Job on the host stack —
  the upstream escape hatch documented for installing a CNI from the Job
  (implies `dnsPolicy: Default`, so `ghcr.io`/`quay.io` pulls resolve via
  the Talos `ResolverConfig` upstream nameservers `1.1.1.1`/`8.8.8.8`).
- `gitops_resources.prerequisites.charts` installs the Cilium Helm chart
  **before** the Flux Operator, so pod networking + ClusterIP exist when
  Flux takes over.

What runs pre-Flux vs post-Flux:

| Phase | What | Owner |
| --- | --- | --- |
| Pre-Flux (this module) | Cilium chart (node networking), Flux Operator chart, FluxInstance | Terraform / bootstrap Job |
| Post-Flux (Flux tenants) | Cilium + CoreDNS HelmReleases via the infra ResourceSet (`../tenants/infra.yaml`, inputs #1/#2) | Flux (adopts the Cilium release + namespace via `flux_adoption_check`; Terraform then stops touching it) |

CoreDNS is deliberately **not** a prerequisite: pre-Cilium the Job uses
host DNS, and post-Cilium it keeps host DNS while Flux reconciles CoreDNS
as a tenant with no ordering dependency on the Job — the Job never needs
in-cluster DNS before CoreDNS lands.

Per-cluster prerequisite differences: only `var.cilium_k8s_service_host`
(the Talos K8s API VIP / Layer2VIP baked into the Cilium
`k8sServiceHost` value) — `192.168.1.198` for prd/stg, `192.168.1.248`
for dev, matching the `controllers/<env>/` kustomization patches. It is
not the LB pool VIP (`.199` prd / `.249` dev). The variable validates to
one of the two known VIPs.

## Single source of truth

`main.tf` reuses the same files Flux reconciles (no duplicated versions or
values):

| Terraform input | Source file (also reconciled by Flux) |
| --- | --- |
| `gitops_resources.instance_yaml` | `../clusters/<cluster_name>/flux-system/flux-instance.yaml` |
| `operator_chart.values_yaml` | `../clusters/<cluster_name>/flux-system/flux-operator-values.yaml` |
| `operator_chart.repository` / `version` | `versions.yaml` (repository must match the `OCIRepository` url in `../clusters/<cluster_name>/flux-system/flux-operator.yaml`; the GitOps ref itself tracks semver `*` per D2, `versions.yaml` records the bootstrap install version) |
| `prerequisites.charts[0]` (repository = OCI url minus `oci://`, version = ref tag, values = HelmRelease `spec.values`) | `../../infra/components/cilium/controllers/base/cilium.yaml` (the same OCIRepository + HelmRelease Flux reconciles; `k8sServiceHost` placeholder filled from `var.cilium_k8s_service_host`) |

`tests/versions.tftest.hcl` asserts the operator mapping and
`tests/prerequisites.tftest.hcl` asserts the Cilium mapping (prd + dev
VIPs); `tofu test` fails on drift.

## First bootstrap order (the `stable` chicken-and-egg)

The prd `FluxInstance` syncs `ref: stable`, but `stable` does not exist until
the first `flux-fleet-vX.Y.Z` release is tagged — and cutting that release
from never-bootstrapped fleet content is a blind bet. Resolve it dev-first:

1. **Bootstrap dev first.** `acme-dev-bdo1-talos-apps-01` syncs `ref: dev`,
   published from every `main` commit by `flux-fleet-push.yaml` — no release
   tag needed. `tofu apply` with `cluster_name=acme-dev-bdo1-talos-apps-01`
   and `cilium_k8s_service_host=192.168.1.248`, then validate end to end on
   the live cluster: Cilium prerequisite → Flux Operator → infra tenants
   (Cilium adoption via `flux_adoption_check`, CoreDNS `kube-dns` answering
   at `10.96.0.10`).
2. **Publish stable.** Tag `flux-fleet-vX.Y.Z` once dev is green — the release
   workflow pushes `stable` + `stable-<version>` and cosigns them, which is
   exactly what the prd verify pin
   (`flux-instance.yaml` → `flux-fleet-release.yaml@refs/tags/...`) expects.
3. **Bootstrap prd pinned to stable.** `tofu apply` with
   `cluster_name=acme-prd-bdo1-talos-apps-01` and
   `cilium_k8s_service_host=192.168.1.198`. The Job reads the prd
   `instance_yaml` verbatim — it already says `stable`, now resolvable.

The bootstrap Job itself is ref-agnostic (it consumes local files, never
pulls the OCI tag), so this order is purely about making `stable` exist and
trustworthy before prd points at it. Later bootstraps (recovery, new prd
hardware) skip straight to step 3.

## Usage (manual, no live apply in CI)

```shell
cd flux/fleet/terraform
tofu init -backend=false
tofu validate
tofu test
tofu plan \
  -var oci_token="${GITHUB_TOKEN}" \
  -var cluster_name="acme-prd-bdo1-talos-apps-01" \
  -var cluster_region="home-lab" \
  -var cilium_k8s_service_host="192.168.1.198"
```

Use `192.168.1.248` for `acme-dev-bdo1-talos-apps-01`. `tofu plan` needs
no cluster access beyond validation (mock providers in tests; real plan
resolves local files only until apply).

Bump `var.bootstrap_revision` to trigger a new bootstrap run.
