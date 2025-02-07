# bootc images for CentOS Stream

Custom images for CentOS Stream that can be used with bootc. Currently supports CentOS Stream 9 and Alma Linux 9.

- [`base`](#base-image): Includes some basic system tools
- [`tailscale`](#tailscale-image): Includes Tailscale (built on top of `base`)
- [`k3s`](#k3s-image): Includes K3s (built on top of `base`)
- [`zfs`](#zfs-image): Includes ZFS as a kernel module (built on top of `base`)
- [`monitoring`](#monitoring-image): Includes Grafana Alloy (built on top of `base`)
- [`monitoring-zfs`](#monitoring-zfs-image): Includes Grafana Alloy (built on top of `zfs`)

These images are built using GitHub Actions every Monday and Friday, from the upstream images of CentOS Stream.

Images are published on GitHub Packages and available for linux/amd64 and linux/arm64 (except ZFS).

## `base` image

Includes:

- Utilities: `screen`, `pv`, `sqlite`, `jq`, `tmux`, `tree`, `rsync`

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/base:latest
ghcr.io/italypaleale/bootc/alma-linux-9/base:latest
```

[Source](./el9/containers/base/)

## `tailscale` image

Includes:

- Everything in the [`base` image](#base-image)
- [Tailscale](https://tailscale.com/)

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/tailscale:latest
ghcr.io/italypaleale/bootc/alma-linux-9/tailscale:latest
```

[Source](./el9/containers/tailscale/)

## `k3s` image

Includes:

- Everything in the [`base` image](#base-image)
- [K3s](https://k3s.io/), available as server or agent only

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/k3s:latest
ghcr.io/italypaleale/bootc/alma-linux-9/k3s:latest
```

[Source](./el9/containers/k3s/)

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
ghcr.io/italypaleale/bootc/centos-stream-9/zfs:latest
ghcr.io/italypaleale/bootc/alma-linux-9/zfs:latest
```

[Source](./el9/containers/zfs/)

## `monitoring` image

Includes:

- Everything in the [`base` image](#base-image)
- [Grafana Alloy](https://github.com/grafana/alloy)
- [prometheus-podman-exporter](https://github.com/containers/prometheus-podman-exporter)

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/monitoring:latest
ghcr.io/italypaleale/bootc/alma-linux-9/monitoring:latest
```

[Source](./el9/containers/monitoring/)

## `monitoring-zfs` image

Includes:

- Everything in the [`zfs` image](#zfs-image)
- [Grafana Alloy](https://github.com/grafana/alloy)
- [prometheus-podman-exporter](https://github.com/containers/prometheus-podman-exporter)

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/monitoring-zfs:latest
ghcr.io/italypaleale/bootc/alma-linux-9/monitoring-zfs:latest
```

[Source](./el9/containers/monitoring-zfs/)

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
   .bin/tools update-versions --work-dir ./el9
   ```

3. Build an image. The command below is an example to build the [base](./el9/containers/base) image, pushing it to Docker Hub at `docker.io/username/bootc/centos-stream-9/base` with the tag as the current date.

   ```sh
   .bin/tools build \
      base \
      --default-base-image "centos-stream-9" \
      --work-dir ./el9 \
      --arch amd64,arm64 \
      --repository "docker.io/username/bootc/centos-stream-9" \
      --push \
      --tag "$(date +"%Y%m%d")"
   ```

## Use with RHEL

The Containerfiles are compatible with RHEL too, currently supporting RHEL 9. Due to licensing reasons, the RHEL-based images are not published from this repo automatically.

> For building RHEL container images, the host OS must be running RHEL as well, or the container will not be able to connect to the Red Hat repositories.

To build images based on RHEL locally:

1. Make sure Podman is authenticated with the Red Hat Container Registry (use `podman login`) **and** so is Docker (the credentials for the registry must be available in the file `~/.docker/config.json` for the `update-versions` tool to work)
2. Create the file `el9/config.override.yaml` file:

   ```yaml
   baseImages:
     rhel-9:
       image: registry.redhat.io/rhel9/rhel-bootc
       tag: latest
       digest: ''
   ```

3. Run the `update-versions` tool to fetch the latest digests:

   ```sh
   .bin/tools update-versions --work-dir ./el9
   ```

4. Build the containers using `rhel-9` as default base image. For example, to build the [base](./el9/containers/base) image:

   ```sh
   .bin/tools build \
      base \
      --default-base-image "rhel-9" \
      --work-dir ./el9 \
      --arch amd64,arm64 \
      --repository "docker.io/username/bootc/rhel9" \
      --push \
      --tag "$(date +"%Y%m%d")"
   ```
