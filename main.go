package main

import (
	"context"
	"os"
	"reflect"

	"app-iotdb-export/pkg/config"
	"app-iotdb-export/pkg/transforms"

	"github.com/edgexfoundry/app-functions-sdk-go/v3/pkg"
	"github.com/edgexfoundry/app-functions-sdk-go/v3/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/clients/logger"
)

const (
	serviceKey = "app-iotdb-export"
)

type App struct {
	service       interfaces.ApplicationService
	lc            logger.LoggingClient
	appCtx        context.Context
	serviceConfig *config.ServiceConfig
	configChanged chan bool
}

func main() {
	app := App{}
	code := app.CreateAndRunAppService(serviceKey, pkg.NewAppService)
	os.Exit(code)
}

func (app *App) CreateAndRunAppService(serviceKey string,
	newServiceFactory func(string) (interfaces.ApplicationService, bool)) int {

	var ok bool
	app.service, ok = newServiceFactory(serviceKey)
	if !ok {
		return -1
	}

	app.lc = app.service.LoggingClient()

	app.serviceConfig = &config.ServiceConfig{}
	if err := app.service.LoadCustomConfig(app.serviceConfig, "IotDBConfig"); err != nil {
		app.lc.Errorf("failed load custom configuration: %s", err.Error())
		return -1
	}

	if err := app.serviceConfig.IotDB.Validate(); err != nil {
		app.lc.Errorf("custom configuration failed validation: %s", err.Error())
		return -1
	}

	if err := app.service.ListenForCustomConfigChanges(&app.serviceConfig.IotDB,
		"IotDBConfig", app.ProcessConfigUpdates); err != nil {
		app.lc.Errorf("unable to watch custom writable configuration: %s", err.Error())
		return -1
	}

	if err := app.service.SetDefaultFunctionsPipeline(
		transforms.NewSender(app.serviceConfig.IotDB, true).Send,
	); err != nil {
		app.lc.Errorf("SetFunctionsPipeline returned error: %s", err.Error())
		return -1
	}

	app.appCtx = app.service.AppContext()

	if err := app.service.Run(); err != nil {
		app.lc.Errorf("Run returned error: %s", err.Error())
		return -1
	}

	return 0
}
func (app *App) ProcessConfigUpdates(rawWritableConfig interface{}) {
	updated, ok := rawWritableConfig.(*config.IotDBConfig)
	if !ok {
		app.lc.Error("unable to process config updates: Can not cast raw config to type 'IotDBConfig'")
		return
	}

	previous := app.serviceConfig.IotDB
	app.serviceConfig.IotDB = *updated

	if reflect.DeepEqual(previous, updated) {
		app.lc.Info("No changes detected")
		return
	}
}
