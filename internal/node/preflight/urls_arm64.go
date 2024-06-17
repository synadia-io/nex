package preflight

const (
	vmLinuxKernelURL    = "https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.5/aarch64/vmlinux-5.10.186"
	vmLinuxKernelSHA256 = ""

	cniPluginsTarballURL    = "https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-arm64-v1.3.0.tgz"
	cniPluginsTarballSHA256 = "https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-arm64-v1.3.0.tgz.sha256"

	tcRedirectCNIPluginURL    = "https://github.com/jordan-rash/tc-redirect-tap/releases/download/v0.0.3/tc-redirect-tap-arm64"
	tcRedirectCNIPluginSHA256 = "https://github.com/jordan-rash/tc-redirect-tap/releases/download/v0.0.3/tc-redirect-tap-arm64.sha256"

	firecrackerTarballURL    = "https://github.com/firecracker-microvm/firecracker/releases/download/v1.5.0/firecracker-v1.5.0-aarch64.tgz"
	firecrackerTarballSHA256 = "https://github.com/firecracker-microvm/firecracker/releases/download/v1.5.0/firecracker-v1.5.0-aarch64.tgz.sha256.txt"

	rootfsGzipURLTemplate    = "https://github.com/synadia-io/nex/releases/download/%s/rootfs.linux.arm64.ext4.gz"
	rootfsGzipSHA256Template = "https://github.com/synadia-io/nex/releases/download/%s/rootfs.linux.arm64.ext4.gz.sha256"

	nexAgentLinuxURLTemplate         = "https://github.com/synadia-io/nex/releases/download/%s/nex-agent_%s_linux_arm64"
	nexAgentLinuxURLTemplateSHA256   = "https://github.com/synadia-io/nex/releases/download/%s/nex-agent_%s_linux_arm64.sha256"
	nexAgentDarwinTemplate           = "https://github.com/synadia-io/nex/releases/download/%s/nex-agent_%s_darwin_arm64"
	nexAgentDarwinURLTemplateSHA256  = "https://github.com/synadia-io/nex/releases/download/%s/nex-agent_%s_darwin_arm64.sha256"
	nexAgentWindowsTemplate          = "https://github.com/synadia-io/nex/releases/download/%s/nex-agent_%s_windows_arm64.exe"
	nexAgentWindowsURLTemplateSHA256 = "https://github.com/synadia-io/nex/releases/download/%s/nex-agent_%s_windows_arm64.exe.sha256"
)
