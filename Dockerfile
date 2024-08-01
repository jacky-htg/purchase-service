FROM golang:alpine

WORKDIR /app
COPY purchase-service /app/
COPY ./cmd/cmd-purchase-service /app/

CMD ["/app/purchase-service"]
