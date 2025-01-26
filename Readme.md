# Yet Another Modbus Proxy (yamprox)

## Background

Some appliances only support a single Modbus client at a time (notably, some power inverters for solar energy).
This small program connects to the server and acts as a server for multiple clients.
All requests are forwarded to the original server.

## Docker Image

A docker image can be built with

    docker build . -t yamprox

## License

The program is licensed under the GNU General Public License, version 3.
Copyright 2024-2025 Daniel Plagge
