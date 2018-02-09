package main

import (
	"net/http"
	"log"
	"io/ioutil"
	"io"
	"time"
	"os"
	"encoding/json"
	"flag"
	"strings"
	"os/exec"
	"path"
	"path/filepath"
)

var verbose = flag.Bool("verbose", false, "Log verbosely")
var dryRun = flag.Bool("dry-run", false, "Perform dry run")

func main() {
	fromFile := flag.String("from-file", "", "A json document matching the repos Bitbucket REST api, incompatible with --project-url")
	url := flag.String("project-url", "", "The bitbucket url, incompatible with --from-file")
	tokenFile := flag.String("token-file", "", "The path of the readToken file, only used when HTTP request is made")
	projectDir := flag.String("project-dir", currentWorkingDir(), "The path of the project directory, if none specified, use current working directory")
	flag.Parse()

	if (*fromFile == "" && *url == "") || (*fromFile != "" && *url != "") {
		log.Println(flag.Args())
		flag.PrintDefaults()
		os.Exit(1)
	}

	var payload repositories
	if *url != "" {
		payload = readPayload(reposApi(*url, *tokenFile))
	}
	if *fromFile != "" {
		payload = readPayload(reposFile(*fromFile))
	}

	gitUrls := collectGitUrls(payload)

	cloneOrPull(gitUrls, *projectDir)
}

func cloneOrPull(gitUrls map[string]string, projectDir string) {
	if *verbose {
		absolutePath, _ := filepath.Abs(projectDir)
		log.Println("Using project dir : " + absolutePath)
	}
	for repoName, gitUrl := range gitUrls {
		var args []string
		if _, err := os.Stat(path.Join(projectDir, repoName)); os.IsNotExist(err) {
			args = []string{
				"git",
				"clone",
				gitUrl,
				path.Join(projectDir, repoName),
			}
		} else {
			// assuming repo has been cloned using default name
			args = []string{
				"git",
				"--git-dir",
				path.Join(projectDir, repoName, ".git"),
				"pull",
				"--rebase",
				"--prune",
			}
		}
		if *dryRun {
			log.Println(args)
		} else {
			if *verbose {
				log.Println(args)
			}
			command := exec.Command(args[0], args[1:]...)
			//command.Dir = projectDir
			output, err := command.CombinedOutput()
			if err != nil {
				os.Stderr.WriteString(err.Error())
			}
			log.Println(string(output))
		}
	}
}

func collectGitUrls(repos repositories) map[string]string {
	gitUrls := make(map[string]string, 10)
	for _, repo := range repos.Repos {
		if repo.ScmId != "git" || repo.State != "AVAILABLE" {
			break
		}
		for _, cloneLink := range repo.Links["clone"] {
			if cloneLink.Name == "ssh" || cloneLink.Name == "git" {
				gitUrls[repo.Name] = cloneLink.Href
			}
		}
	}
	return gitUrls
}

func reposApi(url string, tokenFile string) io.ReadCloser {
	if *verbose {
		log.Println("Configuring HTTP client")
	}
	client := &http.Client{
		Timeout: time.Second * 2,
	}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatal(err)
	}
	token := readToken(tokenFile)
	if token != "" {
		if *verbose {
			log.Println("Assigning authorization header")
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if *verbose {
		log.Println("Executiong HTTP request")
	}
	resp, err := client.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != 200 {
		log.Fatal("Bad status : " + resp.Status)
	}
	if *verbose {
		log.Println("Returning response body")
	}
	return resp.Body
}

func reposFile(path string) *os.File {
	if *verbose {
		log.Println("Reading project repositories json document")
	}
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	//defer file.Close()
	return file
}

func readToken(path string) string {
	if *verbose {
		log.Println("Read token file : " + path)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if *verbose {
			log.Println("No token file")
		}
		return ""
	}
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	if *verbose {
		log.Println("Token file content has been read")
	}
	return strings.TrimSpace(string(bytes))
}

func readPayload(Body io.ReadCloser) repositories {
	payload, err := ioutil.ReadAll(Body)
	if err != nil {
		log.Fatal(err)
		panic(err)
	}
	defer Body.Close()

	if *verbose {
		log.Println("Reading JSON document")
	}
	repos := repositories{}
	jsonErr := json.Unmarshal(payload, &repos)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	return repos
}


func currentWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	return dir
}

type project struct {
	Id    int               `json:"id"`
	Name  string            `json:"name"`
	Links map[string][]link `json:"links"`
}

type link struct {
	Href string `json:"href"`
	Name string `json:"name"`
}

type repo struct {
	Id      int               `json:"id"`
	Name    string            `json:"name"`
	ScmId   string            `json:"scmId"`
	State   string            `json:"state"`
	Project project           `json:"project"`
	Links   map[string][]link `json:"links"`
}

type repositories struct {
	Size          int    `json:"size"`
	Limit         int    `json:"limit"`
	IsLastPage    bool   `json:"isLastPage"`
	Start         int    `json:"start"`
	NextPageStart int    `json:"nextPageStart"`
	Repos         []repo `json:"values"`
}
