package bytebufferpool

import "fmt"

const defaultMaxPooledCapacity = 1 << 20

// Mode selects a Pool retention policy.
type Mode uint8

const (
	// Fast prioritizes throughput and treats retained storage as best-effort.
	Fast Mode = iota
	// Bounded enforces a retained-capacity budget.
	Bounded
)

// Config defines immutable Pool behavior.
type Config struct {
	Mode              Mode
	Classes           []int
	MaxPooledCapacity int
	MaxRetainedBytes  int64
	MaxAcquireSize    int
	ZeroOnRelease     bool
	ValidationEnabled bool
	StatsEnabled      bool
}

// DefaultConfig returns the default configuration for mode.
func DefaultConfig(mode Mode) Config {
	classes := make([]int, 0, 15)
	for size := 64; size <= defaultMaxPooledCapacity; size <<= 1 {
		classes = append(classes, size)
	}

	config := Config{
		Mode:              mode,
		Classes:           classes,
		MaxPooledCapacity: defaultMaxPooledCapacity,
	}
	if mode == Bounded {
		config.MaxRetainedBytes = 32 << 20
	}
	return config
}

func normalizeConfig(config Config) (Config, error) {
	if config.Mode != Fast && config.Mode != Bounded {
		return Config{}, fmt.Errorf("%w: unsupported mode %d", ErrInvalidConfig, config.Mode)
	}
	if config.MaxPooledCapacity < 0 {
		return Config{}, fmt.Errorf("%w: negative pooling cutoff %d", ErrInvalidConfig, config.MaxPooledCapacity)
	}
	if config.MaxPooledCapacity == 0 {
		config.MaxPooledCapacity = defaultMaxPooledCapacity
	}
	if config.MaxAcquireSize < 0 || (config.MaxAcquireSize > 0 && config.MaxAcquireSize < config.MaxPooledCapacity) {
		return Config{}, fmt.Errorf("%w: acquisition limit %d is below pooling cutoff %d", ErrInvalidConfig, config.MaxAcquireSize, config.MaxPooledCapacity)
	}
	if config.Mode == Fast && config.MaxRetainedBytes != 0 {
		return Config{}, fmt.Errorf("%w: Fast mode cannot promise retained bytes", ErrInvalidConfig)
	}
	if config.Mode == Bounded && config.MaxRetainedBytes <= 0 {
		return Config{}, fmt.Errorf("%w: Bounded mode requires a positive retained-capacity budget", ErrInvalidConfig)
	}
	if len(config.Classes) == 0 {
		config.Classes = DefaultConfig(config.Mode).Classes
	} else {
		config.Classes = append([]int(nil), config.Classes...)
	}
	previous := 0
	for _, size := range config.Classes {
		if size <= previous {
			return Config{}, fmt.Errorf("%w: Capacity Classes must be positive and strictly increasing", ErrInvalidConfig)
		}
		if size > config.MaxPooledCapacity {
			return Config{}, fmt.Errorf("%w: Capacity Class %d exceeds pooling cutoff %d", ErrInvalidConfig, size, config.MaxPooledCapacity)
		}
		previous = size
	}
	return config, nil
}

// PowerOfTwo returns every power-of-two Capacity Class from minimum through maximum.
func PowerOfTwo(minimum, maximum int) ([]int, error) {
	if !isPowerOfTwo(minimum) || !isPowerOfTwo(maximum) || minimum > maximum {
		return nil, fmt.Errorf("%w: power-of-two bounds %d..%d", ErrInvalidConfig, minimum, maximum)
	}

	classes := make([]int, 0)
	for size := minimum; ; size <<= 1 {
		classes = append(classes, size)
		if size == maximum {
			return classes, nil
		}
	}
}

func isPowerOfTwo(value int) bool {
	return value > 0 && value&(value-1) == 0
}
