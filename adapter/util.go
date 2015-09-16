package adapter

import (
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/common/glob"
)


func CollectArtifactsIn(dir string, artifacts []string, clientLog *client.Log) ([]string, error) {
    matches, skipped, err := glob.GlobTreeRegular(dir, artifacts)
    for i, s := range skipped {
        if i == 10 {
            clientLog.Printf("And %d more.", len(skipped) - i)
            break
        }
        clientLog.Printf("Skipped matching non-regular file %s", s)
    }
    return matches, err
}
