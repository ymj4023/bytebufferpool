package bytebufferpool

import "strconv"

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
	// DroppedUnpooled is valid storage within the pooling cutoff that has no Capacity Class.
	DroppedUnpooled
)

var releaseStatusNames = [...]string{
	"Retained",
	"DroppedFull",
	"DroppedOversize",
	"DroppedInvalid",
	"DroppedStale",
	"RejectedForeign",
	"RejectedDuplicate",
	"IgnoredNil",
	"DroppedUnpooled",
}

// String returns the stable name of status.
func (status ReleaseStatus) String() string {
	if int(status) < len(releaseStatusNames) {
		return releaseStatusNames[status]
	}
	return "ReleaseStatus(" + strconv.Itoa(int(status)) + ")"
}
