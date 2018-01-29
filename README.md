# http-logger
toy logging daemon

# checking out
	mkdir ${GOPATH:-${HOME}/go}/src/github.com/tbarker25/
	cd ${GOPATH:-${HOME}/go}/src/github.com/tbarker25/
	git clone git@github.com:tbarker25/http-logger.git

# usage example
	go build
	./util/fake-apache-log-generator --num=-1 --sleep=0.001 | ./http-logger
note that `pip install` might be necessary for ./util/fake-apache-log-generator

# running tests
	go test ./...
