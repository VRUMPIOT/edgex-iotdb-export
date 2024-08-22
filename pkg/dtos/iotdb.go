package dtos

import "github.com/apache/iotdb-client-go/client"

type Config struct {
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
}

type Readings struct {
	DeviceIds    []string
	Measurements [][]string
	DataTypes    [][]client.TSDataType
	Values       [][]interface{}
	Timestamps   []int64
}
