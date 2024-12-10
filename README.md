# bootc images for CentOS Stream

Custom images for CentOS Stream that can be used with bootc. Currently supports CentOS Stream 9 and Alma Linux 9.

- [`tailscale`](#tailscale-image): Includes Tailscale
- [`base`](#base-image): Includes some basic system tools (built on top of `tailscale`)
- [`zfs`](#zfs-image): Includes ZFS as a kernel module (built on top of `base`)
- [`monitoring`](#monitoring-image): Includes Grafana Alloy (built on top of `base`)
- [`monitoring-zfs`](#monitoring-zfs-image): Includes Grafana Alloy (built on top of `zfs`)

These images are built using GitHub Actions every Monday and Friday, from the upstream images of CentOS Stream.

Images are published on GitHub Packages and available for linux/amd64 and linux/arm64 (except ZFS).

## `tailscale` image

Includes:

- [Tailscale](https://tailscale.com/)

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/tailscale:latest
ghcr.io/italypaleale/bootc/alma-linux-9/tailscale:latest
```

[Source](./el9/tailscale/)

## `base` image

Includes:

- Everything in the [`tailscale` image](#tailscale-image)
- [restic](https://github.com/restic/restic)
- [gotop](https://github.com/xxxserxxx/gotop)
- Utilities: `screen`, `pv`, `sqlite`, `jq`

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/base:latest
ghcr.io/italypaleale/bootc/alma-linux-9-9/base:latest
```

[Source](./el9/base/)

## `zfs` image

Includes:

- Everything in the [`base` image](#base-image)
- ZFS as a kernel module

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/zfs:latest
ghcr.io/italypaleale/bootc/alma-linux-9-9/zfs:latest
```

[Source](./el9/zfs/)

## `monitoring` image

Includes:

- Everything in the [`base` image](#base-image)
- [Grafana Alloy](https://github.com/grafana/alloy)
- [prometheus-podman-exporter](https://github.com/containers/prometheus-podman-exporter)

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/monitoring:latest
ghcr.io/italypaleale/bootc/alma-linux-9-9/monitoring:latest
```

[Source](./el9/monitoring/)

## `monitoring-zfs` image

Includes:

- Everything in the [`zfs` image](#zfs-image)
- [Grafana Alloy](https://github.com/grafana/alloy)
- [prometheus-podman-exporter](https://github.com/containers/prometheus-podman-exporter)

Image:

```text
ghcr.io/italypaleale/bootc/centos-stream-9/monitoring-zfs:latest
ghcr.io/italypaleale/bootc/alma-linux-9-9/monitoring-zfs:latest
```

[Source](./el9/monitoring-zfs/)

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
   .bin/tools update-versions -f ./versions.yaml
   ```

3. Build an image. The command below is an example to build the [tailscale](./el9/tailscale) image, pushing it to Docker Hub at `docker.io/username/bootc/centos-stream-9/tailscale` with the tag as the current date.

   ```sh
   .bin/tools build \
      ./el9/tailscale \
      --default-base-image "centos-stream-9" \
      --versions-file versions.yaml \
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
2. Create a `versions.local.yaml` file:

   ```yaml
   baseImages:
     rhel-9:
       image: registry.redhat.io/rhel9/rhel-bootc
       tag: latest
       digest: ''
   ```

3. Run the `update-versions` tool to fetch the latest digests:

   ```sh
   .bin/tools update-versions -f ./versions.local.yaml
   ```

4. Build the containers using `rhel-9` as default base image. For example, to build the [tailscale](./el9/tailscale) image:

   ```sh
   .bin/tools build \
      ./el9/tailscale \
      --default-base-image "rhel-9" \
      --versions-file versions.yaml \
      --arch amd64,arm64 \
      --repository "docker.io/username/bootc/rhel9" \
      --push \
      --tag "$(date +"%Y%m%d")"
   ```
