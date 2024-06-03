#!/bin/sh

set -e

command -v jq >/dev/null 2>&1 || {
	echo "Please install jq"
	exit 1
}

with_agent=false
while getopts ":with_agent:" opt; do
	case ${opt} in
	a)
		with_agent=true
		;;
	esac
done

get_version() {
	echo $(
		command curl -L -s \
			-H "Accept: application/vnd.github+json" \
			-H "X-GitHub-Api-Version: 2022-11-28" \
			https://api.github.com/repos/synadia-io/nex/releases/latest |
			command jq -r '.name'
	)
}

get_arch() {
	a=$(uname -m)
	case ${a} in
	"x86_64" | "amd64")
		echo "amd64"
		;;
	"i386" | "i486" | "i586")
		echo "386"
		;;
	"aarch64" | "arm64" | "arm")
		echo "arm64"
		;;
	"mips64el")
		echo "mips64el"
		;;
	"mips64")
		echo "mips64"
		;;
	"mips")
		echo "mips"
		;;
	*)
		echo ${NIL}
		;;
	esac
}

get_os() {
	echo $(uname -s | awk '{print tolower($0)}')
}

echo "
  	   ▐ ▄ ▄▄▄ .▐▄• ▄ 
	  •█▌▐█▀▄.▀· █▌█▌▪
	  ▐█▐▐▌▐▀▀▪▄ ·██· 
	  ██▐█▌▐█▄▄▌▪▐█·█▌
	  ▀▀ █▪ ▀▀▀ •▀▀ ▀▀
"

os=$(get_os)
arch=$(get_arch)
binary_version=$(get_version)
file_name="nex_${binary_version}_${os}_${arch}"
agent_file_name="nex-agent_${binary_version}_${os}_${arch}"
asset_uri="https://github.com/synadia-io/nex/releases/download/${binary_version}/${file_name}"
agent_asset_uri="https://github.com/synadia-io/nex/releases/download/${binary_version}/${agent_file_name}"

downloadFolder="${TMPDIR:-/tmp}"
mkdir -p ${downloadFolder}
downloaded_file="${downloadFolder}/nex"
downloaded_agent_file="${downloadFolder}/nex-agent"
executable_folder="/usr/local/bin"

echo "[1/3] Download ${asset_uri} to ${downloadFolder}"
rm -f ${downloaded_file}
curl --silent --fail --location --output "${downloaded_file}" "${asset_uri}"
if [ "$with_agent" = true ]; then
	curl --silent --fail --location --output "${downloaded_agent_file}" "${agent_asset_uri}"
fi

echo "[2/3] Install nex to ${executable_folder}"
mv ${downloaded_file} ${executable_folder}
exe=${executable_folder}/nex
chmod +x ${exe}
if [ "$with_agent" = true ]; then
	mv ${downloaded_agent_file} ${executable_folder}
	chmod +x ${executable_folder}/nex-agent
fi

echo "[3/3] Check environment variables"
echo ""
echo "nex was installed successfully to ${exe}"
if command -v nex --version >/dev/null; then
	echo "Run 'nex --help' to get started"
else
	echo "Manually add the directory to your \$HOME/.bash_profile (or similar)"
	echo "  export PATH=${executable_folder}:\$PATH"
	echo "Run '$exe_name --help' to get started"
fi

exit 0
