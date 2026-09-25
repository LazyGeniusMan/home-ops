# apprise-go-api

Stateless notification sink (`projects/apprise-go-api`): forwards
notification-controller Alert/Provider posts to caller-supplied apprise URLs.
Credential-free -- no Secret, ExternalSecret, or Certificate ships here.
Callers pass full apprise URLs per `POST /notify` request (missing `urls`
-> HTTP 204). Internal ClusterIP only: no Gateway/HTTPRoute, no
ServiceMonitor.

## Layout

```text
flux/infra/components/apprise-go-api/
├── controllers/{base,dev,prd}/  # Deployment+Service, HPA, VPA
└── configs/{base,dev,prd}/      # empty (no consumer wiring ships here)
```

Consumers (matrix tenant) own the `apprise-stateless-urls` ExternalSecret
fallback and Provider/Alert wiring, pointing at:

```text
http://apprise-go-api.apprise-go-api.svc:80/notify
```

## Versions / updates

Image `ghcr.io/lazygeniusman/home-ops/projects/apprise-go-api:dev`
(marker `infra:apprise-go-api:tag`,
`update-policies/apprise-go-api.yaml` >=0.1.0).

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | HPA 1-2 |
| `prd` | HPA 2-4 |

Base `replicas: 1` is the create-time seed; HPA owns the runtime count. VPA
recommender-only (`updateMode: Off`) alongside the HPA.
