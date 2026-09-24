# CoreDNS (§8.3)

CoreDNS v1.14.7, answering `home-ops.yansyah.my.id`, `*.home-ops.yansyah.my.id`,
and nested `*.*.home-ops.yansyah.my.id` with `192.168.1.199` (the Gateway LB VIP)
and forwarding everything else upstream.

## Chart source

The component consumes the official upstream OCI chart
`oci://ghcr.io/coredns/charts/coredns` via `OCIRepository` (`coredns-chart`,
interval 1h, same shape as cert-manager/cilium) with the `$imagepolicy`
marker `infra:coredns:tag` on `ref.tag`. Chart 1.47.1 carries app 1.14.6,
so the app image is pinned explicitly to `1.14.7`; the update policy
tracks the chart at `>=1.47.1` while the app pin rides in
`values.image.tag`.

## Corefile chain (template → hosts → forward)

The ConfigMap in `configs/base/corefile.yaml` (name `coredns`, key `Corefile`)
carries the `home-ops.yansyah.my.id:53` server block. Order matters: the
`template` stanzas answer first (apex, single-level wildcard, nested
wildcard → `192.168.1.199`, each with `fallthrough`), then `hosts` (static
entries, empty by default, `fallthrough`), then `forward . /etc/resolv.conf`
for everything else. The chart has no `corefile` values key (Corefile renders
from `servers:`), and a values-embedded block would collide with the chart's
own `<fullname>/Corefile` ConfigMap — so the chart ConfigMap is skipped
(`deployment.skipConfig: true`) and the standalone ConfigMap is what the
chart Deployment mounts at `/etc/coredns`. The `servers:` values only carry
the default `.` zone (errors/health/ready/forward/cache/loop/reload/
loadbalance).

## Validation

```sh
# Apex, single-level wildcard, nested wildcard:
dig @<coredns-svc-ip> home-ops.yansyah.my.id +short
dig @<coredns-svc-ip> app.home-ops.yansyah.my.id +short
dig @<coredns-svc-ip> foo.bar.home-ops.yansyah.my.id +short
# All three must print 192.168.1.199; anything else must forward upstream:
dig @<coredns-svc-ip> example.com +short
```

## Bootstrap / environments

Deployed at bootstrap alongside Cilium (§8.2, initial wave) so cluster DNS is
live before any workload that resolves the LAN names lands. Base carries the
full chain with `__BASE_DOMAIN__` / `__LB_VIP__` placeholders; each env
overlay (`dev`/`prd`) replaces the LAN zone block with its domain + VIP
(env-independent `cluster.local` / `.` blocks ride along unchanged).

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (single-instance) | Corefile LAN zone `home-ops-dev.yansyah.my.id` → `.249` |
| `prd` | 2 recommended (survive a node loss once multi-node) | Corefile LAN zone `home-ops.yansyah.my.id` → `.199` |

Each env pins its replica seed in `controllers/{dev,prd}` (1 dev, 2 prd);
the out-of-band HPA owns the runtime count (base min 2 / max 4,
`controllers/dev` patches 1 / 2).

Upstream reference (read-only): `/tmp/home-ops-docs/coredns-docs`.

## kube-dns Service IP coupling

`controllers/base/kube-dns.yaml` pins `clusterIP: 10.96.0.10` — the 10th
address of the Talos default service subnet `10.96.0.0/12`. If a
`serviceSubnet` override is ever added to `talos/`, the kube-dns
`clusterIP` MUST move to the 10th address of the new range.

## Telemetry-off / monitoring / updates

- No reporting knobs in chart values; `prometheus.service` only adds
  scrape annotations. `ServiceMonitor` disabled until CRDs land.
- Chart bumps via `update-policies/coredns.yaml` → PR automation.

## Upgrade runbook

- Version source: the `ref.tag` pin in `controllers/base/coredns.yaml`
  (OCI chart 1.47.1, app image pinned separately in
  `values.image.tag` to 1.14.7) plus the node-local-dns cache image in
  `configs/base/node-local-dns.yaml`.
- Changelog (app): https://github.com/coredns/coredns/releases.
  Changelog (chart): https://github.com/coredns/helm/releases.
- Bump: let the ImageUpdateAutomation propose the chart `ref.tag` move via
  the `$imagepolicy` marker (`infra:coredns:tag`), move the app image pin in
  the same file, and keep the node-cache marker on the supported line
  (`update-policies/coredns.yaml`).
- Verify: re-run the three `dig` checks in Validation above (apex +
  both wildcards → LB VIP, everything else forwards upstream).
