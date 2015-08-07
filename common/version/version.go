package version

const (
	version = "0.0.8"
)

var gitVersion string

func GetVersion() string {
	return version + "-" + gitVersion
}
