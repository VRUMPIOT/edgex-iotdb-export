package transforms

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/apache/iotdb-client-go/v2/client"
	"github.com/edgexfoundry/app-functions-sdk-go/v3/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/clients/logger"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"
	gometrics "github.com/rcrowley/go-metrics"

	"app-iotdb-export/pkg/config"
	iotdbDTOs "app-iotdb-export/pkg/dtos"
	sm "app-iotdb-export/pkg/iotdb"
)

const (
	parallelThreshold = 200
)

type Sender struct {
	lc             logger.LoggingClient
	config         config.IotDBConfig
	persistOnError bool
	sizeMetrics    gometrics.Histogram
	errorMetric    gometrics.Counter
}

func NewSender(config config.IotDBConfig, persistOnError bool) *Sender {
	sender := &Sender{
		config:         config,
		persistOnError: persistOnError,
		errorMetric:    gometrics.NewCounter(),
		sizeMetrics:    gometrics.NewHistogram(gometrics.NewUniformSample(1028)),
	}
	return sender
}

func (sender *Sender) Send(ctx interfaces.AppFunctionContext, data any) (bool, any) {
	correlationID := ctx.CorrelationID()

	if sender.lc == nil {
		sender.lc = ctx.LoggingClient()
	}

	sender.lc.Debugf("[%s] Starting processing of incoming data", correlationID)
	defer func() {
		sender.lc.Debugf("[%s] Completed processing", correlationID)
	}()

	readings, err := sender.parseInputData(data)
	if err != nil {
		sender.errorMetric.Inc(1)
		return false, fmt.Errorf("failed to parse input data: %w", err)
	}

	if len(readings.DeviceIds) == 0 {
		sender.lc.Warn("No valid readings to process")
		return true, nil
	}
	sender.lc.Infof("[%s] Processing %d readings",
		correlationID, len(readings.DeviceIds))

	sessionManager := sender.initSessionManager()
	defer sessionManager.Close()

	err = sessionManager.SendRecords(*readings)
	if err != nil {
		sender.errorMetric.Inc(1)

		if err := sender.setRetryData(ctx, data); err != nil {
			sender.lc.Errorf("Failed to set retry data: %v", err)
		}

		return false, fmt.Errorf("failed to send data after retries: %w", err)
	}

	sender.updateMetrics(data)

	sender.lc.Infof("Successfully processed event from devices: %s", readings.DeviceIds)
	return true, nil
}

func (sender *Sender) parseInputData(data any) (*iotdbDTOs.Readings, error) {
	sender.lc.Debug("Parsing input data")

	switch d := data.(type) {
	case []byte:
		sender.lc.Debug("Input is []byte, attempting to unmarshal")
		var readings iotdbDTOs.Readings
		if err := json.Unmarshal(d, &readings); err == nil {
			sender.lc.Debugf("Successfully unmarshaled %d readings directly", len(readings.DeviceIds))
			return &readings, nil
		}
		sender.lc.Debug("Attempting to unmarshal as Event")
		var event dtos.Event
		if err := json.Unmarshal(d, &event); err != nil {
			return nil, fmt.Errorf("failed to unmarshal as either Readings or Event: %w", err)
		}
		return sender.transformEvent(event)

	case iotdbDTOs.Readings:
		sender.lc.Debugf("Input is Readings struct with %d entries", len(d.DeviceIds))

		return &d, nil

	case *iotdbDTOs.Readings:
		sender.lc.Debugf("Input is *Readings with %d entries", len(d.DeviceIds))
		return d, nil

	case dtos.Event:
		sender.lc.Debugf("Input is Event with %d readings", len(d.Readings))
		return sender.transformEvent(d)

	case *dtos.Event:
		sender.lc.Debugf("Input is *Event with %d readings", len(d.Readings))
		return sender.transformEvent(*d)

	default:
		return nil, fmt.Errorf("unsupported data type: %T", data)
	}
}

func (sender *Sender) transformEvent(event dtos.Event) (*iotdbDTOs.Readings, error) {
	readings := iotdbDTOs.NewReadings(len(event.Readings))

	sender.lc.Debugf("Transforming %d event readings (parallel threshold: %d)",
		len(event.Readings), parallelThreshold)

	if len(event.Readings) > parallelThreshold {
		sender.lc.Debug("Using parallel transformation")
		return sender.transformParallel(event, readings)
	}
	return sender.transformSequential(event, readings)
}

func (sender *Sender) transformSequential(event dtos.Event, readings *iotdbDTOs.Readings) (*iotdbDTOs.Readings, error) {
	var errs []error
	for _, reading := range event.Readings {
		if err := sender.processReading(reading, readings); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return readings, fmt.Errorf("%d errors: %v", len(errs), errs)
	}
	return readings, nil
}

