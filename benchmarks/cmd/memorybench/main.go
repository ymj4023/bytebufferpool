package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"sync"
	"time"

	libp2ppool "github.com/libp2p/go-buffer-pool"
	prompool "github.com/prometheus/prometheus/util/pool"
	bytebufferpool "github.com/ymj4023/bytebufferpool"
	grpcmem "google.golang.org/grpc/mem"
)

type options struct {
	contender       string
	smallSize       int
	smallIterations int
	peakSize        int
	peakCount       int
	repeat          int
	runIndex        int
	output          string
	profileDir      string
}

type sample struct {
	Phase                string `json:"phase"`
	HeapAlloc            uint64 `json:"heap_alloc"`
	HeapInuse            uint64 `json:"heap_inuse"`
	HeapSys              uint64 `json:"heap_sys"`
	NumGC                uint32 `json:"num_gc"`
	RetainedAvailable    bool   `json:"retained_available"`
	RetainedStorageCount int64  `json:"retained_storage_count"`
	RetainedCapacity     int64  `json:"retained_capacity"`
}

type result struct {
	Contender       string        `json:"contender"`
	Run             int           `json:"run"`
	GoVersion       string        `json:"go_version"`
	GOOS            string        `json:"goos"`
	GOARCH          string        `json:"goarch"`
	LogicalCPUs     int           `json:"logical_cpus"`
	GOMAXPROCS      int           `json:"gomaxprocs"`
	GOGC            string        `json:"gogc"`
	MemoryLimit     int64         `json:"memory_limit"`
	VCSRevision     string        `json:"vcs_revision"`
	VCSModified     bool          `json:"vcs_modified"`
	SmallSize       int           `json:"small_size"`
	SmallIterations int           `json:"small_iterations"`
	PeakSize        int           `json:"peak_size"`
	PeakCount       int           `json:"peak_count"`
	Elapsed         time.Duration `json:"elapsed_ns"`
	Samples         []sample      `json:"samples"`
	HeapProfile     string        `json:"heap_profile,omitempty"`
}

type summary struct {
	Contender            string `json:"contender"`
	Phase                string `json:"phase"`
	Runs                 int    `json:"runs"`
	HeapAllocMin         uint64 `json:"heap_alloc_min"`
	HeapAllocMean        uint64 `json:"heap_alloc_mean"`
	HeapAllocMax         uint64 `json:"heap_alloc_max"`
	HeapInuseMin         uint64 `json:"heap_inuse_min"`
	HeapInuseMean        uint64 `json:"heap_inuse_mean"`
	HeapInuseMax         uint64 `json:"heap_inuse_max"`
	RetainedCapacityMean int64  `json:"retained_capacity_mean"`
}

type suiteResult struct {
	Results   []result  `json:"results"`
	Summaries []summary `json:"summaries"`
}

type borrowed struct {
	bytes   []byte
	pointer *[]byte
}

type memoryPool interface {
	Acquire(size int, value *borrowed)
	Release(*borrowed)
	Inventory() (buffers, capacity int64, available bool)
}

type makePool struct{}

func (makePool) Acquire(size int, value *borrowed) { value.bytes = make([]byte, size) }
func (makePool) Release(value *borrowed)           { value.bytes = nil }
func (makePool) Inventory() (int64, int64, bool)   { return 0, 0, false }

type syncPool struct {
	maxRetained int
	pool        sync.Pool
}

func (p *syncPool) Acquire(size int, value *borrowed) {
	if cached := p.pool.Get(); cached != nil {
		value.pointer = cached.(*[]byte)
	} else {
		value.pointer = new([]byte)
	}
	if cap(*value.pointer) < size {
		*value.pointer = make([]byte, size)
	} else {
		*value.pointer = (*value.pointer)[:size]
	}
	value.bytes = *value.pointer
}
func (p *syncPool) Release(value *borrowed) {
	*value.pointer = value.bytes[:0]
	if p.maxRetained == 0 || cap(value.bytes) <= p.maxRetained {
		p.pool.Put(value.pointer)
	}
	value.bytes = nil
	value.pointer = nil
}
func (*syncPool) Inventory() (int64, int64, bool) { return 0, 0, false }

type projectPool struct{ pool *bytebufferpool.Pool }

