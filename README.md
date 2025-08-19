# Hades Schema Registry

Hades is an open source [Buf](https://github.com/bufbuild/buf) compatible schema registry.

## Getting started
```bash
docker compose -f development/docker-compose-dev.yaml up -d
go run ./cmd/hades/. serve --config config/dev.yaml
```

## Development env
For development purposes you need to follow these steps:

Add `example.com` to hosts
```bash
sudo sh -c 'echo "127.0.0.1 example.com" >> /etc/hosts'
```

Allow the app to run on port 443
```bash
echo "net.ipv4.ip_unprivileged_port_start=0" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

The Buf CLI expects the app to run on port 443 with TLS enabled.
Set up TLS certificates:
Install [mkcert](https://github.com/FiloSottile/mkcert) for self-signed certificates
Generate certificates by running:
```bash
mkcert -install
cd config && mkcert example.com && cd ..
```

You also need to do the migrations:
```bash
make migrate-up
```

Copy the sample config file and fill it with proper configs.
```bash
cp config/config.sample.yaml config/dev.yaml
```

And run the application
```bash
go run ./cmd/hades/. serve --config config/dev.yaml
```

### Initialize development environment
cd into development folder
```bash
./install_tools.sh
./init.sh
```

- Creates a user: googleapis / googleapis
- Creates a module: googleapis with sample protos
- Sets up a project in protos/simpleproject that depends on the googleapis module

I coded this fast, focusing on getting things working first rather than following the best practices from day one. So there’s room for improvement, refactoring, and optimizations.

If you spot something that could be improved, better architecture, cleaner code, or best practices, feel free to open an issue or a PR. Your contributions are highly appreciated!

Enjoy hacking on Hades!  

### Features ready to tests:
1. buf dep update
2. buf push
Navigate to `development/protos/simpleproject` and run:  

```bash
buf dep update  # Updates the googleapis dependency
buf generate    # Generates code
```
NOTE: the SKD module is not yet developed.

You can also change the protos in `development/protos/googleapis` and then push them using `buf push`

#### Licensing
This project includes files from Google APIs for development purposes, which are licensed under the Apache License 2.0. See the `LICENSE` file in `development/protos/googleapis` for details.

### Docker
```bash
docker run -v ./config/config.yaml:/app/config/config.yaml DOCKER_IMAGE:TAG
```
Mount TLS certificates and adjust ports/configs according to config/config.yaml.
