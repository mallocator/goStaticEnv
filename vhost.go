package main

import (
	"errors"
	"log"
	"net/http"
	"path"
	"strings"
)

func vhostFromHostname(host string) (string, error) {
	pieces := strings.Split(host, ".")
	if len(pieces) == 1 || len(pieces) == 2 {
		return "", errors.New("No vhost")
	}
	return pieces[0], nil
}

func vhostify(base http.Handler, f http.FileSystem) http.Handler {
	vhosts := detectVhosts(f)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vhost, err := vhostFromHostname(r.Host)
		if err != nil {
			base.ServeHTTP(w, r)
			return
		}
		host, exists := vhosts[vhost]
		if exists {
			host.handler.ServeHTTP(w, r)
			return
		}
		base.ServeHTTP(w, r)
	})
}

type VHost struct {
	prefix  string
	handler http.Handler
}

func detectVhosts(fileSystem http.FileSystem) map[string]VHost {
	vhostRoot, err := fileSystem.Open(*vhostPrefix)
	if err != nil {
		log.Fatalf("Error opening vhost root: %v", err)
	}
	vhostDirs, err := vhostRoot.Readdir(512)
	vhosts := make(map[string]VHost)
	vhostBase := path.Join(*basePath, *vhostPrefix)
	for _, dir := range vhostDirs {
		if dir.IsDir() {
			name := dir.Name()
			vhosts[name] = VHost{name, http.FileServer(http.Dir(path.Join(vhostBase, name)))}
		}
	}
	return vhosts
}
