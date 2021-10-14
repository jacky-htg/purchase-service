init:
	go mod init purchase
	
gen:
	protoc --proto_path=proto --go_out=paths=source_relative,plugins=grpc:./pb proto/*/*.proto
	
migrate:
	go run cmd/cli.go migrate
	
seed:
	go run cmd/cli.go seed
	
server:
	go run server.go
	
.PHONY: init gen migrate seed server