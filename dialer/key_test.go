package dialer

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"testing"
)

func TestCert(t *testing.T) {
	if _, xerr := os.Stat("../certs/rootCA.crt"); os.IsNotExist(xerr) {
		cmd := exec.Command("bash", "-c", "./gen.sh")
		cmd.Dir = "../certs"
		err := cmd.Run()
		if err != nil {
			t.Error(err)
			return
		}
	}
	cert1, err := LoadPEMBlock("../certs/", "server.crt")
	if err != nil {
		t.Error(err)
		return
	}
	cert2, err := LoadPEMBlock("../certs", string(cert1))
	if err != nil || !bytes.Equal(cert1, cert2) {
		t.Error(err)
		return
	}
	cert3, err := LoadPEMBlock("../certs", "0x"+hex.EncodeToString(cert1))
	if err != nil || !bytes.Equal(cert1, cert3) {
		t.Error(err)
		return
	}

	_, err = LoadX509KeyPair("../certs", "server.crt", "server.key")
	if err != nil {
		t.Error(err)
	}

	_, err = LoadX509KeyPair("../certs", "server.xxx", "server.key")
	if err == nil {
		t.Error(err)
	}

	_, err = LoadX509KeyPair("../certs", "server.crt", "server.xxx")
	if err == nil {
		t.Error(err)
	}
}
