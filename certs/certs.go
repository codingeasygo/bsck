package main

import (
	"io/ioutil"
	"net/http"
	"os"

	"github.com/codingeasygo/util/xcrypto"
)

func main() {
	if os.Args[1] == "gen" {
		caCert, caKey, rootCert, rootKey, err := xcrypto.GenerateRootCA([]string{"Coding Easy GO"}, "bsrouter", 2048)
		if err != nil {
			panic(err)
		}
		_, serverCert, serverKey, err := xcrypto.GenerateWeb(caCert, caKey, false, "Coding Easy GO", "test.loc,www.test.loc", "127.0.0.1", 2048)
		if err != nil {
			panic(err)
		}
		ioutil.WriteFile("rootCA.crt", rootCert, os.ModePerm)
		ioutil.WriteFile("rootCA.key", rootKey, os.ModePerm)
		ioutil.WriteFile("server.crt", serverCert, os.ModePerm)
		ioutil.WriteFile("server.key", serverKey, os.ModePerm)
	} else {
		http.ListenAndServeTLS(":8422", "server.crt", "server.key", nil)
	}
}
