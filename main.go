//usr/bin/env go run $0 "$@"; exit

// Housekeeping for Nexus, i.e. artifact deletion.
// Deleting released artifacts is considered a no-go, especially in Maven land,
// but some of us operate on limited disk space, and control dependants.
//
// return codes:
//  1: number of artifacts found exceeds expected result size
//  3: truncated search

package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

const (
	defaultServer   = "localhost"
	defaultPort     = "8081"
	defaultUsername = "admin"
	defaultPassword = "admin123"

	defaultRepository = "releases"
)

// NexusInstance holds coordinates of a Nexus installation
type NexusInstance struct {
	Protocol    string
	Server      string
	Port        string
	Contextroot string
	Username    string
	Password    string
}

// NexusRepository holds coordinates of a Nexus repository
type NexusRepository struct {
	NexusInstance
	RepositoryID string
}

type searchNGResponse struct {
	// Count is just a copy of the 'count' request value
	Count int `xml:"count"`
	// From is just a copy of the 'from' request value
	From           int  `xml:"from"`
	TotalCount     int  `xml:"totalCount"`
	TooManyResults bool `xml:"tooManyResults"`
	Artifacts      []struct {
		Group        string `xml:"groupId"`
		Artifact     string `xml:"artifactId"`
		Version      string `xml:"version"`
		ArtifactHits []struct {
			RepositoryID  string `xml:"repositoryId"`
			ArtifactLinks []struct {
				Packaging  string `xml:"extension"`
				Classifier string `xml:"classifier"`
			} `xml:"artifactLinks>artifactLink"`
		} `xml:"artifactHits>artifactHit"`
	} `xml:"data>artifact"`
}

// Gav are the standard Maven coordinates
type Gav struct {
	Group      string `xml:"groupId"`
	Artifact   string `xml:"artifactId"`
	Version    string `xml:"version"`
	Classifier string `xml:"classifier"`
	Packaging  string `xml:"packaging"`
}

// Fqa holds coordincates to a fully qualified artifact
type Fqa struct {
	NexusRepository
	Gav
}

// ContentURL return a fetchable URL
func (a Fqa) ContentURL() string {
	return fmt.Sprintf("%s://%s:%s%s/content/repositories/%s/%s",
		a.Protocol, a.Server, a.Port, a.Contextroot, a.RepositoryID,
		a.DefaultLayout())
}

// RedirectURL returns a REST URL that will redirect to the specific version
// such as LATEST, SNAPSHOT, ...
func (a Fqa) RedirectURL() string {
	s := fmt.Sprintf("%s://%s:%s%s"+
		"/service/local/artifact/maven/redirect"+
		"?r=%s&g=%s&a=%s&v=%s&p=%s",
		a.Protocol, a.Server, a.Port, a.Contextroot,
		a.RepositoryID,
		a.Group, a.Artifact, a.Version, a.Packaging)
	return s
}

// ConciseNotation returns group:artifact:version[:classifier][@packaging]
func (a Gav) ConciseNotation() string {
	s := fmt.Sprintf("%v:%v:%v",
		a.Group,
		a.Artifact,
		a.Version)
	if a.Classifier != "" {
		s += fmt.Sprintf(":%s", a.Classifier)
	}
	if a.Packaging != "" {
		s += fmt.Sprintf("@%s", a.Packaging)
	}
	return s
}

// DefaultLayout translates a Gav into a file system hierarchy without leading /
func (a Gav) DefaultLayout() string {
	return fmt.Sprintf("%s/%s/%s/%s",
		strings.Replace(a.Group, ".", "/", -1),
		a.Artifact,
		a.Version,
		a.Filename())
}

// Filename returns the basename part of a GAV default layout
func (a Gav) Filename() string {
	filename := fmt.Sprintf("%s-%s", a.Artifact, a.Version)
	if a.Classifier != "" {
		filename = fmt.Sprintf("%s-%s", filename, a.Classifier)
	}
	if a.Packaging == "" {
		a.Packaging = "jar"
	}
	return fmt.Sprintf("%s.%s", filename, a.Packaging)
}

// LuceneSearch builds a request path for given GAV
func (a Gav) LuceneSearch() string {
	url := ""
	if a.Group != "" {
		url += fmt.Sprintf("g=%s", a.Group)
	}
	if a.Artifact != "" {
		url += fmt.Sprintf("&a=%s", a.Artifact)
	}
	if a.Version != "" {
		url += fmt.Sprintf("&v=%s", a.Version)
	}
	if a.Packaging != "" {
		url += fmt.Sprintf("&p=%s", a.Packaging)
	}
	if a.Classifier != "" {
		url += fmt.Sprintf("&c=%s", a.Classifier)
	}
	return url
}

