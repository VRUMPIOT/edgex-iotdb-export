package config

import (
	"app-iotdb-export/pkg/dtos"
	"errors"
	"fmt"
	"strings"
)

type ServiceConfig struct {
	IotDB IotDBConfig
}

func (c *ServiceConfig) UpdateFromRaw(rawConfig interface{}) bool {
	configuration, ok := rawConfig.(*ServiceConfig)
	if !ok {
		fmt.Println("unable to cast raw config to type 'ServiceConfig'")
		return false
	}
	*c = *configuration
	return true
}

type IotDBConfig struct {
	Host              string
	Port              string
	UserName          string
	Password          string
	FetchSize         int32
	TimeZone          string
	ConnectRetryMax   int
	RPCCompression    bool
	ConnectionTimeout int
	Prefix            string
	Precision         dtos.Precision
}

func (c *IotDBConfig) Validate() error {
	if len(strings.TrimSpace(c.Host)) == 0 {
		return errors.New("configuration missing value for Host")
	}
	if len(strings.TrimSpace(c.Port)) == 0 {
		return errors.New("configuration missing value for Port")
	}
	if len(strings.TrimSpace(c.UserName)) == 0 {
		return errors.New("configuration missing value for UserName")
	}
	if len(strings.TrimSpace(c.Password)) == 0 {
		return errors.New("configuration missing value for Password")
	}
	if c.FetchSize <= 0 {
		return errors.New("configuration incorrect value for FetchSize")
	}
	if len(strings.TrimSpace(c.TimeZone)) == 0 {
		return errors.New("configuration missing value for TimeZone")
	}
	if c.ConnectRetryMax <= 0 {
		return errors.New("configuration incorrect value for ConnectRetryMax")
	}
	if c.ConnectionTimeout <= 0 {
		return errors.New("configuration incorrect value for ConnectionTimeout")
	}
	if c.Precision != dtos.S || c.Precision != dtos.MS || c.Precision != dtos.US || c.Precision != dtos.NS {
		return errors.New("configuration incorrect value for Precision supports s, ms, us and ns")
	}

	return nil
}
