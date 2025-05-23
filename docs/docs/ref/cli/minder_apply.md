---
title: minder apply
---
## minder apply

Apply multiple minder resources

### Synopsis

The apply subcommand lets you apply multiple Minder resources at once.

```
minder apply [flags]
```

### Options

```
  -f, --file strings   Input file or directory
  -h, --help           help for apply
```

### Options inherited from parent commands

```
      --config string            Config file (default is $PWD/config.yaml)
      --grpc-host string         Server host (default "api.custcodian.dev")
      --grpc-insecure            Allow establishing insecure connections
      --grpc-port int            Server port (default 443)
      --identity-client string   Identity server client ID (default "minder-cli")
      --identity-url string      Identity server issuer URL (default "https://auth.custcodian.dev")
  -v, --verbose                  Output additional messages to STDERR
```

### SEE ALSO

* [minder](minder.md)	 - Minder controls the hosted minder service

