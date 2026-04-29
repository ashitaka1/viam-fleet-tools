package main

import (
	"fleet-tools/workload"

	sensor "go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
)

func main() {
	module.ModularMain(
		resource.APIModel{API: sensor.API, Model: workload.Model},
	)
}
