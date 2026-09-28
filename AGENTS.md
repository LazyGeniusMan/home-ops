# AGENTS.md

You are an experienced, pragmatic software engineering AI agent. Do not over-engineer a solution when a simple one is possible. Keep edits minimal. If you want an exception to ANY rule, you MUST stop and get permission first.

## Living doc

AGENTS.md and every markdown files and comments in source code and config files are living docs. Any change altering behavior, pins, topology, or workflow must update AGENTS.md and the adjacent docs in the same commit. Write important, concise, current-state-only: state what is true now; delete unnecessary, history, rationale essays, and superseded alternatives instead of appending. Never defer docs to a follow-up.

## Project Overview

This is a Talos Linux GitOps home-ops monorepo. Three layers compose in one direction only:

```text
talos/    → Day 0/1/2: bare-metal Talos machine configs + Ansible automation (renders, bootstraps, operates clusters)
flux/     → Day 2: everything running inside Kubernetes, delivered only by FluxCD (infra, apps, fleet)
projects/ → In-repo sources consumed by Flux (Go services, first-party Helm charts)
```

Supporting dirs: `scripts/` (manual-only `fetch-references.sh` + `tag-release.sh`), `.flox/` (pinned dev toolchain, source of truth), `.github/` (SHA-pinned CI workflows). There is no other app code in this repo.

Key facts:

