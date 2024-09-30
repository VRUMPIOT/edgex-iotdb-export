# IotDB Export

This an EdgeX Foundry application service for sending data (readings) to [IotDB](https://iotdb.apache.org/).

## IotDB Version

This example was created with IotDB version 1.3.2.

## IotDB Setup and Configuration

See the [IotDB documentation](https://iotdb.apache.org/UserGuide/latest/QuickStart/QuickStart.html) on how to setup and configure IotDB.

## Build and Run

A Makefile has been provide to easily create and execute this service. In order to build the micro service executable run the `make build` from the root directory.

Once the micro service has successfully been compiled, run the executable created in the root directory with `./iotdb-export`.

## Configuration

In order to supply data from your EdgeX instance to your IotDB instance, you must provide the . Open the `configuration.toml` file in the `res` folder and change the attributes in the `IotDBConfig` configuration section.

```yaml

IotDBConfig:
  Prefix: ""
  DeviceNameToPath: false #Put Device Name to IoTDB root path after prefix
  DeviceProfileNameToPath: false  #Put Device Profile Name to IoTDB root path after prefix or/and Device Name
  Host: localhost
  Port: '6667'
  UserName: root
  Password: root
  FetchSize: 1024
  TimeZone: UTC
  ConnectRetryMax: 5
  RPCCompression: false
  ConnectionTimeout: 5
  Precision: "s"
```

## Data Payload

EdgeX utilizes the IoTDB payload format to export EdgeX Readings (associated with each EdgeX Event) to IoTDB. The data must be structured as follows when sent to IoTDB:

```json
[
  {
    "Timestamps": ["2020-07-08 16:16:07.263657+00:00"],
    "DeviceIds": ["motor1"],
    "Measurements": [["current"], ["rpm"], ["voltage"]],
    "Values": [[342.1], [1003], [242.1]],
    "DataTypes": [["FLOAT"], ["INT32"], ["FLOAT"]]
  }
]
```

### Field Descriptions

- **Timestamps**: Each timestamp corresponds to a block of measurements.

- **DeviceIds**: The `DeviceIds` field follows the format `root.<prefix>.<device_name>.<profile_name>.<measurement_traits>`. For example, if a measurement is named `test.current.phase1`, `phase1` will be the measurement, and `test.current` will be concatenated into the `DeviceId`.

- **Measurements**: This field lists the measurements for each timestamp. Each measurement name must be specified in a nested array.

- **Values**: The `Values` field contains one value per timestamp for each corresponding measurement.

- **DataTypes**: The `DataTypes` field specifies the data type of each measurement, listed in the same order as the measurements.
