package native

import (
	"os/exec"
	"runtime"
)

func sysproxyPath() string {
	var runner = ExecDir + "\\sysproxy.exe"
	if runtime.GOARCH == "amd64" {
		runner = ExecDir + "\\sysproxy64.exe"
	}
	return runner
}

var networksetupPath = sysproxyPath()

func ChangeProxyModeNative(args ...string) (message string, err error) {
	var cmd *exec.Cmd
	switch args[0] {
	case "auto":
		cmd = exec.Command(networksetupPath, "pac", args[1])
	case "global":
		cmd = exec.Command(networksetupPath, "global", args[1]+":"+args[2])
	default:
		cmd = exec.Command(networksetupPath, "off")
	}
	out, err := cmd.CombinedOutput()
	message = string(out)
	return
}
