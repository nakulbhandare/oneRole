# Stage 1: Build Backend
FROM golang:1.23 AS api-build
WORKDIR /app/api
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Stage 2: Final Runtime
FROM node:20-slim
WORKDIR /app
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy backend
COPY --from=api-build /app/api/server ./server
# Copy your EXACT local public folder directly (this matches local UI)
COPY oneRole_UI/public/ ./public/
# Copy the server script and package.json
COPY oneRole_UI/server.js ./server.js
COPY oneRole_UI/package.json ./package.json

# Install runtime dependencies for UI server
RUN npm install --production

EXPOSE 8082
CMD ["sh", "-c", "./server & node server.js"]