func (p *projectPool) Acquire(size int, value *borrowed) {
	value.bytes = p.pool.AcquireSlice(size)
}
func (p *projectPool) Release(value *borrowed) {
	p.pool.ReleaseSlice(value.bytes)
	value.bytes = nil
}
func (p *projectPool) Inventory() (int64, int64, bool) {
	stats := p.pool.Stats()
	return stats.RetainedStorageCount, stats.RetainedCapacity, stats.RetainedAvailable
}

type libp2pPool struct{ pool libp2ppool.BufferPool }

func (p *libp2pPool) Acquire(size int, value *borrowed) { value.bytes = p.pool.Get(size) }
func (p *libp2pPool) Release(value *borrowed) {
	p.pool.Put(value.bytes)
	value.bytes = nil
}
func (*libp2pPool) Inventory() (int64, int64, bool) { return 0, 0, false }

type grpcPool struct{ pool grpcmem.BufferPool }

func (p *grpcPool) Acquire(size int, value *borrowed) {
	value.pointer = p.pool.Get(size)
	value.bytes = *value.pointer
}
func (p *grpcPool) Release(value *borrowed) {
	p.pool.Put(value.pointer)
	value.bytes = nil
	value.pointer = nil
}
func (*grpcPool) Inventory() (int64, int64, bool) { return 0, 0, false }

type prometheusPool struct{ pool *prompool.Pool }

func (p *prometheusPool) Acquire(size int, value *borrowed) {
	buffer := p.pool.Get(size).([]byte)
	value.bytes = buffer[:size]
}
func (p *prometheusPool) Release(value *borrowed) {
	p.pool.Put(value.bytes)
	value.bytes = nil
}
func (*prometheusPool) Inventory() (int64, int64, bool) { return 0, 0, false }

var contenderNames = []string{
	"make",
	"sync-naive",
	"sync-cutoff",
	"project-fast",
	"project-fast-zero",
	"project-fast-stats",
	"project-fast-validation",
	"project-bounded",
	"libp2p-v0.1.0",
	"grpc-v1.83.2-zero-on-acquire",
	"prometheus-v0.314.0",
}

