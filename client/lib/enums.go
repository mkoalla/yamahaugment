package lib

type SourceType uint8

const (
	None SourceType = iota
	ContentFile
	ContentDirect
)

func (s SourceType) String() string {
	switch s {
	case None:
		return "None"
	case ContentFile:
		return "ContentFile"
	case ContentDirect:
		return "ContentDirect"
	default:
		return "Unknown"
	}
}
func ToSourceType(s string) SourceType {
	switch s {
	case "ContentFile":
		return ContentFile
	case "ContentDirect":
		return ContentDirect
	default:
		return None
	}
}

type PlayerStatus uint8

const (
	Unknown PlayerStatus = iota
	Stopped
	Paused
	Playing
	Awaiting
)

func (p PlayerStatus) String() string {
	switch p {
	case Stopped:
		return "Stopped"
	case Paused:
		return "Paused"
	case Playing:
		return "Playing"
	case Awaiting:
		return "Awaiting"
	default:
		return "Unknown"
	}
}
func ToPlayerStatus(s string) PlayerStatus {
	switch s {
	case "Stopped":
		return Stopped
	case "Paused":
		return Paused
	case "Playing":
		return Playing
	case "Awaiting":
		return Awaiting
	default:
		return Unknown
	}
}
