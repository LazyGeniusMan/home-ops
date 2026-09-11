# CoreDNS (§8.3)

CoreDNS v1.14.7, answering `home-ops.yansyah.my.id`, `*.home-ops.yansyah.my.id`,
and nested `*.*.home-ops.yansyah.my.id` with `192.168.1.199` (the Gateway LB VIP)
and forwarding everything else upstream.

## Chart source

OCI-first was attempted per house policy but is unavailable for CoreDNS:

1. D2 docs (`flux-d2-docs/d2-infra`) prescribe no CoreDNS source (no matches).
2. `oci://ghcr.io/coredns/charts/coredns:1.14.7` — not found.
3. `ghcr.io/controlplaneio-fluxcd/charts/coredns` — does not exist (denied).

So the component uses the classic `HelmRepository` (`coredns/coredns`,
https://coredns.github.io/helm — the upstream-published chart repo). Chart
1.47.0 carries app 1.14.6, so the app image is pinned explicitly to `1.14.7`
(verified live on Docker Hub); the update policy tracks the chart at
`>=1.47.0` while the app pin rides in `values.image.tag`.

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
overlay (`dev`/`stg`/`prd`) replaces the LAN zone block with its domain + VIP
(env-independent `cluster.local` / `.` blocks ride along unchanged).

## kube-dns Service IP assumption (break-glass)

`controllers/base/kube-dns.yaml` pins `clusterIP: 10.96.0.10` — the 10th
address of the Talos **default** service subnet `10.96.0.0/12` (K8s
convention: API at `.1`, kubelets default `--cluster-dns` to `.10`; nothing
in `talos/` overrides `serviceSubnet`, so the default holds and kubelets need
no per-node `clusterDNS`). This is a validated assumption, not a guess — but
it is silent-break: if a `serviceSubnet` override is ever added to
`talos/`, the kube-dns `clusterIP` MUST move to the 10th address of the new
range or every kubelet will point at a non-existent DNS IP. Making it
configurable was considered and rejected: nothing consumes a variable here
(the IP is a literal inside a static Service manifest), so a comment + this
note is the honest record.

## Telemetry-off / monitoring / updates

- No reporting knobs in chart values (verified: no
  telemetry/usageReporting/phoneHome/analytics keys); `prometheus.service`
  only adds scrape annotations. `ServiceMonitor` disabled until CRDs land.
- Chart bumps via `update-policies/coredns.yaml` → PR automation.
