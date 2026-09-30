FROM golang:latest AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY app ./app
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -o /indico_be ./cmd/server

FROM alpine:3.22
WORKDIR /app
RUN mkdir -p /app/data
COPY --from=build /indico_be /indico_be
VOLUME ["/app/data"]
EXPOSE 8085
CMD ["/indico_be"]
