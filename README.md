# weron

![Logo](./assets/logo-readme.png)

Overlay networks based on WebRTC.

⚠️ weron has not yet been audited! While we try to make weron as secure as possible, it has not yet undergone a formal security audit by a third party. Please keep this in mind if you use it for security-critical applications. ⚠️

[![hydrun CI](https://github.com/pojntfx/weron/actions/workflows/hydrun.yaml/badge.svg)](https://github.com/pojntfx/weron/actions/workflows/hydrun.yaml)
[![Docker CI](https://github.com/pojntfx/weron/actions/workflows/docker.yaml/badge.svg)](https://github.com/pojntfx/weron/actions/workflows/docker.yaml)
[![Go Reference](https://pkg.go.dev/badge/github.com/pojntfx/weron.svg)](https://pkg.go.dev/github.com/pojntfx/weron)
[![Matrix](https://img.shields.io/matrix/weron:matrix.org)](https://matrix.to/#/#weron:matrix.org?via=matrix.org)
[![Binary Downloads](https://img.shields.io/github/downloads/pojntfx/weron/total?label=binary%20downloads)](https://github.com/pojntfx/weron/releases)

## Overview

weron provides lean, fast & secure overlay networks based on WebRTC.

🚧 This project is a work-in-progress! Instructions will be added as soon as it is usable. 🚧

## Installation

### Containerized

You can get the OCI image like so:

```shell
$ podman pull ghcr.io/pojntfx/weron
```

### Natively

Static binaries are available on [GitHub releases](https://github.com/pojntfx/weron/releases).

On Linux, you can install them like so:

```shell
$ curl -L -o /tmp/weron "https://github.com/pojntfx/weron/releases/latest/download/weron.linux-$(uname -m)"
$ sudo install /tmp/weron /usr/local/bin
$ sudo setcap cap_net_admin+ep /usr/local/bin/weron # This allows rootless execution
```

On macOS, you can use the following:

```shell
$ curl -L -o /tmp/weron "https://github.com/pojntfx/weron/releases/latest/download/weron.darwin-$(uname -m)"
$ sudo install /tmp/weron /usr/local/bin
```

On Windows, the following should work (using PowerShell as administrator):

```shell
PS> Invoke-WebRequest https://github.com/pojntfx/weron/releases/latest/download/weron.windows-x86_64.exe -OutFile \Windows\System32\weron.exe
```

You can find binaries for more operating systems and architectures on [GitHub releases](https://github.com/pojntfx/weron/releases).

## Usage

> TL;DR: Join a layer 3 (IP) overlay network with `sudo weron vpn ip --community mycommunity --password mypassword --key mykey --ips 2001:db8::1/32,192.0.2.1/24` and a layer 2 (Ethernet) overlay network with `sudo weron vpn ethernet --community mycommunity --password mypassword --key mykey`

### 1. Start a Signaling Server with `weron signaler`

While it is possible to use the hosted signaling server (which is currently hosted on `wss://weron.herokuapp.com/`), hosting the server yourself has many benefits, such as lower latency and better privacy. The signaling server can use an in-process broker with an in-memory database or Redis and PostgreSQL; for production use the latter configuration is strongly recommended, as it allows you to easily scale the signaling server horizontally.

First, start Redis and Postgres:

```shell
$ sudo podman network create weron
$ sudo podman run -d --name weron-postgres --network weron -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=weron_communities postgres
$ sudo podman run -d --name weron-redis --network weron redis
```

Now, start the signaling server:

```shell
$ sudo podman run -d --name weron-signaler --network weron -p 1337:1337 -e DATABASE_URL='postgres://postgres@weron-postgres:5432/weron_communities?sslmode=disable' -e REDIS_URL='redis://localhost:6379/1' -e API_PASSWORD='myapipassword' ghcr.io/pojntfx/weron:unstable weron signaler
```

It should now be reachable on `ws://localhost:1337/`.

To use it in production, put this signaling server behind a TLS-enabled reverse proxy such as [Caddy](https://caddyserver.com/) or [Traefik](https://traefik.io/). You may also either want to keep `API_PASSWORD` empty to disable the management API completely or use OpenID Connect to authenticate instead; see the [signaling server reference](#signaling-server).

## Reference

### Command Line Arguments

#### Global Arguments

```shell
$ weron --help
Overlay networks based on WebRTC.

Find more information at:
https://github.com/pojntfx/weron

Usage:
  weron [command]

Available Commands:
  chat        Chat over the overlay network
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  manager     Manage a signaling server
  signaler    Start a signaling server
  utility     Utilities for overlay networks
  vpn         Join virtual private networks built on overlay networks

Flags:
  -h, --help          help for weron
  -v, --verbose int   Verbosity level (0 is disabled, default is info, 7 is trace) (default 5)

Use "weron [command] --help" for more information about a command.
```

#### Signaling Server

```shell
$ weron signaler --help
Start a signaling server

Usage:
  weron signaler [flags]

Aliases:
  signaler, sgl, s

Flags:
      --api-password string     Password for the management API (can also be set using the API_PASSWORD env variable). Ignored if any of the OIDC parameters are set.
      --api-username string     Username for the management API (can also be set using the API_USERNAME env variable). Ignored if any of the OIDC parameters are set. (default "admin")
      --cleanup                 (Warning: Only enable this after stopping all other servers accessing the database!) Remove all ephermal communities from database and reset client counts before starting
      --ephermal-communities    Enable the creation of ephermal communities (default true)
      --heartbeat duration      Time to wait for heartbeats (default 10s)
  -h, --help                    help for signaler
      --laddr string            Listening address (can also be set using the PORT env variable) (default ":1337")
      --oidc-client-id string   OIDC Client ID (i.e. myoidcclientid) (can also be set using the OIDC_CLIENT_ID env variable)
      --oidc-issuer string      OIDC Issuer (i.e. https://pojntfx.eu.auth0.com/) (can also be set using the OIDC_ISSUER env variable)
      --postgres-url string     URL of PostgreSQL database to use (i.e. postgres://myuser:mypassword@myhost:myport/mydatabase) (can also be set using the DATABASE_URL env variable). If empty, a in-memory database will be used.
      --redis-url string        URL of Redis database to use (i.e. redis://myuser:mypassword@myhost:myport/1) (can also be set using the REDIS_URL env variable). If empty, a in-process broker will be used.

Global Flags:
  -v, --verbose int   Verbosity level (0 is disabled, default is info, 7 is trace) (default 5)
```

#### Manager

```shell
$ weron manager --help
Manage a signaling server

Usage:
  weron manager [command]

Aliases:
  manager, mgr, m

Available Commands:
  create      Create a persistent community
  delete      Delete a persistent or ephermal community
  list        List persistent and ephermal communities

Flags:
  -h, --help   help for manager

Global Flags:
  -v, --verbose int   Verbosity level (0 is disabled, default is info, 7 is trace) (default 5)

Use "weron manager [command] --help" for more information about a command.
```

#### Chat

```shell
$ weron chat --help
Chat over the overlay network

Usage:
  weron chat [flags]

Aliases:
  chat, cht, c

Flags:
      --channels strings    Comma-separated list of channels in community to join (default [weron/chat/primary])
      --community string    ID of community to join
      --force-relay         Force usage of TURN servers
  -h, --help                help for chat
      --ice strings         Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp) (default [stun:stun.l.google.com:19302])
      --id-channel string   Channel to use to negotiate names (default "weron/chat/id")
      --key string          Encryption key for community
      --kicks duration      Time to wait for kicks (default 5s)
      --names strings       Comma-separated list of names to try and claim one from
      --password string     Password for community
      --raddr string        Remote address (default "wss://weron.herokuapp.com/")
      --timeout duration    Time to wait for connections (default 10s)

Global Flags:
  -v, --verbose int   Verbosity level (0 is disabled, default is info, 7 is trace) (default 5)
```

#### Latency Measurement Utility

```shell
$ weron utility latency --help
Measure the latency of the overlay network

Usage:
  weron utility latency [flags]

Aliases:
  latency, ltc, l

Flags:
      --community string    ID of community to join
      --force-relay         Force usage of TURN servers
  -h, --help                help for latency
      --ice strings         Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp) (default [stun:stun.l.google.com:19302])
      --key string          Encryption key for community
      --packet-length int   Size of packet to send and acknowledge (default 128)
      --password string     Password for community
      --pause duration      Time to wait before sending next packet (default 1s)
      --raddr string        Remote address (default "wss://weron.herokuapp.com/")
      --server              Act as a server
      --timeout duration    Time to wait for connections (default 10s)

Global Flags:
  -v, --verbose int   Verbosity level (0 is disabled, default is info, 7 is trace) (default 5)
```

#### Throughput Measurement Utility

```shell
$ weron utility throughput --help
Measure the throughput of the overlay network

Usage:
  weron utility throughput [flags]

Aliases:
  throughput, thr, t

Flags:
      --community string    ID of community to join
      --force-relay         Force usage of TURN servers
  -h, --help                help for throughput
      --ice strings         Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp) (default [stun:stun.l.google.com:19302])
      --key string          Encryption key for community
      --packet-count int    Amount of packets to send before waiting for acknowledgement (default 1000)
      --packet-length int   Size of packet to send (default 50000)
      --password string     Password for community
      --raddr string        Remote address (default "wss://weron.herokuapp.com/")
      --server              Act as a server
      --timeout duration    Time to wait for connections (default 10s)

Global Flags:
  -v, --verbose int   Verbosity level (0 is disabled, default is info, 7 is trace) (default 5)
```

#### Layer 3 (IP) Overlay Networks

```shell
$ weron vpn ip --help
Join a layer 3 overlay network

Usage:
  weron vpn ip [flags]

Aliases:
  ip, i

Flags:
      --community string    ID of community to join
      --dev string          Name to give to the TAP device (i.e. weron0) (default is auto-generated; only supported on Linux, macOS and Windows)
      --force-relay         Force usage of TURN servers
  -h, --help                help for ip
      --ice strings         Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp) (default [stun:stun.l.google.com:19302])
      --id-channel string   Channel to use to negotiate names (default "weron/ip/id")
      --ips strings         Comma-separated list of IP networks to claim an IP address from and and give to the TUN device (i.e. 2001:db8::1/32,192.0.2.1/24) (on Windows, only one IPv4 and one IPv6 address are supported; on macOS, IPv4 addresses are ignored)
      --key string          Encryption key for community
      --kicks duration      Time to wait for kicks (default 5s)
      --max-retries int     Maximum amount of times to try and claim an IP address (default 200)
      --parallel int        Amount of threads to use to decode frames (default 8)
      --password string     Password for community
      --raddr string        Remote address (default "wss://weron.herokuapp.com/")
      --static              Try to claim the exact IPs specified in the --ips flag statically instead of selecting a random one from the specified network
      --timeout duration    Time to wait for connections (default 10s)

Global Flags:
  -v, --verbose int   Verbosity level (0 is disabled, default is info, 7 is trace) (default 5)
```

#### Layer 2 (Ethernet) Overlay Networks

```shell
$ weron vpn ethernet --help
Join a layer 2 overlay network

Usage:
  weron vpn ethernet [flags]

Aliases:
  ethernet, eth, e

Flags:
      --community string   ID of community to join
      --dev string         Name to give to the TAP device (i.e. weron0) (default is auto-generated; only supported on Linux, macOS and Windows)
      --force-relay        Force usage of TURN servers
  -h, --help               help for ethernet
      --ice strings        Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp) (default [stun:stun.l.google.com:19302])
      --key string         Encryption key for community
      --mac string         MAC address to give to the TAP device (i.e. 3a:f8:de:7b:ef:52) (default is auto-generated; only supported on Linux)
      --parallel int       Amount of threads to use to decode frames (default 8)
      --password string    Password for community
      --raddr string       Remote address (default "wss://weron.herokuapp.com/")
      --timeout duration   Time to wait for connections (default 10s)

Global Flags:
  -v, --verbose int   Verbosity level (0 is disabled, default is info, 7 is trace) (default 5)
```

### Environment Variables

All command line arguments described above can also be set using environment variables; for example, to set `--max-retries` to `300` with an environment variable, use `WERON_MAX_RETRIES=300`.

## Acknowledgements

- [songgao/water](https://github.com/songgao/water) provides the TUN/TAP device library for weron.
- [pion/webrtc](https://github.com/pion/webrtc) provides the WebRTC functionality.
- All the rest of the authors who worked on the dependencies used! Thanks a lot!

## Contributing

To contribute, please use the [GitHub flow](https://guides.github.com/introduction/flow/) and follow our [Code of Conduct](./CODE_OF_CONDUCT.md).

To build and start a development version of weron locally, run the following:

```shell
$ git clone https://github.com/pojntfx/weron.git
$ cd weron
$ make depend
$ make && sudo make install
$ weron signal # Starts the signaling server
# In another terminal
$ weron chat --raddr ws://localhost:1337 --community mycommunity --password mypassword --key mykey --names user1,user2,user3 --channels one,two,three
# In another terminal
$ weron chat --raddr ws://localhost:1337 --community mycommunity --password mypassword --key mykey --names user1,user2,user3 --channels one,two,three
```

Of course, you can also contribute to the utilities and VPNs like this.

Have any questions or need help? Chat with us [on Matrix](https://matrix.to/#/#weron:matrix.org?via=matrix.org)!

## License

weron (c) 2022 Felicitas Pojtinger and contributors

SPDX-License-Identifier: AGPL-3.0
