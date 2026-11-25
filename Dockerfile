FROM node:20-slim
WORKDIR /app
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy pre-compiled linux/amd64 Go binary
COPY server ./server
RUN chmod +x ./server

# Copy UI assets and server
COPY oneRole_UI/public/ ./public/
COPY oneRole_UI/server.js ./server.js
COPY oneRole_UI/package.json ./package.json

RUN npm install --production

EXPOSE 8082
CMD ["sh", "-c", "./server & node server.js"]
