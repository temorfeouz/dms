all: buildarm build
buildarm:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" -o bin/dmsarm .
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/dms .