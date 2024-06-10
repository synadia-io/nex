package templates

var FcnetConfig string = `{
  "name": "{{.NetworkName}}",
  "cniVersion": "0.4.0",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "br0",
      "isGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "subnet": "{{.Subnet}}",
        "resolvConf": "/etc/resolv.conf",
        "routes": [
          {
            "dst": "0.0.0.0/0"
          }
        ]
      },
      "dns": {
        "nameservers": ["127.0.0.53"],
        "domain": ".",
        "search": []
      }
    },
    {
      "type": "tc-redirect-tap"
    }
  ]
}`
