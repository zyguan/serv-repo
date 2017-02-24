package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	"srcd.works/go-git.v4"
)

func repo(t testing.TB, repoPath string, cacheSize int) TmplRepo {
	local, err := git.PlainOpen(repoPath)
	if err != nil {
		t.Fatal("failed to open local git repo: " + err.Error())
	}
	var repo TmplRepo
	repo = &GitTmplRepo{Repository: local}
	if cacheSize > 0 {
		repo, err = NewCachedTmplRepo(repo, cacheSize)
		if err != nil {
			t.Fatal("failed to create cached repo: " + err.Error())
		}
	}
	return repo
}

const INIT_COMMIT = "dd2bd7756e32a84ed2f2495087e626d4ed648f3a"

func server(repo TmplRepo) *httptest.Server {
	r := mux.NewRouter()
	r.PathPrefix("/raw/{hash:[0-9a-z]{40}}/").HandlerFunc(RawHandler(repo, ExtractRefFromMuxVars))
	r.PathPrefix("/md5/{hash:[0-9a-z]{40}}/").HandlerFunc(MD5Handler(repo, ExtractRefFromMuxVars))
	return httptest.NewServer(r)
}

func TestFindFileFailure(t *testing.T) {
	r := repo(t, ".", 32)
	var ref FileRef
	_, err := r.GetTemplate(ref, false)
	assert.Equal(t, ErrCommitNotFound, err)

	ref.CommitHash = INIT_COMMIT
	_, err = r.GetTemplate(ref, false)
	assert.Equal(t, ErrFileNotFound, err)
}

func TestHandleFailure(t *testing.T) {
	s := server(repo(t, ".", 32))
	defer s.Close()

	url := s.URL + "/raw/0000000000000000000000000000000000000000/templates/hi.txt?who=world"
	resp, err := http.Get(url)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	url = s.URL + "/md5/0000000000000000000000000000000000000000/templates/hi.txt?who=world"
	resp, err = http.Get(url)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	url = s.URL + "/raw/" + INIT_COMMIT + "/templates/hi.txt?oops=world"
	resp, err = http.Get(url)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	url = s.URL + "/md5/" + INIT_COMMIT + "/templates/hi.txt?oops=world"
	resp, err = http.Get(url)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRawHandler(t *testing.T) {
	s := server(repo(t, ".", 32))
	defer s.Close()

	url := s.URL + "/raw/" + INIT_COMMIT + "/templates/hi.txt?who=world"

	resp, err := http.Get(url)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err, "body should be read")
	assert.Equal(t, "Hi, world!\n", string(body))
}

func TestMD5Handler(t *testing.T) {
	s := server(repo(t, ".", 32))
	defer s.Close()

	url := s.URL + "/md5/" + INIT_COMMIT + "/templates/hi.txt?who=world"

	resp, err := http.Get(url)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "07197f7673c0074a7e0a64839ba45dd5  hi.txt\n", string(body))
}

func BenchmarkTmplRepoWithoutCache(b *testing.B) {
	s := server(repo(b, ".", 0))
	defer s.Close()

	benchServer(b, s)
}

func BenchmarkTmplRepoWithCache(b *testing.B) {
	s := server(repo(b, ".", 32))
	defer s.Close()

	benchServer(b, s)
}

func benchServer(b *testing.B, s *httptest.Server) {
	url := s.URL + "/raw/" + INIT_COMMIT + "/templates/hi.txt?who=world"
	for i := 0; i < b.N; i++ {
		resp, err := http.Get(url)
		if err != nil {
			b.Fatal("failed to get response: " + err.Error())
		}
		// avoid 'too many open files'
		resp.Body.Close()
	}
}
