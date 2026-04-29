package main

import (
	"context"

	"fleet-tools/workload"

	sensor "go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

func main() {
	if err := realMain(); err != nil {
		panic(err)
	}
}

func realMain() error {
	ctx := context.Background()
	logger := logging.NewLogger("cli")

	deps := resource.Dependencies{}

	cfg := workload.Config{}

	s, err := workload.New(ctx, deps, sensor.Named("workload-cli"), &cfg, logger)
	if err != nil {
		return err
	}
	defer s.Close(ctx)

	return nil
}
