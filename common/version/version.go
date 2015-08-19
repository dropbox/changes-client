package version

const (
	version = "0.0.9"
)

var gitVersion string

func GetVersion() string {
	return version + "-" + gitVersion
}
