init:
	go mod init purchase
	
gen:
	protoc --proto_path=proto --go_out=paths=source_relative,:./pb --go-grpc_out=paths=source_relative,:./pb proto/*/*.proto
	
migrate:
	go run cmd/cli.go migrate
	
seed:
	go run cmd/cli.go seed
	
server:
	go run server.go

build:
	env GOOS=linux GOARCH=amd64 go build -o purchase-service
	
.PHONY: init gen migrate seed server