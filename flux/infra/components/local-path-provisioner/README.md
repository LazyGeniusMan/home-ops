# local-path-provisioner (§9.5)

Local Path Provisioner v0.0.37 with two `StorageClass`es aligned to the
Talos `UserVolumeConfig` mounts (`/var/mnt/<name>`):

- `local-ssd-nvme` → `/var/mnt/nvme-data`, **default** (`is-default-class`).
- `local-ssd-sata` → `/var/mnt/sata-data`, non-default.

Default-class discipline: exactly one default; workloads needing SATA must
name `local-ssd-sata` explicitly. Both `WaitForFirstConsumer`, `Delete`.

## Backups (not CSI snapshots alone)

local-path volumes are node-local hostPath bind-mounts: no snapshots, no
migration, data dies with the node/disk. Every data service (§10) must back
up at the operator/app level (e.g. database native backup to object
storage); the StorageClasses alone are not a backup story.

## Telemetry-off / monitoring / updates

- No reporting knobs upstream. The chart exposes no metrics endpoint.
- Chart bumps: `update-policies/local-path-provisioner.yaml` → PR automation.

## Upgrade runbook

- Version source: the `OCIRepository` tag in
  `controllers/base/local-path-provisioner.yaml` (chart == app v0.0.37).
- Changelog: https://github.com/rancher/local-path-provisioner/releases.
- Bump: let the ImagePolicy PR land (marker
  `infra:local-path-provisioner:tag`,
  `update-policies/local-path-provisioner.yaml`). Blast radius is high
  (default `StorageClass`) but the change surface is small — still merge
  off-peak.
- Verify: both Deployments `Ready`, the two `StorageClass`es still list
  (`kubectl get sc`), a test PVC provisions, and existing volumes
  re-mount on pod restart.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (helper + provisioner singletons) | none — inherits `../base` unchanged |
| `prd` | 1 (helper + provisioner singletons) | none — inherits `../base` unchanged |

Node-local provisioner: exactly 1 of each Deployment in every env by
design — never scale it.

Upstream reference (read-only): `/tmp/home-ops-docs/local-path-provisioner-docs`.
