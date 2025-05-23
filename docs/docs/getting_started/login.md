---
title: Logging in to Minder
sidebar_position: 20
---

To start using Minder, you must first log in. Logging in to a Minder server for
the first time will create your account.

By default as of 0.0.89, the Minder CLI will log in to `api.custcodian.dev`, a
[free public instance provided by Custcodian](../../#minder-public-instance). If
you have your own hosted instance of Minder, you can log in to that server
instead.

## Prerequisites

Before you can log in, you must have
[installed the `minder` CLI application](install_cli).

## Logging in to the Minder Public Instance

The `minder` CLI defaults to using the hosted Custcodian public instance. When
using the hosted environment, you do not need to set up a server; you simply log
in to the Custcodian authentication instance using your GitHub credentials.

You can use the Custcodian public instance by running:

```bash
minder auth login
```

A new browser window will open and you will be prompted to log in to the
Custcodian instance using your GitHub credentials. Once you have logged in,
proceed to enroll your credentials in Minder.

## Logging in to your own Minder instance

To log in to a Minder server which you are running (self-hosted), you will need
to know the URL of the Minder server and of the Keycloak instance used for
authentication. If you are using
[`docker compose` to run Minder on your local machine](../run_minder_server/run_the_server.md),
these addresses will be `localhost:8090` for Minder and `localhost:8081` for
Keycloak.

You can log in to Minder using:

```bash
minder auth login --grpc-host localhost --grpc-port 8090 --identity-url http://localhost:8081
```

Your web browser will be opened to log in to Keycloak, and then a banner will be
printed an

```plain
    You have successfully logged in.

 Here are your details:

┌────────────────────────────────────────────────┐
│ Key                    Value                   │
│ Project Name           KeyCloak-username       │
│ Minder Server          localhost:8090          │
└────────────────────────────────────────────────┘
Your access credentials have been saved to ~/.config/minder/credentials.json
```

Once you have logged in, you'll want to
[enroll your GitHub credentials in Minder so that it can act on your behalf](./enroll_provider.md).