- **talos/**: `clusters/_base` (shared barebone `patches.yml` + vanilla `schematics.yml`) plus one dir per cluster (`acme-dev-bdo1-talos-apps-01`, `acme-prd-bdo1-talos-apps-01`). Each cluster has multi-doc `patches.yml` (v1alpha1, applied via `talosctl gen config --config-patch`), 3-layer deep-merged `schematics.yml` (base → cluster → node), committed `pat.yml.template` / `secrets.yml.template`, and `nodes/<name>/{patches,schematics}.yml` per node. Barebone by design: no workloads, CNI, DNS, or discovery in machine config. Docs: `talos/README.md`, `talos/ansible/README.md`, `talos/ansible/RUNBOOK.md` §0–§6.
- **talos/ansible/**: three entrypoints — `playbooks/day0.yml` (render), `playbooks/day1.yml` (bootstrap), `playbooks/day2.yml` (operate) — all `hosts: localhost`, `connection: local`, roles `talos_render` / `talos_bootstrap` / `talos_operate`, plus the Netbird Terraform data-plane under `roles/talos_render/files/netbird/*.tf`. Pins live in `talos/ansible/group_vars/all.yml` (`talos_version`, `talos_kubernetes_pinned_version`), `talos/ansible/requirements.yml` (exact `community.general` pin — never a floor range; bump only after re-testing day-0), and the netbird `versions.tf` (tofu floor bound, pessimistic `~>` provider floor) — read them there, never from here. Day-0 writes `talos/ansible/build/<cluster>/{talosconfig,secrets.bundle.yml,proton-pass-pat}` (`talos_build_dir` = `playbook_dir/../build`); day-1 adds `kubeconfig` via `talosctl kubeconfig -f` — always gitignored. The Talos network fabric is Ansible-managed root-side (see Talos renders, Flux delivers).
- **flux/**: Day-2 GitOps. Shapes and counts live in `flux/{infra,apps,fleet}/README.md`: infra `controllers|configs/{base,dev,prd}` (plus `crds/` or `terraform/` where noted), apps flat `{base,dev,prd}` + `README`, fleet tenants + overlays + clusters. Quirks (empty `resources: []` shells, operator CRs) are stated beside the code.
- **flux wiring**: bootstrap (bare Talos + kubeconfig, then Terraform provisions the Flux Operator with the Cilium prerequisite first), reconcile cadences, and per-component pins live in `flux/fleet/README.md`, the tenant files, and each `update-policies/<name>.yaml` header — read them there, never from here.
- **projects/**: Go services (`apprise-go-api`, `eso-proton-pass`, `external-dns-netbird`) and first-party Helm charts (`helm-rclone` plus `helm-<name>/` for chartless upstreams) — layouts, registries, and workflows per each `projects/` README; Chart.yaml `version` == Flux wrapper `ref.tag` on every hand-bumped chart. Versions live there, never here.
- **Scope rule**: create or edit only what the task asks for. Never modify `talos/`, `flux/`, `projects/`, `scripts/`, workflows, or configs as a side effect. This repo produces a single-file class of change per task; keep diffs reviewable.

## Reference

- Read the docs that describe current state before acting, in this order: the relevant `README.md` / `RUNBOOK.md` next to the code, then `.github/WORKFLOW.md` for CI, then reference docs under `/tmp/home-ops-docs`, then the web. Refresh references by hand (`scripts/fetch-references.sh -m zip`; see script help for modes).
- When a task touches an unfamiliar tool, check whether a matching agent skill is available and use it instead of improvising. Vendored `.agents/skills/` are generic upstream guidance, not repo rules — where they conflict with this file (OCI `:latest` examples, dependabot/renovate advice, promql-cli tooling, Talos image-factory links), this file wins.
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
- Keep every doc current-state-only (see Living doc).
- Toolchain source of truth is `.flox/env/manifest.toml` (`allow.unfree = true` is scoped to the four installs with no free catalog substitute: AGPL MCP server, BSD netbird, GPL pass-cli, MIT rclone — see the `[options]` why-comment). `talosctl` (version + sha256 track `talos_version` in `talos/ansible/group_vars/all.yml`) is curl-fetched by the `on-activate` hook into `.flox/cache/bin`, not from the catalog. On any tool upgrade, migrate every consumer together so pins keep parity across the Flox manifest, `talos/ansible/group_vars/all.yml`, Terraform/Ansible version constraints, GitHub workflows, Dockerfiles, and Flux manifests.
- Secrets live in Proton Pass and are injected with the `pass-cli` binary. Gate on `pass-cli info` for login state, inject with double-brace templates plus `item view`, and always export the hardened env (`PROTON_PASS_DISABLE_TELEMETRY=1`, key provider `fs`, agent reason set, `*_FILE` file-backed pattern). Unencrypted secrets are gitignored at repo root and under `talos/.gitignore`; only double-brace `{{ }}` placeholders are ever committed. For every `pass://` reference you add, document its full path length, one redacted example, and the command that generates the value.

## Essential commands

Run everything from the repo root inside Flox (`terraform` already means `tofu`). Copy-paste as-is; do not invent flags.

```bash
# Talos Day 0/1/2 — replace <cluster> with acme-dev-bdo1-talos-apps-01 or acme-prd-bdo1-talos-apps-01
ansible-playbook talos/ansible/playbooks/day0.yml -i localhost, -e talos_cluster=<cluster>   # render
ansible-playbook talos/ansible/playbooks/day1.yml -i localhost, -e talos_cluster=<cluster>   # bootstrap
ansible-playbook talos/ansible/playbooks/day2.yml -i localhost, -e talos_cluster=<cluster>   # operate
ansible-playbook talos/ansible/playbooks/day2.yml -i localhost, -e talos_cluster=<cluster> --check --diff  # dry run
talosctl validate -c talos/ansible/build/<cluster>/nodes/<node>/controlplane.yaml -m metal   # validate rendered machine config
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
yq '.version' projects/helm-<name>/Chart.yaml && yq '.spec.ref.tag' flux/infra/components/<name>/{controllers,crds}/base/*.yaml  # first-party chart parity: Chart.yaml version == wrapper ref.tag
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
helm lint projects/helm-rclone && helm template projects/helm-rclone | head -50
bash projects/helm-rclone/ci/verify.sh
tofu -chdir=<module> init -backend=false && tofu -chdir=<module> validate && tofu -chdir=<module> test
cosign sign ghcr.io/lazygeniusman/home-ops/projects/<name>:<tag>
```
```bash
# Docs + secret gate
scripts/fetch-references.sh -m zip   # manual-only refresh of /tmp/home-ops-docs
pass-cli info                        # must succeed (logged in) before any secret injection
```
```bash
# Local CI gate (prek 0.5.3, Flox-pinned): staged-file-scoped hooks mirror CI path filters
prek install                         # install pre-commit + pre-push shims (once per checkout)
prek run                             # staged files, pre-commit stage (fast gates only)
prek run --all-files                 # whole repo, pre-commit stage (pre-PR check)
prek run --all-files --stage pre-push  # pre-push stage (tofu, full go, talosctl pin)
prek run ansible-day2-check --stage manual  # day-2 --check --diff (needs pass-cli + cluster)
prek run --dry-run --files <path>    # preview which hooks a path selects
```

CI mirrors these gates per path (Go workflows, `flux-*-validate.yaml`, `lint-shell-ansible.yaml`, push/release/image-update flows use deny-all `permissions: {}` with per-job minimums, concurrency groups, and path filters). Ansible changes require the `--check --diff` dry run plus `talosctl validate` and FQCN lint (syntax + lint in CI, `--check --diff` stays local — day-2 needs a live `pass-cli` session). Never run `kubectl` or `helm` against a cluster directly; only the Terraform-bootstrapped Flux Operator mutates cluster state.

Local commits gate on the same CI test/validate/lint legs via prek (`.pre-commit-config.yaml`, staged-files-only): pre-commit runs the fast per-scope gates (meta hygiene, no-artifact guard, shellcheck, yamllint, ansible syntax+lint, `validate.sh -d` per scope, go-fast — vet + gofmt + tidy + build — per service, helm `ci/verify.sh` per chart); pre-push runs the slow whole-scope gates (tofu trio, go-full — test -race + lint + vuln — per service, talosctl pin); day-2 `--check --diff` is manual-only (needs live `pass-cli` + cluster). Flux scope validates and fetch-time chart verifies need GitHub network at pre-commit like CI does (pinned schema/URL fetches; retry with network on failure, never skip). Publishing (push/release/sign), image-update bots, and scheduled check-PR workflows never gate — CI-only. Run inside `flox activate` so `language: system` hooks resolve the Flox toolchain.

## Patterns

- **Declare, don't imperatively apply.** Every cluster mutation ships as a manifest so Flux's update monitoring and auto-PR automation can see it. If you catch yourself reaching for an imperative command, convert it into a declarative manifest (or a template that renders one) while keeping update automation intact.
- **Talos renders, Flux delivers.** Talos produces a bare, telemetry-free machine; Flux installs CNI, DNS, and all workloads. Keep that handoff clean: machine config never carries workloads, Flux never carries machine config.
- **Talos conformance.** Barebone machine rules (volume `maxSize` arithmetic, ≥2 NTP servers, no-telemetry-by-absence, 64-hex schematic IDs) are stated beside the code in `talos/`; node headers + `talos/ansible/RUNBOOK.md` carry the alpha, rotation, PAT, `skip_health`, NFS, and encryption rules — never duplicated here.
- **Base holds the shape, overlays hold the difference.** Whenever dev and prd diverge, put a placeholder in `base` (`__PROTON_PASS_BASE__`, `__WILDCARD_TLS_SECRET__`, `__SERVICE_HOST__`, `__BASE_DOMAIN__`, `__ACME_EMAIL__`) and an explicit replacement in each environment overlay. Never fork a whole file per environment.
- **Pin parity on upgrade.** Flox manifest, `group_vars/all.yml`, Terraform/Ansible constraints, workflow `uses:` SHAs, Dockerfile `FROM` pins (Go toolchain in `.flox/env/manifest.toml`, each `Dockerfile`/`go.mod`), prek hook `rev` SHAs plus the `prek` Flox pin, and Flux image/chart refs all move together. A half-upgraded pin is a bug.
- **Images and charts update themselves.** Mechanics (policy shape, inline `$imagepolicy` markers, automation cadence, hand-bumped exceptions) live in each `flux/{infra,apps}/update-policies/<name>.yaml` header and the component docs — read them there; automation proposes, human merges.
- **First-party charts for chartless upstreams.** Upstream CRDs with no Helm chart never vendor YAML into `flux/` — they ship as `projects/helm-<name>/` (Chart.yaml `version` = upstream release, `ci/fetch.sh` fetches upstream at CI publish time, no CRDs committed, two workflows: daily check-PR + on-push fetch-publish OCI; fetch-time helm-* charts publish purely from the parsed Chart version on push (no git tags; re-publish overwrites the same OCI tag)). Consume in Flux via `OCIRepository` + `HelmRelease` with CRD `CreateReplace` on install+upgrade plus prune:false-equivalent safety, no ImagePolicy/marker (atomic hand-bump like `helm-rclone`).
- **Upgrade lifecycle (check versions -> check changelog -> bump -> migrate/verify).** Check the available version (policy `range:` floor plus upstream tags), read the changelog first (see Changelog-first under Reference), bump the pin, then migrate values/CRDs and verify with the gates. Dev soaks first; promote dev->stable/prd only after dev is green. Chart<->app and coupled pairs move atomically — the pair list and per-type mechanics live in each `flux/{infra,apps}/update-policies/<name>.yaml` header and the component docs, not here.
- **Platform / supply chain.** Cosign legs pin the binary explicitly (`cosign-release`, matching the cosign pin in `.flox/env/manifest.toml`) with `# match .flox` parity comments on setup lines; always quote `"$DIGEST_URL"`. No dependabot/renovate — SHA pins and Flox pins refresh by hand; no config files.
- **Helm OCI-first; official preferred, chartproxy for classic-only, first-party for chartless.** New charts use `OCIRepository` + `chartRef` (interval 1h); no classic `HelmRepository`, no third-party mirrors, never vendor upstream YAML. Chartproxy mappings carry a header comment stating the upstream classic source proxied; local docs under `/tmp/home-ops-docs/helm-charts-oci-proxy`.
- **Production-ready by default.** Follow upstream best practices and harden for security on every deployment: health checks, resource requests/limits, autoscaling, and secrets-via-ESO on everything you add (details below). Scalable, observable, private-by-default is the baseline.
- **Workload standards (all values tuned per component docs):**
  - Health check on every Deployment.
  - Resources (requests/limits) on every Deployment; every CPU limit (or
  deliberate no-limit, e.g. node-local DNS cache) carries a why-comment, and
  CNI/dataplane agents stay requests-only so they are never CPU-throttled.
  - Stateless scales with HPA: dev `min 1 / max 2`, prd `min 2 / max 4`.
  - Stateful stays at 1 replica unless its operator makes HA safe on a `local-path` PVC.
  - VPA (`mode: Off` alongside any HPA) for stateful sets, daemonsets, and single-replica services.
  - Never combine an active VPA mode with HPA on the same workload.
  - PDB (`minAvailable: 1`) on every HPA-scaled Deployment except 1-replica singletons and CNPG-managed workloads (a PDB cannot protect 1 replica).
  - Singletons stay 1/1: a singleton behind an HPA is capped `min 1 / max 1` in every env with a header why-comment; never scale it. PDB-less singletons carry a disrupt note (single replica cannot tolerate eviction); every CPU limit carries a why-comment.
  - CronJobs carry explicit `activeDeadlineSeconds: 3600` + `concurrencyPolicy: Forbid` (stated on every CronJob, never inherited-by-silence); rclone legs use `operation: sync` (sync-mirror; source deletion is the storage backend or db/app operator's job) — every leg carries a sync-mirror header comment.
- **Expose through Gateway API only — never Ingress.** Apps declare their own `HTTPRoute` with `parentRefs` to the shared `main` Gateway. In-cluster traffic uses short service names (no FQDN); Gateway hostnames are reserved for user-facing or public-URL endpoints. Split admin and user domains where the app allows it.
- **Secrets via ESO Proton Pass webhook.** `ExternalSecret` → cluster `pass://<cluster>/<namespace>/<field>` through the shared `ClusterSecretStore` (refresh 1h); Talos-level secrets use `pass://<cluster>/talos/<field>`. Day-0/day-2 role order, preflight, NetBird plane, and re-keying rules live beside the roles and in `talos/ansible/RUNBOOK.md` — never duplicated here.
- **Credential lifecycle (no non-expiring credentials without a runbook).** Every credential ships with its bootstrap obtain + renew/rotate procedure documented beside its code: Talos/Ansible creds (NetBird setup key, Talos PKI) carry renew/rotate tasks next to bootstrap (`talos/ansible/RUNBOOK.md`); k8s/Flux rotating creds via ESO + Reloader opt-in `reloader.stakater.com/auto: "true"` (see `flux/infra/components/reloader`); ESO's own bootstrap cred renews via Talos Ansible. A non-expiring credential is forbidden without a dated rotation tracker + owner/date + age alert.
- **Encryption boundary.** Service-domain cleartext is MANDATORY — CNI/mesh owns pod/node/cluster wire encryption (Cilium WireGuard: `encryption.type: wireguard`, node encryption off on the trusted LAN). TLS is mandatory ONLY at the edge: Gateway API `main` Gateway + cert-manager `letsencrypt` ClusterIssuer (Cloudflare DNS-01) with one wildcard `Certificate` per namespace; overlays swap the placeholder secret name and ACME email. Never add TLS to in-cluster service traffic. Dev uses LE production (not staging — staging certs are untrusted and would break the NetBird/OIDC trust chain; see cert-manager README for the staging escape hatch). Wildcard scope is one DNS label only — nested hosts (`a.b.base`) need a second `*.b.base` cert (coder pattern).
- **Single-node is production-ready if it is scale-ready.** One node + one control-plane is accepted with an accepted-RTO note, day-2 health gates, a single-speaker VIP note, and a tested scale-to-3-CP procedure (details in `talos/ansible/RUNBOOK.md` §4). Keeper/CNPG affinity stays `preferred` (or replica=1) until multi-node — never `required` on one node. Stateful bases carry the production replica count as a live placeholder (dev overlay → 1, prd overlay → production count); base classifies (stateless HPA 1/2 dev + 2/4 prd, stateful 1, singleton 1/1) and overlays only patch bounds.
- **Data plane choices (do not substitute without permission):** S3 via SeaweedFS/COSI, CNPG/Dragonfly/FerretDB/ClickHouse for state, NetBird mesh, Zitadel SSO, Reloader, `apprise-go-api` → Matrix, ClickStack + OTel observability, `helm-rclone` sync-mirror offsite to Proton Drive. Per-component values and backup legs are tuned in the component docs — this list names the stack, not the settings.
- **File-based stateful storage at 1 non-HA replica is accepted while under evaluation** (e.g. tuwunel pre-postgres). HA/backup/restore/replication is the DB/app operator's job on `local-path` for performance; the bare minimum from day one is a periodic backup to in-cluster SeaweedFS S3 plus a `helm-rclone` sync-mirror offsite to Proton Drive at `home-ops/backups/{cluster}/{ns}/{bucket}`.
- **Apps conformance.** Pin `tag@digest` in Flux (`tag` via `$imagepolicy`, one-off tooling pinned by digest with a re-pin comment); coupled hand-bumps (Headlamp), opaque-`config.json` exception, Coder `/env` ordering, and kill-switch checklists live beside the code in `flux/apps/` and the app READMEs.
- **Go services:** `cmd/` + `internal/` layout, `ARG VERSION` in the Dockerfile, distroless `base-debian13` nonroot, `CGO_ENABLED=0`; tags are `<svc>-v*`, never `:latest`. Fail-fast, timeout, trace, and validation rules live beside the code and in each service README.

- **Observability (ClickHouse-native).** ClickHouse is the only store, HyperDX the only UI — no Prometheus/Grafana. Rules live in `flux/OBSERVABILITY.md`: infra OTLP endpoint convention, always-on monitors + `infra-crds` gate, fleet ordering gates, `otel-scrape:true` namespaces, `/healthz` + `/readyz` probes (fixed-path exceptions carry why-comments), VPA `Off` with HPA / `Initial` for singletons, HyperDX webhook → `apprise-go-api` → Matrix alerts.

## Anti-patterns

Each pattern above has a matching negative — this list keeps only negatives that add new information beyond their pattern:

- Do NOT mutate clusters imperatively (`kubectl`/`helm` against a cluster); only the Terraform-bootstrapped Flux Operator writes state.
- Do NOT commit secrets or machine build artifacts (kubeconfigs, talosconfigs, `secrets.bundle.yml`, `proton-pass-pat`); only `*.template` files and double-brace placeholders are committed.
- Do NOT let the flux-operator-ui policy touch the fleet sync path (UI stays `serverOnly`, `installCRDs: false`); CRDs render only through the fleet prune:false `infra-crds` Kustomization — never add a CRD file to a controllers Kustomization under the shared CRD-free contract.
- Do NOT add a chart OCIRepository without cosign verification through the fleet tenant allowlist, or an explicit `No verify:` comment stating why.
- Do NOT skip the doc update, the reference-docs check, or the skill lookup when one applies (see Living doc).

## Code style

- YAML: 2-space indent, `yamllint`-clean, `kubeconform`-strict valid; multi-doc Talos patches keep each `---` document's `apiVersion`/`kind` explicit.
- Kustomize: `base` is deployable-shaped with placeholders; overlays only patch. Keep `interval: 30m` Kustomizations, `dependsOn` infra for apps, and `Ready` readiness semantics consistent with neighboring components.
- Go: `gofmt`-clean, `go vet` + `golangci-lint` (pinned in `.flox/env/manifest.toml`) + `govulncheck` green; conventional layout (`cmd/`, `internal/`), wrapped errors, no dead code.
- Terraform/OpenTofu: `tofu fmt`-clean, `init -backend=false` + `validate` + `test` green; `approvePlan: auto` + `destroy: false` on Flux-managed consumers; outputs that Flux needs go through `writeOutputsToSecret`.
- Ansible: FQCN everywhere (`community.general.*`, `ansible.builtin.*`), no bare `shell:` when a module exists, group vars carry pins.
- Shell: `shellcheck`-clean (pinned in `.flox/env/manifest.toml`); `set -euo pipefail` (documented `set -uo pipefail` exception: `scripts/fetch-references.sh:17`), justified inline `disable=` with why-comment.
- Dockerfiles: pinned `FROM` with digest where available, `ARG VERSION` threaded into labels/binary, nonroot distroless runtime.
- Comments: Flux YAMLs that wrap a local project link back to the `projects/` source path; chartproxy mappings and singleton quirks carry a header comment stating the upstream classic source proxied (chartproxy) or the reason (singleton).

## Commit and Pull Request Guidelines

- Conventional Commits, all lowercase, imperative subject: `type(scope): subject`. Examples: `docs: add AGENTS.md contribution guide`, `feat(netbird): bound request-log route labels`, `fix(eso-proton-pass): bound log route`. Keep the subject under ~72 chars; explain the why in the body.
- Image/chart tags are `<svc>-v*` (automation proposes, human merges); never tag or reference `:latest`.
- One logical change per commit; docs travel with their code/config change (see Living doc).
- Before pushing, run the gates for every scope you touched — the prek shims already ran them on commit/push, so `prek run --all-files` (+ `--stage pre-push` before pushing) is the check; the per-tool equivalents are Go vet/build/test + lint + vuln check; `helm lint`/`template` + `ci/verify.sh`; `flux/scripts/validate.sh -d flux/{apps,infra,fleet}` per scope; `tofu init -backend=false/validate/test`; `cosign sign` for published images; Ansible `--check --diff` + `talosctl validate -c <node file> -m metal` + FQCN lint; `shellcheck` on touched `*.sh`. Never `--no-verify` unasked. Report gate results in the PR.
- Workflows stay hardened: SHA-pinned `uses:`, deny-all `permissions: {}` default with per-job minimums (`contents: read` plus `packages: write` / `id-token: write` only where push/sign needs them), concurrency groups, path-gated triggers.
- Open a PR for review; do not push to a protected branch.
