package main

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"

	"github.com/edgexfoundry/app-functions-sdk-go/v3/pkg"
	"github.com/edgexfoundry/app-functions-sdk-go/v3/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/clients/logger"

	"app-iotdb-export/pkg/config"
	"app-iotdb-export/pkg/transforms"
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
	configMutex   sync.RWMutex
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

	// Initialize configuration
	if err := app.initConfig(); err != nil {
		app.lc.Errorf("Configuration initialization failed: %s", err.Error())
		return -1
	}

	// Set up processing pipeline
	if err := app.setupProcessingPipeline(); err != nil {
		app.lc.Errorf("Pipeline setup failed: %s", err.Error())
		return -1
	}

	app.appCtx = app.service.AppContext()

	// Run the service
	if err := app.service.Run(); err != nil {
		app.lc.Errorf("Service run failed: %s", err.Error())
		return -1
	}

	return 0
}

func (app *App) initConfig() error {
	app.serviceConfig = &config.ServiceConfig{}
	if err := app.service.LoadCustomConfig(app.serviceConfig, "IotDBConfig"); err != nil {
		return fmt.Errorf("failed to load custom configuration: %w", err)
	}

	app.lc.Debugf("IotDB Config: %+v", app.serviceConfig)

	if err := app.serviceConfig.IotDBConfig.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	return app.service.ListenForCustomConfigChanges(
		&app.serviceConfig.IotDBConfig,
		"IotDBConfig",
		app.ProcessConfigUpdates,
	)
}

func (app *App) setupProcessingPipeline() error {
	return app.service.SetDefaultFunctionsPipeline(
		transforms.NewSender(app.serviceConfig.IotDBConfig, true).Send,
	)
}

func (app *App) ProcessConfigUpdates(rawWritableConfig any) {
	updated, ok := rawWritableConfig.(*config.IotDBConfig)
	if !ok {
		app.lc.Error("unable to process config updates: Can not cast raw config to type 'IotDBConfig'")
		return
	}

	app.configMutex.Lock()
	defer app.configMutex.Unlock()

	if reflect.DeepEqual(app.serviceConfig.IotDBConfig, *updated) {
		app.lc.Info("No configuration changes detected")
		return
	}

	app.lc.Info("Applying updated IoTDB configuration")
	app.serviceConfig.IotDBConfig = *updated
}
