package node

import "net/url"

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

// Obtains an artifact from the specified location. If the indicated location allows for
// differentiation using tags (e.g. OCI, Object Store), then the supplied tag will be used,
// otherwise it will be ignored
func GetArtifact(name string, tag string, location *url.URL) (*ArtifactReference, error) {
	switch location.Scheme {
	case SchemeFile:
		return cacheFile(name, location)
	case SchemeNATS:
		return cacheObjectStoreArtifact(name, tag, location)
	case SchemeOCI:
		return cacheOciArtifact(name, tag, location)
	}
	return nil, nil
}

func cacheFile(name string, location *url.URL) (*ArtifactReference, error) {
	// TODO: Implement
	return nil, nil
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
