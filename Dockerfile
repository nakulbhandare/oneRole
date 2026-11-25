FROM node:20-slim
WORKDIR /app
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy pre-compiled linux/amd64 Go binary
COPY server ./server
RUN chmod +x ./server

EXPOSE 8088
CMD ["./server"]
