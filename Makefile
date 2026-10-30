WINRES := github.com/tc-hib/go-winres@v0.3.3
EXE    := build/bin/Intencje Mszalne.exe

.PHONY: run test windows clean

run:
	go run .

test:
	test -z "$$(gofmt -l .)"
	go vet ./...
	GOOS=windows GOARCH=amd64 go vet -tags desktop,production ./...
	go test ./...

windows:
	go run $(WINRES) make --in build/windows/winres.json --arch amd64
	GOOS=windows GOARCH=amd64 go build -tags desktop,production -ldflags "-H windowsgui -s -w" -o "$(EXE)" .

clean:
	rm -rf build/bin rsrc_windows_*.syso
