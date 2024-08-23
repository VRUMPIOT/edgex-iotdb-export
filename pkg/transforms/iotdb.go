package transforms

import (
	"app-iotdb-export/pkg/config"
	iotdbDTOs "app-iotdb-export/pkg/dtos"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/apache/iotdb-client-go/client"
	"github.com/edgexfoundry/app-functions-sdk-go/v3/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/clients/logger"
	coreCommon "github.com/edgexfoundry/go-mod-core-contracts/v3/common"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"
	gometrics "github.com/rcrowley/go-metrics"
)

type Sender struct {
	Lock           sync.Mutex
	LC             logger.LoggingClient
	Session        client.Session
	Config         config.IotDBConfig
	PersistOnError bool
	SizeMetrics    gometrics.Histogram
	ErrorMetric    gometrics.Counter
}

func NewSender(config config.IotDBConfig, persistOnError bool) *Sender {
	sender := &Sender{
		Config:         config,
		PersistOnError: persistOnError,
	}
	sender.ErrorMetric = gometrics.NewCounter()
	sender.SizeMetrics = gometrics.NewHistogram(gometrics.NewUniformSample(1028))

	return sender
}

func (sender *Sender) NewSession(lc logger.LoggingClient) error {
	sender.Lock.Lock()
	defer sender.Lock.Unlock()

	lc.Info("Initializing IotDB Session")
	config := &client.Config{
		Host:      sender.Config.Host,
		Port:      sender.Config.Port,
		UserName:  sender.Config.UserName,
		Password:  sender.Config.Password,
		FetchSize: sender.Config.FetchSize,
		TimeZone:  sender.Config.TimeZone,
	}
	sender.Session = client.NewSession(config)
	return nil
}

func (sender *Sender) Open(ctx interfaces.AppFunctionContext) error {
	sender.Lock.Lock()
	defer sender.Lock.Unlock()

	ctx.LoggingClient().Info("Connecting to iotdb server for export")
	if err := sender.Session.Open(sender.Config.RPCCompression,
		sender.Config.ConnectionTimeout); err != nil {
		return fmt.Errorf("in pipeline '%s', could not connect to iotdb for export. Error: %s",
			ctx.PipelineId(), err.Error())
	}
	ctx.LoggingClient().Infof("Connected to iotdb server for export in pipeline '%s'",
		ctx.PipelineId())
	return nil
}

func (sender *Sender) Close() {
	sender.Session.Close()
}

func (sender *Sender) setRetryData(ctx interfaces.AppFunctionContext,
	data interface{}) error {
	if sender.PersistOnError {
		exportData, err := json.Marshal(data)
		if err != nil {
			return err
		}
		ctx.SetRetryData(exportData)
	}
	return nil
}

func (sender *Sender) onConnected(_ client.Session) {
	sender.LC.Tracef("IotDB Broker for export connected")
}

func (sender *Sender) onConnectionLost(_ client.Session, _ error) {
	sender.LC.Tracef("IotDB Broker for export lost connection")

}

func (sender *Sender) onReconnecting(_ client.Session, _ *client.Config) {
	sender.LC.Tracef("IotDB Broker for export re-connecting")
}

func (sender *Sender) Send(ctx interfaces.AppFunctionContext,
	data interface{}) (bool, interface{}) {
	defer sender.Close()

	if sender.LC == nil {
		sender.LC = ctx.LoggingClient()
	}

	sender.LC.Debugf("IotDB Config: %s", sender.Config)

	event, ok := data.(dtos.Event)
	if !ok {
		return false,
			errors.New("TransformToIotDB: didn't receive expect Event type")
	}
	sender.LC.Debugf("EdgeX Payload: %s", event)

	readings, err := transformation(event, sender.Config.Prefix, sender.Config.Precision)
	if err != nil {
		return false,
			fmt.Errorf("failed to transform iotdb data: %s", err)
	}

	sender.LC.Debugf("IotDB Payload: %s", readings)

	if err := sender.NewSession(ctx.LoggingClient()); err != nil {
		sender.ErrorMetric.Inc(1)
		return false, err
	}

	if err := sender.Open(ctx); err != nil {
		sender.ErrorMetric.Inc(1)
		sender.setRetryData(ctx, data)
		return false, err
	}

	status, err := sender.Session.InsertRecords(readings.DeviceIds,
		readings.Measurements, readings.DataTypes, readings.Values, readings.Timestamps)
	if err != nil || status.Code != 200 {
		sender.ErrorMetric.Inc(1)
		sender.setRetryData(ctx, data)
		return false,
			fmt.Errorf("function IotDBSend in pipeline '%s': Error occurred %s with status code %s",
				ctx.PipelineId(), err, status)
	}
	sender.LC.Debugf("IotDBSend status code %s error", status)

	// capture the size for metrics
	byteData, err := json.Marshal(data)
	if err != nil {
		return false, err
	}
	dataBytes := len(byteData)
	sender.SizeMetrics.Update(int64(dataBytes))

	sender.LC.Debugf("Sent %d bytes of data to IotDB in pipeline '%s'", dataBytes, ctx.PipelineId())
	sender.LC.Tracef("Data exported to IotDB in pipeline '%s': %s=%s", ctx.PipelineId(),
		coreCommon.CorrelationHeader, ctx.CorrelationID())

	return true, nil
}

