package dialer

import (
	"crypto/tls"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

func LoadPEMBlock(dir, from string) (block []byte, err error) {
	if strings.HasPrefix(from, "-----BEGIN") {
		block = []byte(from)
	} else if strings.HasPrefix(from, "0x") {
		block, err = hex.DecodeString(strings.TrimPrefix(from, "0x"))
	} else {
		if !filepath.IsAbs(from) {
			from = filepath.Join(dir, from)
		}
		block, err = os.ReadFile(from)
	}
	return
}

func LoadX509KeyPair(dir, cert, key string) (tls.Certificate, error) {
	certPEMBlock, err := LoadPEMBlock(dir, cert)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEMBlock, err := LoadPEMBlock(dir, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certPEMBlock, keyPEMBlock)
}
