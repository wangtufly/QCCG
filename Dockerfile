FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o qccg .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/qccg .
# baseprompt.json is embedded at build time, no need to copy separately
ENV QODER_PAT=""
EXPOSE 8963
CMD ["./qccg"]
