FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/backend-relay .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bin/backend-relay /backend-relay
EXPOSE 3000
ENTRYPOINT ["/backend-relay"]
