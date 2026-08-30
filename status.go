package bytebufferpool

// ReleaseStatus reports what a Pool did with released Backing Storage.
type ReleaseStatus uint8

const (
	Retained ReleaseStatus = iota
	DroppedFull
	DroppedOversize
	DroppedInvalid
	DroppedStale
	RejectedForeign
	RejectedDuplicate
	IgnoredNil
)
