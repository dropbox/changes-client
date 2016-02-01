package version

const (
	version = "0.1"
)

var gitVersion string

func GetVersion() string {
	return version + "-" + gitVersion
}
