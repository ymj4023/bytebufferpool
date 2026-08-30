module github.com/ymj4023/bytebufferpool/benchmarks

go 1.26.0

require (
	github.com/libp2p/go-buffer-pool v0.1.0
	github.com/oxtoacart/bpool v0.0.0-20190530202638-03653db5a59c
	github.com/prometheus/prometheus v0.314.0
	github.com/valyala/bytebufferpool v1.0.0
	github.com/ymj4023/bytebufferpool v0.0.0
	google.golang.org/grpc v1.83.2
)

require (
	github.com/aclements/go-moremath v0.0.0-20210112150236-f10218a38794 // indirect
	golang.org/x/perf v0.0.0-20260825160852-19be9d8e6c70 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace github.com/ymj4023/bytebufferpool => ..

tool golang.org/x/perf/cmd/benchstat