func main() {
	configuration := parseFlags()
	if err := run(configuration); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() options {
	configuration := options{}
	flag.StringVar(&configuration.contender, "contender", "all", "contender name or all")
	flag.IntVar(&configuration.smallSize, "small-size", 1024, "steady request size")
	flag.IntVar(&configuration.smallIterations, "small-iterations", 10000, "steady requests per phase")
	flag.IntVar(&configuration.peakSize, "peak-size", 8<<20, "concurrent peak request size")
	flag.IntVar(&configuration.peakCount, "peak-count", 8, "concurrent peak request count")
	flag.IntVar(&configuration.repeat, "repeat", 1, "isolated runs per contender")
	flag.IntVar(&configuration.runIndex, "run-index", 1, "child run index")
	flag.StringVar(&configuration.output, "output", "", "write JSON to this path instead of stdout")
	flag.StringVar(&configuration.profileDir, "profile-dir", "", "write one heap profile per child")
	flag.Parse()
	return configuration
}

func run(configuration options) error {
	if configuration.contender == "all" {
		suite, err := runSuite(configuration)
		if err != nil {
			return err
		}
		return writeJSON(configuration.output, suite)
	}
	pool, err := newMemoryPool(configuration.contender)
	if err != nil {
		return err
	}
	measurement, err := runScenario(configuration, pool)
	if err != nil {
		return err
	}
	return writeJSON(configuration.output, measurement)
}

func runSuite(configuration options) (suiteResult, error) {
	executable, err := os.Executable()
	if err != nil {
		return suiteResult{}, err
	}
	suite := suiteResult{}
	for _, name := range contenderNames {
		for runIndex := 1; runIndex <= configuration.repeat; runIndex++ {
			arguments := []string{
				"-contender", name,
				"-small-size", fmt.Sprint(configuration.smallSize),
				"-small-iterations", fmt.Sprint(configuration.smallIterations),
				"-peak-size", fmt.Sprint(configuration.peakSize),
				"-peak-count", fmt.Sprint(configuration.peakCount),
				"-run-index", fmt.Sprint(runIndex),
			}
			if configuration.profileDir != "" {
				arguments = append(arguments, "-profile-dir", configuration.profileDir)
			}
			command := exec.Command(executable, arguments...)
			command.Stderr = os.Stderr
			output, err := command.Output()
			if err != nil {
				return suiteResult{}, fmt.Errorf("run %s/%d: %w", name, runIndex, err)
			}
			var measurement result
			if err := json.Unmarshal(output, &measurement); err != nil {
				return suiteResult{}, fmt.Errorf("decode %s/%d: %w", name, runIndex, err)
			}
			suite.Results = append(suite.Results, measurement)
		}
	}
	suite.Summaries = summarize(suite.Results)
	return suite, nil
}

func runScenario(configuration options, pool memoryPool) (result, error) {
	if configuration.smallSize <= 0 || configuration.smallIterations < 0 || configuration.peakSize <= 0 || configuration.peakCount <= 0 {
		return result{}, errors.New("sizes and peak count must be positive; iterations cannot be negative")
	}
	started := time.Now()
	revision, modified := vcsMetadata()
	measurement := result{
		Contender:       configuration.contender,
		Run:             configuration.runIndex,
		GoVersion:       runtime.Version(),
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
		LogicalCPUs:     runtime.NumCPU(),
		GOMAXPROCS:      runtime.GOMAXPROCS(0),
		GOGC:            environmentOrDefault("GOGC"),
		MemoryLimit:     debug.SetMemoryLimit(-1),
		VCSRevision:     revision,
		VCSModified:     modified,
		SmallSize:       configuration.smallSize,
		SmallIterations: configuration.smallIterations,
		PeakSize:        configuration.peakSize,
		PeakCount:       configuration.peakCount,
	}

	runtime.GC()
	runSmall(pool, configuration.smallSize, configuration.smallIterations)
	measurement.Samples = append(measurement.Samples, takeSample("steady-small", pool))

	peak := acquirePeak(pool, configuration.peakSize, configuration.peakCount)
	measurement.Samples = append(measurement.Samples, takeSample("peak-held", pool))
	releasePeak(pool, peak)
	peak = nil
	measurement.Samples = append(measurement.Samples, takeSample("peak-released", pool))

	runSmall(pool, configuration.smallSize, configuration.smallIterations)
	measurement.Samples = append(measurement.Samples, takeSample("recovered-small", pool))
	runtime.GC()
	measurement.Samples = append(measurement.Samples, takeSample("gc-1", pool))
	runtime.GC()
	measurement.Samples = append(measurement.Samples, takeSample("gc-2", pool))

	if configuration.profileDir != "" {
		if err := os.MkdirAll(configuration.profileDir, 0o755); err != nil {
			return result{}, err
		}
		profilePath := filepath.Join(configuration.profileDir, fmt.Sprintf("%s-run-%02d.pprof", configuration.contender, configuration.runIndex))
		file, err := os.Create(profilePath)
		if err != nil {
			return result{}, err
		}
		err = pprof.WriteHeapProfile(file)
		closeErr := file.Close()
		if err != nil {
			return result{}, err
		}
		if closeErr != nil {
			return result{}, closeErr
		}
		measurement.HeapProfile = profilePath
	}
	measurement.Elapsed = time.Since(started)
	return measurement, nil
}

func vcsMetadata() (revision string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unavailable", false
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		revision = "unavailable"
	}
	return revision, modified
}

func runSmall(pool memoryPool, size, iterations int) {
	var value borrowed
	for i := 0; i < iterations; i++ {
		pool.Acquire(size, &value)
		touch(value.bytes)
		pool.Release(&value)
	}
}

func acquirePeak(pool memoryPool, size, count int) []borrowed {
	values := make([]borrowed, count)
	var wait sync.WaitGroup
	for i := range values {
		wait.Add(1)
		go func(value *borrowed) {
			defer wait.Done()
			pool.Acquire(size, value)
			touch(value.bytes)
		}(&values[i])
	}
	wait.Wait()
	return values
}

func releasePeak(pool memoryPool, values []borrowed) {
	var wait sync.WaitGroup
	for i := range values {
		wait.Add(1)
		go func(value *borrowed) {
			defer wait.Done()
			pool.Release(value)
		}(&values[i])
	}
	wait.Wait()
}

func takeSample(phase string, pool memoryPool) sample {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	buffers, capacity, available := pool.Inventory()
	return sample{
		Phase:                phase,
		HeapAlloc:            memory.HeapAlloc,
		HeapInuse:            memory.HeapInuse,
		HeapSys:              memory.HeapSys,
		NumGC:                memory.NumGC,
		RetainedAvailable:    available,
		RetainedStorageCount: buffers,
		RetainedCapacity:     capacity,
	}
}

