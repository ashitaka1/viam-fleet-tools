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
	TopNProcesses       int                  `json:"top_n_processes,omitempty"`
	ExtraDiskDevices    []string             `json:"extra_disk_devices,omitempty"`
	MonitoredComponents []MonitoredComponent `json:"monitored_components,omitempty"`
}

// MonitoredComponent declares a peer resource on the same machine whose
// app-level state should be merged into each workload reading. On every
// Readings call, workload sends DoCommand({AppStateCommand: true}) to the
// resource named Name and stores the response under app_state[Key].
type MonitoredComponent struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

func (cfg *Config) Validate(path string) ([]string, []string, error) {
	if cfg.TopNProcesses < 0 {
		return nil, nil, fmt.Errorf("%s: top_n_processes must be >= 0", path)
	}
	seenKeys := make(map[string]struct{}, len(cfg.MonitoredComponents))
	deps := make([]string, 0, len(cfg.MonitoredComponents))
	for i, mc := range cfg.MonitoredComponents {
		if mc.Name == "" {
			return nil, nil, fmt.Errorf("%s: monitored_components[%d].name is required", path, i)
		}
		if mc.Key == "" {
			return nil, nil, fmt.Errorf("%s: monitored_components[%d].key is required", path, i)
		}
		if _, dup := seenKeys[mc.Key]; dup {
			return nil, nil, fmt.Errorf("%s: monitored_components: duplicate key %q", path, mc.Key)
		}
		seenKeys[mc.Key] = struct{}{}
		deps = append(deps, mc.Name)
	}
	return deps, nil, nil
}

type Sensor struct {
	resource.Named

	logger   logging.Logger
	thermSrc *thermalSource
	rapl     []raplDomain

	// mu guards cfg, lastSnap, and monitored against concurrent Readings/Reconfigure.
	mu        sync.Mutex
	cfg       *Config
	lastSnap  *Snapshot
	monitored map[string]resource.Resource

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
	deps resource.Dependencies,
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
		monitored:  resolveMonitored(deps, conf, logger),
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
		"workload sensor ready: thermal_source=%s rapl_domains=%d top_n_processes=%d monitored=%d",
		therm, len(s.rapl), s.topNProcesses(), len(s.monitored),
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
	deps resource.Dependencies,
	raw resource.Config,
) error {
	conf, err := resource.NativeConfig[*Config](raw)
	if err != nil {
		return err
	}
	monitored := resolveMonitored(deps, conf, s.logger)

	s.mu.Lock()
	old := s.cfg
	oldMonitored := len(s.monitored)
	s.cfg = conf
	s.monitored = monitored
	s.mu.Unlock()

	changed := old == nil ||
		old.TopNProcesses != conf.TopNProcesses ||
		oldMonitored != len(monitored)
	if changed {
		s.logger.Infof("workload reconfigured: top_n_processes=%d monitored=%d",
			conf.TopNProcesses, len(monitored))
	}
	return nil
}

func (s *Sensor) Readings(
	ctx context.Context,
	_ map[string]interface{},
) (map[string]interface{}, error) {
	s.mu.Lock()
	prev := s.lastSnap
	monitored := s.monitored
	s.mu.Unlock()

	out, snap := s.sample(ctx, prev, time.Now())

	if app := gatherAppState(ctx, monitored, s.logger); app != nil {
		out["app_state"] = app
	}

	s.mu.Lock()
	s.lastSnap = snap
	s.mu.Unlock()

	return out, nil
}

func (s *Sensor) Close(_ context.Context) error {
	s.cancelFunc()
	return nil
}
