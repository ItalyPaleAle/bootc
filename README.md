# bootc images for CentOS Stream

Custom images for CentOS that can be used with bootc.

- [`tailscale`](#tailscale-image): Includes Tailscale
- [`base`](#base-image): Includes Tailscale and Cloudflare Tunnel client (cloudflared)
- [`zfs`](#zfs-image): Includes ZFS as a kernel module (built on top of `base`)
<!--
- [`monitoring`](#monitoring-image): Includes Grafana Alloy (built on top of `base`)
- [`monitoring-zfs`](#monitoring-zfs-image): Includes Grafana Alloy (built on top of `zfs`)
-->

These images are built using GitHub Actions every Monday and Friday, from the upstream images of CentOS Stream

Images are published on GitHub Packages and available for linux/amd64 and linux/arm64 (except ZFS).

## `tailscale` image

Includes:

- [Tailscale](https://tailscale.com/)

Image:

```text
ghcr.io/italypaleale/bootc/centos9/tailscale:latest
```

[Containerfile](./el9/tailscale/Containerfile)

## `base` image

Includes:

- [Tailscale](https://tailscale.com/)
- [restic](https://github.com/restic/restic)
- [gotop](https://github.com/xxxserxxx/gotop)
- Utilities: `screen`, `pv`, `sqlite`, `jq`

Image:

```text
ghcr.io/italypaleale/bootc/centos9/base:latest
```

[Containerfile](./el9/base/Containerfile)

## `zfs` image

Includes:

- Everything in the [`base` image](#base-image)
- ZFS as a kernel module
- [Tailscale](https://tailscale.com/)
- [Cloudflare Tunnel client](https://github.com/cloudflare/cloudflared) (cloudflared)

Image:

```text
ghcr.io/italypaleale/bootc/centos9/zfs:latest
```

[Containerfile](./el9/zfs/Containerfile)

<!--
## `monitoring` image

Includes:

- Everything in the [`base` image](#base-image)
- [Grafana Alloy](https://github.com/grafana/alloy)
- [prometheus-podman-exporter](https://github.com/containers/prometheus-podman-exporter)

Image:

```text
ghcr.io/italypaleale/bootc/centos9/monitoring:latest
```

[Containerfile](./el9/monitoring/Containerfile)

## `monitoring-zfs` image

Includes:

- Everything in the [`zfs` image](#zfs-image)
- [Grafana Alloy](https://github.com/grafana/alloy)
- [prometheus-podman-exporter](https://github.com/containers/prometheus-podman-exporter)

Image:

```text
ghcr.io/italypaleale/bootc/centos9/monitoring-zfs:latest
```

[Containerfile](./el9/monitoring/Containerfile)
-->
