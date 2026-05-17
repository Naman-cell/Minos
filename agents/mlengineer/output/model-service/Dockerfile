FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/model-service .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bin/model-service /model-service
EXPOSE 8080
ENTRYPOINT ["/model-service"]
