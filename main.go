package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"golang.org/x/crypto/ssh"

	"srcd.works/go-git.v4"
	gitssh "srcd.works/go-git.v4/plumbing/transport/ssh"

	"github.com/gorilla/mux"
	"github.com/zyguan/just"
)

func logFatal(err error) error {
	log.Fatal(err)
	return nil
}

func logHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s from %s", r.Method, r.URL, r.RemoteAddr)
		handler.ServeHTTP(w, r)
	})
}

var (
	gituser string
	keypath string
	sync    bool
	port    int
)

func init() {
	home, _ := os.LookupEnv("HOME")

	flag.StringVar(&gituser, "u", "git", "git user used to fetching the remote repo")
	flag.StringVar(&keypath, "k", home+"/.ssh/id_rsa", "path to private key for authorization")
	flag.BoolVar(&sync, "s", true, "sync remote when starting up")
	flag.IntVar(&port, "p", 8080, "http port to listen on")

	flag.Usage = usage
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] [path] (default \".\")\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "Options:")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  %s -p=80 -s=false\n", os.Args[0])
}

func main() {
	defer just.CatchF(logFatal)(nil)
	flag.Parse()

	repopath := "."
	if len(flag.CommandLine.Args()) == 1 {
		repopath = flag.CommandLine.Args()[0]
	} else if len(flag.CommandLine.Args()) > 1 {
		usage()
		os.Exit(1)
	}
	repo := openRepo(gituser, keypath, repopath, sync)

	r := mux.NewRouter()
	r.PathPrefix("/raw/{hash:[0-9a-z]{40}}/").HandlerFunc(
		RawHandler(repo, ExtractRefFromMuxVars),
	)
	r.PathPrefix("/md5/{hash:[0-9a-z]{40}}/").HandlerFunc(
		MD5Handler(repo, ExtractRefFromMuxVars),
	)
	http.Handle("/", logHandler(r))
	log.Printf("try to bind to 0.0.0.0:%d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func openRepo(gitUser, keyPath, repoPath string, sync bool) TmplRepo {
	// read private key
	pem := just.TryTo("read key file: ")(ioutil.ReadFile(keyPath)).([]byte)
	signer := just.TryTo("parse pem key: ")(ssh.ParsePrivateKey(pem)).(ssh.Signer)
	key := &gitssh.PublicKeys{User: gitUser, Signer: signer}

	// open local git repo
	local := just.TryTo("open local git repo: ")(git.PlainOpen(repoPath)).(*git.Repository)
	gitRepo := &GitTmplRepo{Repository: local, Auth: key}

	// new tmpl repo
	repo := just.TryTo("new cached tmpl repo: ")(NewCachedTmplRepo(gitRepo, 4096)).(TmplRepo)

	if sync {
		switch err := repo.Sync(); err {
		case nil:
			log.Print("repo has been updated")
		case git.NoErrAlreadyUpToDate:
			log.Print("repo is already up-to-date")
		default:
			log.Fatal("failed to fetch remote: ", err)
		}
	}

	return repo
}
