package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"strings"
	"text/template"

	"github.com/gorilla/mux"
	lru "github.com/hashicorp/golang-lru"
	"srcd.works/go-git.v4"
	"srcd.works/go-git.v4/plumbing"
	"srcd.works/go-git.v4/plumbing/object"
	"srcd.works/go-git.v4/plumbing/transport"
)

type FileRef struct {
	CommitHash string
	FilePath   string
}

func (r *FileRef) String() string {
	return r.CommitHash + "::" + r.FilePath
}

type TmplRepo interface {
	GetTemplate(ref FileRef, sync bool) (*template.Template, error)
	Sync() error
}

type GitTmplRepo struct {
	*git.Repository
	Auth transport.AuthMethod
}

var (
	ErrCommitNotFound = errors.New("failed to find the commit in repo")
	ErrFileNotFound   = errors.New("failed to find the file in commit")
)

func (r *GitTmplRepo) FindFile(ref FileRef) (*object.File, error) {
	commit, err := r.Commit(plumbing.NewHash(ref.CommitHash))
	if err != nil {
		return nil, ErrCommitNotFound
	}
	file, err := commit.File(ref.FilePath)
	if err != nil {
		return nil, ErrFileNotFound
	}
	return file, nil
}

func (r *GitTmplRepo) GetTemplate(ref FileRef, sync bool) (*template.Template, error) {
	file, err := r.FindFile(ref)
	if err != nil {
		if err == ErrFileNotFound || !sync {
			return nil, err
		}
		r.Sync()
		file, err = r.FindFile(ref)
		if err != nil {
			return nil, err
		}
	}

	in, err := file.Reader()
	if err != nil {
		return nil, err
	}
	defer in.Close()

	raw, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, err
	}

	tpl, err := template.New(ref.String()).Parse(string(raw))
	if err != nil {
		return nil, err
	}
	return tpl.Option("missingkey=error"), nil
}

func (r *GitTmplRepo) Sync() error {
	return r.Fetch(&git.FetchOptions{Auth: r.Auth})
}

type CachedTmplRepo struct {
	TmplRepo
	Cache *lru.Cache
}

func NewCachedTmplRepo(repo TmplRepo, size int) (TmplRepo, error) {
	cache, err := lru.New(size)
	if err != nil {
		return nil, err
	}
	return &CachedTmplRepo{repo, cache}, nil
}

func (r *CachedTmplRepo) GetTemplate(ref FileRef, sync bool) (*template.Template, error) {
	key := ref.String()
	if tmpl, ok := r.Cache.Get(key); ok {
		return tmpl.(*template.Template), nil
	}
	tmpl, err := r.TmplRepo.GetTemplate(ref, sync)
	if err != nil {
		return nil, err
	}
	r.Cache.Add(key, tmpl)
	return tmpl, nil
}

func ExtractRefFromMuxVars(r *http.Request) (FileRef, error) {
	hash := mux.Vars(r)["hash"]
	pos := strings.Index(r.URL.Path, hash)
	return FileRef{
		CommitHash: hash,
		FilePath:   r.URL.Path[pos+41:],
	}, nil
}

func RawHandler(repo TmplRepo, extract func(r *http.Request) (FileRef, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// prepare data
		data, err := parseData(r)
		if checkFailure(err, http.StatusBadRequest, w) {
			return
		}

		// extract file ref
		ref, err := extract(r)
		if checkFailure(err, http.StatusBadRequest, w) {
			return
		}

		// get template
		tpl, err := repo.GetTemplate(ref, true)
		switch err {
		case nil:
		case ErrCommitNotFound, ErrFileNotFound:
			checkFailure(err, http.StatusNotFound, w)
			return
		default:
			log.Print("failed to get template: " + err.Error())
			checkFailure(err, http.StatusInternalServerError, w)
			return
		}

		// render template
		out, err := render(tpl, data)
		if err != nil && strings.Contains(err.Error(), "map has no entry for key") {
			checkFailure(err, http.StatusBadRequest, w)
			return
		}
		if checkFailure(err, http.StatusInternalServerError, w) {
			return
		}
		w.Write(out)
	}
}

func MD5Handler(repo TmplRepo, extract func(r *http.Request) (FileRef, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// prepare data
		data, err := parseData(r)
		if checkFailure(err, http.StatusBadRequest, w) {
			return
		}

		// extract file ref
		ref, err := extract(r)
		if checkFailure(err, http.StatusBadRequest, w) {
			return
		}

		// get template
		tpl, err := repo.GetTemplate(ref, true)
		switch err {
		case nil:
		case ErrCommitNotFound, ErrFileNotFound:
			checkFailure(err, http.StatusNotFound, w)
			return
		default:
			log.Print("failed to get template: " + err.Error())
			checkFailure(err, http.StatusInternalServerError, w)
			return
		}

		// render template
		out, err := render(tpl, data)
		if err != nil && strings.Contains(err.Error(), "map has no entry for key") {
			checkFailure(err, http.StatusBadRequest, w)
			return
		}
		if checkFailure(err, http.StatusInternalServerError, w) {
			return
		}

		hash := md5.New()
		hash.Write(out)
		w.Write([]byte(hex.EncodeToString(hash.Sum(nil)) + "  " + path.Base(ref.FilePath) + "\n"))
	}
}

func checkFailure(err error, status int, w http.ResponseWriter) bool {
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), status)
		return true
	}
	return false
}

func parseData(r *http.Request) (map[string]string, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, err
	}
	data := make(map[string]string)
	for key := range r.Form {
		data[key] = r.FormValue(key)
	}
	return data, nil
}

func render(tpl *template.Template, data map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	err := tpl.Execute(&buf, data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
