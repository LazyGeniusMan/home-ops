# AGENTS.md

You are an experienced, pragmatic software engineering AI agent. Do not over-engineer a solution when a simple one is possible. Keep edits minimal. If you want an exception to ANY rule, you MUST stop and get permission first.

## Living doc

AGENTS.md and every markdown files and comments in source code and config files are living docs. Any change altering behavior, pins, topology, or workflow must update AGENTS.md and the adjacent docs in the same commit. Write concise current-state-only: state what is true now; delete history, rationale essays, and superseded alternatives instead of appending. Never defer docs to a follow-up.

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
- **talos/ansible/**: three entrypoints — `playbooks/day0.yml` (render), `playbooks/day1.yml` (bootstrap), `playbooks/day2.yml` (operate) — all `hosts: localhost`, `connection: local`, roles `talos_render` / `talos_bootstrap` / `talos_operate`, plus the Netbird Terraform data-plane under `roles/talos_render/files/netbird/*.tf`. Pins live in `talos/ansible/group_vars/all.yml` (`talos_version`, `talos_kubernetes_pinned_version`), `talos/ansible/requirements.yml` (exact `community.general` pin — never a floor range; bump only after re-testing day-0), and the netbird `versions.tf` (tofu floor bound, exact provider pin) — read them there, never from here. Day-0 writes `talos/ansible/build/<cluster>/{talosconfig,secrets.bundle.yml,proton-pass-pat}` (`talos_build_dir` = `playbook_dir/../build`); day-1 adds `kubeconfig` via `talosctl kubeconfig -f` — always gitignored. The Talos network fabric is Ansible-managed root-side (see Talos renders, Flux delivers).
- **flux/**: Day-2 GitOps. `flux/infra/components/<name>/controllers|configs/{base,dev,prd}` (`crds/` only in gateway-api, cosi, otel-operator; `terraform/` only in netbird, zitadel); intentional empty `resources: []` shells — netbird controllers+configs (shared `terraform/` root consumed via cross-ns `sourceRef`, no update policy), gateway-api/controllers, apprise-go-api/configs, tofu-controller/configs; otel-collectors controllers are operator `OpenTelemetryCollector` CRs, not HelmReleases. `flux/apps/components/<name>/{base,dev,prd}` flat + `README` (8 apps: clickstack, coder, flux-operator-ui, headlamp, hubble-ui, matrix, talos-vm, win11-vm). `flux/fleet/` = `tenants/{infra.yaml (22 inputs),apps.yaml (8 inputs),policies.yaml}` + `tenants/overlays/<cluster>/` + `clusters/<name>/` (`flux-system` + `tenants` wiring) + `clusters/update/automation.yaml` (`ImageUpdateAutomation` 30m, Setters push `image-updates-*` branches) + `fleet/terraform/`. Bootstrap order is strict: bare Talos + kubeconfig first, then Terraform provisions the Flux Operator (Cilium chart installed as a Terraform prerequisite first, then adopted by Flux) and every deployment flows from it. Single `tofu-controller` (semver floor in its update policy); consumers are `infra.contrib.fluxcd.io/v1alpha2` Terraform objects with `approvePlan: auto`, `destroy: false`, `varsFrom` an ESO Secret, `writeOutputsToSecret`. Gateway API (`main` Gateway, cilium class, :80→:443 redirect, :443 TLS terminate, wildcard cert), cert-manager `letsencrypt` ClusterIssuer (Cloudflare DNS-01) plus per-namespace wildcard Certificates, ESO `ClusterSecretStore` (`proton-pass` webhook at `http://eso-proton-pass.external-secrets:8080`, refresh 1h). Reconcile cadence: tenant OCIRepository 5m, chart OCIRepository 1h, FluxInstance OCI 10m, ImageRepository 12h, Kustomization 30m, ResourceSet 5m; apps `dependsOn` infra `Ready`.
- **projects/**: `apprise-go-api`, `eso-proton-pass`, `external-dns-netbird` (Go `cmd`+`internal` layout, `Dockerfile` with `ARG VERSION`, distroless `base-debian13` nonroot, `CGO_ENABLED=0`) published to `ghcr.io/lazygeniusman/home-ops/projects/<name>:{dev,stable}` and consumed in Flux only through `$imagepolicy` markers; `helm-rclone` (versions in `projects/helm-rclone/Chart.yaml`, `templates/cronjob`) published to `oci://ghcr…/helm-rclone` and consumed by the six `rclone-sync-*.yaml` wrappers. Upstream CRDs with no Helm chart ship as `projects/helm-<name>/` instead of vendored YAML: Chart.yaml `version` = upstream release, `ci/fetch.sh` fetches upstream at CI publish time, no CRDs committed, two workflows (daily check-PR + on-push fetch-publish OCI, `tag-release.sh` prefix) — today `helm-kubevirt` 1.9.0, `helm-multus` 4.3.0, `helm-gateway-api` 1.6.1, `helm-cosi` 0.2.2, `helm-otel-monitoring-crds` 0.93.1. Every Flux YAML that wraps a local project comments back to its `projects/` source. `helm-rclone/ci/verify.sh` checks 10 directions.
- **Scope rule**: create or edit only what the task asks for. Never modify `talos/`, `flux/`, `projects/`, `scripts/`, workflows, or configs as a side effect. This repo produces a single-file class of change per task; keep diffs reviewable.

## Reference

- Read the docs that describe current state before acting, in this order: the relevant `README.md` / `RUNBOOK.md` next to the code, then `.github/WORKFLOW.md` for CI, then reference docs under `/tmp/home-ops-docs`, then the web. Refresh references by hand (`scripts/fetch-references.sh -m zip`; see script help for modes).
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
- Keep every doc current-state-only (see Living doc).
- Toolchain source of truth is `.flox/env/manifest.toml`; `talosctl` (version + sha256 track `talos_version` in `talos/ansible/group_vars/all.yml`) is curl-fetched by the `on-activate` hook into `.flox/cache/bin`, not from the catalog. On any tool upgrade, migrate every consumer together so pins keep parity across the Flox manifest, `talos/ansible/group_vars/all.yml`, Terraform/Ansible version constraints, GitHub workflows, Dockerfiles, and Flux manifests.
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

CI mirrors these gates per path (Go workflows, `flux-*-validate.yaml`, `lint-shell-ansible.yaml`, push/release/image-update flows use deny-all `permissions: {}` with per-job minimums, concurrency groups, and path filters). Ansible changes require the `--check --diff` dry run plus `talosctl validate` and FQCN lint (syntax + lint in CI, `--check --diff` stays local — day-2 needs a live `pass-cli` session). Never run `kubectl` or `helm` against a cluster directly; only the Terraform-bootstrapped Flux Operator mutates cluster state.

## Patterns

- **Declare, don't imperatively apply.** Every cluster mutation ships as a manifest so Flux's update monitoring and auto-PR automation can see it. If you catch yourself reaching for an imperative command, convert it into a declarative manifest (or a template that renders one) while keeping update automation intact.
- **Talos renders, Flux delivers.** Talos produces a bare, telemetry-free machine; Flux installs CNI, DNS, and all workloads. Keep that handoff clean: machine config never carries workloads, Flux never carries machine config.
- **Talos conformance.** Volume `maxSize` arithmetic (negative = disk-minus-N) must fit the disk with the numbers recorded in the node header. Machine config carries at least two NTP servers. Telemetry is claimed only by absence-of-config, verified with `grep -rni 'telemetry\|siderolink\|TelemetryConfig' talos/clusters/` returning nothing — never claim a `TelemetryConfig` kind. Schematic IDs are gated on the 64-hex shape in both the render and upgrade planes. Alpha Talos is allowed ONLY when a system extension requires it (`nfs-server` needs >=1.15 — the `talos_version` pin must carry a why-alpha comment and RUNBOOK install/upgrade examples must track the same line). RegistryAuth rotation, PAT 1y max-lifetime, `skip_health` break-glass scope, and NFS sizing basis live as headers beside the code pointing at `talos/ansible/RUNBOOK.md` — never duplicated here. NFS exports accept the LAN CIDR; access limiting is the VLAN's responsibility — stated beside the exports. Disk encryption: homelab runs unencrypted (accepted); real production REQUIRES `SystemDiskEncryption` (TPM2/KMS or an ESO-held key) plus a custody/recovery doc.
- **Base holds the shape, overlays hold the difference.** Whenever dev and prd diverge, put a placeholder in `base` (`__PROTON_PASS_BASE__`, `__WILDCARD_TLS_SECRET__`, `__SERVICE_HOST__`, `__BASE_DOMAIN__`, `__ACME_EMAIL__`) and an explicit replacement in each environment overlay. Never fork a whole file per environment.
- **Pin parity on upgrade.** Flox manifest, `group_vars/all.yml`, Terraform/Ansible constraints, workflow `uses:` SHAs, Dockerfile `FROM` pins (Go toolchain in `.flox/env/manifest.toml`, each `Dockerfile`/`go.mod`), and Flux image/chart refs all move together. A half-upgraded pin is a bug.
- **Images and charts update themselves.** Every OCI image and Helm chart gets an image policy (`flux/{infra,apps}/update-policies/<name>.yaml`: `ImageRepository` 12h + semver `ImagePolicy`) plus the `# {"$imagepolicy":"area:name:tag"}` marker inline on each version field (header-comment markers are dead — automation only rewrites inline markers), driven by the `flux/fleet/clusters/update/` `ResourceSet` + `ImageUpdateAutomation` (30m, Setters push `image-updates-*` branches). Deliberate no-policy cases: first-party `helm-*` charts (`helm-rclone`, `helm-kubevirt`, `helm-multus`, `helm-gateway-api`, `helm-cosi`, `helm-otel-monitoring-crds`) bump atomically by hand (Chart.yaml `version` + wrapper `ref.tag` in one PR, no ImagePolicy/marker — Helm OCIRepositories are untracked); netbird (shared terraform root, no policy); vpa semi-manual; mautrix-discord pinned, no policy; win11-vm staged ISO (non-semver `24H2` — manual-bump comment-only, never a live semver range). Apps policies for element-web + tuwunel have no matching component (bundled inside the matrix app). Human merges the resulting PR after review — automation proposes, never merges.
- **First-party charts for chartless upstreams.** Upstream CRDs with no Helm chart never vendor YAML into `flux/` — they ship as `projects/helm-<name>/` (Chart.yaml `version` = upstream release, `ci/fetch.sh` fetches upstream at CI publish time, no CRDs committed, two workflows: daily check-PR + on-push fetch-publish OCI; fetch-time helm-* charts publish purely from the parsed Chart version on push (no git tags; re-publish overwrites the same OCI tag)). Consume in Flux via `OCIRepository` + `HelmRelease` with CRD `CreateReplace` on install+upgrade plus prune:false-equivalent safety, no ImagePolicy/marker (atomic hand-bump like `helm-rclone`).
- **Upgrade lifecycle (check versions -> check changelog -> bump -> migrate/verify).** Check the available version (policy `range:` floor plus upstream tags), read the changelog first (see Changelog-first under Reference), bump the pin, then migrate values/CRDs and verify with the gates. Dev soaks first; promote dev->stable/prd only after dev is green. Chart<->app and coupled pairs move atomically — the pair list and per-type mechanics live in each `flux/{infra,apps}/update-policies/<name>.yaml` header and the component docs, not here.
- **Platform / supply chain.** Cosign legs pin the binary explicitly (`cosign-release`, matching the cosign pin in `.flox/env/manifest.toml`) with `# match .flox` parity comments on setup lines; always quote `"$DIGEST_URL"`. No dependabot/renovate — SHA pins and Flox pins refresh by hand; no config files.
- **Helm OCI-first; official preferred, chartproxy for classic-only, first-party for chartless.** New charts use `OCIRepository` + `chartRef` (interval 1h); no classic `HelmRepository` is allowed. Prefer the upstream official OCI chart; if official publishes classic-only, use the chartproxy.container-registry.com proxy (`oci://chartproxy.container-registry.com/<host>[/subpath]/<chart>`, local docs `/tmp/home-ops-docs/helm-charts-oci-proxy`); if upstream ships no chart at all, use a first-party `projects/helm-<name>/` chart per the rule above — never vendor upstream YAML into `flux/`. No third-party/community mirrors. Chartproxy mappings carry a header comment stating the upstream classic source they proxy.
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
- **Secrets via ESO Proton Pass webhook.** `ExternalSecret` → cluster `pass://<cluster>/<namespace>/<field>` through the shared `ClusterSecretStore` (refresh 1h). Talos-level secrets use `pass://<cluster>/talos/<field>`.
- **Credential lifecycle (no non-expiring credentials without a runbook).** Every credential ships with its bootstrap obtain + renew/rotate procedure documented beside its code: Talos/Ansible creds (NetBird setup key, Talos PKI) carry renew/rotate tasks next to bootstrap (`talos/ansible/RUNBOOK.md`); k8s/Flux rotating creds via ESO + Reloader opt-in `reloader.stakater.com/auto: "true"` (see `flux/infra/components/reloader`); ESO's own bootstrap cred renews via Talos Ansible. A non-expiring credential is forbidden without a dated rotation tracker + owner/date + age alert.
- **Encryption boundary.** Service-domain cleartext is MANDATORY — CNI/mesh owns pod/node/cluster wire encryption (Cilium WireGuard: `encryption.type: wireguard`, node encryption off on the trusted LAN). TLS is mandatory ONLY at the edge: Gateway API `main` Gateway + cert-manager `letsencrypt` ClusterIssuer (Cloudflare DNS-01) with one wildcard `Certificate` per namespace; overlays swap the placeholder secret name and ACME email. Never add TLS to in-cluster service traffic. Dev uses LE production (not staging — staging certs are untrusted and would break the NetBird/OIDC trust chain; see cert-manager README for the staging escape hatch). Wildcard scope is one DNS label only — nested hosts (`a.b.base`) need a second `*.b.base` cert (coder pattern).
- **Single-node is production-ready if it is scale-ready.** One node + one control-plane is accepted with an accepted-RTO note, day-2 health gates, a single-speaker VIP note, and a tested scale-to-3-CP procedure (details in `talos/ansible/RUNBOOK.md` §4). Keeper/CNPG affinity stays `preferred` (or replica=1) until multi-node — never `required` on one node.
- **Data plane choices (do not substitute without permission):** S3 via SeaweedFS with a COSI `Claim` + `Access` per bucket; NetBird consumers pass an explicit per-env `service_lb_ip` (Cilium LB VIP, no cross-env default) matching the in-mesh backend listener; KubeVirt/Multus bumps move the first-party chart (Chart.yaml + wrapper `ref.tag`) with the local narrowing owned by `projects/helm-<name>/ci/fetch.sh`; bare-minimum backup leg = periodic backup to in-cluster SeaweedFS S3, then `helm-rclone` sync-mirror offsite to Proton Drive at `home-ops/backups/{cluster}/{ns}/{bucket}`; Postgres via CNPG with S3 backup; Redis API via Dragonfly Operator; Mongo API via FerretDB backed by CNPG; ClickHouse via the Altinity operator with S3 backup; observability via ClickStack + ClickHouse + OpenTelemetry; secret auto-reload via Reloader (`reloader.stakater.com/auto: "true"` opt-in, annotations strategy, `watchGlobally: true`); notifications via `apprise-go-api` to Matrix (tuwunel) provisioned by Tofu Controller Terraform in a dedicated read-only room; SSO via Zitadel provisioned by Tofu Controller Terraform (admin gets access to everything, dedicated groups/roles/OIDC per app; add `oauth2-proxy` + `ExternalAuth` only for apps without native SSO, with a distinct `cookie-name` per instance sharing a base-domain scope); clusters stay private by default and reach the internet through Netbird provisioned by Tofu Controller Terraform.
- **File-based stateful storage at 1 non-HA replica is accepted while under evaluation** (e.g. tuwunel pre-postgres). HA/backup/restore/replication is the DB/app operator's job on `local-path` for performance; the bare minimum from day one is a periodic backup to in-cluster SeaweedFS S3 plus a `helm-rclone` sync-mirror offsite to Proton Drive at `home-ops/backups/{cluster}/{ns}/{bucket}`.
- **Apps conformance.** No unpinned images: every image (incl. bootstrap tooling like `python:digest`) pins by digest. Headlamp bumps are manual-only and coupled (`ref.tag` + `image.tag` + plugin pins in one PR — Helm OCIRepositories are untracked). `element-web` prd-hosts base is the one opaque-`config.json` exception (whole-string, not per-key patches). Coder `/env` edits append LAST and keep stable indices (load-bearing order). Localhost `router-reservation` values mirror the overlay value with a comment. Staged kill-switch flips (`CODER_DISABLE_PASSWORD_AUTH`, `TUWUNEL_GRANT_ADMIN_TO_FIRST_USER`) ship safe bootstrap defaults with flip checklists in the READMEs; trim follow-ups (e.g. RBAC) carry an owner/date note.
- **Go services:** `cmd/` + `internal/` layout, `ARG VERSION` in the Dockerfile, distroless `base-debian13` nonroot, `CGO_ENABLED=0`; tags are `<svc>-v*`, never `:latest`. Chart + Go fail-fast rules: no `CHANGEME`/`REPLACE-ME`/`EXAMPLE` defaults (empty default + placeholder rejection); image `tag@digest` required (`:latest` rejected, version-without-digest fails); chart `operation` defaults to `sync` (sync-mirror; source deletion is the storage backend or db/app operator's job); env numerics/bools fail fast on non-empty unparseable values; accepted-but-unenforced knobs are marked as no-ops in code + README (e.g. `PLUGIN_PATHS`); sender timeouts are observable (`send_timeouts_total` + in-flight gauge, 64 cap, over-cap 503); outbound webhooks strip W3C trace headers while in-cluster clients inject them (deliberate contrast); readiness probes never `MkdirAll` (match the request path); list-form tags validate per token; provider updates/deletes iterate all refs per key with split-brain errors; `AUTO_CREATE` is strictly boolean; NetBird trace injection is documented as embedded-trust.

- **Observability (ClickHouse-native).** ClickHouse is the only store, HyperDX the only UI — no Prometheus/Grafana. Rules live in `flux/OBSERVABILITY.md`: infra OTLP endpoint convention, always-on monitors + `infra-crds` gate, fleet ordering gates, `otel-scrape:true` namespaces, `/healthz` + `/readyz` probes (fixed-path exceptions carry why-comments), VPA `Off` with HPA / `Initial` for singletons, HyperDX webhook → `apprise-go-api` → Matrix alerts.

## Anti-patterns

- Do NOT run `kubectl apply/edit/delete`, `helm install/upgrade`, or any direct cluster mutation. All runtime state flows from the Terraform-bootstrapped Flux Operator; bypassing it causes drift and breaks update automation.
- Do NOT manage Talos with anything but the pinned `talosctl` (`talos_version` in `talos/ansible/group_vars/all.yml`, fetched via the Flox `on-activate` hook). No third-party provisioners, no hand-built machine configs.
- Do NOT put bootstrap manifests, CNI, CoreDNS, or discovery config into Talos machine patches (see Talos renders, Flux delivers).
- Do NOT commit secrets, kubeconfigs, talosconfigs, `secrets.bundle.yml`, or `proton-pass-pat` files. They are gitignored build artifacts; only `*.template` files and double-brace placeholders are committed.
- Do NOT leave telemetry enabled on any deployment (Talos machine config, controllers, apps). If a chart defaults it on, explicitly disable it.
- Do NOT add a classic `HelmRepository` for a new chart, vendor upstream YAML into `flux/`, use `:latest` tags, or consume an image without an `$imagepolicy` marker and update policy (first-party `helm-*` charts are the sanctioned no-marker exception).
- Do NOT skip the changelog-first rule (see Reference) or half-bump a chart<->app pair or coupled move (see Upgrade lifecycle).
- Do NOT let the flux-operator-ui policy touch the fleet sync path (UI stays `serverOnly`, `installCRDs: false`; bootstrap `flux-operator` owns CRDs and sync), and do NOT prune CRDs on upgrade — keep the explicit CRD policy (Helm `CreateReplace` on every charted bundle, cert-manager `keep:true`, fleet prune:false `infra-crds` delivery). Never add a CRD file to a controllers Kustomization under the shared CRD-free contract (cosi, gateway-api, otel-operator) — CRDs render only through the fleet prune:false `infra-crds` Kustomization.
- Do NOT use Ingress, FQDNs for in-cluster traffic, or Gateway hostnames for internal-only endpoints. Do NOT add TLS to in-cluster service-domain traffic — Cilium WireGuard encrypts the wire; TLS terminates only at the Gateway.
- Do NOT add a chart OCIRepository without cosign verification wired through the fleet tenant allowlist (`flux/fleet/tenants/policies.yaml` + `verify` in `tenants/{infra,apps}.yaml`) or an explicit `No verify:` comment stating why (first-party unsigned chart, chartproxy-translated classic, no pinned upstream identity).
- Do NOT fork per-environment manifests instead of base-placeholder + overlay-override, and do NOT scale a singleton (singleton HPAs stay capped 1/1 with a header why-comment).
- Do NOT use `TODO`/`FIXME` in first-party code, push unpinned `uses:` refs in workflows, or run `flux/scripts/validate.sh` against the wrong scope and call it coverage.
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
- Before pushing, run the gates for every scope you touched (Go vet/build/test + lint + vuln check; `helm lint`/`template` + `ci/verify.sh`; `flux/scripts/validate.sh -d flux/{apps,infra,fleet}` per scope; `tofu init -backend=false/validate/test`; `cosign sign` for published images; Ansible `--check --diff` + `talosctl validate -c <node file> -m metal` + FQCN lint; `shellcheck` on touched `*.sh`). Report gate results in the PR.
- Workflows stay hardened: SHA-pinned `uses:`, deny-all `permissions: {}` default with per-job minimums (`contents: read` plus `packages: write` / `id-token: write` only where push/sign needs them), concurrency groups, path-gated triggers.
- Open a PR for review; do not push to a protected branch.
