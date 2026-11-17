# Stage 1: Build Frontend
FROM node:20 AS ui-build
WORKDIR /app/ui
COPY oneRole_UI/package*.json ./
RUN npm install
COPY oneRole_UI/ .
RUN npm run build

# Stage 2: Build Backend
FROM golang:1.22 AS api-build
WORKDIR /app/api
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Stage 3: Final Runtime
FROM node:20-slim
WORKDIR /app
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy build artifacts explicitly
COPY --from=api-build /app/api/server ./server
COPY --from=ui-build /app/ui/dist ./public
COPY --from=ui-build /app/ui/server.js ./server.js
COPY --from=ui-build /app/ui/package.json ./package.json

# Install runtime dependencies for UI server
RUN npm install --production

EXPOSE 8082
CMD ["sh", "-c", "./server & node server.js"]
