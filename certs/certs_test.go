package main

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/codingeasygo/util/xmap"
)

const CA = `
-----BEGIN CERTIFICATE-----
MIIC7DCCAdQCCQCMyBSSQqpftjANBgkqhkiG9w0BAQsFADA4MREwDwYDVQQDDAh0
ZXN0LmxvYzELMAkGA1UEBhMCVVMxFjAUBgNVBAcMDVNhbiBGcmFuc2lzY28wHhcN
MjMwNjA0MDcyNTIzWhcNMjQwNTI1MDcyNTIzWjA4MREwDwYDVQQDDAh0ZXN0Lmxv
YzELMAkGA1UEBhMCVVMxFjAUBgNVBAcMDVNhbiBGcmFuc2lzY28wggEiMA0GCSqG
SIb3DQEBAQUAA4IBDwAwggEKAoIBAQDDXqarncycUpEMfxw7+aYeLakU2GsNtXem
BxPMy2qm/QYqBAwoyrwaJ3pdv5HD1TMFRJHDCPpHLJg4arZPyrpynitlG7P9Npo5
QPVKCWEgpu8SaFPF4o/NStKGF5ZQFE020WASPGXyc7RfFSjXOV0vvIMCyij+yNQq
uvSjypN2xU2N+i5oGjPRPll2x8pLELWdwVeC12qhmtGU1kGu7fMNP4yYL+pcoOoM
pmu/KvCRsz8X+X1p795aXCnvZgyFJcYQ9pmOPY7aFcFhe+p1Z5re18ZemVwQewAL
/aPT0NDP2xKd1lNsoKcsuGc7zU6buekKEQQ67qSRwCFm3gh2RHzXAgMBAAEwDQYJ
KoZIhvcNAQELBQADggEBADpnzYO1gvWa6/l8NNaWnFiA0xaiSKV/HsX1Uf9+c9O3
vODcnpi2Z/2QT8Uub0TnRK0W64ZSnrnOiCbyZSxuyX8vpWrcyHUgdOrrQpGEg8eU
mrNua5FXAswaZOSCRL5GpOdwPYpzqlTNAamxEQf2f9cCByd7sisBWHbvjwtE4rB4
30+dMQHq97n7si/qe0+cb8ZjD8J9WFUZ1UqzVQ61Bn+XpLsctaqGLHJdzoMT+LCI
FhDCyiSnoCen2cSB0QQvDhaz8AP0p0ID1lZ7NaNQ4NOH8j5TURCYbljQ/86RFLyG
7tOc0HN+lxTp/YCrbqzWztNmBBT4RM2NTkA0EBtFLNU=
-----END CERTIFICATE-----
`

func TestPem2Hex(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	writer, _ := gzip.NewWriterLevel(buf, gzip.BestCompression)
	writer.Write([]byte(CA))
	writer.Close()
	buf2 := hex.EncodeToString([]byte(CA))
	buf3 := hex.EncodeToString(buf.Bytes())
	fmt.Printf("--->%v\n", len(CA))
	fmt.Printf("--->%v\n", buf.Len())
	fmt.Printf("--->%v\n", len(buf2))
	fmt.Printf("--->%v\n", len(buf3))
	// config := xmap.M{
	// 	"name": "NX",
	// 	"forwards": xmap.M{
	// 		"tun~socks://127.0.0.1:0": "N0->${HOST}",
	// 	},
	// 	"channels": xmap.M{
	// 		"N0": xmap.M{
	// 			"name":   "NX",
	// 			"remote": "tls://192.168.1.100:13100",
	// 			"domain": "test.loc",
	// 			"token":  "123",
	// 			"tls_ca": CA,
	// 		},
	// 	},
	// }
	// ioutil.WriteFile("config.json", []byte(converter.JSON(config)), os.ModePerm)
}

func TestXXX(t *testing.T) {
	xxx := "{\"channels\":{\"N0\":{\"domain\":\"test.loc\",\"name\":\"NX\",\"remote\":\"quic://192.168.1.100:13100\",\"tls_ca\":\"\\n-----BEGIN CERTIFICATE-----\\nMIIC7DCCAdQCCQCMyBSSQqpftjANBgkqhkiG9w0BAQsFADA4MREwDwYDVQQDDAh0\\nZXN0LmxvYzELMAkGA1UEBhMCVVMxFjAUBgNVBAcMDVNhbiBGcmFuc2lzY28wHhcN\\nMjMwNjA0MDcyNTIzWhcNMjQwNTI1MDcyNTIzWjA4MREwDwYDVQQDDAh0ZXN0Lmxv\\nYzELMAkGA1UEBhMCVVMxFjAUBgNVBAcMDVNhbiBGcmFuc2lzY28wggEiMA0GCSqG\\nSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDDXqarncycUpEMfxw7+aYeLakU2GsNtXem\\nBxPMy2qm/QYqBAwoyrwaJ3pdv5HD1TMFRJHDCPpHLJg4arZPyrpynitlG7P9Npo5\\nQPVKCWEgpu8SaFPF4o/NStKGF5ZQFE020WASPGXyc7RfFSjXOV0vvIMCyij+yNQq\\nuvSjypN2xU2N+i5oGjPRPll2x8pLELWdwVeC12qhmtGU1kGu7fMNP4yYL+pcoOoM\\npmu/KvCRsz8X+X1p795aXCnvZgyFJcYQ9pmOPY7aFcFhe+p1Z5re18ZemVwQewAL\\n/aPT0NDP2xKd1lNsoKcsuGc7zU6buekKEQQ67qSRwCFm3gh2RHzXAgMBAAEwDQYJ\\nKoZIhvcNAQELBQADggEBADpnzYO1gvWa6/l8NNaWnFiA0xaiSKV/HsX1Uf9+c9O3\\nvODcnpi2Z/2QT8Uub0TnRK0W64ZSnrnOiCbyZSxuyX8vpWrcyHUgdOrrQpGEg8eU\\nmrNua5FXAswaZOSCRL5GpOdwPYpzqlTNAamxEQf2f9cCByd7sisBWHbvjwtE4rB4\\n30+dMQHq97n7si/qe0+cb8ZjD8J9WFUZ1UqzVQ61Bn+XpLsctaqGLHJdzoMT+LCI\\nFhDCyiSnoCen2cSB0QQvDhaz8AP0p0ID1lZ7NaNQ4NOH8j5TURCYbljQ/86RFLyG\\n7tOc0HN+lxTp/YCrbqzWztNmBBT4RM2NTkA0EBtFLNU=\\n-----END CERTIFICATE-----\\n\",\"token\":\"123\"}},\"forwards\":{\"tun~socks://127.0.0.1:0\":\"N0->${HOST}\"},\"name\":\"NX\"}"
	res, err := xmap.MapVal(xxx)
	if err != nil {
		panic(err)
	}
	fmt.Println("--->", res)
}
