# apprise-go-api

Shared platform webhook sink (`projects/apprise-go-api`): the single target
for notification-controller Alert/Provider posts, forwarding to
caller-supplied apprise URLs. Credential-free by design — like
cert-manager/external-secrets plumbing, it carries ZERO service
credentials: no stateless-URLs fallback env, no secret key, no
token-embedded URLs. Callers pass full per-request apprise URLs in each
`POST /notify` `urls`; the app
boots clean with an empty fallback (missing `urls` -> HTTP 204, no crash).

Internal ClusterIP only — no Gateway/HTTPRoute, no ServiceMonitor
(`monitoring.coreos.com` CRDs not present).

## Layout

```text
flux/infra/components/apprise-go-api/
├── README.md
├── controllers/{base,dev,prd}/  # Deployment+Service, HPA, VPA
└── configs/{base,dev,prd}/      # empty on purpose (no consumer wiring ships here)
```

`configs/` stays empty (`resources: []`, inheriting-base env overlays)
because consumers own their credentials: the matrix tenant keeps its
`apprise-stateless-urls` ExternalSecret fallback and Provider/Alert wiring,
pointing at the in-cluster Service DNS below.

## Consumer contract

In-cluster Service DNS (tenant == namespace == `apprise-go-api`):

```text
http://apprise-go-api.apprise-go-api.svc:80/notify
```

## Credentials

None ship here (no Secret, no ExternalSecret, no Certificate). Seed nothing
for this component.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | HPA 1–2 | `controllers/dev` pins HPA min 1 / max 2 |
| `prd` | HPA 2–4 | `controllers/prd` pins HPA min 2 / max 4 (base values) |

Base `replicas: 1` is the create-time seed; the HPA owns the runtime count
(HPA `minReplicas` enforces the floor).

## Monitoring / updates

- `ServiceMonitor` disabled until `monitoring.coreos.com` CRDs land.
- VPA is recommender-only (`updateMode: Off` — the HPA scales the same
  CPU/memory metrics).
- Image bumps: `update-policies/apprise-go-api.yaml` → PR automation
  (`$imagepolicy` marker `infra:apprise-go-api:tag`).

## Upgrade runbook

- Version source: the image tag in
  `controllers/base/apprise-go-api.yaml` (first-party, tags
  `apprise-go-api-v*` published by
  `.github/workflows/apprise-go-api.yml`).
- Changelog: in-repo (`projects/apprise-go-api` — no external feed).
- Bump: let the ImagePolicy PR land (marker
  `infra:apprise-go-api:tag`, `update-policies/apprise-go-api.yaml`).
  Risk is low (stateless, credential-free sink).
- Verify: Deployment `Ready`, then `POST /notify` with a test apprise
  URL returns 2xx and the matrix consumer wiring still posts.
