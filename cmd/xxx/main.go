package main

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func main() {
	fooo()
	
	//defaultQueryCfg := query_config.Config{
	//	State: &query_config.State{},
	//}
	//defaultQueryCfg.SetDefaultsIfNotSet()
	//nvidia_query.SetDefaultPoller()
	//nvidia_query.GetDefaultPoller().Start(context.Background(), defaultQueryCfg, "test")
	//for {
	//	time.Sleep(1 * time.Second)
	//}
}

func fooo() {
	nvmlInterface := nvml.New()
	ret := nvmlInterface.Init()
	if ret != nvml.SUCCESS {
		panic(ret.String())
	}
	devLib := device.New(nvmlInterface)
	devices, err := devLib.GetDevices()
	if err != nil {
		panic(err)
	}
	for {
		for _, v := range devices {
			id, _ := v.GetUUID()
			fmt.Printf("process %v", id)
			for link := 0; link < nvml.NVLINK_MAX_LINKS; link++ {
				values := []nvml.FieldValue{
					{FieldId: nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX}, // NVLink RX Data throughput + protocol overhead in KiB
					{FieldId: nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX}, // NVLink TX Data throughput + protocol overhead in KiB
				}
				ret = nvml.DeviceGetFieldValues(v, values)
				if ret == nvml.SUCCESS {
					for _, value := range values {
						if value.FieldId == nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX {
							fmt.Println(binary.NativeEndian.Uint64(value.Value[:]) * 1024)
						}
						if value.FieldId == nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX {
							fmt.Println(binary.NativeEndian.Uint64(value.Value[:]) * 1024)
						}
					}
				}
			}
		}

		time.Sleep(5 * time.Second)
	}
}
