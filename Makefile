.PHONY: test deps clean

test:
	go test -v $(glide novendor)

deps:
	go get -v github.com/Masterminds/glide
	glide install

clean:
	go clean
