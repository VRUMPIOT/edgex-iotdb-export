package transforms

import (
	"encoding/json"
	"fmt"
	iotdbDTOs "iotdb-export/pkg/dtos"
	"sync"

	"github.com/apache/iotdb-client-go/client"
	"github.com/edgexfoundry/app-functions-sdk-go/v3/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/clients/logger"
	coreCommon "github.com/edgexfoundry/go-mod-core-contracts/v3/common"
	gometrics "github.com/rcrowley/go-metrics"
)

type Sender struct {
	Lock           sync.Mutex
	LC             logger.LoggingClient
	Session        client.Session
	Config         iotdbDTOs.Config
	PersistOnError bool
	SizeMetrics    gometrics.Histogram
	ErrorMetric    gometrics.Counter
}

func NewSender(config iotdbDTOs.Config, persistOnError bool) *Sender {
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
	if err := sender.Session.Open(sender.Config.RPCCompression, sender.Config.ConnectionTimeout); err != nil {
		return fmt.Errorf("in pipeline '%s', could not connect to iotdb for export. Error: %s", ctx.PipelineId(), err.Error())
	}
	ctx.LoggingClient().Infof("Connected to iotdb server for export in pipeline '%s'", ctx.PipelineId())
	return nil
}

func (sender *Sender) Close() {
	sender.Session.Close()
}

func (sender *Sender) setRetryData(ctx interfaces.AppFunctionContext, data iotdbDTOs.Readings) error {
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

func (sender *Sender) Send(ctx interfaces.AppFunctionContext, data interface{}) (bool, interface{}) {
	if sender.LC == nil {
		sender.LC = ctx.LoggingClient()
	}
	if d, ok := data.(iotdbDTOs.Readings); ok {
		if len(d.DeviceIds) == 0 {
			// We didn't receive a result
			return false, fmt.Errorf("function IotDBSend in pipeline '%s': No Data Received", ctx.PipelineId())
		}

		if err := sender.NewSession(ctx.LoggingClient()); err != nil {
			sender.ErrorMetric.Inc(1)
			sender.setRetryData(ctx, d)
			return false, err
		}

		if err := sender.Open(ctx); err != nil {
			sender.ErrorMetric.Inc(1)
			sender.setRetryData(ctx, d)
			return false, err
		}

		if status, err := sender.Session.InsertRecords(d.DeviceIds, d.Measurements, d.DataTypes, d.Values, d.Timestamps); err != nil {
			sender.ErrorMetric.Inc(1)
			sender.setRetryData(ctx, d)
			return false, fmt.Errorf("function IotDBSend in pipeline '%s': Error occurred %s with status code %s", ctx.PipelineId(), err, status)
		}
		// capture the size for metrics
		byteData, err := json.Marshal(data)
		if err != nil {
			return false, err
		}
		dataBytes := len(byteData)
		sender.SizeMetrics.Update(int64(dataBytes))

		sender.LC.Debugf("Sent %d bytes of data to IotDB in pipeline '%s'", dataBytes, ctx.PipelineId())
		sender.LC.Tracef("Data exported to IotDB in pipeline '%s': %s=%s", ctx.PipelineId(), coreCommon.CorrelationHeader, ctx.CorrelationID())

	}
	return true, nil
}
