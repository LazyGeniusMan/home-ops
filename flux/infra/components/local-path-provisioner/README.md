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

- No reporting knobs upstream. The chart exposes no metrics endpoint; watch
  volumes via kubelet/kube-state-metrics once monitoring lands.
- Chart bumps: `update-policies/local-path-provisioner.yaml` → PR automation.
