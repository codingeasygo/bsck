package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
)

const scriptSSH = `#!/bin/bash
if [ "$#" -lt 1 ]; then
  echo "Usage: $0 URI"
  exit 1
fi
connect=$(echo '$1'| sed 's/->/_/g;s/:/_/g;s/\\./_/g;s/:/_/g')
args=()
for a in ${@:2}
do
    a=$(echo $a| sed "s/bshost/$connect/g;")
    args+=($a)
done
ssh -o ProxyCommand="bsconsole --proxy \"$1\"" ${args[@]}
`

const scriptSCP = `#!/bin/bash
if [ "$#" -lt 3 ]; then
  echo "Usage: $0 URI path path"
  exit 1
fi
connect=$(echo '$1'| sed 's/->/_/g;s/:/_/g;s/\\./_/g;s/:/_/g')
args=()
for a in ${@:2}
do
    a=$(echo $a| sed "s/bshost/$connect/g;")
    args+=($a)
done
scp -o ProxyCommand="bsconsole --proxy \"$1\"" ${args[@]}
`

const scriptSFTP = `#!/bin/bash
if [ "$#" -lt 2 ]; then
  echo "Usage: $0 URI path"
  exit 1
fi
connect=$(echo '$1'| sed 's/->/_/g;s/:/_/g;s/\\./_/g;s/:/_/g')
args=()
for a in ${@:2}
do
    a=$(echo $a| sed "s/bshost/$connect/g;")
    args+=($a)
done
sftp -o ProxyCommand="bsconsole --proxy \"$1\"" ${args[@]}
`

func mklink(link, target string) (err error) {
	var runner string
	var args []string
	if runtime.GOOS == "windows" {
		runner = "cmd"
		args = []string{"/c", "mklink", link + ".exe", target}
	} else {
		runner = "bash"
		args = []string{"-c", "ln", "-s", link, target}
	}
	cmd := exec.Command(runner, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		fmt.Printf("link %v %v fail with %v\n", link, target, err)
	}
	return
}

func removeFile(target string) (err error) {
	fmt.Printf("remove %v\n", target)
	if _, xerr := os.Stat(target); xerr == nil {
		err = os.Remove(target)
		if err != nil {
			fmt.Printf("remove %v fail with %v\n", target, err)
		}
	}
	if _, xerr := os.Stat(target + ".exe"); xerr == nil {
		err = os.Remove(target + ".exe")
		if err != nil {
			fmt.Printf("remove %v.exe fail with %v\n", target, err)
		}
	}
	return
}

func scriptWrite(filename, script string) (err error) {
	fmt.Printf("write script to %v\n", filename)
	err = ioutil.WriteFile(filename, []byte(script), 0x744)
	if err != nil {
		fmt.Printf("write script to %v fail with %v\n", filename, err)
	}
	return
}
