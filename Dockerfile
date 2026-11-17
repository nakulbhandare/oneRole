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

# Stage 3: Combine
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y nodejs npm
WORKDIR /app
COPY --from=api-build /app/api/server .
COPY --from=ui-build /app/ui/dist ./public
COPY oneRole_UI/server.js .
COPY oneRole_UI/package.json .
RUN npm install

EXPOSE 8082
CMD ["sh", "-c", "./server & node server.js"]
