# Incrr Core

#### A Distributed Atomic Incrementing Counter

Incrr Core is an distributed system that can atomically increment numbers, a production version can be found at [incrr.io](https://incrr.io).

- [Features](features)
- [Installation](installation)
- [Configuration](configuring-incrr-core)
- [Contributing](contributing)
- [Licence](licence)

##  Features

- Multiple database backends
- Disributed atomicity 
- Configuraturable

## Installation

To build **Incrr Core** from source requires:

- Go 1.9 or later

To run **Incrr Core** tests requires:

- Docker 17.12 or later

### Steps

1. Clone this repository, i.e: `clone `
2. Modify any config settings in the `config.toml` configuration file.
3. Run `go generate generate.go` to create your `build.go` and `build_internal.go` files. _The build number is expected to increment based on global builds._
4. Run`docker-compose up -d` to start a new compose server with the proper backend 

Once you've done these steps you should have the code running an a docker container on your machine. You can see how the distribution works, by looking at the logs

## Configuring Incrr Core

Below is an annoted TOML config file.

```toml
environment = "dev"      # optional, default = "dev"
show_config = true       # optional, show config on startup

[server]
force_http = true        # optional, the server is https by default

[server.api]
domains = ["localhost"]  # the whitelist of domain names to accept requests for

[datastore]
use_remote_db = "crdb"   # optional, "crdb" or "mysql" is valid. If ommited will use
                         #   the first registered datastore lexagraphlly sorted.
                         # crdb:  cockroachDB
                         # mysql: MySQL Database
                         
[datastore.remote.crdb]  # use the datastore abbrevation for configuration
dsn = "<dsn>"            # required, the DSN to use

[groupcache]
self = "127.0.0.1"       # optional
http_pool = ["http://"]  # required, the server ip addresses or names to use in
                         #    the groupcache pool

```



## Contributing

Contributions are more than welcome. If you've found a bug, or have a feature request, please create an issue.



## Licence

Incrr Core is available under the [MIT License](https://opensource.org/licenses/MIT).

Copyright (c) 2018 Nika Jones [copyright@incrr.io](mailto:copyright@incrr.io) All Rights Reserved.