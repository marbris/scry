APP=scry
VERSION=3.1.0

build:
	go build -o $(APP) .

all: linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64

linux-amd64:
	GOOS=linux GOARCH=amd64 go build -o dist/$(APP)-linux-amd64 .

linux-arm64:
	GOOS=linux GOARCH=arm64 go build -o dist/$(APP)-linux-arm64 .

darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -o dist/$(APP)-darwin-amd64 .

darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -o dist/$(APP)-darwin-arm64 .

windows-amd64:
	GOOS=windows GOARCH=amd64 go build -o dist/$(APP)-windows-amd64.exe .

clean:
	rm -rf dist/ $(APP)

.PHONY: build all clean linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64
