# Distributed Minio Quickstart Guide [![Gitter](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/minio/minio?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge) [![Go Report Card](https://goreportcard.com/badge/minio/minio)](https://goreportcard.com/report/minio/minio) [![Docker Pulls](https://img.shields.io/docker/pulls/minio/minio.svg?maxAge=604800)](https://hub.docker.com/r/minio/minio/) [![codecov](https://codecov.io/gh/minio/minio/branch/master/graph/badge.svg)](https://codecov.io/gh/minio/minio)

Minio in distributed mode lets you pool multiple drives (even on different machines) into a single object storage server. As drives are distributed across several nodes, distributed Minio can withstand multiple node failures and yet ensure full data protection.

## Why distributed Minio?

Minio in distributed mode can help you setup a highly-available storage system with a single object storage deployment. With distributed Minio, you can optimally use storage devices, irrespective of their location in a network.

### Data protection

Distributed Minio provides protection against multiple node/drive failures and [bit rot](https://github.com/minio/minio/blob/master/docs/erasure/README.md#what-is-bit-rot-protection) using [erasure code](https://docs.minio.io/docs/minio-erasure-code-quickstart-guide). As the minimum disks required for distributed Minio is 4 (same as minimum disks required for erasure coding), erasure code automatically kicks in as you launch distributed Minio.

### High availability

A stand-alone Minio server would go down if the server hosting the disks goes offline. In contrast, a distributed Minio setup with _n_ disks will have your data safe as long as _n/2_ or more disks are online. You'll need a minimum of _(n/2 + 1)_ [Quorum](https://github.com/minio/dsync#lock-process) disks to create new objects though.

For example, a 8 nodes distributed Minio setup, with 1 disk per node would stay put, even if upto 4 nodes are offline. But, you'll need atleast 5 nodes online to create new objects.

## Limitations

As with Minio in standalone mode, distributed Minio has the per tenant limit of minimum 4 and maximum 16 drives (imposed by erasure code). This helps maintain simplicity and yet remain scalable. If you need a multiple tenant setup, you can easily spin multiple Minio instances managed by orchestration tools like Kubernetes.

Note that with distributed Minio you can play around with the number of nodes and drives as long as the limits are adhered to. For example you can have 2 nodes with 4 drives each, 4 nodes with 4 drives each, 8 nodes with 2 drives each, and so on.

# Get started

If you're aware of stand-alone Minio set up, the process remains largely the same, as the Minio server automatically switches to standalone or distributed mode, depending on the command line parameters.

## 1. Prerequisites

Install Minio - [Minio Quickstart Guide](https://docs.minio.io/docs/minio).

## 2. Run distributed Minio

To start a distributed Minio instance, you just need to pass drive locations as parameters to the minio server command. Then, you’ll need to run the same command on all the participating nodes.

It is important to note here that all the nodes running distributed Minio need to have same access key and secret key. Otherwise nodes won't connect. To achieve this, you need to export access key and secret key as environment variables on all the nodes before executing Minio server command.

Below examples will clarify further:

Example 1: Start distributed Minio instance with 1 drive each on 8 nodes, by running this command on all the 8 nodes.

```

$ export MINIO_ACCESS_KEY=<ACCESS_KEY>
$ export MINIO_SECRET_KEY=<SECRET_KEY>
$ minio server http://192.168.1.11/export1 http://192.168.1.12/export2
http://192.168.1.13/export3 http://192.168.1.14/export4 http://192.168.1.15/export5 http://192.168.1.16/export6
http://192.168.1.17/export7 http://192.168.1.18/export8

```

![Distributed Minio, 8 nodes with 1 disk each](https://raw.githubusercontent.com/minio/minio/master/docs/screenshots/Architecture-diagram_distributed_8.png)

Example 2: Start distributed Minio instance with 4 drives each on 4 nodes, by running this command on all the 4 nodes.

```

$ export MINIO_ACCESS_KEY=<ACCESS_KEY>
$ export MINIO_SECRET_KEY=<SECRET_KEY>
$ minio server http://192.168.1.11/export1 http://192.168.1.11/export2
http://192.168.1.11/export3 http://192.168.1.11/export4
http://192.168.1.12/export1 http://192.168.1.12/export2
http://192.168.1.12/export3 http://192.168.1.12/export4
http://192.168.1.13/export1 http://192.168.1.13/export2
http://192.168.1.13/export3 http://192.168.1.13/export4
http://192.168.1.14/export1 http://192.168.1.14/export2
http://192.168.1.14/export3 http://192.168.1.14/export4

```

![Distributed Minio, 4 nodes with 4 disks each](https://raw.githubusercontent.com/minio/minio/master/docs/screenshots/Architecture-diagram_distributed_16.png)

Note that these IP addresses and drive paths are for demonstration purposes only, you need to replace these with the actual IP addresses and drive paths.

## 3. Test your setup

To test this setup, access the Minio server via browser or [`mc`](https://docs.minio.io/docs/minio-client-quickstart-guide). You’ll see the combined capacity of all the storage drives as the capacity of this drive.

## Explore Further
- [Minio Erasure Code QuickStart Guide](https://docs.minio.io/docs/minio-erasure-code-quickstart-guide)
- [Use `mc` with Minio Server](https://docs.minio.io/docs/minio-client-quickstart-guide)
- [Use `aws-cli` with Minio Server](https://docs.minio.io/docs/aws-cli-with-minio)
- [Use `s3cmd` with Minio Server](https://docs.minio.io/docs/s3cmd-with-minio)
- [Use `minio-go` SDK with Minio Server](https://docs.minio.io/docs/golang-client-quickstart-guide)
- [The Minio documentation website](https://docs.minio.io)