func touch(buffer []byte) {
	if len(buffer) == 0 {
		return
	}
	buffer[0]++
	buffer[len(buffer)-1]++
}

func newMemoryPool(name string) (memoryPool, error) {
	switch name {
	case "make":
		return makePool{}, nil
	case "sync-naive":
		return &syncPool{}, nil
	case "sync-cutoff":
		return &syncPool{maxRetained: 1 << 20}, nil
	case "project-fast":
		return &projectPool{pool: mustProjectPool(bytebufferpool.DefaultConfig(bytebufferpool.Fast))}, nil
	case "project-fast-zero":
		config := bytebufferpool.DefaultConfig(bytebufferpool.Fast)
		config.ZeroOnRelease = true
		return &projectPool{pool: mustProjectPool(config)}, nil
	case "project-fast-stats":
		config := bytebufferpool.DefaultConfig(bytebufferpool.Fast)
		config.StatsEnabled = true
		return &projectPool{pool: mustProjectPool(config)}, nil
	case "project-fast-validation":
		config := bytebufferpool.DefaultConfig(bytebufferpool.Fast)
		config.ValidationEnabled = true
		return &projectPool{pool: mustProjectPool(config)}, nil
	case "project-bounded":
		return &projectPool{pool: mustProjectPool(bytebufferpool.DefaultConfig(bytebufferpool.Bounded))}, nil
	case "libp2p-v0.1.0":
		return &libp2pPool{}, nil
	case "grpc-v1.83.2-zero-on-acquire":
		pool, err := grpcmem.NewBinaryTieredBufferPool(6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20)
		if err != nil {
			return nil, err
		}
		return &grpcPool{pool: pool}, nil
	case "prometheus-v0.314.0":
		return &prometheusPool{pool: prompool.New(64, 1<<20, 2, func(size int) any {
			return make([]byte, 0, size)
		})}, nil
	default:
		return nil, fmt.Errorf("unknown contender %q", name)
	}
}

func mustProjectPool(config bytebufferpool.Config) *bytebufferpool.Pool {
	pool, err := bytebufferpool.New(config)
	if err != nil {
		panic(err)
	}
	return pool
}

func summarize(results []result) []summary {
	type key struct{ contender, phase string }
	grouped := make(map[key][]sample)
	for _, measurement := range results {
		for _, current := range measurement.Samples {
			grouped[key{measurement.Contender, current.Phase}] = append(grouped[key{measurement.Contender, current.Phase}], current)
		}
	}
	keys := make([]key, 0, len(grouped))
	for current := range grouped {
		keys = append(keys, current)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].contender == keys[j].contender {
			return keys[i].phase < keys[j].phase
		}
		return keys[i].contender < keys[j].contender
	})

	summaries := make([]summary, 0, len(keys))
	for _, current := range keys {
		samples := grouped[current]
		item := summary{
			Contender:    current.contender,
			Phase:        current.phase,
			Runs:         len(samples),
			HeapAllocMin: ^uint64(0),
			HeapInuseMin: ^uint64(0),
		}
		var allocTotal, inuseTotal uint64
		var retainedTotal int64
		for _, currentSample := range samples {
			item.HeapAllocMin = min(item.HeapAllocMin, currentSample.HeapAlloc)
			item.HeapAllocMax = max(item.HeapAllocMax, currentSample.HeapAlloc)
			item.HeapInuseMin = min(item.HeapInuseMin, currentSample.HeapInuse)
			item.HeapInuseMax = max(item.HeapInuseMax, currentSample.HeapInuse)
			allocTotal += currentSample.HeapAlloc
			inuseTotal += currentSample.HeapInuse
			retainedTotal += currentSample.RetainedCapacity
		}
		item.HeapAllocMean = allocTotal / uint64(len(samples))
		item.HeapInuseMean = inuseTotal / uint64(len(samples))
		item.RetainedCapacityMean = retainedTotal / int64(len(samples))
		summaries = append(summaries, item)
	}
	return summaries
}

func writeJSON(path string, value any) error {
	var output *os.File
	if path == "" {
		output = os.Stdout
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		file, err := os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		output = file
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func environmentOrDefault(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return "default"
}
