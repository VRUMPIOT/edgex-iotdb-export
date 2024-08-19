package config

import (
	"errors"
	iotdbDTOs "iotdb-export/pkg/dtos"
	"strings"
)

type ServiceConfig struct {
	IotDBConfig iotdbDTOs.Config
}

func (c *ServiceConfig) UpdateFromRaw(rawConfig interface{}) bool {
	configuration, ok := rawConfig.(*ServiceConfig)
	if !ok {
		errors.New("unable to cast raw config to type 'ServiceConfig'")
		return false
	}

	*c = *configuration

	return true
}

func (c *ServiceConfig) Validate() error {
	if len(strings.TrimSpace(c.IotDBConfig.Host)) == 0 {
		return errors.New("configuration missing value for IotDBConfig.Host")
	}
	if len(strings.TrimSpace(c.IotDBConfig.Port)) == 0 {
		return errors.New("configuration missing value for IotDBConfig.Port")
	}
	if len(strings.TrimSpace(c.IotDBConfig.UserName)) == 0 {
		return errors.New("configuration missing value for IotDBConfig.UserName")
	}
	if len(strings.TrimSpace(c.IotDBConfig.Password)) == 0 {
		return errors.New("configuration missing value for IotDBConfig.Password")
	}
	if c.IotDBConfig.FetchSize <= 0 {
		return errors.New("configuration incorrect value for IotDBConfig.FetchSize")
	}
	if len(strings.TrimSpace(c.IotDBConfig.TimeZone)) == 0 {
		return errors.New("configuration missing value for IotDBConfig.TimeZone")
	}
	if c.IotDBConfig.ConnectRetryMax <= 0 {
		return errors.New("configuration incorrect value for IotDBConfig.ConnectRetryMax")
	}
	if c.IotDBConfig.ConnectionTimeout <= 0 {
		return errors.New("configuration incorrect value for IotDBConfig.ConnectionTimeout")
	}

	return nil
}
