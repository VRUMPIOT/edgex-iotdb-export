package transforms

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	iotdbDTOs "iotdb-export/pkg/dtos"

	"github.com/apache/iotdb-client-go/client"
	"github.com/edgexfoundry/app-functions-sdk-go/v3/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"
)

type Conversion struct {
}

func NewConversion() Conversion {
	return Conversion{}
}

func (f Conversion) TransformToIotDB(ctx interfaces.AppFunctionContext, data interface{}) (bool, interface{}) {
	lc := ctx.LoggingClient()

	lc.Debug("Transforming to IotDB format")

	if data == nil {
		return false, errors.New("TransformToIotDB: No data received")
	}

	event, ok := data.(dtos.Event)
	if !ok {
		return false, errors.New("TransformToIotDB: didn't receive expect Event type")
	}

	readings, err := transformation(event)
	if err != nil {
		return false, errors.New(fmt.Sprintf("Failed to transform IotDB data: %s", err))
	}
	msg, err := json.Marshal(readings)
	lc.Debugf("IotDB Payload: %s", msg)

	return true, readings
}

func transformation(event dtos.Event) (*iotdbDTOs.Readings, error) {
	readings := &iotdbDTOs.Readings{}

	for _, reading := range event.Readings {
		deviceId := reading.DeviceName + "." + reading.ProfileName
		measurement := reading.ResourceName
		idx := strings.LastIndex(reading.ResourceName, ".")
		if idx > -1 {
			deviceId += reading.ResourceName[:idx]
			measurement = reading.ResourceName[idx+1:]
		}

		dataType, err := dataTypeConversion(reading.ValueType)
		if err != nil {
			return readings, err
		}

		var value []interface{}
		value = append(value, reading.Value)

		// TODO timestamp conversion
		readings.DeviceIds = append(readings.DeviceIds, deviceId)
		readings.Measurements = append(readings.Measurements, []string{measurement})
		readings.DataTypes = append(readings.DataTypes, []client.TSDataType{dataType})
		readings.Values = append(readings.Values, value)
		readings.Timestamps = append(readings.Timestamps, event.Origin)
	}

	return readings, nil
}

func dataTypeConversion(data_type string) (client.TSDataType, error) {
	if data_type == "Bool" {
		return client.BOOLEAN, nil
	}

	if data_type == "Text" {
		return client.TEXT, nil
	}

	if (strings.Contains(data_type, "Uint") || strings.Contains(data_type, "Int")) && !strings.Contains(data_type, "64") {
		return client.INT32, nil
	}

	if (strings.Contains(data_type, "Uint") || strings.Contains(data_type, "Int")) && strings.Contains(data_type, "64") {
		return client.INT64, nil
	}

	if strings.Contains(data_type, "Float") && !strings.Contains(data_type, "64") {
		return client.FLOAT, nil
	}

	if strings.Contains(data_type, "Float") && strings.Contains(data_type, "64") {
		return client.DOUBLE, nil
	}

	return client.BOOLEAN, errors.New(fmt.Sprintf("dataTypeConversion: Unsupported data type %s", data_type))
}
