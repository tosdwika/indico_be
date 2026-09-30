FROM golang:latest AS build
WORKDIR /src
COPY go.mod ./
COPY app ./app
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -o /indico_be ./cmd/server

FROM golang:latest
WORKDIR /src
COPY --from=build /src ./ 
COPY --from=build /indico_be /indico_be
EXPOSE 8085
CMD ["/indico_be"]
