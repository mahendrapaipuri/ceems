package collector

import (
	"log/slog"

	"kernel.org/pub/linux/libs/security/libcap/cap"
)

// setupCollectorCaps sets up the required capabilities for collector.
func setupCollectorCaps(logger *slog.Logger, subSystem string, capabilities []string) []cap.Value {
	// If there is nothing to setup, return
	if len(capabilities) == 0 {
		return nil
	}

	// Make a allocation
	if _, ok := collectorCaps[subSystem]; !ok {
		collectorCaps[subSystem] = make([]cap.Value, 0)
	}

	var caps []cap.Value

	for _, name := range capabilities {
		value, err := cap.FromName(name)
		if err != nil {
			logger.Error("Error parsing capability %s: %w", name, err)

			continue
		}

		caps = append(caps, value)
		collectorCaps[subSystem] = append(collectorCaps[subSystem], value)
	}

	return caps
}
