FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go Makefile ./
COPY ui ./ui
RUN make build

###

FROM scratch AS runner

WORKDIR /app

COPY --chown=1000:1000 --from=builder /app/dist/llmsee ./

EXPOSE 5050

USER 1000:1000

ENV llmsee_CONFIGFILE=/app/llmsee.json
ENV llmsee_DATABASEFILE=/app/llmsee.db

CMD ["/app/llmsee"]
