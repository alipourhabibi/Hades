# Hades Schema Registry

Hades is an open source [Buf](https://github.com/bufbuild/buf) compatible schema registry.

## Getting started
```sh
docker compose -f development/docker-compose-dev.yaml up -d
go run ./cmd/hades/. serve --config config/dev.yaml
```

## Development env
Add `127.0.0.1 example.com` to /etc/hosts
Run the following command to let you run app on port 443
```bash
echo "net.ipv4.ip_unprivileged_port_start=0" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```
Let self-signed ssl be accepted
Install [mkcert](https://github.com/FiloSottile/mkcert)
and run the following
```bash
mkcert -install
cd config && mkcert example.com && cd ..
```

And run the application

### Initialize development environment
cd into development folder
```bash
./install_tools.sh
./init.sh
```

Now you have a user called `googleapis` with password `googleapis` \
And a module with `googleapis` name that has some protos in it alongside a project 
that uses this as its dependency in `protos/simpleproject`

I coded this fast, focusing on getting things working first rather than following the best practices from day one. So there’s room for improvement, refactoring, and optimizations.

If you spot something that could be improved, better architecture, cleaner code, or best practices, feel free to open an issue or a PR. Your contributions are highly appreciated!

Enjoy hacking on Hades!  

### Features ready to tests:
1. buf dep update
2. buf push
Navigate to `development/protos/simpleproject` and run:  

```bash
buf dep update  # Updates the googleapis dependency
buf generate    # Generates codes
```
NOTE: the SKD module is not yet developed.

You can also change the protos in `development/protos/googleapis` and then push them using `buf push`

#### Licensing
This project includes files from Google APIs for development purposes, which are licensed under the Apache License 2.0. See the `LICENSE` file in `development/protos/googleapis` for details.

### Docker
```bash
docker run -v ./config/config.yaml:/app/config/config.yaml DOCKER_IMAGE:TAG
```
Add the tls volume files the port and other configs based on your config file in config/config.yaml file
