package actors

import (
	"net/url"
	"os"
	"strings"
)

const (
	SchemeFile = "file"
	SchemeNATS = "nats"
	SchemeOCI  = "oci"
)

type ArtifactReference struct {
	// The name of the artifact, as indicated by a deployment request. This is NOT a filename
	Name string
	// An optional tag for the artifact. Empty string if one isn't used
	Tag string
	// The location URL as contained on the original deployment request
	OriginalLocation *url.URL
	// Once downloaded, this is the local file system path to the artifact. Used for loading
	// or execution
	LocalCachePath string
	// The calculated digest of the artifact file as it sits on disk
	Digest string
	// The size of the artifact, in bytes
	Size int
}

func extractTag(location *url.URL) string {
	return location.Fragment
}

// Obtains an artifact from the specified location. If the indicated location allows for
// differentiation using tags (e.g. OCI, Object Store), then the supplied tag will be used,
// otherwise it will be ignored
func getArtifact(name string, uri string) (*ArtifactReference, error) {
	location, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	switch location.Scheme {
	case SchemeFile:
		return cacheFile(name, location)
	case SchemeNATS:
		return cacheObjectStoreArtifact(name, extractTag(location), location)
	case SchemeOCI:
		return cacheOciArtifact(name, extractTag(location), location)
	}
	return nil, nil
}

func cacheFile(name string, location *url.URL) (*ArtifactReference, error) {
	filePath, tag := "", ""
	sPath := strings.SplitN(location.Path, ":", 2) // splits on first : only
	if len(sPath) == 2 {
		filePath = sPath[0]
		tag = sPath[1]
	} else {
		filePath = location.Path
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, nil
	}

	return &ArtifactReference{
		Name:             name,
		Tag:              tag,
		OriginalLocation: location,
		LocalCachePath:   filePath,
		Digest:           "",
		Size:             int(info.Size()),
	}, nil
}

func cacheObjectStoreArtifact(name string, tag string, location *url.URL) (*ArtifactReference, error) {
	// TODO: implement
	return nil, nil
}

func cacheOciArtifact(name string, tag string, location *url.URL) (*ArtifactReference, error) {
	// TODO: implement
	// NOTE: for now let's assume that if the target registry requires auth, it will be supplied in the location.User field
	return nil, nil
}
