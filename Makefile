build: build_fe build_server package

docker:
	docker build -t imail:internal .

clean:
	rm -rf output


build_fe:
	cd fe && yarn && yarn build
	rm -rf server/listen/http_server/dist
	cd server && cp -rf ../fe/dist listen/http_server

build_server:
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X 'main.goVersion=$(go version)' -X 'main.gitHash=$(git show -s --format=%H)' -X 'main.buildTime=$(TZ=UTC-8 date +%Y-%m-%d" "%H:%M:%S)'" -o imail_linux_amd64  main.go
	cd server && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X 'main.goVersion=$(go version)' -X 'main.gitHash=$(git show -s --format=%H)' -X 'main.buildTime=$(TZ=UTC-8 date +%Y-%m-%d" "%H:%M:%S)'" -o imail_windows_amd64.exe  main.go
	cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w -X 'main.goVersion=$(go version)' -X 'main.gitHash=$(git show -s --format=%H)' -X 'main.buildTime=$(TZ=UTC-8 date +%Y-%m-%d" "%H:%M:%S)'" -o imail_mac_amd64  main.go
	cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w -X 'main.goVersion=$(go version)' -X 'main.gitHash=$(git show -s --format=%H)' -X 'main.buildTime=$(TZ=UTC-8 date +%Y-%m-%d" "%H:%M:%S)'" -o imail_mac_arm64  main.go

package: clean
	mkdir output
	mv server/imail* output/
	mkdir output/config
	if [ -d server/config/dkim ]; then cp -r server/config/dkim output/config/; fi
	if [ -d server/config/ssl ]; then cp -r server/config/ssl output/config/; fi
	cp config.example.json output/config/config.json
	cp README.md output/

test:
	export setup_port=17888 && cd server && export IMAIL_ROOT=$(CURDIR)/server/ && go test -v -p 1 ./...

test_mysql:
	export setup_port=17888 && cd server && export IMAIL_ROOT=$(CURDIR)/server/ && go test -args "mysql" -v -p 1 ./...

test_postgres:
	export setup_port=17888 && cd server && export IMAIL_ROOT=$(CURDIR)/server/ && go test -args "postgres" -v -p 1 ./...