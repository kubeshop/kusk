!/usr/bin/env sh

set -e

if [ ! -z "${DEBUG}" ]; 
then set -x 
fi

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
        if [ -z "$KGW_VERSION" ]
        then
                local version=`curl -s https://api.github.com/repos/kubeshop/kgw/releases/latest 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'`
                echo "https://github.com/kubeshop/kgw/releases/download/${version}/kgw_${version}_${os}_${arch}.tar.gz"
        else
                echo "https://github.com/kubeshop/kgw/releases/download/${KGW_VERSION}/kgw_${KGW_VERSION}_${os}_${arch}.tar.gz"
        fi
}

echo "Downloading kgw from URL: $(_download_url)"
curl -sSLf $(_download_url) > kgw.tar.gz
tar -xzf kgw.tar.gz kgw
rm kgw.tar.gz
mv kgw /usr/local/bin/kgw
echo "kgw installed in /usr/local/bin/kgw"

