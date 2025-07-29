package dtos

import (
	"github.com/apache/iotdb-client-go/v2/client"
)

type Precision string

const (
	S  Precision = "s"
	MS Precision = "ms"
	US Precision = "us"
	NS Precision = "ns"
)

type Readings struct {
	DeviceIds    []string
	Measurements [][]string
	DataTypes    [][]client.TSDataType
	Values       [][]any
	Timestamps   []int64
}

func NewReadings(capacity int) *Readings {
	return &Readings{
		DeviceIds:    make([]string, 0, capacity),
		Measurements: make([][]string, 0, capacity),
		DataTypes:    make([][]client.TSDataType, 0, capacity),
		Values:       make([][]any, 0, capacity),
		Timestamps:   make([]int64, 0, capacity),
	}
}

func (rd *Readings) AddReading(deviceId string, measurement []string,
	dataType []client.TSDataType, value []any, timestamp int64) {
	rd.DeviceIds = append(rd.DeviceIds, deviceId)
	rd.Measurements = append(rd.Measurements, measurement)
	rd.DataTypes = append(rd.DataTypes, dataType)
	rd.Values = append(rd.Values, value)
	rd.Timestamps = append(rd.Timestamps, timestamp)
}
