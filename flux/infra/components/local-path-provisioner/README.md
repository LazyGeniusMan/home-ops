# local-path-provisioner

Local Path Provisioner chart == app v0.0.37
(`oci://ghcr.io/rancher/local-path-provisioner/charts/local-path-provisioner`)
with two `StorageClass`es aligned to the Talos `UserVolumeConfig` mounts
(`/var/mnt/<name>`):

- `local-ssd-nvme` -> `/var/mnt/nvme-data`, default (`is-default-class`).
- `local-ssd-sata` -> `/var/mnt/sata-data`, non-default.

Exactly one default; workloads needing SATA name `local-ssd-sata`
explicitly. Both `WaitForFirstConsumer`, `Delete`.

## Backups

local-path volumes are node-local hostPath bind-mounts: no snapshots, no
migration, data dies with the node/disk. Every data service backs up at
the operator/app level (database native backup to object storage); the
StorageClasses alone are not a backup story.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | 1 (helper + provisioner singletons) |
| `prd` | 1 (helper + provisioner singletons) |

Never scale the provisioner.

## Telemetry / monitoring / updates

No reporting knobs upstream; the chart exposes no metrics endpoint. Bumps:
`update-policies/local-path-provisioner.yaml` (>=0.0.37, marker
`infra:local-path-provisioner:tag`) -> PR automation. Blast radius is high
(default `StorageClass`) -- merge off-peak.
Changelog: https://github.com/rancher/local-path-provisioner/releases.
