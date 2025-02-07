FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go Makefile ./
COPY ui ./ui
RUN make build

RUN mkdir /data

###

FROM scratch AS runner

WORKDIR /app

COPY --chown=1000:1000 --chmod=0500 --from=builder /app/dist/llmsee ./
COPY --chown=1000:1000 --chmod=0700 --from=builder /data /data

USER 1000:1000

ENV LLMSEE_HOST=0.0.0.0
ENV LLMSEE_PORT=5050
ENV LLMSEE_CONFIGFILE=/data/llmsee.json
ENV LLMSEE_DATABASEFILE=/data/llmsee.db
ENV LLMSEE_LOCALHOST=host.docker.internal

EXPOSE 5050

CMD ["/app/llmsee"]
