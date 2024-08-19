package main

import (
	"iotdb-export/pkg/config"
	iotdbTransforms "iotdb-export/pkg/transforms"
	"os"

	"github.com/edgexfoundry/app-functions-sdk-go/v3/pkg"
)

const serviceKey = "app-iotdb-export"

func main() {
	_ = os.Setenv("EDGEX_SECURITY_SECRET_STORE", "false")

	service, ok := pkg.NewAppService(serviceKey)
	if !ok {
		os.Exit(-1)
	}

	lc := service.LoggingClient()

	config := &config.ServiceConfig{}
	if err := service.LoadCustomConfig(config, "IotDBConfig"); err != nil {
		lc.Errorf("LoadCustomConfig failed: %s", err.Error())
		os.Exit(-1)
	}

	if err := config.Validate(); err != nil {
		lc.Errorf("Config validation failed: %s", err.Error())
		os.Exit(-1)
	}

	if err := service.SetDefaultFunctionsPipeline(
		iotdbTransforms.NewConversion().TransformToIotDB,
		iotdbTransforms.NewSender(config.IotDBConfig, true).Send,
	); err != nil {
		lc.Errorf("SDK SetDefaultFunctionsPipeline failed: %s\n", err.Error())
		os.Exit(-1)
	}

	if err := service.Run(); err != nil {
		lc.Errorf("Run returned error: %s", err.Error())
		os.Exit(-1)
	}

	os.Exit(0)
}
