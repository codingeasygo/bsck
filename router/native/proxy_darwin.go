package native

import (
	"os"
	"os/exec"
	"strings"
)

var ChangeProxyScript = `
#!/bin/bash

services=$(networksetup -listnetworkserviceorder | grep 'Hardware Port')

while read line; do
    sname=$(echo $line | awk -F  "(, )|(: )|[)]" '{print $2}')
    sdev=$(echo $line | awk -F  "(, )|(: )|[)]" '{print $4}')
    #echo "Current service: $sname, $sdev, $currentservice"
    if [ -n "$sdev" ]; then
        ifout="$(ifconfig $sdev 2>/dev/null)"
        echo "$ifout" | grep 'status: active' > /dev/null 2>&1
        rc="$?"
        if [ "$rc" -eq 0 ]; then
            currentservice="$sname"
            currentdevice="$sdev"
            currentmac=$(echo "$ifout" | awk '/ether/{print $2}')

            # may have multiple active devices, so echo it here
            echo "$currentservice, $currentdevice, $currentmac"
        fi
    fi
done <<< "$(echo "$services")"

if [ -z "$currentservice" ]; then
    >&2 echo "Could not find current service"
    exit 1
fi

case "$1" in
    auto)
    networksetup -setautoproxyurl "$currentservice" "$2"
    networksetup -setautoproxystate "$currentservice" on
    networksetup -setsocksfirewallproxystate "$currentservice" off
    ;;
    global)
    networksetup -setsocksfirewallproxy "$currentservice" "$2" "$3"
    networksetup -setautoproxystate "$currentservice" off
    networksetup -setsocksfirewallproxystate "$currentservice" on
    ;;
    manual)
    networksetup -setautoproxystate "$currentservice" off
    networksetup -setsocksfirewallproxystate "$currentservice" off
    ;;
    *)
    >&2 echo "not supported mode $1"
    exit 1
    ;;
esac
`

func ChangeProxyModeNative(args ...string) (message string, err error) {
	err = os.WriteFile("/tmp/networksetup.sh", []byte(strings.TrimSpace(ChangeProxyScript)+"\n"), 0700)
	if err != nil {
		return
	}
	out, err := exec.Command("/tmp/networksetup.sh", args...).CombinedOutput()
	message = string(out)
	return
}
