# To Run

```shell
└─❯ go run . node up --logger.with-pid \
--nats.context default --logger.short \
--logger.level debug \
--node-seed SNADXGZUCPRH3IN46BERGG42QCOMW73MVY64RDXZLS3DETNGEDDNFLMWVA
```

### Sample Config

```json
{
  "agents": [
    {
      "uri": "/home/crash/Development/synadia-io/nexlet.docker/cmd/nexlet.docker",
      "argv": ["run"]
    }
  ]
}
```

To use, and a --config to the above command
