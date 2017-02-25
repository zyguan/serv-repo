# serv-repo

[![Build Status](https://travis-ci.org/zyguan/serv-repo.svg?branch=master)](https://travis-ci.org/zyguan/serv-repo)

## Example

Build and start the tool firstly:
```sh
make deps build  # build the tool
./serv-repo  # start to serve files in this repo
```

Then, you are able to do following things:
```sh
curl localhost:8080/raw/dd2bd7756e32a84ed2f2495087e626d4ed648f3a/templates/hi.txt?who=$USER
#=> Hi, ...!
curl localhost:8080/md5/dd2bd7756e32a84ed2f2495087e626d4ed648f3a/templates/hi.txt?who=$USER
#=> 7b5f29dac804718a6a71a26b50ac8f2  hi.txt
```
