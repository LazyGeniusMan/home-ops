# CoreDNS

CoreDNS app 1.14.7 (chart 1.47.1, `oci://ghcr.io/coredns/charts/coredns`;
policy floor `>=1.47.1`, marker `infra:coredns:tag`; app pin rides
`values.image.tag`), answering `home-ops.yansyah.my.id`,
`*.home-ops.yansyah.my.id`, and nested `*.*.home-ops.yansyah.my.id` with the
Gateway LB VIP and forwarding everything else upstream.

## Corefile chain

`configs/base/corefile.yaml` (ConfigMap `coredns`, key `Corefile`) carries
the LAN zone block. Order: `template` stanzas first (apex, single-level
wildcard, nested wildcard -> LB VIP, each with `fallthrough`), then `hosts`
(static entries, empty by default, `fallthrough`), then
`forward . /etc/resolv.conf`. The chart ConfigMap is skipped
(`deployment.skipConfig: true`); the standalone ConfigMap is what the chart
Deployment mounts at `/etc/coredns`. `servers:` values carry only the
default `.` zone.

Validate: `dig @<coredns-svc-ip>` apex + both wildcards -> LB VIP,
everything else forwards upstream.

## Bootstrap / environments

Deployed at bootstrap alongside Cilium so cluster DNS is live before any
workload lands. Base carries `__BASE_DOMAIN__` / `__LB_VIP__` placeholders;
each overlay replaces the LAN zone block with its domain + VIP.

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 | Corefile LAN zone `home-ops-dev.yansyah.my.id` -> `.249` |
| `prd` | 2 | Corefile LAN zone `home-ops.yansyah.my.id` -> `.199` |

Dev seeds 1 replica, prd seeds 2. The out-of-band HPA owns the runtime
count (dev min 1 / max 2, prd min 2 / max 4).

## kube-dns Service IP

`controllers/base/kube-dns.yaml` pins `clusterIP: 10.96.0.10` (10th address
of Talos default service subnet `10.96.0.0/12`). If a `serviceSubnet`
override is added to `talos/`, the kube-dns `clusterIP` moves to the 10th
address of the new range.

## Telemetry / monitoring / updates

No reporting knobs in chart values (`prometheus.service` only adds scrape
annotations). `ServiceMonitor` off. Chart bumps via
`update-policies/coredns.yaml` -> PR automation (chart `ref.tag` marker +
app image pin in the same file; node-cache image
`registry.k8s.io/dns/k8s-dns-node-cache:1.26.8` in
`configs/base/node-local-dns.yaml`, marker `infra:node-cache:tag`,
policy `>=1.26.0`).
Changelogs: https://github.com/coredns/coredns/releases,
https://github.com/coredns/helm/releases.
