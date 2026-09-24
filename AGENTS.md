# AGENTS.md

You are an experienced, pragmatic software engineering AI agent. Do not over-engineer a solution when a simple one is possible. Keep edits minimal. If you want an exception to ANY rule, you MUST stop and get permission first.

## Project Overview

This is a Talos Linux GitOps home-ops monorepo. Three layers compose in one direction only:

```text
talos/    → Day 0/1/2: bare-metal Talos machine configs + Ansible automation (renders, bootstraps, operates clusters)
flux/     → Day 2: everything running inside Kubernetes, delivered only by FluxCD (infra, apps, fleet)
projects/ → In-repo sources consumed by Flux (Go services, one Helm chart)
```

Supporting dirs: `scripts/` (manual-only `fetch-references.sh`), `.flox/` (pinned dev toolchain, source of truth), `.github/` (SHA-pinned CI workflows). There is no other app code in this repo.

Key facts:

- **talos/**: `clusters/_base` (shared barebone `patches.yml` + vanilla `schematics.yml`) plus one dir per cluster (`acme-dev-bdo1-talos-apps-01`, `acme-prd-bdo1-talos-apps-01`). Each cluster has multi-doc `patches.yml` (v1alpha1, applied via `talosctl gen config --config-patch`), 3-layer deep-merged `schematics.yml` (base → cluster → node), committed `pat.yml.template` / `secrets.yml.template`, and `nodes/<name>/{patches,schematics}.yml` per node. Barebone by design: no inline/external bootstrap manifests, no Flannel, no CoreDNS, no kube-proxy userspace mode surprises, no discovery. Docs: `talos/README.md`, `talos/ansible/README.md`, `talos/ansible/RUNBOOK.md` §0–§6.
- **talos/ansible/**: three entrypoints — `playbooks/day0.yml` (render), `playbooks/day1.yml` (bootstrap), `playbooks/day2.yml` (operate) — all `hosts: localhost`, `connection: local`, roles `talos_render` / `talos_bootstrap` / `talos_operate`, plus the Netbird Terraform data-plane under `roles/talos_render/files/netbird/*.tf`. Pins live in `talos/ansible/group_vars/all.yml` (Talos v1.15.0-alpha.0, Kubernetes 1.37.0) with `community.general>=9.0.0`, OpenTofu `>=1.11`, Netbird provider `~>0.0.10`. Day-0 automation writes the handoff artifact `ansible/build/<cluster>/{kubeconfig,talosconfig,secrets.bundle.yml,proton-pass-pat}` — always gitignored. The Talos network fabric is Ansible-managed root-side and is never touched by Flux.
- **flux/**: Day-2 GitOps. `flux/infra/components/<name>/{controllers,configs}/` plus `base` + `dev`/`prd` overlays; `flux/apps/components/*` follows the same shape; `flux/fleet/clusters/` holds per-tenant wiring plus `update/`. Bootstrap order is strict: bare Talos + kubeconfig first, then Terraform provisions the Flux Operator and every deployment flows from it. Single `tofu-controller` (0.16.5, concurrency 24); consumers are `infra.contrib.fluxcd.io/v1alpha2` Terraform objects with `approvePlan: auto`, `destroy: false`, `varsFrom` an ESO Secret, `writeOutputsToSecret`. Gateway API (`main` Gateway, cilium class, :80→:443 redirect, :443 TLS terminate, wildcard cert), cert-manager `letsencrypt` ClusterIssuer (Cloudflare DNS-01) plus per-namespace wildcard Certificates, ESO `ClusterSecretStore` (`proton-pass` webhook at `http://eso-proton-pass.external-secrets:8080`, refresh 1h). Reconcile cadence: OCI 5m, chart 1h, Kustomization 30m, ResourceSet 5m; apps `dependsOn` infra `Ready`.
- **projects/**: `apprise-go-api`, `eso-proton-pass`, `external-dns-netbird` (Go `cmd`+`internal` layout, `Dockerfile` with `ARG VERSION`, distroless `base-debian13` nonroot, `CGO_ENABLED=0`) published to `ghcr.io/lazygeniusman/home-ops/projects/<name>:{dev,stable}` and consumed in Flux only through `$imagepolicy` markers; `helm-rclone-sync` (Chart 0.1.0, app 1.75.0, `templates/cronjob`) published to `oci://ghcr…/helm-rclone-sync` and consumed by the six `rclone-sync-*.yaml` wrappers. Every Flux YAML that wraps a local project comments back to its `projects/` source. `helm-rclone-sync/ci/verify.sh` checks 10 directions.
- **Scope rule**: create or edit only what the task asks for. Never modify `talos/`, `flux/`, `projects/`, `scripts/`, workflows, or configs as a side effect. This repo produces a single-file class of change per task; keep diffs reviewable.

## Reference

- Read the docs that describe current state before acting, in this order: the relevant `README.md` / `RUNBOOK.md` next to the code, then reference docs under `/tmp/home-ops-docs`, then the web. Refresh references by hand when stale:
  `scripts/fetch-references.sh -m zip` (modes: `http` default, `ssh`, `zip`; wipes and re-clones into `/tmp/home-ops-docs`; one failure never aborts the rest).
- When a task touches an unfamiliar tool, check whether a matching agent skill is available and use it instead of improvising.
- Always resolve a Kubernetes question in two passes: search the Talos docs first for prerequisites, known issues, and workarounds, then read the component's own docs. Talos constraints (e.g. no kube-proxy sidecars, schematic-gated extensions) invalidate otherwise-correct upstream advice.
- **Changelog-first on every bump.** Never bump a pin without reading its upstream changelog for breaking changes, deprecations, and CRD/value migrations first. Canonical release pages per component:
  | Component | Changelog |
  |---|---|
  | Cilium | https://github.com/cilium/cilium/releases |
  | Gateway API | https://github.com/kubernetes-sigs/gateway-api/releases |
  | cert-manager | https://github.com/cert-manager/cert-manager/releases |
  | CNPG | https://github.com/cloudnative-pg/cloudnative-pg/releases |
  | External Secrets (ESO) | https://github.com/external-secrets/external-secrets/releases |
  | SeaweedFS (+ operator/CSI) | https://github.com/seaweedfs/seaweedfs/releases |
  | COSI | https://github.com/kubernetes-sigs/container-object-storage-interface/releases |
  | KubeVirt | https://github.com/kubevirt/kubevirt/releases |
  | Multus | https://github.com/k8snetworkplumbingwg/multus-cni/releases |
  | Flux + Flux Operator | https://github.com/fluxcd/flux2/releases + https://github.com/controlplaneio-fluxcd/flux-operator/releases |
  | tofu-controller | https://github.com/flux-iac/tofu-controller/releases |
  | Dragonfly operator | https://github.com/dragonflydb/dragonfly-operator/releases |
  | VPA | https://github.com/kubernetes/autoscaler/releases |
  | ClickHouse (server + Altinity operator) | https://github.com/ClickHouse/ClickHouse/releases + https://github.com/Altinity/clickhouse-operator/releases |
- Keep every doc current-state-only. After any code or config change, update the adjacent doc in the same commit; never leave a doc describing a past design.
- Toolchain source of truth is `.flox/env/manifest.toml` (ansible 2.21.4, ansible-lint 25.8.2, go 1.26.7, gopls 0.23.0, golangci-lint 2.13.2, govulncheck 1.8.0, opentofu 1.12.6, kubectl 1.37.0, helm 4.3.0, kustomize 5.8.1, fluxcd 2.9.5, yq 4.53.3, kubeconform 0.8.0, yamllint 1.37.1, cosign 3.1.3, oras 1.3.4, proton-pass-cli 2.3.3; `terraform` is aliased to `tofu`). `talosctl` v1.15.0-alpha.0 is the one exception: it is fetched by the `on-activate` hook via curl into `.flox/cache/bin`, not from the catalog. On any tool upgrade, migrate every consumer together so pins keep parity across the Flox manifest, `talos/ansible/group_vars/all.yml`, Terraform/Ansible version constraints, GitHub workflows, Dockerfiles, and Flux manifests.
- Secrets live in Proton Pass and are injected with the `pass-cli` binary (not `proton-pass-cli`). Gate on `pass-cli info` for login state, inject with double-brace templates plus `item view`, and always export the hardened env (`PROTON_PASS_DISABLE_TELEMETRY=1`, key provider `fs`, agent reason set, `*_FILE` file-backed pattern). Unencrypted secrets are gitignored at repo root and under `talos/.gitignore`; only double-brace `{{ }}` placeholders are ever committed. For every `pass://` reference you add, document its full path length, one redacted example, and the command that generates the value.

## Essential commands

Run everything from the repo root inside Flox (`terraform` already means `tofu`). Copy-paste as-is; do not invent flags.

```bash
# Talos Day 0/1/2 — replace <cluster> with acme-dev-bdo1-talos-apps-01 or acme-prd-bdo1-talos-apps-01
ansible-playbook talos/ansible/playbooks/day0.yml -i localhost -e talos_cluster=<cluster>   # render
ansible-playbook talos/ansible/playbooks/day1.yml -i localhost -e talos_cluster=<cluster>   # bootstrap
ansible-playbook talos/ansible/playbooks/day2.yml -i localhost -e talos_cluster=<cluster>   # operate
ansible-playbook talos/ansible/playbooks/day2.yml -i localhost -e talos_cluster=<cluster> --check --diff  # dry run
talosctl validate -c ansible/build/<cluster>/nodes/<node>/controlplane.yaml -m metal   # validate rendered machine config
```

```bash
# Flux validation (one scope at a time)
flux/scripts/validate.sh -d flux/infra   # yq + kubeconform strict + kustomize build
flux/scripts/validate.sh -d flux/apps
flux/scripts/validate.sh -d flux/fleet
```
```bash
# Upgrade loop: check versions -> check changelog -> bump -> migrate/verify
helm show chart oci://<registry>/<chart> --version <tag>          # inspect chart before bumping
helm show values oci://<registry>/<chart> --version <tag>         # diff values for breaking changes
oras repo tags ghcr.io/<org>/<image> | sort -V | tail -5          # list candidate image tags
yq '.spec.policy.semver.range' flux/{infra,apps}/update-policies/<name>.yaml  # confirm policy floors before/after (local, no cluster)
curl -sSL -o /tmp/new.yaml <release-url>/kubevirt-operator.yaml && diff <(head -100 flux/infra/components/kubevirt/controllers/base/kubevirt-operator.yaml) <(head -100 /tmp/new.yaml)  # re-vendor then diff (KubeVirt/Multus)
kustomize build flux/infra/components/<name>/controllers/dev      # overlay renders after bump
helm template <release> oci://<registry>/<chart> --version <new> --values <values-file> | diff <(helm template <release> oci://<registry>/<chart> --version <old> --values <values-file>) - | head -80  # chart diff
tofu -chdir=<module> init -backend=false && tofu -chdir=<module> validate && tofu -chdir=<module> test  # touched modules only
```

```bash
# Go projects (run from projects/<name>)
go vet ./... && go build ./... && go test -race -shuffle=on ./...
gofmt -l . && golangci-lint run ./... && govulncheck ./...
```

```bash
# Helm chart + OpenTofu + supply chain
helm lint projects/helm-rclone-sync && helm template projects/helm-rclone-sync | head -50
bash projects/helm-rclone-sync/ci/verify.sh
tofu -chdir=<module> init -backend=false && tofu -chdir=<module> validate && tofu -chdir=<module> test
cosign sign ghcr.io/lazygeniusman/home-ops/projects/<name>:<tag>
```

```bash
# Docs + secret gate
scripts/fetch-references.sh -m zip   # manual-only refresh of /tmp/home-ops-docs
pass-cli info                        # must succeed (logged in) before any secret injection
ls AGENTS.md && head -20 AGENTS.md   # sanity check for this file
```

CI mirrors these gates per path (Go workflows, `flux-*-validate.yaml`, push/release/image-update flows use `contents: read`, concurrency groups, and path filters). Ansible has no CI gate — the `--check --diff` dry run plus `talosctl validate` and FQCN lint are the manual equivalent. Never run `kubectl` or `helm` against a cluster directly; only the Terraform-bootstrapped Flux Operator mutates cluster state.

## Patterns

- **Declare, don't imperatively apply.** Every cluster mutation ships as a manifest so Flux's update monitoring and auto-PR automation can see it. If you catch yourself reaching for an imperative command, convert it into a declarative manifest (or a template that renders one) while keeping the image/chart update watcher and its PR flow intact.
- **Talos renders, Flux delivers.** Talos produces a bare, telemetry-free machine; Flux installs CNI, DNS, and all workloads. Keep that handoff clean: machine config never carries workloads, Flux never carries machine config.
- **Base holds the shape, overlays hold the difference.** Whenever dev and prd diverge, put a placeholder in `base` (`__PROTON_PASS_BASE__`, `__WILDCARD_TLS_SECRET__`, `__SERVICE_HOST__`, `__BASE_DOMAIN__`, `__ACME_EMAIL__`) and an explicit replacement in each environment overlay. Never fork a whole file per environment.
- **Pin parity on upgrade.** Flox manifest, `group_vars/all.yml`, Terraform/Ansible constraints, workflow `uses:` SHAs, Dockerfile `FROM` pins (Go `go.mod` 1.26.7 + `golang:1.26.7@sha` + distroless; `PASS_CLI` 2.3.3 with sha), and Flux image/chart refs all move together. A half-upgraded pin is a bug.
- **Images and charts update themselves.** Every OCI image and Helm chart gets an image policy (`flux/{infra,apps}/update-policies/<name>.yaml`: `ImageRepository` 12h + semver `ImagePolicy`) plus the `# {"$imagepolicy":"area:name:tag"}` marker at each consumption site, driven by the `flux/fleet/clusters/update/` `ResourceSet` + `ImageUpdateAutomation` (30m, Setters push `image-updates-*` branches). Human merges the resulting PR after review — automation proposes, never merges.
- **Upgrade lifecycle (check versions -> check changelog -> bump -> migrate/verify).** Every bump follows the same manual loop: check the available version (policy `range:` floor plus upstream tags), read the changelog first (canonical pages under Reference), bump the pin, then migrate values/CRDs and verify with the gates. Dev soaks first; promote dev->stable/prd only after dev is green. Automation proposes (ImageUpdateAutomation PR), human merges after review — never auto-merge. Per-type mechanics: OCI chart/image — let the `$imagepolicy` marker move via the automation PR, then hand-verify; classic `HelmRepository` (coder, seaweedfs, headlamp, coredns, VPA) — hand-bump `version:`/`tag:` plus the policy floor, never convert to OCI; vendored bundles (KubeVirt operator+CR, Multus) — re-download at the new tag, diff, then bump the marker; provider/ISO/schematic (Netbird `versions.tf` `~>`, Talos `talos_version`, `tofu-controller` digest) — bump version and digest together. Chart<->app divergence checklist: coder chart vs app image, zitadel chart vs app, coredns chart vs app, clickhouse-operator vs server/keeper, rclone-sync chart vs appVersion (all six wrappers atomically), `tofu-controller` chart vs `tofu-runner`. Coupled moves: Cilium<->Gateway API, Hubble UI<->Cilium, flux-operator-ui chart tag <-> `flux/fleet/terraform/versions.yaml` `operator_chart_version`, clickhouse-server<->keeper, oauth2-proxy single shared marker (`apps:oauth2-proxy:tag`) across clickstack/hubble-ui/seaweedfs.
- **Prefer Helm OCI; document the exception.** New charts use `OCIRepository` + `chartRef` (interval 1h). Classic `HelmRepository` (coder 2.37.0, plus seaweedfs alongside it) is grandfathered — keep its header comment explaining why.
- **Production-ready by default.** Follow upstream best practices and harden for security on every deployment: health checks, resource requests/limits, autoscaling, and secrets-via-ESO on everything you add (details below). Scalable, observable, private-by-default is the baseline, not a stretch goal.
- **Workload standards:**
  - Health check on every Deployment, tuned per that component's own docs.
  - Resources (requests/limits) on every Deployment, tuned per docs.
  - Stateless scales with HPA: dev `min 1 / max 2`, prd `min 2 / max 4`, tuned per docs.
  - Stateful stays at 1 replica unless its operator makes HA safe on a `local-path` PVC.
  - VPA (`mode: Off` alongside any HPA) for stateful sets, daemonsets, and single-replica services, tuned per docs.
  - Never combine an active VPA mode with HPA on the same workload.
- **Expose through Gateway API only — never Ingress.** Apps declare their own `HTTPRoute` with `parentRefs` to the shared `main` Gateway. In-cluster traffic uses short service names (no FQDN); Gateway hostnames are reserved for user-facing or public-URL endpoints. Split admin and user domains where the app allows it.
- **Secrets via ESO Proton Pass webhook.** `ExternalSecret` → cluster `pass://<cluster>/<namespace>/<field>` through the shared `ClusterSecretStore` (refresh 1h). Talos-level secrets use `pass://<cluster>/talos/<field>`.
- **TLS via cert-manager + Cloudflare DNS-01.** `letsencrypt` ClusterIssuer plus one wildcard `Certificate` per namespace; overlays swap the placeholder secret name and ACME email.
- **Data plane choices (do not substitute without permission):** S3 via SeaweedFS with a COSI `Claim` + `Access` per bucket; back up each COSI claim with a `helm-rclone-sync` cron to Proton Drive at `home-ops/backups/{cluster}/{ns}/{bucket}`; Postgres via CNPG with S3 backup; Redis API via Dragonfly Operator; Mongo API via FerretDB backed by CNPG; ClickHouse via the Altinity operator with S3 backup; observability via ClickStack + ClickHouse + OpenTelemetry; notifications via `apprise-go-api` to Matrix (tuwunel) provisioned by Tofu Controller Terraform in a dedicated read-only room with the token taken from bootstrap; SSO via Zitadel provisioned by Tofu Controller Terraform (initial Helm credentials bootstrapped once, admin gets access to everything, dedicated groups/roles/OIDC per app, multi-user from day one; add `oauth2-proxy` + `ExternalAuth` only for apps without native SSO that explicitly requested it); clusters stay private by default and reach the internet through Netbird provisioned by Tofu Controller Terraform.
- **Go services:** `cmd/` + `internal/` layout, `ARG VERSION` in the Dockerfile, distroless `base-debian13` nonroot, `CGO_ENABLED=0`; tags are `<svc>-v*`, never `:latest`.

## Anti-patterns

- Do NOT run `kubectl apply/edit/delete`, `helm install/upgrade`, or any direct cluster mutation. All runtime state flows from the Terraform-bootstrapped Flux Operator; bypassing it causes drift and breaks update automation.
- Do NOT manage Talos with anything but `talosctl`, and only v1.15+ semantics (pre-1.14 syntax differs — verify before copying old examples). No third-party provisioners, no hand-built machine configs.
- Do NOT put bootstrap manifests, CNI, CoreDNS, or discovery config into Talos machine patches. Talos stays barebone; Flux installs everything above the machine.
- Do NOT commit secrets, kubeconfigs, talosconfigs, `secrets.bundle.yml`, or `proton-pass-pat` files. They are gitignored build artifacts; only `*.template` files and double-brace placeholders are committed.
- Do NOT leave telemetry enabled on any deployment (Talos machine config, controllers, apps). If a chart defaults it on, explicitly disable it.
- Do NOT add a classic `HelmRepository` for a new chart, use `:latest` tags, or consume an image without an `$imagepolicy` marker and update policy.
- Do NOT skip the changelog on a bump, half-bump a chart<->app pair (coder, zitadel, coredns, clickhouse-operator/server/keeper, rclone-sync chart/appVersion, `tofu-controller`/`tofu-runner`), or let one side of a coupled move drift (Cilium/Gateway, Hubble/Cilium, UI/`versions.yaml`, server/keeper, oauth2-proxy markers).
- Do NOT let the flux-operator-ui policy touch the fleet sync path (UI stays `serverOnly`, `installCRDs: false`; bootstrap `flux-operator` owns CRDs and sync), and do NOT prune CRDs on upgrade — keep the explicit CRD policy (Cilium/cert-manager/COSI/Gateway keep, fleet `infra-crds` delivery).
- Do NOT use Ingress, FQDNs for in-cluster traffic, or Gateway hostnames for internal-only endpoints.
- Do NOT fork per-environment manifests instead of base-placeholder + overlay-override, and do NOT put a singleton behind HPA.
- Do NOT use `TODO`/`FIXME` in first-party code, push unpinned `uses:` refs in workflows, or run `flux/scripts/validate.sh` against the wrong scope and call it coverage.
- Do NOT skip the doc update, the reference-docs check, or the skill lookup when one applies. Stale docs and improvised tool usage rot this repo faster than anything else.

## Code style

- YAML: 2-space indent, `yamllint`-clean, `kubeconform`-strict valid; multi-doc Talos patches keep each `---` document's `apiVersion`/`kind` explicit.
- Kustomize: `base` is deployable-shaped with placeholders; overlays only patch. Keep `interval: 30m` Kustomizations, `dependsOn` infra for apps, and `Ready` readiness semantics consistent with neighboring components.
- Go: `gofmt`-clean, `go vet` + `golangci-lint` (v2.13.2 config) + `govulncheck` green; conventional layout (`cmd/`, `internal/`), wrapped errors, no dead code.
- Terraform/OpenTofu: `tofu fmt`-clean, `init -backend=false` + `validate` + `test` green; `approvePlan: auto` + `destroy: false` on Flux-managed consumers; outputs that Flux needs go through `writeOutputsToSecret`.
- Ansible: FQCN everywhere (`community.general.*`, `ansible.builtin.*`), no bare `shell:` when a module exists, group vars carry pins.
- Dockerfiles: pinned `FROM` with digest where available, `ARG VERSION` threaded into labels/binary, nonroot distroless runtime.
- Comments: Flux YAMLs that wrap a local project link back to the `projects/` source path; grandfathered exceptions (classic HelmRepository, singleton quirks) carry a header comment stating the reason.

## Commit and Pull Request Guidelines

- Conventional Commits, all lowercase, imperative subject: `type(scope): subject`. Examples: `docs: add AGENTS.md contribution guide`, `feat(netbird): bound request-log route labels`, `fix(eso-proton-pass): bound log route`. Keep the subject under ~72 chars; explain the why in the body.
- Image/chart tags are `<svc>-v*` (automation proposes, human merges); never tag or reference `:latest`.
- One logical change per commit; docs travel with their code/config change (current-state-only, no "part 2" docs promises).
- Before pushing, run the gates for every scope you touched (Go vet/build/test + lint + vuln check; `helm lint`/`template` + `ci/verify.sh`; `flux/scripts/validate.sh -d flux/{apps,infra,fleet}` per scope; `tofu init -backend=false/validate/test`; `cosign sign` for published images; Ansible `--check --diff` + `talosctl validate -c <node file> -m metal` + FQCN lint). Report gate results in the PR.
- Workflows stay hardened: SHA-pinned `uses:`, `contents: read` least privilege, concurrency groups, path-gated triggers.
- Open a PR for review; do not push to a protected branch. The image-update bot's PRs get manual review and merge like any other.
