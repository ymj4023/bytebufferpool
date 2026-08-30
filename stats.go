package bytebufferpool

// Stats is a point-in-time view of Pool inventory.
type Stats struct {
	RetainedAvailable bool
	RetainedBuffers   int64
	RetainedCapacity  int64
}

// Stats returns a point-in-time view of Pool inventory.
func (p *Pool) Stats() Stats {
	if p.config.Mode != Bounded {
		return Stats{}
	}
	generation := p.current.Load()
	return Stats{
		RetainedAvailable: true,
		RetainedBuffers:   generation.retainedBuffers.Load(),
		RetainedCapacity:  generation.retainedBytes.Load(),
	}
}
