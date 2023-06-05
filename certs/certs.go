package main

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"os"
	"strings"

	"github.com/codingeasygo/util/xcrypto"
)

func main() {
	caCert, caKey, rootCert, rootKey, err := GenerateRootCA("Coding Easy GO", "bsrouter", 2048)
	if err != nil {
		panic(err)
	}
	_, serverCert, serverKey, err := GenerateWeb(caCert, caKey, "Coding Easy GO", "test.loc,www.test.loc", "127.0.0.1", 2048)
	if err != nil {
		panic(err)
	}
	ioutil.WriteFile("rootCA.crt", rootCert, os.ModePerm)
	ioutil.WriteFile("rootCA.key", rootKey, os.ModePerm)
	ioutil.WriteFile("server.crt", serverCert, os.ModePerm)
	ioutil.WriteFile("server.key", serverKey, os.ModePerm)
}

func GenerateRootCA(org string, name string, bits int) (cert *x509.Certificate, privKey *rsa.PrivateKey, certPEM, privPEM []byte, err error) {
	orgAll := []string{}
	if len(org) > 0 {
		orgAll = strings.Split(org, ",")
	}
	cert, privKey, certPEM, privPEM, err = xcrypto.GenerateRootCA(orgAll, name, bits)
	return
}

func GenerateWeb(parent *x509.Certificate, rootKey *rsa.PrivateKey, org, domain, ip string, bits int) (cert tls.Certificate, certPEM, privPEM []byte, err error) {
	domains := strings.Split(domain, ",")
	ipAddress := []net.IP{}
	if len(ip) > 0 {
		ips := strings.Split(ip, ",")
		for _, v := range ips {
			ipAddress = append(ipAddress, net.ParseIP(v))
		}
	}
	orgAll := []string{}
	if len(org) > 0 {
		orgAll = strings.Split(org, ",")
	}
	_, _, certPEM, privPEM, err = xcrypto.GenerateCert(parent, rootKey, nil, orgAll, domain, domains, ipAddress, bits)
	if err == nil {
		cert, err = tls.X509KeyPair(certPEM, privPEM)
	}
	return
}
