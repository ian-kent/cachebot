all:
	go get github.com/mitchellh/gox
	gox -osarch="linux/amd64" -output="cachebot_linux_amd64"
	zip cachebot.zip cachebot_linux_amd64 Dockerfile

.PHONY: all
