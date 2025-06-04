FROM golang:1.24-alpine AS build

COPY . /var/tmp/
WORKDIR /var/tmp/
ENV CGO_ENABLED=0
RUN go build -o /tmp/app

FROM scratch AS final

USER 8484
COPY --from=build /tmp/app /app
CMD ["/app", "server"]