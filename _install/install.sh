#!/bin/sh

set -e

command -v jq >/dev/null 2>&1 || {
	echo "Please install jq"
	exit 1
}

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
	# darwin/amd64: Darwin axetroydeMacBook-Air.local 20.5.0 Darwin Kernel Version 20.5.0: Sat May  8 05:10:33 PDT 2021; root:xnu-7195.121.3~9/RELEASE_X86_64 x86_64
	# linux/amd64: Linux test-ubuntu1804 5.4.0-42-generic #46~18.04.1-Ubuntu SMP Fri Jul 10 07:21:24 UTC 2020 x86_64 x86_64 x86_64 GNU/Linux
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
	# darwin: Darwin
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
file_name="nex_${binary_version}_${os}_${arch}" # the file name should be download
asset_uri="https://github.com/synadia-io/nex/releases/download/${binary_version}/${file_name}"

downloadFolder="${TMPDIR:-/tmp}"
mkdir -p ${downloadFolder}              # make sure download folder exists
downloaded_file="${downloadFolder}/nex" # the file path should be download
executable_folder="/usr/local/bin"      # Eventually, the executable file will be placed here

echo "[1/3] Download ${asset_uri} to ${downloadFolder}"
rm -f ${downloaded_file}
curl --silent --fail --location --output "${downloaded_file}" "${asset_uri}"

echo "[2/3] Install nex to the ${executable_folder}"
mv ${downloaded_file} ${executable_folder}
exe=${executable_folder}/nex
chmod +x ${exe}

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