// search executes Nexus REST search, optionally multiple times to find
// every match
// returns a boolean to indicate if the search has been complete, or if too many
// wildcards have been used that confuse Nexus
func search(repo NexusRepository, gav Gav) searchNGResponse {
	params := gav.LuceneSearch()
	url := fmt.Sprintf("%s://%s:%s%s/service/local/lucene/search?%s",
		repo.Protocol, repo.Server, repo.Port, repo.Contextroot, params)
	if repo.RepositoryID != "" {
		url += fmt.Sprintf("&repositoryId=%s", repo.RepositoryID)
	}
	response, err := http.Get(url)
	if err != nil {
		log.Fatalf("Cannot read url %v: %v\n", url, err)
	}
	log.Printf("%v returns HTTP status code %v\n",
		url, response.StatusCode)
	if response.StatusCode != 200 {
		log.Fatalf("Expected status 200 but got %v\n",
			response.StatusCode)
	}
	log.Printf("Header: %+v\n", response.Header)
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	log.Println(string(body))
	if err != nil {
		log.Fatal(err)
	}
	var found searchNGResponse
	err = xml.Unmarshal(body, &found)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("search returns count=%d, total count=%d, "+
		"overflow=%v, artifacts=%d\n",
		found.Count, found.TotalCount, found.TooManyResults,
		len(found.Artifacts))

	return found
}

func locations(res searchNGResponse, inst NexusInstance) []Fqa {
	var ls []Fqa
	for _, a := range res.Artifacts {
		fmt.Printf("%+v\n", a)
		for _, hit := range a.ArtifactHits {
			for _, link := range hit.ArtifactLinks {
				gav := Gav{a.Group, a.Artifact, a.Version,
					link.Classifier, link.Packaging,
				}
				ls = append(ls, Fqa{
					NexusRepository: NexusRepository{
						inst,
						hit.RepositoryID},
					Gav: gav,
				})
			}
		}
	}
	return ls
}

func main() {
	var (
		// Nexus coordinates
		protocol = flag.String("protocol", "http", "Nexus protocol")
		server   = flag.String("server", defaultServer,
			"Nexus server name")
		port        = flag.String("port", defaultPort, "Nexus port")
		contextroot = flag.String("contextroot", "/nexus",
			"Nexus context root")
		username = flag.String("username", defaultUsername,
			"Nexus user")
		password = flag.String("password", defaultPassword,
			"Nexus password")
		repository = flag.String("repository", defaultRepository,
			"Nexus repository ID, empty for global search")

		// Search coordinates
		group      = flag.String("group", "", "Maven group")
		artifact   = flag.String("artifact", "", "Maven artifact")
		version    = flag.String("version", "", "Maven version")
		packaging  = flag.String("packaging", "", "Maven packaging")
		classifier = flag.String("classifier", "", "Maven classifier")

		// Download
		fetch = flag.Bool("fetch", true, "Download files found")
	)

	flag.Parse()
	inst := NexusInstance{*protocol, *server, *port, *contextroot,
		*username, *password}
	repo := NexusRepository{inst, *repository}

	gav := Gav{*group, *artifact, *version, *classifier, *packaging}
	log.Printf("searching %+v\n", gav)

	res := search(repo, gav)
	log.Printf("Found %v artifacts\n", len(res.Artifacts))

	ls := locations(res, inst)
	for _, a := range ls {
		// Ignore POMs
		if a.Gav.Packaging == "pom" {
			continue
		}
		log.Printf("artifact: %+v [%s]\n",
			a.Gav.ConciseNotation(), a.NexusRepository.RepositoryID)
		log.Printf("default layout: %s\n", a.DefaultLayout())
		var url string
		// Optionally resolve Maven SNAPSHOTS
		log.Printf("Version: %s\n", a.Gav.Version)
		if strings.HasSuffix(a.Gav.Version, "SNAPSHOT") {
			url = a.RedirectURL()
		} else {
			url = a.ContentURL()
		}

		if !*fetch {
			continue
		}

		log.Printf("fetching %s\n", url)
		res, err := http.Get(url)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Header: %+v\n", res.Header)
		defer res.Body.Close()
		buf, err := ioutil.ReadAll(res.Body)
		if err != nil {
			log.Fatal(err)
		}
		f := a.Gav.Filename()
		log.Printf("writing %s\n", f)
		if err := ioutil.WriteFile(f, buf, 0644); err != nil {
			log.Fatal(err)
		}
	}
}