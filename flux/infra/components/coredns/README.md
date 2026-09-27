# CoreDNS

CoreDNS app 1.14.7 (chart 1.47.1, `oci://ghcr.io/coredns/charts/coredns`; app pin rides `values.image.tag`), answering `home-ops.yansyah.my.id`, `*.home-ops.yansyah.my.id`, and nested `*.*.home-ops.yansyah.my.id` with the Gateway LB VIP and forwarding everything else upstream. Deployed at bootstrap alongside Cilium so cluster DNS is live before any workload lands.

`configs/base/corefile.yaml` (ConfigMap `coredns`, key `Corefile`) carries the LAN zone block: `template` stanzas first (apex, single-level wildcard, nested wildcard -> LB VIP, each with `fallthrough`), then `hosts` (empty by default, `fallthrough`), then `forward . /etc/resolv.conf`. The chart ConfigMap is skipped (`deployment.skipConfig: true`); the standalone ConfigMap mounts at `/etc/coredns`. Validate: `dig @<coredns-svc-ip>` apex + both wildcards -> LB VIP, everything else forwards upstream.

## kube-dns Service IP

`controllers/base/kube-dns.yaml` pins `clusterIP: 10.96.0.10` (10th address of Talos default service subnet `10.96.0.0/12`).

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (HPA 1-2) | LAN zone `home-ops-dev.yansyah.my.id` -> `.249` |
| `prd` | 2 (HPA 2-4) | LAN zone `home-ops.yansyah.my.id` -> `.199` |

Base carries `__BASE_DOMAIN__` / `__LB_VIP__` placeholders; overlays replace the LAN zone block.

## Updates

`update-policies/coredns.yaml` (chart `ref.tag` marker + app image marker `infra:coredns-app:tag`; node-cache image `registry.k8s.io/dns/k8s-dns-node-cache:1.26.8` in `configs/base/node-local-dns.yaml`, marker `infra:node-cache:tag`, policy `>=1.26.0`). Changelogs: https://github.com/coredns/coredns/releases, https://github.com/coredns/helm/releases.
