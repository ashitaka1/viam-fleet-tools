package workload

import (
	"context"
	"sync"
	"time"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

// AppStateCommand is the DoCommand key workload sends to each monitored
// component on every Readings call. Registrants should respond with a
// structpb-compatible map describing their current application state.
const AppStateCommand = "get_workload_state"

const appStatePollTimeout = 1 * time.Second

// resolveMonitored maps each MonitoredComponent.Key to the actual resource
// handle from deps, looking up by short name. Resources declared in config
// but missing from deps are warned and skipped — Validate has already
// listed them as required deps, so the framework normally rejects the
// config before we get here.
func resolveMonitored(
	deps resource.Dependencies,
	cfg *Config,
	logger logging.Logger,
) map[string]resource.Resource {
	if cfg == nil || len(cfg.MonitoredComponents) == 0 {
		return nil
	}
	out := make(map[string]resource.Resource, len(cfg.MonitoredComponents))
	for _, mc := range cfg.MonitoredComponents {
		var hit resource.Resource
		for n, r := range deps {
			if n.Name == mc.Name || n.ShortName() == mc.Name {
				hit = r
				break
			}
		}
		if hit == nil {
			logger.Warnf("workload: monitored component %q not present in dependencies; skipping", mc.Name)
			continue
		}
		out[mc.Key] = hit
	}
	return out
}

// gatherAppState polls every monitored component in parallel and returns
// the merged result, keyed by config key. Components that error or time
// out are dropped silently from this poll (logged at debug). Returns nil
// when there is nothing to include.
func gatherAppState(
	ctx context.Context,
	monitored map[string]resource.Resource,
	logger logging.Logger,
) map[string]any {
	if len(monitored) == 0 {
		return nil
	}
	cmd := map[string]any{AppStateCommand: true}

	type result struct {
		key   string
		state map[string]any
	}
	results := make(chan result, len(monitored))

	var wg sync.WaitGroup
	for key, res := range monitored {
		wg.Add(1)
		go func(key string, res resource.Resource) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, appStatePollTimeout)
			defer cancel()
			state, err := res.DoCommand(cctx, cmd)
			if err != nil {
				logger.Debugf("workload: monitored %q DoCommand failed: %v", key, err)
				return
			}
			if len(state) == 0 {
				return
			}
			results <- result{key: key, state: state}
		}(key, res)
	}
	wg.Wait()
	close(results)

	out := make(map[string]any, len(monitored))
	for r := range results {
		out[r.key] = r.state
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
