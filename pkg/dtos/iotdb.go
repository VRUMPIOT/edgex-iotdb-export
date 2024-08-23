package dtos

import (
	"github.com/apache/iotdb-client-go/client"
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
	Values       [][]interface{}
	Timestamps   []int64
}
