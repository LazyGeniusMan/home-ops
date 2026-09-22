# apprise-go-api

Stateless-only notification webhook sink (`projects/apprise-go-api`):
the single internal target for notification-controller Alert/Provider
posts, forwarding to Matrix. No Gateway/HTTPRoute (internal ClusterIP
only — hubble-ui precedent has HTTPRoutes for user browsers; this service
has no browser surface), no ServiceMonitor (monitoring.coreos.com CRDs
not present — same §9 deviation).

## Layout (environment-direct, apps area)

`base/` holds every manifest (`apprise-go-api.yaml` Deployment+Service,
`apprise-go-api-hpa.yaml`, `apprise-go-api-vpa.yaml`); env overlays
`{dev,prd}/` patch only HPA replica bounds via `resources: [../base]` +
RFC-6902 patches. Tenant wiring (`flux/fleet/tenants/apps.yaml`) and the
workflow matrix (`.github/workflows/flux-apps-push.yaml`) are shared
onboarding — NOT done here (see Follow-ups).

## Image policy

`ghcr.io/lazygeniusman/home-ops/projects/apprise-go-api` — branch leg
`dev` + `dev-<sha>`, tag leg `stable` + `<semver>` (see
`.github/workflows/apprise-go-api.yml`). Marker in base:

```text
image: ghcr.io/lazygeniusman/home-ops/projects/apprise-go-api:dev # {"$imagepolicy": "apps:apprise-go-api:tag"}
```

owned by `flux/apps/update-policies/apprise-go-api.yaml` once the shared
onboarding lands (follow the §7 update-policy contract:
ImageRepository + ImagePolicy + this marker).

## Env table

| Variable | Value | Meaning |
|---|---|---|
| `HTTP_PORT` | `8080` | Listen port (`containerPort: 8080`) |
| `LOG_LEVEL` | `info` | `debug\|info\|warn\|warning\|error` |
| `WORKER_COUNT` | `0` | Concurrent notify fan-out bound (`0` = `GOMAXPROCS`) |
| `TIMEOUT` | `30` | Seconds bounding a single notify call |
| `APPRISE_STATEFUL_MODE` | `disabled` | Mandatory — any other value fails startup |
| `APPRISE_STATELESS_STORAGE` | `no` | Mandatory — persistence is unsupported by design |
| `APPRISE_ALLOW_SERVICES` | `matrix` | Matrix-only posture (exclusive; see below) |
| `APPRISE_DENY_SERVICES` | unset | Empty while the allowlist is exclusive |
| `APPRISE_ATTACH_ALLOW_URL` | `*` | SSRF allowlist (empty = `*`) |
| `APPRISE_ATTACH_REJECT_URL` | `127.0.* localhost* internal` | SSRF denylist + internal-token guard |
| `SECRET_KEY` / `SECRET_KEY_FILE` | unset | Set only if webhook callback auth is needed |
| `APPRISE_ATTACH_DIR` | unset | Default `os.TempDir()` — request-scoped staging only |

Full knob reference: `projects/apprise-go-api/README.md`
(Configuration). `APPRISE_STATELESS_URLS` intentionally unset (see Matrix
strategy). Debug/log-verbosity knobs (`DEBUG`, `TZ`, `APPRISE_BASE_URL`,
`ALLOWED_HOSTS`, attachment limits, webhook remap depth) ride defaults.

## Matrix strategy (decided: per-request urls, preferred multi-room)

Default: **per-request `urls` (preferred)**. Each notify carries its own
Matrix target URLs (`matrix://` / `matrixs://`, one entry per room), so
multi-room fan-out needs no redeploy and no stored credentials:

```text
urls=matrixs://user:pass@matrix.example.com/%23room:example.com
```

- `APPRISE_STATELESS_URLS` fallback: available when a request carries no
  `urls`, but NOT set here — a shared fallback pins one room/credential
  set and conflicts with per-room fan-out.
- ESO secret: NOT provisioned here. If Matrix credentials must leave
  request bodies later, mount them via an ExternalSecret into
  `APPRISE_STATELESS_URLS` (single-room fallback) or into a per-env env
  patch — a follow-up, not this change.
- Enforcement is allow/deny on the URL **scheme prefix** (see
  `internal/notify/sender.go` `serviceAllowed`: entries reduce to a
  leading `[a-z][a-z0-9]+` prefix; non-empty allow wins over deny):
  `APPRISE_ALLOW_SERVICES=matrix` prefix-matches `matrix://` +
  `matrixs://` and rejects every other service. `APPRISE_DENY_SERVICES`
  stays unset while the exclusive allowlist holds.
- SSRF posture for remote attachment fetch (deny-first, allow-second):
  `APPRISE_ATTACH_ALLOW_URL=*` with
  `APPRISE_ATTACH_REJECT_URL=127.0.* localhost* internal` — the
  `internal` token (opt-in, never default) resolves each attachment host
  via DNS and blocks loopback, private, link-local, reserved,
  unspecified, multicast, and CGN (`100.64.0.0/10`) addresses, including
  via DNS or alternate IP encodings; unresolvable hosts block too.

## Consumers

In-namespace and cross-namespace callers post to the ClusterIP Service:

```text
http://apprise-go-api.<ns>.svc:80/notify
```

(`<ns>` = the tenant namespace the fleet `apps` Kustomization targets,
e.g. `apprise-go-api` once onboarded.) notification-controller
Provider/Alert wiring is a follow-up (zero Provider/Alert exist today).

## Probes / metrics

- Liveness + readiness: `GET /status` (always JSON; non-GET → `405`).
- Future monitor: `GET /metrics` exposes Prometheus exposition
  (`apprise_go_api_up`, `apprise_go_api_build_info`,
  `apprise_go_api_attach_writable`, `apprise_go_api_supported_services`)
  — wire a ServiceMonitor once the monitoring stack lands.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | HPA min 1 / max 2 | HPA bounds only |
| `prd` | HPA min 2 / max 4 | HPA bounds only |

Static `replicas: 1` in base is the create-time seed; HPA owns runtime
count. Requests (`cpu: 50m`, `memory: 64Mi`, limits `500m/256Mi`) satisfy
the HPA CPU denominator. Distroless nonroot assumptions hold: no
volumes, `securityContext.allowPrivilegeEscalation: false` +
`capabilities.drop: [ALL]` (hubble-ui shape).

## Follow-ups (out of scope here)

- Tenant input in `flux/fleet/tenants/apps.yaml` + component matrix entry
  in `.github/workflows/flux-apps-push.yaml` (shared onboarding).
- `flux/apps/update-policies/apprise-go-api.yaml` (ImageRepository +
  ImagePolicy for the `$imagepolicy` marker above).
- notification-controller Provider/Alert pointing at the consumer URL.
- ServiceMonitor for `GET /metrics` once monitoring CRDs land.
- `SECRET_KEY(_FILE)` only if webhook callback auth is needed.
