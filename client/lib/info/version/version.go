package version

var (
	version = "vUnknown"
	source  = "Unknown"
)

func Version() string {
	return version
}
func Source() string {
	return source
}
