package bsrouter

import (
	"os"
	"path/filepath"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xcrypto"
	"github.com/codingeasygo/util/xmap"

	_ "golang.org/x/mobile/bind"
)

var lastError error
var service *router.Service

func Start(config string) (err string) {
	if service != nil {
		err = "started"
		return
	}
	dir := filepath.Dir(config)
	if _, xerr := os.Stat(filepath.Join(dir, "bsrouter.key")); os.IsNotExist(xerr) {
		rootCert, rootKey, rootCertPEM, rootKeyPEM, xerr := xcrypto.GenerateRootCA([]string{"bsrouter"}, "bsrouter", 2048)
		if xerr != nil {
			lastError = xerr
			err = xerr.Error()
			return
		}
		_, _, certPEM, keyPEM, xerr := xcrypto.GenerateCert(rootCert, rootKey, nil, []string{"bsrouter"}, "bsrouter", []string{"bsrouter"}, nil, 2048)
		if xerr != nil {
			lastError = xerr
			err = xerr.Error()
			return
		}
		xerr = os.WriteFile(filepath.Join(dir, "rootCA.key"), rootKeyPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			err = xerr.Error()
			return
		}
		xerr = os.WriteFile(filepath.Join(dir, "rootCA.pem"), rootCertPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			err = xerr.Error()
			return
		}
		xerr = os.WriteFile(filepath.Join(dir, "bsrouter.key"), keyPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			err = xerr.Error()
			return
		}
		xerr = os.WriteFile(filepath.Join(dir, "bsrouter.pem"), certPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			err = xerr.Error()
			return
		}
		router.InfoLog("Server create cert on %v by bsrouter.key/bsrouter.pem", dir)
	}
	service = router.NewService()
	service.ConfigPath = config
	xerr := service.Start()
	if xerr != nil {
		lastError = xerr
		err = xerr.Error()
	}
	return
}

func State(config string) (state string) {
	configValue, err := xmap.ReadJSON(config)
	if err != nil {
		return converter.JSON(xmap.M{"code": 100, "message": "not config", "debug": err.Error()})
	}
	nodeName := configValue.StrDef("none", "name")
	if service == nil || service.Node == nil || service.Node.Router == nil {
		return converter.JSON(xmap.M{"code": 200, "message": "not started", "name": nodeName})
	}
	nodeState := service.Node.State()
	nodeState["code"] = 0
	nodeState["name"] = nodeName
	if lastError != nil {
		nodeState["debug"] = lastError.Error()
	}
	state = converter.JSON(nodeState)
	return
}

func Stop() (err string) {
	if service == nil {
		err = "stopped"
		return
	}
	xerr := service.Stop()
	if xerr != nil {
		err = xerr.Error()
	}
	return
}
