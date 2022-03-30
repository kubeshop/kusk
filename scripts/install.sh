#!/usr/bin/env sh

set -e

if [ ! -z "${DEBUG}" ]; 
then set -x 
fi

_sudo () {
    [[ $EUID = 0 ]] || set -- command sudo "$@"
    "$@"
}

_detect_arch() {
    case $(uname -m) in
    amd64|x86_64) echo "x86_64"
    ;;
    arm64|aarch64) echo "arm64"
    ;;
    i386) echo "i386"
    ;;
    *) echo "Unsupported processor architecture";
    return 1
    ;;
     esac
}

_detect_os(){
    case $(uname) in
    Linux) echo "Linux"
    ;;
    Darwin) echo "macOS"
    ;;
    Windows) echo "Windows"
    ;;
     esac
}

_download_url() {
        local arch="$(_detect_arch)"
        local os="$(_detect_os)"
        local version=$KGW_VERSION

        if [ -z "$KGW_VERSION" ]
        then
                version=`curl -s https://api.github.com/repos/kubeshop/kgw/releases/latest 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'`
        fi

        local trailedVersion=`echo $version | tr -d v`
        echo "https://github.com/kubeshop/kgw/releases/download/${version}/kgw_${trailedVersion}_${os}_${arch}.tar.gz"
}

echo "Downloading kgw from URL: $(_download_url)"
curl -sSLf $(_download_url) > kgw.tar.gz
tar -xzf kgw.tar.gz kgw
rm kgw.tar.gz

install_dir=$1
if [ "$install_dir" != "" ]; then
        mkdir -p "$install_dir"
        mv kgw "${install_dir}/kgw"
        echo "kgw installed in ${install_dir}"
        exit 0
fi

if [ "$(uname)" == "Linux" ]; then
        echo "On Linux sudo rights are needed to move the binary to /usr/local/bin, please type your password when asked"
        _sudo mv kgw /usr/local/bin/kgw
else
        mv kgw /usr/local/bin/kgw
fi

echo "kgw installed in /usr/local/bin/kgw"

