build:
	rm -rf ./bin/*
	GOOS=linux GOARCH=amd64 go build -o ./bin/test-module ./test-module
# 	cd voice-player-module && GOOS=linux GOARCH=amd64 go build -o ../bin/voice-player-module .
	cd chatanium-musicbot && GOOS=linux GOARCH=amd64 go build -o ../bin/chatanium-musicbot .
	GOOS=linux GOARCH=amd64 go build -o ./bin/runtime .
buf:
	rm -rf ./proto/*.pb.go ./proto/**/*.pb.go && cd proto && buf generate
run:
	make build && ./bin/runtime
