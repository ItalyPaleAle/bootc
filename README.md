# bootc images for Enterprise Linux

Custom images for Enterprise Linux that can be used with bootc. Currently supports:

- **CentOS Stream 10 and 9**: Published on GitHub Packages
- **Alma Linux 10 and 9**: Published on GitHub Packages
- **Red Hat Enterprise Linux 10 and 9**: [See instructions](#use-with-rhel)

Images:

- [`base`](#base-image): Includes some basic system tools
- [`tailscale`](#tailscale-image): Includes Tailscale (built on top of `base`)
- [`k3s`](#k3s-image): Includes K3s (built on top of `base`)
- [`zfs`](#zfs-image): Includes ZFS as a kernel module (built on top of `base`)
- [`monitoring`](#monitoring-image): Includes Grafana Alloy (built on top of `base`)
- [`monitoring-zfs`](#monitoring-zfs-image): Includes Grafana Alloy (built on top of `zfs`)

These images are built using GitHub Actions at least bi-weekly.

Images are published on GitHub Packages and available for **linux/amd64** and **linux/arm64** (except ZFS).

## `base` image

Includes:

- Utilities: `screen`, `pv`, `sqlite`, `jq`, `tmux`, `tree`, `rsync`, `yq`

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-10/base:latest
ghcr.io/italypaleale/bootc/centos-stream-9/base:latest
ghcr.io/italypaleale/bootc/alma-linux-10/base:latest
ghcr.io/italypaleale/bootc/alma-linux-9/base:latest
```

Source: [el10](./el10/containers/base/), [el9](./el9/containers/base/)

## `tailscale` image

Includes:

- Everything in the [`base` image](#base-image)
- [Tailscale](https://tailscale.com/)

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-10/tailscale:latest
ghcr.io/italypaleale/bootc/centos-stream-9/tailscale:latest
ghcr.io/italypaleale/bootc/alma-linux-10/tailscale:latest
ghcr.io/italypaleale/bootc/alma-linux-9/tailscale:latest
```

Source: [el10](./el10/containers/tailscale/), [el9](./el9/containers/tailscale/)

## `k3s` image

Includes:

- Everything in the [`base` image](#base-image)
- [K3s](https://k3s.io/), available as server or agent only

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-10/k3s:latest
ghcr.io/italypaleale/bootc/centos-stream-9/k3s:latest
ghcr.io/italypaleale/bootc/alma-linux-10/k3s:latest
ghcr.io/italypaleale/bootc/alma-linux-9/k3s:latest
```

Source: [el10](./el10/containers/k3s/), [el9](./el9/containers/k3s/)

### Using K3s

The image contains K3s pre-installed at the latest version, but it is not started automaticaly. K3s can be configured using the [YAML config file format](https://docs.k3s.io/installation/configuration) or by setting environmental variables.

- For a K3s server (which normally starts an agent too, unless configured otherwise):
   1. Configure K3s by editing the file `/etc/rancher/k3s/config/k3s-server.yaml`
   2. Optionally set environmental variables in the file `/etc/systemd/system/k3s-server.service.env`
   3. Enable and start the systemd unit with: `systemctl enable --now k3s-server`
- For a K3s agent only:
   1. Configure K3s by editing the file `/etc/rancher/k3s/config/k3s-agent.yaml`
   2. Optionally set environmental variables in the file `/etc/systemd/system/k3s-agent.service.env`
   3. Enable and start the systemd unit with: `systemctl enable --now k3s-agent`

## `zfs` image

Includes:

- Everything in the [`base` image](#base-image)
- ZFS as a kernel module

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-10/zfs:latest
ghcr.io/italypaleale/bootc/centos-stream-9/zfs:latest
ghcr.io/italypaleale/bootc/alma-linux-10/zfs:latest
ghcr.io/italypaleale/bootc/alma-linux-9/zfs:latest
```

Source: [el10](./el10/containers/zfs/), [el9](./el9/containers/zfs/)

## `monitoring` image

Includes:

- Everything in the [`base` image](#base-image)
- [Grafana Alloy](https://github.com/grafana/alloy)
- [prometheus-podman-exporter](https://github.com/containers/prometheus-podman-exporter)

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-10/monitoring:latest
ghcr.io/italypaleale/bootc/centos-stream-9/monitoring:latest
ghcr.io/italypaleale/bootc/alma-linux-10/monitoring:latest
ghcr.io/italypaleale/bootc/alma-linux-9/monitoring:latest
```

Source: [el10](./el10/containers/monitoring/), [el9](./el9/containers/monitoring/)

## `monitoring-zfs` image

Includes:

- Everything in the [`zfs` image](#zfs-image)
- [Grafana Alloy](https://github.com/grafana/alloy)
- [prometheus-podman-exporter](https://github.com/containers/prometheus-podman-exporter)

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-10/monitoring-zfs:latest
ghcr.io/italypaleale/bootc/centos-stream-9/monitoring-zfs:latest
ghcr.io/italypaleale/bootc/alma-linux-10/monitoring-zfs:latest
ghcr.io/italypaleale/bootc/alma-linux-9/monitoring-zfs:latest
```

Source: [el10](./el10/containers/monitoring-zfs/), [el9](./el9/containers/monitoring-zfs/)

## Build images

The repository contains a CLI tool in the [tools](./tools/) directory, which can be used to build images. This is the same tool used in this repo's GitHub Actions.

To build images locally, you will need these tools installed:

- Go
- Podman 5+
  - Although Docker can be used as well, Podman is strongly recommended

1. First, build the CLI tools:

   ```sh
   mkdir -p .bin
   (cd tools; go build -v -o ../.bin/tools)
   ```

2. (Optional) to update the versions of apps and base images, run the `update-versions` command:

   ```sh
   .bin/tools update-versions --work-dir ./el10
   ```

3. Build an image. The command below is an example to build the [base](./el10/containers/base) image, pushing it to Docker Hub at `docker.io/username/bootc/centos-stream-10/base` with the tag as the current date.

   ```sh
   .bin/tools build \
      base \
      --default-base-image "centos-stream-10" \
      --work-dir ./el10 \
      --arch amd64,arm64 \
      --repository "docker.io/username/bootc/centos-stream-10" \
      --push \
      --tag "$(date +"%Y%m%d")"
   ```

## Use with RHEL

The Containerfiles are compatible with RHEL too, currently supporting RHEL 10 and 9. Due to licensing reasons, the RHEL-based images are not published from this repo automatically.

> For building RHEL container images, the host OS must be running RHEL as well, or the container will not be able to connect to the Red Hat repositories.

To build images based on RHEL locally:

1. Make sure Podman is authenticated with the Red Hat Container Registry (use `podman login registry.redhat.io`) **and** so is Docker (the credentials for the registry must be available in the file `~/.docker/config.json` for the `update-versions` tool to work). You can create [Token Based Registries](https://access.redhat.com/terms-based-registry/) instead of passwords. Full docs for [registry authentication](https://access.redhat.com/articles/RegistryAuthentication).
2. Create the file `el10/config.override.yaml` file:

   ```yaml
   baseImages:
     rhel-10:
       image: registry.redhat.io/rhel10/rhel-bootc
       tag: latest
       digest: ''
   ```

3. Run the `update-versions` tool to fetch the latest digests:

   ```sh
   .bin/tools update-versions --config-file-name config.override.yaml --work-dir ./el10
   ```

4. Build the containers using `rhel-10` as default base image. For example, to build the [base](./el10/containers/base) image:

   ```sh
   .bin/tools build \
      base \
      --default-base-image "rhel-10" \
      --work-dir ./el10 \
      --arch amd64 \
      --repository "docker.io/username/bootc/rhel10" \
      --push \
      --tag "$(date +"%Y%m%d")"
   ```
