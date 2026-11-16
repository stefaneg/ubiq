# The Server for the dev stack, built from this checkout (build context: the repository root).
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -o /ubiq .

FROM alpine:3.24
# The Server runs the host's git; the origin and clone are bind mounts and volumes owned by whoever made them.
RUN apk add --no-cache git && git config --system safe.directory '*'
COPY --from=build /ubiq /usr/local/bin/ubiq
ENTRYPOINT ["ubiq"]
