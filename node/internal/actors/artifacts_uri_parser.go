package actors

import (
	"fmt"
	"strings"
)

type uri struct {
	schema   string
	location string
	path     string
	tag      string
}

func (u uri) String() string {
	return fmt.Sprintf("%s://%s/%s:%s", u.schema, u.location, u.path, u.tag)
}

func parseUri(inUri string) (*uri, error) {
	u := new(uri)

	// example oci://domain:port/repo:tag
	sUri := strings.Split(inUri, "://")
	if len(sUri) != 2 {
		return nil, fmt.Errorf("invalid uri: %s", inUri)
	}

	// oci
	u.schema = sUri[0]

	if u.schema == SchemeFile {
		// /tmp/foo
		u.path = sUri[1]
		return u, nil
	}

	// split should be: [0]=domain:port [1]=repo:tag
	sUriPath := strings.SplitN(sUri[1], "/", 2)
	if len(sUriPath) != 2 {
		return nil, fmt.Errorf("invalid uri: %s", inUri)
	}
	// domain:port
	location := sUriPath[0]
	u.location = location

	sPath := strings.SplitN(sUriPath[1], ":", 2)
	if len(sPath) != 2 {
		u.path = sUriPath[1]
		u.tag = "latest"
		return u, nil
	}

	u.path = sPath[0]
	u.tag = sPath[1]
	return u, nil
}
