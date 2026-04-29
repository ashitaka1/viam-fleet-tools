package workload

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type raplDomain struct {
	dir       string
	wrapBound uint64 // max_energy_range_uj, used by rateDelta for wrap recovery.
	name      string
}

// discoverRAPL skips intel-rapl sub-domains (e.g. intel-rapl:0:0 = "core")
// and reports only top-level package domains, since per-package power is
// what workload characterization needs.
func discoverRAPL() []raplDomain {
	const root = "/sys/class/powercap"
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var domains []raplDomain
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "intel-rapl:") {
			continue
		}
		rest := strings.TrimPrefix(name, "intel-rapl:")
		if strings.ContainsRune(rest, ':') {
			continue
		}
		dir := filepath.Join(root, name)
		domainName, _ := readTrimmed(filepath.Join(dir, "name"))
		boundStr, err := readTrimmed(filepath.Join(dir, "max_energy_range_uj"))
		if err != nil {
			continue
		}
		bound, err := strconv.ParseUint(boundStr, 10, 64)
		if err != nil {
			continue
		}
		domains = append(domains, raplDomain{dir: dir, wrapBound: bound, name: domainName})
	}
	return domains
}

func sampleRAPL(domains []raplDomain, prev *Snapshot, now time.Time) (map[string]any, map[string]uint64) {
	if len(domains) == 0 {
		return map[string]any{}, nil
	}
	curr := map[string]uint64{}
	for _, d := range domains {
		s, err := readTrimmed(filepath.Join(d.dir, "energy_uj"))
		if err != nil {
			continue
		}
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			continue
		}
		curr[d.dir] = v
	}
	out := map[string]any{}
	if prev == nil || prev.RAPL == nil {
		return out, curr
	}
	dt := now.Sub(prev.Taken)
	var totalWatts float64
	var saw bool
	for _, d := range domains {
		c, hasCurr := curr[d.dir]
		p, hasPrev := prev.RAPL[d.dir]
		if !hasCurr || !hasPrev {
			continue
		}
		uJperSec := rateDelta(c, p, d.wrapBound, dt)
		totalWatts += uJperSec / 1_000_000.0
		saw = true
	}
	if saw {
		out["pkg_watts"] = round2(totalWatts)
	}
	return out, curr
}
