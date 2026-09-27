# local-path-provisioner

Local Path Provisioner chart == app v0.0.37 (`oci://ghcr.io/rancher/local-path-provisioner/charts/local-path-provisioner`) with two `StorageClass`es aligned to the Talos `UserVolumeConfig` mounts (`/var/mnt/<name>`): `local-ssd-nvme` -> `/var/mnt/nvme-data` (default) and `local-ssd-sata` -> `/var/mnt/sata-data` (non-default). Both `WaitForFirstConsumer`, `Delete`. Exactly one default; workloads needing SATA name `local-ssd-sata` explicitly.

local-path volumes are node-local hostPath bind-mounts (no snapshots, no migration) — every data service backs up at the operator/app level to object storage.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | 1 (helper + provisioner singletons) |
| `prd` | 1 (helper + provisioner singletons) |

Never scale the provisioner.

## Updates

`update-policies/local-path-provisioner.yaml` (>=0.0.37, marker `infra:local-path-provisioner:tag`). Blast radius is high (default `StorageClass`) — merge off-peak. Changelog: https://github.com/rancher/local-path-provisioner/releases.
