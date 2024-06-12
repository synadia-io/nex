package templates

var FcnetConfig string = `{
  "name": "{{.NetworkName}}",
  "cniVersion": "0.4.0",
  "plugins": [
    {
      "type": "ptp",
      "ipMasq": true,
      {{ if .Nameservers }}"dns": { "nameservers": {{ AddQuotes .Nameservers }} },{{ end }}
      "ipam": {
        "type": "host-local",
        "subnet": "{{.Subnet}}",
        "resolvConf": "/etc/resolv.conf"
      }
    },
    {
      "type": "tc-redirect-tap"
    }
  ]
}`
