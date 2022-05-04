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

🚧 This project is a work-in-progress! Instructions will be added as soon as it is usable. 🚧

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
