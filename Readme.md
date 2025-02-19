# Yet Another Modbus Proxy (yamprox)

## Background

Some appliances only support a single Modbus client at a time (notably, some power inverters for solar energy).
This small program connects to the server and acts as a server for multiple clients.
All requests are forwarded to the original server.

## Usage

You need to specify the server name and port number for which yamprox should act as a proxy for.
Example:

    yamprox my-inverter.internal.network:502

(502 is the default Modbus port)
By default, the proxy listens on port 2502.
You can specify any other port with the `--port` option.
It is not recommended to run `yamprox` as root.
If you need to listen on port 502, I recomment to run it in a Docker container.

The basic usage is:

    NAME:
       yamprox - yamprox <server:port>
                 Creates a proxy for a modbus server.

    USAGE:
       yamprox [global options] command [command options]

    COMMANDS:
       help, h  Shows a list of commands or help for one command

    GLOBAL OPTIONS:
       --port value       port number to listen on (default: 2502)
       --interface value  interface to listen on
       --debug            debug logging (default: false)
       --help, -h         show help


## Docker Image

### Starting the service with Docker Compose

An example for a simple `docker-compose.yml` file that uses the pre-build image is:

    services:
      yamprox:
        image: dplagge/yamprox:latest
        command: "my-inverter.internal.network:502"
        ports:
          - 502:2502
        restart: always

You can start the service with

    docker-compose up -d

### Building the Docker image

You can build a Docker image yourself with

    docker build . -t yamprox

Built Docker images can be found at https://hub.docker.com/repository/docker/dplagge/yamprox.

## License

The program is licensed under the GNU General Public License, version 3.

Copyright 2024-2025 Daniel Plagge
