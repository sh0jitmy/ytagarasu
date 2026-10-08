FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata curl bash procps jq

WORKDIR /app

# Copy pre-compiled pure Go static binaries
COPY demo/bin/ytg /usr/local/bin/ytg
COPY demo/bin/demo-api /app/bin/demo-api
COPY demo/bin/demo-worker /app/bin/demo-worker

# Copy manifest
COPY demo/manifest.yaml /app/manifest.yaml

# Make sure all binaries are executable
RUN chmod +x /usr/local/bin/ytg /app/bin/demo-api /app/bin/demo-worker

# Expose Web API port
EXPOSE 8080

# Run supervisor
ENTRYPOINT ["/usr/local/bin/ytg"]
CMD ["supervisor", "run", "-c", "/app/manifest.yaml"]
