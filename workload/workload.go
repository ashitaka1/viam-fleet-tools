package workload

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	sensor "go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

var Model = resource.NewModel("avery", "fleet-tools", "workload")

const (
	defaultTopNProcesses = 3
	warmupInterval       = 200 * time.Millisecond
	sysfsRoot            = "/sys"
)

type Config struct {
	TopNProcesses    int      `json:"top_n_processes,omitempty"`
	ExtraDiskDevices []string `json:"extra_disk_devices,omitempty"`
}

func (cfg *Config) Validate(path string) ([]string, []string, error) {
	if cfg.TopNProcesses < 0 {
		return nil, nil, fmt.Errorf("%s: top_n_processes must be >= 0", path)
	}
	return nil, nil, nil
}

type Sensor struct {
	resource.Named

	logger   logging.Logger
	thermSrc *thermalSource
	rapl     []raplDomain

	// mu guards cfg and lastSnap against concurrent Readings/Reconfigure.
	mu       sync.Mutex
	cfg      *Config
	lastSnap *Snapshot

	cancelCtx  context.Context
	cancelFunc func()
}

func init() {
	resource.RegisterComponent(sensor.API, Model,
		resource.Registration[sensor.Sensor, *Config]{
			Constructor: newRegistered,
		},
	)
}

func newRegistered(
	ctx context.Context,
	deps resource.Dependencies,
	raw resource.Config,
	logger logging.Logger,
) (sensor.Sensor, error) {
	conf, err := resource.NativeConfig[*Config](raw)
	if err != nil {
		return nil, err
	}
	return New(ctx, deps, raw.ResourceName(), conf, logger)
}

func New(
	ctx context.Context,
	_ resource.Dependencies,
	name resource.Name,
	conf *Config,
	logger logging.Logger,
) (sensor.Sensor, error) {
	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	thermSrc, _ := resolveTempSource(sysfsRoot)
	rapl := discoverRAPL()

	s := &Sensor{
		Named:      name.AsNamed(),
		logger:     logger,
		cfg:        conf,
		thermSrc:   thermSrc,
		rapl:       rapl,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}

	s.logCapabilities()

	// Two samples a brief interval apart prime gopsutil's internal CPU
	// counter state so the first Readings() call has non-zero rates.
	_, s.lastSnap = s.sample(ctx, nil, time.Now())
	select {
	case <-time.After(warmupInterval):
	case <-ctx.Done():
		cancelFunc()
		return nil, ctx.Err()
	case <-cancelCtx.Done():
		return nil, cancelCtx.Err()
	}
	_, s.lastSnap = s.sample(ctx, s.lastSnap, time.Now())

	return s, nil
}

func (s *Sensor) logCapabilities() {
	therm := "none"
	if s.thermSrc != nil {
		therm = s.thermSrc.dir
	}
	s.logger.Infof(
		"workload sensor ready: thermal_source=%s rapl_domains=%d top_n_processes=%d",
		therm, len(s.rapl), s.topNProcesses(),
	)
}

func (s *Sensor) sample(ctx context.Context, prev *Snapshot, now time.Time) (map[string]any, *Snapshot) {
	out := map[string]any{}

	memFields, swap := sampleMemory(ctx, prev, now)
	maps.Copy(out, memFields)

	diskFields, disks := sampleDisk(ctx, prev, now)
	maps.Copy(out, diskFields)

	netFields, nets := sampleNetwork(ctx, prev, now)
	maps.Copy(out, netFields)

	raplFields, rapl := sampleRAPL(s.rapl, prev, now)
	maps.Copy(out, raplFields)

	maps.Copy(out, sampleCPU(ctx))
	maps.Copy(out, sampleThermal(s.thermSrc))
	maps.Copy(out, sampleGPU(ctx))
	maps.Copy(out, sampleTopProcesses(ctx, s.topNProcesses()))
	maps.Copy(out, sampleSystem(ctx, now))

	snap := &Snapshot{
		Taken:   now,
		Disks:   disks,
		Nets:    nets,
		RAPL:    rapl,
		PSwpIn:  swap.pswpin,
		PSwpOut: swap.pswpout,
	}
	return out, snap
}

func (s *Sensor) topNProcesses() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg == nil || s.cfg.TopNProcesses == 0 {
		return defaultTopNProcesses
	}
	return s.cfg.TopNProcesses
}

func (s *Sensor) Reconfigure(
	_ context.Context,
	_ resource.Dependencies,
	raw resource.Config,
) error {
	conf, err := resource.NativeConfig[*Config](raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.cfg
	s.cfg = conf
	if old == nil || old.TopNProcesses != conf.TopNProcesses {
		s.logger.Infof("workload reconfigured: top_n_processes=%d", conf.TopNProcesses)
	}
	return nil
}

func (s *Sensor) Readings(
	ctx context.Context,
	_ map[string]interface{},
) (map[string]interface{}, error) {
	s.mu.Lock()
	prev := s.lastSnap
	s.mu.Unlock()

	out, snap := s.sample(ctx, prev, time.Now())

	s.mu.Lock()
	s.lastSnap = snap
	s.mu.Unlock()

	return out, nil
}

func (s *Sensor) Close(_ context.Context) error {
	s.cancelFunc()
	return nil
}
