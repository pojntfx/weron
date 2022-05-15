# weron

![Logo](./assets/logo-readme.png)

Overlay networks based on WebRTC.

⚠️ weron has not yet been audited! While we try to make weron as secure as possible, it has not yet undergone a formal security audit by a third party. Please keep this in mind if you use it for security-critical applications. ⚠️

[![hydrun CI](https://github.com/pojntfx/weron/actions/workflows/hydrun.yaml/badge.svg)](https://github.com/pojntfx/weron/actions/workflows/hydrun.yaml)
[![Docker CI](https://github.com/pojntfx/weron/actions/workflows/docker.yaml/badge.svg)](https://github.com/pojntfx/weron/actions/workflows/docker.yaml)
![Go Version](https://img.shields.io/badge/go%20version-%3E=1.18-61CFDD.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/pojntfx/weron.svg)](https://pkg.go.dev/github.com/pojntfx/weron)
[![Matrix](https://img.shields.io/matrix/weron:matrix.org)](https://matrix.to/#/#weron:matrix.org?via=matrix.org)
[![Binary Downloads](https://img.shields.io/github/downloads/pojntfx/weron/total?label=binary%20downloads)](https://github.com/pojntfx/weron/releases)

## Overview

weron provides lean, fast & secure overlay networks based on WebRTC.

It enables you too ...

- **Access nodes behind NAT**: Because weron uses WebRTC to establish connections between nodes, it can easily traverse corporate firewalls and NATs using STUN, or even use a TURN server to tunnel traffic. This can be very useful to for example SSH into your homelab without forwarding any ports on your router.
- **Secure your home network**: Due to the relatively low overhead of WebRTC in low-latency networks, weron can be used to secure traffic between nodes in a LAN without a significant performance hit.
- **Join local nodes into a cloud network**: If you run for example a Kubernetes cluster with nodes based on cloud instances but also want to join your on-prem nodes into it, you can use weron to create a trusted network.
- **Bypass censorship**: The underlying WebRTC suite, which is what popular videoconferencing tools such as Zoom, Teams and Meet are built on, is hard to block on a network level, making it a valuable addition to your toolbox for bypassing state or corporate censorship.
- **Write your own peer-to-peer protocols**: The simple API makes writing distributed applications with automatic reconnects, multiple datachannels etc. easy.

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

> TL;DR: Join a layer 3 (IP) overlay network on the hosted signaling server with `sudo weron vpn ip --community mycommunity --password mypassword --key mykey --ips 2001:db8::1/32,192.0.2.1/24` and a layer 2 (Ethernet) overlay network with `sudo weron vpn ethernet --community mycommunity --password mypassword --key mykey`

### 1. Start a Signaling Server with `weron signaler`

The signaling server connects peers with each other by exchanging connection information between them. It also manages access to communities through the `--password` flag of clients and can maintain persistent communities even after all peers have disconnected.

While it is possible and resonably private (in addition to TLS, connection information is encrypted using the `--key` flag of clients) to use the hosted signaling server at `wss://weron.herokuapp.com/`, hosting it yourself has many benefits, such as lower latency and even better privacy.

The signaling server can use an in-process broker with an in-memory database or Redis and PostgreSQL; for production use the latter configuration is strongly recommended, as it allows you to easily scale the signaling server horizontally. This is particularly important if you want to scale your server infrastructure across multiple continents, as intra-cloud backbones usually have lower latency than residental connections, which reduces the amount of time required to connect peers with each other.

<details>
  <summary>Expand containerized instructions</summary>

```shell
$ sudo podman network create weron

$ sudo podman run -d --restart=always --label "io.containers.autoupdate=image" --name weron-postgres --network weron -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=weron_communities postgres
$ sudo podman generate systemd --new weron-postgres | sudo tee /lib/systemd/system/weron-postgres.service

$ sudo podman run -d --restart=always --label "io.containers.autoupdate=image" --name weron-redis --network weron redis
$ sudo podman generate systemd --new weron-redis | sudo tee /lib/systemd/system/weron-redis.service

$ sudo podman run -d --restart=always --label "io.containers.autoupdate=image" --name weron-signaler --network weron -p 1337:1337 -e DATABASE_URL='postgres://postgres@weron-postgres:5432/weron_communities?sslmode=disable' -e REDIS_URL='redis://weron-redis:6379/1' -e API_PASSWORD='myapipassword' ghcr.io/pojntfx/weron:unstable weron signaler
$ sudo podman generate systemd --new weron-signaler | sudo tee /lib/systemd/system/weron-signaler.service

$ sudo systemctl daemon-reload

$ sudo systemctl enable --now weron-postgres
$ sudo systemctl enable --now weron-redis
$ sudo systemctl enable --now weron-signaler

$ sudo firewall-cmd --permanent --add-port=1337/tcp
$ sudo firewall-cmd --reload
```

</details>

<details>
  <summary>Expand native instructions</summary>

```shell
sudo podman run -d --restart=always --label "io.containers.autoupdate=image" --name weron-postgres -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=weron_communities -p 127.0.0.1:5432:5432 postgres
sudo podman generate systemd --new weron-postgres | sudo tee /lib/systemd/system/weron-postgres.service

sudo podman run -d --restart=always --label "io.containers.autoupdate=image" --name weron-redis -p 127.0.0.1:6379:6379 redis
sudo podman generate systemd --new weron-redis | sudo tee /lib/systemd/system/weron-redis.service

sudo tee /etc/systemd/system/weron-signaler.service<<'EOT'
[Unit]
Description=weron Signaling Server
After=weron-postgres.service weron-redis.service

[Service]
ExecStart=/usr/local/bin/weron signaler --verbose=7
Environment="DATABASE_URL=postgres://postgres@localhost:5432/weron_communities?sslmode=disable"
Environment="REDIS_URL=redis://localhost:6379/1"
Environment="API_PASSWORD=myapipassword"

[Install]
WantedBy=multi-user.target
EOT

sudo systemctl daemon-reload

sudo systemctl restart weron-postgres
sudo systemctl restart weron-redis
sudo systemctl restart weron-signaler

sudo firewall-cmd --permanent --add-port=1337/tcp
sudo firewall-cmd --reload
```

</details>

It should now be reachable on `ws://localhost:1337/`.

To use it in production, put this signaling server behind a TLS-enabled reverse proxy such as [Caddy](https://caddyserver.com/) or [Traefik](https://traefik.io/). You may also either want to keep `API_PASSWORD` empty to disable the management API completely or use OpenID Connect to authenticate instead; for more information, see the [signaling server reference](#signaling-server). You can also embed the signaling server in your own application using it's [Go API](https://pkg.go.dev/github.com/pojntfx/weron/pkg/wrtcsgl).

### 2. Manage Communities with `weron manager`

While it is possible to create ephermal communities on a signaling server without any kind of authorization, you probably want to create a persistent community for most applications. Ephermal communities get created and deleted automatically as clients join or leave, persistent communities will never get deleted automatically. You can manage these communities using the manager CLI.

If you want to work on your self-hosted signaling server, first set the remote address:

```shell
$ export WERON_RADDR='http://localhost:1337/'
```

Next, set the API password using the `API_PASSWORD` env variable:

```shell
$ export API_PASSWORD='myapipassword'
```

If you use OIDC to authenticate, you can instead set the API password using [goit](https://github.com/pojntfx/goit) like so:

```shell
$ export OIDC_CLIENT_ID='Ab7OLrQibhXUzKHGWYDFieLa2KqZmFzb' OIDC_ISSUER='https://pojntfx.eu.auth0.com/' OIDC_REDIRECT_URL='http://localhost:11337'
$ export API_KEY="$(goit)"
```

If we now list the communities, we see that none currently exist:

```shell
$ weron manager list
id,clients,persistent
```

We can create a persistent community using `weron create`:

```shell
$ weron manager create --community mycommunity --password mypassword
id,clients,persistent
mycommunity,0,true
```

It is also possible to delete communities using `weron delete`, which will also disconnect all joined peers:

```shell
$ weron manager delete --community mycommunity
```

For more information, see the [manager reference](#manager). You can also embed the manager in your own application using it's [Go API](https://pkg.go.dev/github.com/pojntfx/weron/pkg/wrtcmgr).

### 3. Test the System with `weron chat`

If you want to work on your self-hosted signaling server, first set the remote address:

```shell
$ export WERON_RADDR='ws://localhost:1337/'
```

The chat is an easy way to test if everything is working correctly. To join a chatroom, run the following:

```shell
$ weron chat --community mycommunity --password mypassword --key mykey --names user1,user2,user3 --channels one,two,three
```

On another peer, run the following (if your signaling server is public, you can run this anywhere on the planet):

```shell
$ weron chat --community mycommunity --password mypassword --key mykey --names user1,user2,user3 --channels one,two,three
.wss://weron.herokuapp.com/
user2!
+user1@one
+user1@two
+user1@three
user2>
```

You can now start sending and receiving messages or add new peers to your chatroom to test the network.

For more information, see the [chat reference](#chat). You can also embed the chat in your own application using it's [Go API](https://pkg.go.dev/github.com/pojntfx/weron/pkg/wrtcchat).

### 4. Measure Latency with `weron utility latency`

An insightful metric of your network is it's latency, which you can measure with this utility; think of this as `ping`, but for WebRTC. First, start the latency measurement server like so:

```shell
$ weron utility latency --community mycommunity --password mypassword --key mykey --server
```

On another peer, launch the client, which should start measuring the latency immediately; press <kbd>CTRL</kbd> <kbd>C</kbd> to stop it and get the total statistics:

```shell
$ weron utility latency --community mycommunity --password mypassword --key mykey
# ...
128 B written and acknowledged in 110.111µs
128 B written and acknowledged in 386.12µs
128 B written and acknowledged in 310.458µs
128 B written and acknowledged in 335.341µs
128 B written and acknowledged in 264.149µs
^CAverage latency: 281.235µs (5 packets written) Min: 110.111µs Max: 386.12µs
```

For more information, see the [latency measurement utility reference](#latency-measurement-utility). You can also embed the utility in your own application using it's [Go API](https://pkg.go.dev/github.com/pojntfx/weron/pkg/wrtcltc).

### 5. Measure Throughput with `weron utility throughput`

If you want to transfer large amounts of data, your network's throughput is a key characteristic. This utility allows you to measure this metric between two nodes; think of it as `iperf`, but for WebRTC. First, start the throughput measurement server like so:

```shell
$ weron utility throughput --community mycommunity --password mypassword --key mykey --server
```

On another peer, launch the client, which should start measuring the throughput immediately; press <kbd>CTRL</kbd> <kbd>C</kbd> to stop it and get the total statistics:

```shell
$ weron utility throughput --community mycommunity --password mypassword --key mykey
# ...
97.907 MB/s (783.253 Mb/s) (50 MB read in 510.690403ms)
64.844 MB/s (518.755 Mb/s) (50 MB read in 771.076908ms)
103.360 MB/s (826.881 Mb/s) (50 MB read in 483.745832ms)
89.335 MB/s (714.678 Mb/s) (50 MB read in 559.692495ms)
85.582 MB/s (684.657 Mb/s) (50 MB read in 584.233931ms)
^CAverage throughput: 74.295 MB/s (594.359 Mb/s) (250 MB written in 3.364971672s) Min: 64.844 MB/s Max: 103.360 MB/s
```

For more information, see the [throughput measurement utility reference](#throughput-measurement-utility). You can also embed the utility in your own application using it's [Go API](https://pkg.go.dev/github.com/pojntfx/weron/pkg/wrtcthr).

### 6. Create a Layer 3 (IP) Overlay Network with `weron vpn ip`

If you want to join multiple nodes into a overlay network, the IP VPN is the best choice. It works in a similar way to i.e. Tailscale/WireGuard and can either dynamically allocate an IP address from a CIDR notation or statically assign one for you. On Windows, make sure to install [TAP-Windows](https://duckduckgo.com/?q=TAP-Windows&t=h_&ia=web) first. To get started, launch the VPN on the first peer:

```shell
$ sudo weron vpn ip --community mycommunity --password mypassword --key mykey --ips 2001:db8::1/64,192.0.2.1/24
{"level":"info","addr":"wss://weron.herokuapp.com/","time":"2022-05-06T22:20:51+02:00","message":"Connecting to signaler"}
{"level":"info","id":"[\"2001:db8::6a/64\",\"192.0.2.107/24\"]","time":"2022-05-06T22:20:56+02:00","message":"Connected to signaler"}
```

On another peer, launch the VPN as well:

```shell
$ sudo weron vpn ip --community mycommunity --password mypassword --key mykey --ips 2001:db8::1/64,192.0.2.1/24
{"level":"info","addr":"wss://weron.herokuapp.com/","time":"2022-05-06T22:22:30+02:00","message":"Connecting to signaler"}
{"level":"info","id":"[\"2001:db8::b9/64\",\"192.0.2.186/24\"]","time":"2022-05-06T22:22:36+02:00","message":"Connected to signaler"}
{"level":"info","id":"[\"2001:db8::6a/64\",\"192.0.2.107/24\"]","time":"2022-05-06T22:22:36+02:00","message":"Connected to peer"}
```

You can now communicate between the peers:

```shell
$ ping 2001:db8::b9
PING 2001:db8::b9(2001:db8::b9) 56 data bytes
64 bytes from 2001:db8::b9: icmp_seq=1 ttl=64 time=1.07 ms
64 bytes from 2001:db8::b9: icmp_seq=2 ttl=64 time=1.36 ms
64 bytes from 2001:db8::b9: icmp_seq=3 ttl=64 time=1.20 ms
64 bytes from 2001:db8::b9: icmp_seq=4 ttl=64 time=1.10 ms
^C
--- 2001:db8::b9 ping statistics ---
4 packets transmitted, 4 received, 0% packet loss, time 3002ms
rtt min/avg/max/mdev = 1.066/1.180/1.361/0.114 ms
```

If you temporarly loose the network connection, the network topology changes etc. it will automatically reconnect. For more information and limitations on proprietary operating systems like macOS, see the [IP VPN reference](#layer-3-ip-overlay-networks). You can also embed the utility in your own application using it's [Go API](https://pkg.go.dev/github.com/pojntfx/weron/pkg/wrtcip).

### 7. Create a Layer 2 (Ethernet) Overlay Network with `weron vpn ethernet`

If you want more flexibility or work on non-IP networks, the ethernet VPN is a good choice. It works in a similar way to `n2n` or ZeroTier. Due to API restrictions, this VPN type [is not available on macOS](https://support.apple.com/guide/deployment/system-and-kernel-extensions-in-macos-depa5fb8376f/web); use [Asahi Linux](https://asahilinux.org/), a computer that respects your freedoms or the layer 3 (IP) VPN instead. To get started, launch the VPN on the first peer:

```shell
$ sudo weron vpn ethernet --community mycommunity --password mypassword --key mykey
{"level":"info","addr":"wss://weron.herokuapp.com/","time":"2022-05-06T22:42:10+02:00","message":"Connecting to signaler"}
{"level":"info","id":"fe:60:a5:8b:81:36","time":"2022-05-06T22:42:11+02:00","message":"Connected to signaler"}
```

If you want to add an IP address to the TAP interface, do so with `iproute2` or your OS tools:

```shell
$ sudo ip addr add 192.0.2.1/24 dev tap0
$ sudo ip addr add 2001:db8::1/32 dev tap0
```

On another peer, launch the VPN as well:

```shell
$ sudo weron vpn ethernet --community mycommunity --password mypassword --key mykey
{"level":"info","addr":"wss://weron.herokuapp.com/","time":"2022-05-06T22:52:56+02:00","message":"Connecting to signaler"}
{"level":"info","id":"b2:ac:ae:b6:32:8c","time":"2022-05-06T22:52:57+02:00","message":"Connected to signaler"}
{"level":"info","id":"fe:60:a5:8b:81:36","time":"2022-05-06T22:52:57+02:00","message":"Connected to peer"}
```

And add the IP addresses:

```shell
$ sudo ip addr add 192.0.2.2/24 dev tap0
$ sudo ip addr add 2001:db8::2/32 dev tap0
```

You can now communicate between the peers:

```shell
$ ping 2001:db8::2
PING 2001:db8::2(2001:db8::2) 56 data bytes
64 bytes from 2001:db8::2: icmp_seq=1 ttl=64 time=1.20 ms
64 bytes from 2001:db8::2: icmp_seq=2 ttl=64 time=1.14 ms
64 bytes from 2001:db8::2: icmp_seq=3 ttl=64 time=1.24 ms
^C
--- 2001:db8::2 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2002ms
rtt min/avg/max/mdev = 1.136/1.193/1.239/0.042 ms
```

If you temporarly loose the network connection, the network topology changes etc. it will automatically reconnect. You can also embed the utility in your own application using it's [Go API](https://pkg.go.dev/github.com/pojntfx/weron/pkg/wrtceth).

### 8. Write your own protocol with `wrtcconn`

It is almost trivial to build your own dstributed applications with weron, similarly to how [PeerJS](https://peerjs.com/) works. Here is the core logic behind a simple echo example:

```go
// ...
for {
	select {
	case id := <-ids:
		log.Println("Connected to signaler", id)
	case peer := <-adapter.Accept():
		log.Println("Connected to peer", peer.PeerID, "and channel", peer.ChannelID)

		go func() {
			defer func() {
				log.Println("Disconnected from peer", peer.PeerID, "and channel", peer.ChannelID)
			}()

			reader := bufio.NewScanner(peer.Conn)
			for reader.Scan() {
				log.Printf("%s", reader.Bytes())
			}
		}()

		go func() {
			for {
				if _, err := peer.Conn.Write([]byte("Hello!\n")); err != nil {
					return
				}

				time.Sleep(time.Second)
			}
		}()
	}
}
```

You can either use the [minimal adapter](https://pkg.go.dev/github.com/pojntfx/weron/pkg/wrtcconn#Adapter) or the [named adapter](https://pkg.go.dev/github.com/pojntfx/weron/pkg/wrtcconn#NamedAdapter); the latter negotiates a username between the peers, while the former does not check for duplicates. For more information, check out the [Go API](https://pkg.go.dev/github.com/pojntfx/weron) and take a look at the other utilities and services in the package for examples.

🚀 **That's it!** We hope you enjoy using weron.

## Reference

### Command Line Arguments

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

<details>
  <summary>Expand subcommand reference</summary>

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

</details>

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