func transformation(event dtos.Event, prefix string,
	precision iotdbDTOs.Precision) (*iotdbDTOs.Readings, error) {
	readings := &iotdbDTOs.Readings{}

	for _, reading := range event.Readings {
		ts := nsecsTo(reading.Origin, precision)

		var deviceId = "root."
		if prefix != "" {
			deviceId += prefix
			if deviceId[len(deviceId)-1] != '.' {
				deviceId += "."
			}
		}
		deviceId += reading.DeviceName + "." + reading.ProfileName
		deviceId = strings.TrimSuffix(deviceId, ".")

		measurement := reading.ResourceName
		idx := strings.LastIndex(measurement, ".")
		if idx > -1 {
			deviceId += measurement[:idx]
			measurement = measurement[idx+1:]
		}

		dataType, value, err := dataTypeConversion(reading.ValueType, reading.Value)
		if err != nil {
			return readings, err
		}

		readings.Timestamps = append(readings.Timestamps, ts)
		readings.DeviceIds = append(readings.DeviceIds, deviceId)
		readings.Measurements = append(readings.Measurements, []string{measurement})
		readings.DataTypes = append(readings.DataTypes, []client.TSDataType{dataType})
		readings.Values = append(readings.Values, value)
	}

	if len(readings.DeviceIds) == 0 {
		return readings,
			fmt.Errorf("function transformation No Data Received")
	}

	return readings, nil
}

func dataTypeConversion(data_type string, value string) (client.TSDataType,
	[]interface{}, error) {
	var val []interface{}

	if data_type == "Bool" {
		v, err := strconv.ParseBool(value)
		if err != nil {
			return client.UNKNOWN, val,
				fmt.Errorf("dataTypeConversion: could not convert value %s to bool", value)

		}
		val = append(val, v)
		return client.BOOLEAN, val, nil
	}

	if data_type == "Text" {
		val = append(val, value)
		return client.TEXT, val, nil
	}

	if (strings.Contains(data_type, "Uint") || strings.Contains(data_type, "Int")) &&
		!strings.Contains(data_type, "64") {
		v, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return client.UNKNOWN, val,
				fmt.Errorf("dataTypeConversion: could not convert value %s to int32", value)

		}
		val = append(val, int32(v))
		return client.INT32, val, nil
	}

	if (strings.Contains(data_type, "Uint") || strings.Contains(data_type, "Int")) &&
		strings.Contains(data_type, "64") {
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return client.UNKNOWN, val,
				fmt.Errorf("dataTypeConversion: could not convert value %s to int64", value)

		}
		val = append(val, v)
		return client.INT64, val, nil
	}

	if strings.Contains(data_type, "Float") && !strings.Contains(data_type, "64") {
		v, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return client.UNKNOWN, val,
				fmt.Errorf("dataTypeConversion: could not convert value %s to float32", value)

		}
		val = append(val, float32(v))
		return client.FLOAT, val, nil
	}

	if strings.Contains(data_type, "Float") && strings.Contains(data_type, "64") {
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return client.UNKNOWN, val,
				fmt.Errorf("dataTypeConversion: could not convert value %s to float64", value)

		}
		val = append(val, v)
		return client.DOUBLE, val, nil
	}

	return client.UNKNOWN, val,
		fmt.Errorf("dataTypeConversion: Unsupported data type %s", data_type)
}

func nsecsTo(nsecs int64, precision iotdbDTOs.Precision) int64 {
	if precision == iotdbDTOs.S {
		return int64(nsecs / 1e9)
	}
	if precision == iotdbDTOs.MS {
		return int64(nsecs / 1e6)
	}
	if precision == iotdbDTOs.US {
		return int64(nsecs / 1e3)
	}

	return nsecs
}
