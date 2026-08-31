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
}

// String returns the stable name of status.
func (status ReleaseStatus) String() string {
	if int(status) < len(releaseStatusNames) {
		return releaseStatusNames[status]
	}
	return "ReleaseStatus(" + strconv.Itoa(int(status)) + ")"
}