func (sender *Sender) transformParallel(event dtos.Event, readings *iotdbDTOs.Readings) (*iotdbDTOs.Readings, error) {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		errChan = make(chan error, len(event.Readings))
	)

	for _, reading := range event.Readings {
		wg.Add(1)
		go func(r dtos.BaseReading) {
			defer wg.Done()

			mu.Lock()
			defer mu.Unlock()
			if err := sender.processReading(r, readings); err != nil {
				errChan <- err
			}
		}(reading)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		var errs []error
		for err := range errChan {
			errs = append(errs, err)
		}
		return readings, fmt.Errorf("%d errors: %v", len(errs), errs)
	}
	return readings, nil
}

func (sender *Sender) processReading(reading dtos.BaseReading, readings *iotdbDTOs.Readings) error {
	path := sender.buildPath(reading)
	suffix, measurement := sender.extractMeasurement(reading.ResourceName)
	dataType, value, err := sender.convertDataType(reading.ValueType, reading.Value)
	if err != nil {
		sender.lc.Warnf("Skipping reading due to conversion error: %v", err)
		return err
	}

	readings.AddReading(
		path+suffix,
		[]string{measurement},
		[]client.TSDataType{dataType},
		value,
		nsecsTo(reading.Origin, sender.config.Precision),
	)
	return nil
}

// buildPath constructs the IoTDB path from the reading
func (sender *Sender) buildPath(reading dtos.BaseReading) string {
	var path strings.Builder
	path.WriteString("root")

	if sender.config.Prefix != "" {
		path.WriteString(".")
		path.WriteString(strings.TrimSuffix(sender.config.Prefix, "."))
	}

	if sender.config.DeviceNameToPath {
		path.WriteString(".")
		path.WriteString(reading.DeviceName)
	}

	if sender.config.DeviceProfileNameToPath {
		path.WriteString(".")
		path.WriteString(reading.ProfileName)
	}

	return path.String()
}

// extractMeasurement handles measurement name extraction
func (sender *Sender) extractMeasurement(resourceName string) (string, string) {
	if idx := strings.LastIndex(resourceName, "."); idx > -1 {
		return "." + resourceName[:idx], resourceName[idx+1:]
	}
	return "", resourceName
}

// convertDataType handles EdgeX to IoTDB data type conversion
func (sender *Sender) convertDataType(dataType string, value string) (client.TSDataType, []any, error) {
	switch {
	case dataType == "Bool":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return client.UNKNOWN, nil, fmt.Errorf("bool conversion failed: %w", err)
		}
		return client.BOOLEAN, []any{v}, nil

	case dataType == "Text":
		return client.TEXT, []any{value}, nil

	case dataType == "Binary":
		return client.BLOB, []any{value}, nil

	case (strings.Contains(dataType, "Uint") || strings.Contains(dataType, "Int")) && !strings.Contains(dataType, "64"):
		v, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return client.UNKNOWN, nil, fmt.Errorf("int32 conversion failed: %w", err)
		}
		return client.INT32, []any{int32(v)}, nil

	case (strings.Contains(dataType, "Uint") || strings.Contains(dataType, "Int")) && strings.Contains(dataType, "64"):
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return client.UNKNOWN, nil, fmt.Errorf("int64 conversion failed: %w", err)
		}
		return client.INT64, []any{v}, nil

	case strings.Contains(dataType, "Float") && !strings.Contains(dataType, "64"):
		v, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return client.UNKNOWN, nil, fmt.Errorf("float32 conversion failed: %w", err)
		}
		return client.FLOAT, []any{float32(v)}, nil

	case strings.Contains(dataType, "Float") && strings.Contains(dataType, "64"):
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return client.UNKNOWN, nil, fmt.Errorf("float64 conversion failed: %w", err)
		}
		return client.DOUBLE, []any{v}, nil

	default:
		return client.UNKNOWN, nil, fmt.Errorf("unsupported data type: %s", dataType)
	}
}

func nsecsTo(nsecs int64, precision iotdbDTOs.Precision) int64 {
	switch precision {
	case iotdbDTOs.S:
		return nsecs / 1e9
	case iotdbDTOs.MS:
		return nsecs / 1e6
	case iotdbDTOs.US:
		return nsecs / 1e3
	default:
		return nsecs
	}
}

func (sender *Sender) updateMetrics(data any) {
	byteData, err := json.Marshal(data)
	if err != nil {
		sender.lc.Warnf("Failed to marshal data for metrics: %v", err)
		return
	}
	sender.sizeMetrics.Update(int64(len(byteData)))
}

func (sender *Sender) setRetryData(ctx interfaces.AppFunctionContext, data any) error {
	if !sender.persistOnError {
		return nil
	}

	exportData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal retry data: %w", err)
	}

	ctx.SetRetryData(exportData)

	return nil
}

func (sender *Sender) initSessionManager() *sm.SessionManager {
	iotdbConfig := client.Config{
		Host:      sender.config.Host,
		Port:      sender.config.Port,
		UserName:  sender.config.UserName,
		Password:  sender.config.Password,
		FetchSize: sender.config.FetchSize,
		TimeZone:  sender.config.TimeZone,
	}

	sessionManager := sm.NewSessionManager(
		sender.lc,
		iotdbConfig,
		sender.config.RPCCompression,
		sender.config.ConnectionTimeout,
	)

	return sessionManager
}
