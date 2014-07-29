package runner

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type Source struct {
	RepositoryType string
	RepositoryURL  string
	PatchID        string
	PatchURL       string
}

func NewSource(config *Config) (*Source, error) {
	var patchid string
	var patchurl string

	if config.Source.Patch.ID != "" {
		patchid = config.Source.Patch.ID
		patchurl = config.Server + "/patches/" + patchid + "/?raw=1"
	}

	return &Source{
		RepositoryType: config.Repository.Backend.ID,
		RepositoryURL:  config.Repository.URL,
		PatchID:        patchid,
		PatchURL:       patchurl,
	}, nil
}

func (source *Source) SetupWorkspace(reporter *Reporter, path string) error {
	vcs, err := source.GetVcsBackend(path)
	if err != nil {
		return err
	}

	err = CloneOrUpdate(vcs)
	if err != nil {
		return err
	}

	if source.PatchID != "" {
		patchpath, err := DownloadPatch(source.PatchURL)
		if err != nil {
			return err
		}

		err = ApplyPatch(vcs, patchpath)
		if err != nil {
			return err
		}
	}

	return nil
}

func (source *Source) GetVcsBackend(sourcepath string) (Vcs, error) {
	if source.RepositoryType == "git" {
		return &GitVcs{
			URL:  source.RepositoryURL,
			Path: sourcepath,
		}, nil
	} else {
		err := fmt.Errorf("Unsupported repository type: %s", source.RepositoryType)
		return nil, err
	}
}

func DownloadPatch(patchurl string) (string, error) {
	out, err := ioutil.TempFile("", "patch-")
	if err != nil {
		return "", err
	}
	defer out.Close()

	resp, err := http.Get(patchurl)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("Request to fetch patch failed with status code: %d", resp.StatusCode)
		return "", err
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return out.Name(), nil
}
